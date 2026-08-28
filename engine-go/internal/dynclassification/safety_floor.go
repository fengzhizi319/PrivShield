// Package dynclassification 提供三层动态分类分级引擎扩展。
//
// safety_floor.go — 安全底线门禁仲裁器
package dynclassification

import (
	"log/slog"
	"sync"
)

// ──────────────────────────────────────────────
// 安全等级排序
// ──────────────────────────────────────────────

var levelRank = map[SecurityLevel]int{
	LevelPublic:       0,
	LevelInternal:     1,
	LevelConfidential: 2,
	LevelSecret:       3,
	LevelTopSecret:    4,
}

// LevelFromString 从字符串解析安全等级
func LevelFromString(s string) SecurityLevel {
	switch s {
	case "public":
		return LevelPublic
	case "internal":
		return LevelInternal
	case "confidential":
		return LevelConfidential
	case "secret":
		return LevelSecret
	case "top_secret":
		return LevelTopSecret
	default:
		return LevelPublic
	}
}

// LevelRank 返回安全等级排名（越高越敏感）
func LevelRank(level SecurityLevel) int {
	if r, ok := levelRank[level]; ok {
		return r
	}
	return 0
}

// MaxLevel 返回两个等级中较高的一个
func MaxLevel(a, b SecurityLevel) SecurityLevel {
	if LevelRank(a) >= LevelRank(b) {
		return a
	}
	return b
}

// ──────────────────────────────────────────────
// 安全底线仲裁器
// ──────────────────────────────────────────────

// SafetyFloorConfig 安全底线配置
type SafetyFloorConfig struct {
	// MinLevel 最低安全等级（任何分类结果不得低于此等级）
	MinLevel SecurityLevel
	// ConfidenceThreshold 置信度阈值（低于此值触发升级）
	ConfidenceThreshold float64
	// ForceUpgradeOnUncertainty 不确定时强制升级
	ForceUpgradeOnUncertainty bool
	// AuditLog 是否记录仲裁日志
	AuditLog bool
}

// DefaultSafetyFloorConfig 默认安全底线配置
func DefaultSafetyFloorConfig() SafetyFloorConfig {
	return SafetyFloorConfig{
		MinLevel:                  LevelPublic,
		ConfidenceThreshold:       0.6,
		ForceUpgradeOnUncertainty: true,
		AuditLog:                  true,
	}
}

// SafetyFloor 安全底线仲裁器
type SafetyFloor struct {
	config SafetyFloorConfig
	mu     sync.RWMutex
	audit  []ArbitrationEvent
}

// ArbitrationEvent 仲裁事件
type ArbitrationEvent struct {
	Field         string        `json:"field"`
	OriginalLevel SecurityLevel `json:"original_level"`
	FinalLevel    SecurityLevel `json:"final_level"`
	Reason        string        `json:"reason"`
	Confidence    float64       `json:"confidence"`
	EngineLayer   string        `json:"engine_layer"`
}

// NewSafetyFloor 创建安全底线仲裁器
func NewSafetyFloor(config SafetyFloorConfig) *SafetyFloor {
	return &SafetyFloor{
		config: config,
	}
}

// Arbitrate 对分类结果执行安全底线仲裁
func (sf *SafetyFloor) Arbitrate(result *ClassificationResult) *ClassificationResult {
	if result == nil {
		return nil
	}

	original := result.Level
	reason := ""

	// 规则 1：不低于最低安全等级
	if LevelRank(result.Level) < LevelRank(sf.config.MinLevel) {
		result.Level = sf.config.MinLevel
		reason = "below_minimum_level"
	}

	// 规则 2：低置信度触发升级
	if result.Confidence < sf.config.ConfidenceThreshold {
		if sf.config.ForceUpgradeOnUncertainty {
			nextLevel := sf.nextLevel(result.Level)
			if LevelRank(nextLevel) > LevelRank(result.Level) {
				result.Level = nextLevel
				if reason != "" {
					reason += "+low_confidence"
				} else {
					reason = "low_confidence"
				}
			}
		}
	}

	// 记录仲裁事件
	if reason != "" && sf.config.AuditLog {
		event := ArbitrationEvent{
			Field:         result.Field,
			OriginalLevel: original,
			FinalLevel:    result.Level,
			Reason:        reason,
			Confidence:    result.Confidence,
			EngineLayer:   result.MatchedBy,
		}
		sf.recordEvent(event)
		slog.Debug("Safety floor arbitration",
			"field", result.Field,
			"original", original,
			"final", result.Level,
			"reason", reason,
		)
	}

	return result
}

// ArbitrateBatch 批量仲裁
func (sf *SafetyFloor) ArbitrateBatch(results []*ClassificationResult) []*ClassificationResult {
	for i, r := range results {
		results[i] = sf.Arbitrate(r)
	}
	return results
}

// nextLevel 返回下一个更高的安全等级
func (sf *SafetyFloor) nextLevel(level SecurityLevel) SecurityLevel {
	switch level {
	case LevelPublic:
		return LevelInternal
	case LevelInternal:
		return LevelConfidential
	case LevelConfidential:
		return LevelSecret
	case LevelSecret:
		return LevelTopSecret
	default:
		return level
	}
}

// recordEvent 记录仲裁事件
func (sf *SafetyFloor) recordEvent(event ArbitrationEvent) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.audit = append(sf.audit, event)
	// 限制审计日志大小
	if len(sf.audit) > 10000 {
		sf.audit = sf.audit[len(sf.audit)-5000:]
	}
}

// AuditEvents 返回仲裁审计事件
func (sf *SafetyFloor) AuditEvents() []ArbitrationEvent {
	sf.mu.RLock()
	defer sf.mu.RUnlock()
	events := make([]ArbitrationEvent, len(sf.audit))
	copy(events, sf.audit)
	return events
}

// UpdateConfig 更新安全底线配置
func (sf *SafetyFloor) UpdateConfig(config SafetyFloorConfig) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.config = config
}
