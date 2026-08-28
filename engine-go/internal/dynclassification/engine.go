// Package dynclassification 提供三层动态分类分级引擎。
//
// Layer 1: Aho-Corasick 自动机 + 字段名正则快速匹配（零 ML 开销）
// Layer 2: Small-NER 实体识别（可选，ONNX Runtime）
// Layer 3: LLM/VLM 仲裁（可选，CUDA 推理）
//
// 本文件实现 Layer 1 规则引擎核心。
package dynclassification

import (
	"regexp"
	"strings"
	"sync"
)

// ──────────────────────────────────────────────
// 类型定义
// ──────────────────────────────────────────────

// SecurityLevel 安全等级
type SecurityLevel string

const (
	LevelPublic     SecurityLevel = "public"
	LevelInternal   SecurityLevel = "internal"
	LevelConfidential SecurityLevel = "confidential"
	LevelSecret     SecurityLevel = "secret"
	LevelTopSecret  SecurityLevel = "top_secret"
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
	ID         string        `yaml:"id"`
	Level      SecurityLevel `yaml:"level"`
	Category   string        `yaml:"category"`
	FieldPatterns []string   `yaml:"field_patterns,omitempty"` // 字段名正则
	ValuePatterns []string   `yaml:"value_patterns,omitempty"` // 值内容正则（AC 自动机）
	Description string       `yaml:"description,omitempty"`
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
	root    *ACNode
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
	cache        sync.Map         // LRU 缓存（简化版）
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
	return engine, nil
}

// Classify 对字段执行分类
func (e *RuleEngine) Classify(field, value string) *ClassificationResult {
	// 检查缓存
	cacheKey := field + ":" + value
	if cached, ok := e.cache.Load(cacheKey); ok {
		return cached.(*ClassificationResult)
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
			e.cache.Store(cacheKey, result)
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
					e.cache.Store(cacheKey, result)
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
	e.cache.Store(cacheKey, result)
	return result
}

// ClassifyBatch 批量分类
func (e *RuleEngine) ClassifyBatch(records []map[string]string) []*ClassificationResult {
	var results []*ClassificationResult
	for _, record := range records {
		for field, value := range record {
			results = append(results, e.Classify(field, value))
		}
	}
	return results
}
