// Package dynclassification 提供三层动态分类分级漏斗（Rule Engine → Small-NER → External LLM Arbitration）。
//
// 架构设计（设计文档 §3）：
//  1. Layer 1（规则层）：基于 Aho-Corasick 自动机与字段名正则快速匹配（零 ML 开销，< 50μs）；
//  2. Layer 2（NER 实体层）：Small-NER 实体抽取（识别姓名、证件、电话、住址、疾病诊断、ICD-10 等）；
//  3. Layer 3（LLM 仲裁层）：通过 HTTP 连接池调度外部独立 LLM（vLLM / Ollama），无需内嵌 PyTorch；
//  4. Safety Floor（安全底线）：全链路异常/超时时的 Fail-closed 安全托底。
package dynclassification

import (
	"context"
	"strings"
	"time"
)

// FunnelConfig 分类漏斗配置
type FunnelConfig struct {
	RuleConfidenceThreshold float64       // Layer 1 规则置信度阈值 (默认 0.85)
	NERConfidenceThreshold  float64       // Layer 2 NER 置信度阈值 (默认 0.80)
	EnableNER               bool          // 是否开启 Layer 2 NER
	EnableLLM               bool          // 是否开启 Layer 3 LLM 外部仲裁
	LLMTimeout              time.Duration // LLM 仲裁单次超时时间
}

// DefaultFunnelConfig 默认漏斗配置
func DefaultFunnelConfig() FunnelConfig {
	return FunnelConfig{
		RuleConfidenceThreshold: 0.85,
		NERConfidenceThreshold:  0.80,
		EnableNER:               true,
		EnableLLM:               false, // 按需通过配置或环境变量开启
		LLMTimeout:              5 * time.Second,
	}
}

// ClassificationFunnel 三层分类分级漏斗执行器
type ClassificationFunnel struct {
	ruleEngine  *RuleEngine
	nerEngine   NerEngine
	llmClient   *LLMClient
	safetyFloor *SafetyFloor
	cfg         FunnelConfig
}

// NewClassificationFunnel 创建三层分类分级漏斗实例
func NewClassificationFunnel(
	rules []RuleDef,
	nerEngine NerEngine,
	llmClient *LLMClient,
	cfg FunnelConfig,
) (*ClassificationFunnel, error) {
	engine, err := NewRuleEngine(rules)
	if err != nil {
		return nil, err
	}

	if nerEngine == nil {
		nerEngine = NewRuleBasedNerEngine()
	}

	return &ClassificationFunnel{
		ruleEngine:  engine,
		nerEngine:   nerEngine,
		llmClient:   llmClient,
		safetyFloor: NewSafetyFloor(DefaultSafetyFloorConfig()),
		cfg:         cfg,
	}, nil
}

// Classify 执行 3 层漏斗分级仲裁
func (f *ClassificationFunnel) Classify(ctx context.Context, field, value string) (*ClassificationResult, error) {
	// ─── Layer 1: 规则引擎匹配 ───
	res := f.ruleEngine.Classify(field, value)
	if res.Confidence >= f.cfg.RuleConfidenceThreshold && res.MatchedBy != "default" {
		return res, nil
	}

	// ─── Layer 2: Small-NER 实体抽取 ───
	if f.cfg.EnableNER && f.nerEngine != nil && f.nerEngine.IsAvailable() && value != "" {
		entities, err := f.nerEngine.Extract(ctx, value)
		if err == nil && len(entities) > 0 {
			bestEntity := selectHighestRiskEntity(entities)
			if bestEntity.Confidence >= f.cfg.NERConfidenceThreshold {
				level, category := mapNERLabelToSecurity(bestEntity.Label)
				return &ClassificationResult{
					Field:      field,
					Value:      value,
					Level:      level,
					Category:   category,
					Confidence: bestEntity.Confidence,
					MatchedBy:  "ner:" + bestEntity.Label,
				}, nil
			}
		}
	}

	// ─── Layer 3: 外部 LLM 仲裁服务 ───
	if f.cfg.EnableLLM && f.llmClient != nil {
		llmCtx, cancel := context.WithTimeout(ctx, f.cfg.LLMTimeout)
		defer cancel()

		if f.llmClient.IsAvailable(llmCtx) {
			llmResp, err := f.llmClient.Classify(llmCtx, LLMRequest{
				Field: field,
				Value: value,
			})
			if err == nil && llmResp != nil && llmResp.Confidence >= 0.70 {
				return &ClassificationResult{
					Field:      field,
					Value:      value,
					Level:      SecurityLevel(llmResp.Level),
					Category:   llmResp.Category,
					Confidence: llmResp.Confidence,
					MatchedBy:  "llm",
				}, nil
			}
		}
	}

	// ─── Safety Floor: 兜底安全等级 ───
	return f.safetyFloor.Arbitrate(res), nil
}

// selectHighestRiskEntity 选出风险最高且置信度最高的实体
func selectHighestRiskEntity(entities []NerEntity) NerEntity {
	var best NerEntity
	bestRank := -1

	for _, e := range entities {
		rank := getRiskRank(e.Label)
		if rank > bestRank || (rank == bestRank && e.Confidence > best.Confidence) {
			best = e
			bestRank = rank
		}
	}
	return best
}

func getRiskRank(label string) int {
	switch strings.ToUpper(label) {
	case "ID_CARD", "BANK_CARD", "PASSPORT", "MILITARY_ID":
		return 5 // TopSecret
	case "DISEASE", "MEDICAL_CONDITION", "ICD10_CODE", "HIV", "PSYCHIATRIC":
		return 4 // Secret
	case "PHONE", "EMAIL", "ADDRESS", "PERSON":
		return 3 // Confidential
	case "ORG", "ORGANIZATION":
		return 2 // Internal
	default:
		return 1 // Public
	}
}

func mapNERLabelToSecurity(label string) (SecurityLevel, string) {
	switch strings.ToUpper(label) {
	case "ID_CARD":
		return LevelTopSecret, "pii.identity"
	case "BANK_CARD":
		return LevelTopSecret, "pii.financial"
	case "PASSPORT", "MILITARY_ID":
		return LevelTopSecret, "pii.identity"
	case "DISEASE", "MEDICAL_CONDITION", "ICD10_CODE", "HIV", "PSYCHIATRIC":
		return LevelSecret, "medical.condition"
	case "PHONE":
		return LevelConfidential, "pii.contact"
	case "EMAIL":
		return LevelConfidential, "pii.contact"
	case "ADDRESS":
		return LevelConfidential, "pii.location"
	case "PERSON":
		return LevelConfidential, "pii.identity"
	case "ORG", "ORGANIZATION":
		return LevelInternal, "entity.organization"
	default:
		return LevelPublic, "unknown"
	}
}
