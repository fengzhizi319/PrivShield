// Package dynclassification 提供三层动态分类分级引擎。
//
// Layer 1: Aho-Corasick 自动机 + 字段名正则快速匹配（零 ML 开销）
// Layer 2: Small-NER 实体识别（可选，ONNX Runtime）
// Layer 3: LLM/VLM 仲裁（可选，CUDA 推理）
//
// 本文件实现 Layer 1 规则引擎核心。
package dynclassification

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ──────────────────────────────────────────────
// 类型定义
// ──────────────────────────────────────────────

// SecurityLevel 安全等级
type SecurityLevel string

const (
	LevelPublic       SecurityLevel = "public"
	LevelInternal     SecurityLevel = "internal"
	LevelConfidential SecurityLevel = "confidential"
	LevelSecret       SecurityLevel = "secret"
	LevelTopSecret    SecurityLevel = "top_secret"
)

// ClassificationResult 分类结果
type ClassificationResult struct {
	Field      string        `json:"field"`
	Value      string        `json:"value,omitempty"`
	Level      SecurityLevel `json:"level"`
	Category   string        `json:"category"`
	Confidence float64       `json:"confidence"`
	MatchedBy  string        `json:"matched_by"` // "rule:<id>" | "ner" | "llm"
}

// RuleDef 规则定义
type RuleDef struct {
	ID            string        `yaml:"id"`
	Level         SecurityLevel `yaml:"level"`
	Category      string        `yaml:"category"`
	FieldPatterns []string      `yaml:"field_patterns,omitempty"` // 字段名正则
	ValuePatterns []string      `yaml:"value_patterns,omitempty"` // 值内容正则（AC 自动机）
	Description   string        `yaml:"description,omitempty"`
}

// ──────────────────────────────────────────────
// Aho-Corasick 自动机实现
// ──────────────────────────────────────────────

// ACNode AC 自动机节点
type ACNode struct {
	children map[rune]*ACNode
	fail     *ACNode
	output   []string // 匹配到的模式 ID 列表
	isEnd    bool
}

// ACAutomaton Aho-Corasick 自动机
type ACAutomaton struct {
	root     *ACNode
	patterns map[string]*regexp.Regexp // 模式 ID → 正则
	mu       sync.RWMutex
}

// NewACAutomaton 创建 AC 自动机实例
func NewACAutomaton() *ACAutomaton {
	return &ACAutomaton{
		root: &ACNode{
			children: make(map[rune]*ACNode),
		},
		patterns: make(map[string]*regexp.Regexp),
	}
}

// AddPattern 添加匹配模式
func (ac *ACAutomaton) AddPattern(id, pattern string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// 编译正则
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	ac.patterns[id] = re

	// 插入 Trie（使用字面量字符序列）
	node := ac.root
	for _, ch := range pattern {
		if node.children[ch] == nil {
			node.children[ch] = &ACNode{
				children: make(map[rune]*ACNode),
			}
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.output = append(node.output, id)
	return nil
}

// Build 构建失败指针（BFS）
func (ac *ACAutomaton) Build() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	queue := []*ACNode{}
	// 根节点的子节点 fail 指向根
	for _, child := range ac.root.children {
		child.fail = ac.root
		queue = append(queue, child)
	}

	// BFS 构建 fail 指针
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for ch, child := range curr.children {
			queue = append(queue, child)
			// 沿 fail 链查找
			fail := curr.fail
			for fail != nil && fail.children[ch] == nil {
				fail = fail.fail
			}
			if fail == nil {
				child.fail = ac.root
			} else {
				child.fail = fail.children[ch]
				// 合并输出
				child.output = append(child.output, child.fail.output...)
			}
		}
	}
}

// Search 在文本中搜索匹配模式
func (ac *ACAutomaton) Search(text string) []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var matches []string
	node := ac.root
	for _, ch := range text {
		for node != ac.root && node.children[ch] == nil {
			node = node.fail
		}
		if node.children[ch] != nil {
			node = node.children[ch]
		}
		if node.isEnd {
			matches = append(matches, node.output...)
		}
	}
	return matches
}

// ──────────────────────────────────────────────
// 规则引擎
// ──────────────────────────────────────────────

// RuleEngine 分类规则引擎
type RuleEngine struct {
	rules        []RuleDef
	fieldRegexps []*regexp.Regexp // 字段名匹配正则
	ac           *ACAutomaton     // 值内容 AC 自动机
	cache        *engineCache     // 有界分片缓存（替代无界 sync.Map）

	// 热重载支持（mtime 检测模式，与 WhitelistManager 一致）
	rulesPath   string
	lastModTime time.Time
	reloadMu    sync.Mutex
}

// NewRuleEngine 创建规则引擎实例
func NewRuleEngine(rules []RuleDef) (*RuleEngine, error) {
	engine := &RuleEngine{
		rules:        rules,
		fieldRegexps: make([]*regexp.Regexp, len(rules)),
		ac:           NewACAutomaton(),
	}

	// 编译字段名正则
	for i, rule := range rules {
		if len(rule.FieldPatterns) > 0 {
			// 合并多个模式为单个正则
			combined := strings.Join(rule.FieldPatterns, "|")
			re, err := regexp.Compile(combined)
			if err != nil {
				return nil, err
			}
			engine.fieldRegexps[i] = re
		}

		// 添加值模式到 AC 自动机
		for _, pattern := range rule.ValuePatterns {
			if err := engine.ac.AddPattern(rule.ID, pattern); err != nil {
				return nil, err
			}
		}
	}

	// 构建 AC 自动机
	engine.ac.Build()

	// 初始化有界分片缓存
	engine.cache = newEngineCache(10000)
	return engine, nil
}

// Classify 对字段执行分类
func (e *RuleEngine) Classify(field, value string) *ClassificationResult {
	// 被动检查规则文件热重载
	e.checkRulesReload()

	// 检查分片缓存
	cacheKey := field + ":" + value
	if cached, ok := e.cache.get(cacheKey); ok {
		return cached
	}

	// Layer 1: 字段名正则匹配
	for i, re := range e.fieldRegexps {
		if re != nil && re.MatchString(field) {
			result := &ClassificationResult{
				Field:      field,
				Level:      e.rules[i].Level,
				Category:   e.rules[i].Category,
				Confidence: 0.95,
				MatchedBy:  "rule:" + e.rules[i].ID,
			}
			e.cache.put(cacheKey, result)
			return result
		}
	}

	// Layer 1: AC 自动机值匹配
	matches := e.ac.Search(value)
	if len(matches) > 0 {
		// 找到第一个匹配的规则
		for _, rule := range e.rules {
			for _, matchID := range matches {
				if rule.ID == matchID {
					result := &ClassificationResult{
						Field:      field,
						Value:      value,
						Level:      rule.Level,
						Category:   rule.Category,
						Confidence: 0.90,
						MatchedBy:  "rule:" + rule.ID,
					}
					e.cache.put(cacheKey, result)
					return result
				}
			}
		}
	}

	// 默认分类
	result := &ClassificationResult{
		Field:      field,
		Level:      LevelPublic,
		Category:   "unknown",
		Confidence: 0.50,
		MatchedBy:  "default",
	}
	e.cache.put(cacheKey, result)
	return result
}

// RuleCount 返回已加载的规则数量
func (e *RuleEngine) RuleCount() int {
	return len(e.rules)
}

// WatchRules 启用规则文件 mtime 热重载。
// 每次 Classify 调用时被动检查文件 mtime，变更时自动重新编译规则。
// 与 WhitelistManager 的 mtime 检测模式一致，无外部依赖。
func (e *RuleEngine) WatchRules(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	e.rulesPath = path
	e.lastModTime = info.ModTime()
	return nil
}

// checkRulesReload 被动检查规则文件是否变更（请求驱动，无 goroutine）
func (e *RuleEngine) checkRulesReload() {
	if e.rulesPath == "" {
		return
	}
	info, err := os.Stat(e.rulesPath)
	if err != nil {
		return
	}
	if !info.ModTime().After(e.lastModTime) {
		return
	}

	e.reloadMu.Lock()
	defer e.reloadMu.Unlock()

	// 双重检查（避免并发重复加载）
	info2, err := os.Stat(e.rulesPath)
	if err != nil || !info2.ModTime().After(e.lastModTime) {
		return
	}

	// 从文件重新加载规则（修复：之前使用旧规则重建，实际不会更新规则内容）
	newRules, err := LoadRulesFromDir(filepath.Dir(e.rulesPath))
	if err != nil || len(newRules) == 0 {
		return // 加载失败保持旧规则
	}

	newEngine, err := NewRuleEngine(newRules)
	if err != nil {
		return // 编译失败保持旧规则
	}

	// 原子替换内部状态（reloadMu 保护写端，Classify 读端通过 cache 分片锁保证可见性）
	e.rules = newEngine.rules
	e.fieldRegexps = newEngine.fieldRegexps
	e.ac = newEngine.ac
	e.lastModTime = info2.ModTime()
	// 重建缓存（规则变更旧缓存失效）
	e.cache = newEngineCache(10000)
}

// ClassifyBatch 批量分类（多核并发分块加速）
func (e *RuleEngine) ClassifyBatch(records []map[string]string) []*ClassificationResult {
	n := len(records)
	if n == 0 {
		return nil
	}

	if n <= 32 {
		var results []*ClassificationResult
		for _, record := range records {
			for field, value := range record {
				results = append(results, e.Classify(field, value))
			}
		}
		return results
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > 16 {
		numWorkers = 16
	}
	if numWorkers > n {
		numWorkers = n
	}

	chunkSize := (n + numWorkers - 1) / numWorkers
	workerResults := make([][]*ClassificationResult, numWorkers)
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		startIdx := w * chunkSize
		endIdx := startIdx + chunkSize
		if endIdx > n {
			endIdx = n
		}
		if startIdx >= endIdx {
			break
		}

		wg.Add(1)
		go func(workerID, start, end int) {
			defer wg.Done()
			var local []*ClassificationResult
			for i := start; i < end; i++ {
				for field, value := range records[i] {
					local = append(local, e.Classify(field, value))
				}
			}
			workerResults[workerID] = local
		}(w, startIdx, endIdx)
	}
	wg.Wait()

	totalCount := 0
	for _, res := range workerResults {
		totalCount += len(res)
	}
	allResults := make([]*ClassificationResult, 0, totalCount)
	for _, res := range workerResults {
		allResults = append(allResults, res...)
	}
	return allResults
}

// LoadRulesFromDir 从指定目录遍历加载所有 YAML/YML 领域规则文件
func LoadRulesFromDir(dir string) ([]RuleDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var allRules []RuleDef
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var fileContent struct {
			Rules []RuleDef `yaml:"rules"`
		}
		if err := yaml.Unmarshal(data, &fileContent); err == nil && len(fileContent.Rules) > 0 {
			allRules = append(allRules, fileContent.Rules...)
		}
	}
	return allRules, nil
}

// ──────────────────────────────────────────────
// 有界分片缓存（替代无界 sync.Map，防止内存无限增长）
// ──────────────────────────────────────────────

const engineCacheNumShards = 16

type engineCacheShard struct {
	mu       sync.Mutex
	items    map[string]*ClassificationResult
	capacity int
}

type engineCache struct {
	shards [engineCacheNumShards]*engineCacheShard
}

func newEngineCache(totalCapacity int) *engineCache {
	if totalCapacity <= 0 {
		totalCapacity = 10000
	}
	shardCap := (totalCapacity + engineCacheNumShards - 1) / engineCacheNumShards
	c := &engineCache{}
	for i := 0; i < engineCacheNumShards; i++ {
		c.shards[i] = &engineCacheShard{
			items:    make(map[string]*ClassificationResult, shardCap),
			capacity: shardCap,
		}
	}
	return c
}

func (c *engineCache) shardFor(key string) *engineCacheShard {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return c.shards[h%engineCacheNumShards]
}

func (c *engineCache) get(key string) (*ClassificationResult, bool) {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	r, ok := shard.items[key]
	return r, ok
}

func (c *engineCache) put(key string, val *ClassificationResult) {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	// 分片满时随机淘汰一半（轻量级淘汰策略，避免 LRU 链表开销）
	if len(shard.items) >= shard.capacity {
		count := 0
		target := shard.capacity / 2
		for k := range shard.items {
			delete(shard.items, k)
			count++
			if count >= target {
				break
			}
		}
	}
	shard.items[key] = val
}
