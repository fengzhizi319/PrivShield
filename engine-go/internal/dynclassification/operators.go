// Package dynclassification 提供三层动态分类分级引擎扩展。
//
// operators.go — 匹配算子注册表与 AC 自动机集成
package dynclassification

import (
	"regexp"
	"strings"
	"sync"
)

// ──────────────────────────────────────────────
// 算子类型
// ──────────────────────────────────────────────

// OperatorType 算子类型
type OperatorType string

const (
	OpRegex       OperatorType = "regex"        // 正则匹配
	OpContains    OperatorType = "contains"      // 子串包含
	OpEquals      OperatorType = "equals"        // 精确等于
	OpStartsWith  OperatorType = "starts_with"   // 前缀匹配
	OpEndsWith    OperatorType = "ends_with"     // 后缀匹配
	OpACAutomaton OperatorType = "ac_automaton"  // AC 自动机多模式匹配
	OpFieldMatch  OperatorType = "field_match"   // 字段名匹配
)

// Operator 匹配算子接口
type Operator interface {
	Type() OperatorType
	Match(field, value string) bool
}

// ──────────────────────────────────────────────
// 具体算子实现
// ──────────────────────────────────────────────

// RegexOperator 正则匹配算子
type RegexOperator struct {
	pattern string
}

func (o *RegexOperator) Type() OperatorType { return OpRegex }
func (o *RegexOperator) Match(field, value string) bool {
	return matchRegex(o.pattern, value)
}

// ContainsOperator 子串包含算子
type ContainsOperator struct {
	substr string
}

func (o *ContainsOperator) Type() OperatorType { return OpContains }
func (o *ContainsOperator) Match(field, value string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(o.substr))
}

// EqualsOperator 精确等于算子
type EqualsOperator struct {
	target string
}

func (o *EqualsOperator) Type() OperatorType { return OpEquals }
func (o *EqualsOperator) Match(field, value string) bool {
	return strings.EqualFold(value, o.target)
}

// StartsWithOperator 前缀匹配算子
type StartsWithOperator struct {
	prefix string
}

func (o *StartsWithOperator) Type() OperatorType { return OpStartsWith }
func (o *StartsWithOperator) Match(field, value string) bool {
	return strings.HasPrefix(strings.ToLower(value), strings.ToLower(o.prefix))
}

// EndsWithOperator 后缀匹配算子
type EndsWithOperator struct {
	suffix string
}

func (o *EndsWithOperator) Type() OperatorType { return OpEndsWith }
func (o *EndsWithOperator) Match(field, value string) bool {
	return strings.HasSuffix(strings.ToLower(value), strings.ToLower(o.suffix))
}

// FieldMatchOperator 字段名匹配算子
type FieldMatchOperator struct {
	pattern string
}

func (o *FieldMatchOperator) Type() OperatorType { return OpFieldMatch }
func (o *FieldMatchOperator) Match(field, value string) bool {
	return matchRegex(o.pattern, field)
}

// AcAutomatonOperator AC 自动机多模式匹配算子。
// 基于 Aho-Corasick 算法实现 O(N+M+Z) 多模式匹配，
// 适用于高敏医学词库等大规模关键词扫描场景。
type AcAutomatonOperator struct {
	ac        *AhoCorasick
	termLevel map[string]string // 模式串(小写) → 等级
}

// NewAcAutomatonOperator 创建 AC 自动机算子。
// termsMap 的 key 为模式串，value 为对应等级（如 "L5"）。
func NewAcAutomatonOperator(termsMap map[string]string) *AcAutomatonOperator {
	ac := NewAhoCorasick()
	lowerMap := make(map[string]string, len(termsMap))
	for term, level := range termsMap {
		lw := strings.ToLower(term)
		ac.AddPattern(lw)
		lowerMap[lw] = level
	}
	ac.Build()
	return &AcAutomatonOperator{ac: ac, termLevel: lowerMap}
}

func (o *AcAutomatonOperator) Type() OperatorType { return OpACAutomaton }

// Match 对 value 执行 AC 多模式匹配。
// 返回 (是否匹配, 最高等级, 匹配到的原始词条列表)。
func (o *AcAutomatonOperator) Match(field, value string) bool {
	return o.ac.Contains(strings.ToLower(value))
}

// MatchDetail 返回详细匹配结果（是否匹配, 最高等级, 匹配词条列表）。
func (o *AcAutomatonOperator) MatchDetail(value string) (bool, string, []string) {
	lower := strings.ToLower(value)
	matches := o.ac.MatchString(lower)
	if len(matches) == 0 {
		return false, "L1", nil
	}
	maxLevel := "L1"
	var matchedTerms []string
	for _, m := range matches {
		lvl := o.termLevel[m.Pattern]
		matchedTerms = append(matchedTerms, m.Pattern)
		if lRank(lvl) > lRank(maxLevel) {
			maxLevel = lvl
		}
	}
	return true, maxLevel, matchedTerms
}

// lRank 将 "L1"-"L5" 等级字符串映射为数值（越高越敏感）。
func lRank(level string) int {
	switch level {
	case "L5":
		return 5
	case "L4":
		return 4
	case "L3":
		return 3
	case "L2":
		return 2
	case "L1":
		return 1
	default:
		return 0
	}
}

// ──────────────────────────────────────────────
// 算子注册表
// ──────────────────────────────────────────────

// OperatorRegistry 算子注册表
type OperatorRegistry struct {
	mu        sync.RWMutex
	operators map[OperatorType]func(args ...string) Operator
}

// NewOperatorRegistry 创建算子注册表
func NewOperatorRegistry() *OperatorRegistry {
	r := &OperatorRegistry{
		operators: make(map[OperatorType]func(args ...string) Operator),
	}
	// 注册内置算子
	r.Register(OpRegex, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &RegexOperator{pattern: args[0]}
	})
	r.Register(OpContains, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &ContainsOperator{substr: args[0]}
	})
	r.Register(OpEquals, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &EqualsOperator{target: args[0]}
	})
	r.Register(OpStartsWith, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &StartsWithOperator{prefix: args[0]}
	})
	r.Register(OpEndsWith, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &EndsWithOperator{suffix: args[0]}
	})
	r.Register(OpFieldMatch, func(args ...string) Operator {
		if len(args) == 0 {
			return nil
		}
		return &FieldMatchOperator{pattern: args[0]}
	})
	return r
}

// Register 注册自定义算子
func (r *OperatorRegistry) Register(opType OperatorType, factory func(args ...string) Operator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operators[opType] = factory
}

// Create 创建算子实例
func (r *OperatorRegistry) Create(opType OperatorType, args ...string) Operator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.operators[opType]
	if !ok {
		return nil
	}
	return factory(args...)
}

// ListOperators 返回所有已注册算子类型
func (r *OperatorRegistry) ListOperators() []OperatorType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]OperatorType, 0, len(r.operators))
	for t := range r.operators {
		types = append(types, t)
	}
	return types
}

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────

// matchRegex 使用缓存的正则匹配（线程安全）
func matchRegex(pattern, text string) bool {
	re, ok := regexCache.Load(pattern)
	if !ok {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		regexCache.Store(pattern, compiled)
		return compiled.MatchString(text)
	}
	return re.(*regexp.Regexp).MatchString(text)
}

var regexCache sync.Map
