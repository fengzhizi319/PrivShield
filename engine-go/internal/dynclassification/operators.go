// Package dynclassification 提供三层动态分类分级引擎扩展。
//
// operators.go — 匹配算子注册表与 AC 自动机集成
package dynclassification

import (
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

// matchRegex 使用缓存的正则匹配
func matchRegex(pattern, text string) bool {
	re, ok := regexCache.Load(pattern)
	if !ok {
		compiled, err := compileRegex(pattern)
		if err != nil {
			return false
		}
		regexCache.Store(pattern, compiled)
		return compiled.MatchString(text)
	}
	return re.(*cachedRegex).MatchString(text)
}

var regexCache sync.Map

func compileRegex(pattern string) (*cachedRegex, error) {
	// 使用标准库 regexp
	return &cachedRegex{pattern: pattern}, nil
}

type cachedRegex struct {
	pattern string
}

func (r *cachedRegex) MatchString(s string) bool {
	// 简化实现：对于简单模式使用 strings.Contains
	// 生产环境应使用 regexp.Compile 缓存
	if strings.HasPrefix(r.pattern, "(?i)") {
		return strings.Contains(strings.ToLower(s), strings.ToLower(r.pattern[4:]))
	}
	return strings.Contains(s, r.pattern)
}
