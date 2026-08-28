// Package service 提供 PrivacyService 统一编排层。
//
// 将隐私原语、分类引擎、预算会计、医疗流水线等组件串联为统一服务接口，
// 供 REST 和 gRPC 控制器调用。
package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/dynclassification"
	"github.com/fengzhizi319/PrivShield/pkg/naming"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/budget"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/dp"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/kano"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/ldp"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/masking"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/medical"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/qol"
)

// ──────────────────────────────────────────────
// PrivacyService 统一编排
// ──────────────────────────────────────────────

// PrivacyService 隐私服务编排器
type PrivacyService struct {
	classifier    *dynclassification.RuleEngine
	safetyFloor   *dynclassification.SafetyFloor
	budget        *budget.BudgetAccountant
	medicalYibao  *medical.Pipeline
	medicalKang   *medical.Pipeline
	mu            sync.RWMutex
}

// Config 服务配置
type Config struct {
	TotalEpsilon    float64
	TotalDelta      float64
	BudgetWindowSec int64
	Rules           []dynclassification.RuleDef
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		TotalEpsilon:    10.0,
		TotalDelta:      1e-5,
		BudgetWindowSec: 3600,
		Rules:           defaultRules(),
	}
}

// NewPrivacyService 创建隐私服务实例
func NewPrivacyService(cfg Config) (*PrivacyService, error) {
	engine, err := dynclassification.NewRuleEngine(cfg.Rules)
	if err != nil {
		return nil, fmt.Errorf("init rule engine: %w", err)
	}

	return &PrivacyService{
		classifier:    engine,
		safetyFloor:   dynclassification.NewSafetyFloor(dynclassification.DefaultSafetyFloorConfig()),
		budget:        budget.NewBudgetAccountant(cfg.TotalEpsilon, cfg.TotalDelta, cfg.BudgetWindowSec),
		medicalYibao:  medical.NewYibaoPipeline(),
		medicalKang:   medical.NewKangyangPipeline(),
	}, nil
}

// ──────────────────────────────────────────────
// 掩码 API
// ──────────────────────────────────────────────

// MaskField 对单个字段执行脱敏
func (s *PrivacyService) MaskField(fieldType, value string) (string, error) {
	switch fieldType {
	case "id_card":
		return masking.MaskIdCard(value), nil
	case "phone":
		return masking.MaskPhone(value), nil
	case "bank_card":
		return masking.MaskBankCard(value), nil
	case "name":
		return masking.MaskChineseName(value), nil
	case "email":
		return masking.MaskEmail(value), nil
	case "address":
		return masking.MaskAddress(value), nil
	case "officer_id":
		return masking.MaskOfficerId(value), nil
	default:
		return "", fmt.Errorf("unknown mask type: %s", fieldType)
	}
}

// MaskRecord 对整条记录执行自动脱敏（基于字段名推断类型）
func (s *PrivacyService) MaskRecord(record map[string]string) map[string]string {
	result := make(map[string]string, len(record))
	for k, v := range record {
		result[k] = s.autoMaskField(k, v)
	}
	return result
}

// MaskBatch 批量脱敏
func (s *PrivacyService) MaskBatch(records []map[string]string) []map[string]string {
	results := make([]map[string]string, len(records))
	for i, r := range records {
		results[i] = s.MaskRecord(r)
	}
	return results
}

// ──────────────────────────────────────────────
// 差分隐私 API
// ──────────────────────────────────────────────

// NoisyCount 噪声计数
func (s *PrivacyService) NoisyCount(ctx context.Context, count int, epsilon float64) (float64, error) {
	if !s.budget.Consume(epsilon, 0) {
		return 0, fmt.Errorf("privacy budget exhausted")
	}
	return dp.NoisyCount(count, epsilon), nil
}

// NoisySum 噪声求和
func (s *PrivacyService) NoisySum(ctx context.Context, values []float64, epsilon, sensitivity float64) (float64, error) {
	if !s.budget.Consume(epsilon, 0) {
		return 0, fmt.Errorf("privacy budget exhausted")
	}
	return dp.NoisySum(values, epsilon, sensitivity), nil
}

// NoisyMean 噪声均值
func (s *PrivacyService) NoisyMean(ctx context.Context, values []float64, epsilon, delta, clipBound float64) (float64, error) {
	if !s.budget.Consume(epsilon, delta) {
		return 0, fmt.Errorf("privacy budget exhausted")
	}
	return dp.NoisyMean(values, epsilon, delta, clipBound), nil
}

// ──────────────────────────────────────────────
// 本地差分隐私 API
// ──────────────────────────────────────────────

// RandomizedResponse 二值随机响应
func (s *PrivacyService) RandomizedResponse(value bool, epsilon float64) bool {
	return ldp.RandomizedResponse(value, epsilon)
}

// ORRResponse 多类别优化随机响应
func (s *PrivacyService) ORRResponse(value int, epsilon float64, domainSize int) int {
	return ldp.ORRResponse(value, epsilon, domainSize)
}

// PerturbBinaryBatch 批量二值扰动（与 Python perturb_binary_batch 对齐）
func (s *PrivacyService) PerturbBinaryBatch(values []int, epsilon float64) []int {
	return ldp.PerturbBinaryBatch(values, epsilon)
}

// PerturbCategoricalBatch 批量类别扰动（与 Python perturb_categorical_batch 对齐）
func (s *PrivacyService) PerturbCategoricalBatch(values []string, categories []string, epsilon float64) []string {
	return ldp.PerturbCategoricalBatch(values, categories, epsilon)
}

// EstimateBinaryFrequency 二值频率无偏估计（与 Python estimate_binary_frequency 对齐）
func (s *PrivacyService) EstimateBinaryFrequency(reportedValues []int, epsilon float64) float64 {
	return ldp.EstimateBinaryFrequency(reportedValues, epsilon)
}

// EstimateCategoricalHistogram 类别直方图无偏估计（与 Python estimate_categorical_histogram 对齐）
func (s *PrivacyService) EstimateCategoricalHistogram(reportedValues []string, categories []string, epsilon float64) map[string]float64 {
	return ldp.EstimateCategoricalHistogram(reportedValues, categories, epsilon)
}

// ──────────────────────────────────────────────
// K-匿名 API
// ──────────────────────────────────────────────

// KAnonymize K-匿名处理
func (s *PrivacyService) KAnonymize(records []kano.Record, qiFields []string, k int) (*kano.AnonymizationResult, error) {
	return kano.Anonymize(records, qiFields, k)
}

// ──────────────────────────────────────────────
// 查询混淆 API
// ──────────────────────────────────────────────

// ObfuscateQuery 查询混淆
func (s *PrivacyService) ObfuscateQuery(query string, numDecoys int, domain string) ([]string, int) {
	return qol.InjectDecoys(query, numDecoys, domain)
}

// ObfuscateQueryBatch 批量查询混淆（与 Python obfuscate_query_batch 对齐）
func (s *PrivacyService) ObfuscateQueryBatch(queries []string, numDecoys int, domain string) [][]string {
	results := make([][]string, len(queries))
	for i, q := range queries {
		injected, _ := qol.InjectDecoys(q, numDecoys, domain)
		results[i] = injected
	}
	return results
}

// ──────────────────────────────────────────────
// 动态分类 API
// ──────────────────────────────────────────────

// Classify 动态分类
func (s *PrivacyService) Classify(field, value string) *dynclassification.ClassificationResult {
	result := s.classifier.Classify(field, value)
	return s.safetyFloor.Arbitrate(result)
}

// ClassifyBatch 批量分类
func (s *PrivacyService) ClassifyBatch(records []map[string]string) []*dynclassification.ClassificationResult {
	results := s.classifier.ClassifyBatch(records)
	return s.safetyFloor.ArbitrateBatch(results)
}

// ──────────────────────────────────────────────
// 医疗流水线 API
// ──────────────────────────────────────────────

// SanitizeMedicalRecord 医疗记录脱敏。
// domain 参数支持任意入站表示（canonical id / api_code / 别名），
// 通过 naming.NormalizeDataSourceID 归一化后路由到对应流水线。
// 未知数据源触发 Fail-Closed（设计文档 §3.3）。
func (s *PrivacyService) SanitizeMedicalRecord(record map[string]string, domain string) (map[string]string, error) {
	dsID, err := naming.NormalizeDataSourceID(domain)
	if err != nil {
		return nil, fmt.Errorf("INVALID_DATASOURCE_ID: %w", err)
	}
	switch dsID {
	case naming.DSYibao:
		return s.medicalYibao.SanitizeRecord(record), nil
	case naming.DSKangyang:
		return s.medicalKang.SanitizeRecord(record), nil
	default:
		return nil, fmt.Errorf("unsupported datasource: %s", dsID)
	}
}

// SanitizeMedicalBatch 批量医疗脱敏（SSOT 归一化 + Fail-Closed）。
func (s *PrivacyService) SanitizeMedicalBatch(records []map[string]string, domain string) ([]map[string]string, error) {
	dsID, err := naming.NormalizeDataSourceID(domain)
	if err != nil {
		return nil, fmt.Errorf("INVALID_DATASOURCE_ID: %w", err)
	}
	switch dsID {
	case naming.DSYibao:
		return s.medicalYibao.SanitizeBatch(records), nil
	case naming.DSKangyang:
		return s.medicalKang.SanitizeBatch(records), nil
	default:
		return nil, fmt.Errorf("unsupported datasource: %s", dsID)
	}
}

// ──────────────────────────────────────────────
// 预算查询 API
// ──────────────────────────────────────────────

// BudgetStatus 预算状态
func (s *PrivacyService) BudgetStatus() map[string]float64 {
	return map[string]float64{
		"total_epsilon":     s.budget.TotalEpsilon(),
		"used_epsilon":      s.budget.UsedEpsilon(),
		"remaining_epsilon": s.budget.RemainingEpsilon(),
		"total_delta":       s.budget.TotalDelta(),
		"used_delta":        s.budget.UsedDelta(),
		"remaining_delta":   s.budget.RemainingDelta(),
	}
}

// BudgetReset 重置预算
func (s *PrivacyService) BudgetReset() map[string]float64 {
	s.budget.Reset()
	return s.BudgetStatus()
}

// ──────────────────────────────────────────────
// HMAC 散列 API
// ──────────────────────────────────────────────

// HashHMAC HMAC 加盐散列
func (s *PrivacyService) HashHMAC(value, salt string) string {
	return masking.HashHMAC(value, salt)
}

// ──────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────

func (s *PrivacyService) autoMaskField(fieldName, value string) string {
	if value == "" {
		return ""
	}
	// 基于字段名推断脱敏策略
	lower := fieldName
	for _, r := range lower {
		if r >= 'A' && r <= 'Z' {
			lower = string(r+32) + lower[1:]
			break
		}
	}
	switch {
	case containsAny(lower, "id_card", "idcard", "cert_no", "identity"):
		return masking.MaskIdCard(value)
	case containsAny(lower, "phone", "mobile", "tel"):
		return masking.MaskPhone(value)
	case containsAny(lower, "bank", "credit_card"):
		return masking.MaskBankCard(value)
	case containsAny(lower, "name", "patient_name", "user_name"):
		return masking.MaskChineseName(value)
	case containsAny(lower, "email", "mail"):
		return masking.MaskEmail(value)
	case containsAny(lower, "address", "addr"):
		return masking.MaskAddress(value)
	default:
		return value
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// defaultRules 默认分类规则
func defaultRules() []dynclassification.RuleDef {
	return []dynclassification.RuleDef{
		{
			ID:            "id_card",
			Level:         dynclassification.LevelSecret,
			Category:      "pii.identity",
			FieldPatterns: []string{`(?i)(id_?card|身份证|identity|cert_no)`},
			Description:   "中国居民身份证",
		},
		{
			ID:            "phone",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)(phone|mobile|手机|电话|tel)`},
			Description:   "手机号码",
		},
		{
			ID:            "email",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)(email|邮箱|邮件|mail)`},
			Description:   "电子邮箱",
		},
		{
			ID:            "bank_card",
			Level:         dynclassification.LevelSecret,
			Category:      "pii.financial",
			FieldPatterns: []string{`(?i)(bank_?card|银行卡|信用卡|credit_card)`},
			Description:   "银行卡号",
		},
		{
			ID:            "name",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.identity",
			FieldPatterns: []string{`(?i)(^name$|patient_name|user_name|姓名)`},
			Description:   "个人姓名",
		},
		{
			ID:            "address",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.location",
			FieldPatterns: []string{`(?i)(address|地址|住址|home_address)`},
			Description:   "个人地址",
		},
		{
			ID:            "medical_record",
			Level:         dynclassification.LevelSecret,
			Category:      "medical.record",
			FieldPatterns: []string{`(?i)(medical_record|病历|诊断|diagnosis)`},
			Description:   "医疗记录",
		},
		{
			ID:            "social_security",
			Level:         dynclassification.LevelTopSecret,
			Category:      "pii.financial",
			FieldPatterns: []string{`(?i)(social_security|社保|医保号)`},
			Description:   "社保号码",
		},
	}
}
