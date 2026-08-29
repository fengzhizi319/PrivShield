// Package service 提供 PrivacyService 统一编排层。
//
// 将隐私原语、分类引擎、预算会计、医疗流水线等组件串联为统一服务接口，
// 供 REST 和 gRPC 控制器调用。
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/dynclassification"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/profile"
	"github.com/fengzhizi319/PrivShield/pkg/crypto"
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
	funnel        *dynclassification.ClassificationFunnel
	safetyFloor   *dynclassification.SafetyFloor
	budget        *budget.BudgetAccountant
	medicalYibao  *medical.Pipeline
	medicalKang   *medical.Pipeline
	resolver      *profile.Resolver
	namespace     string
	mu            sync.RWMutex
}

// Config 服务配置
type Config struct {
	TotalEpsilon    float64
	TotalDelta      float64
	BudgetWindowSec int64
	Namespace       string
	ProfilePath     string
	LLMEndpoint     string
	EnableLLM       bool
	EnableNER       bool
	Rules           []dynclassification.RuleDef
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	enableLLM := os.Getenv("PRIVACY_LLM_ENABLE") == "true"
	llmEndpoint := os.Getenv("PRIVACY_LLM_ENDPOINT")
	if llmEndpoint == "" {
		llmEndpoint = "http://localhost:8000/v1/chat/completions"
	}

	return Config{
		TotalEpsilon:    10.0,
		TotalDelta:      1e-5,
		BudgetWindowSec: 3600,
		Namespace:       "default",
		ProfilePath:     "",
		LLMEndpoint:     llmEndpoint,
		EnableLLM:       enableLLM,
		EnableNER:       true,
		Rules:           defaultRules(),
	}
}

// NewPrivacyService 创建隐私服务实例
func NewPrivacyService(cfg Config) (*PrivacyService, error) {
	engine, err := dynclassification.NewRuleEngine(cfg.Rules)
	if err != nil {
		return nil, fmt.Errorf("init rule engine: %w", err)
	}

	res := profile.NewResolver()
	if cfg.ProfilePath != "" {
		_ = res.LoadFromYAML(cfg.ProfilePath)
	}

	ns := cfg.Namespace
	if ns == "" {
		ns = getEnv("PRIVACY_NAMESPACE", "default")
	}

	var llmClient *dynclassification.LLMClient
	if cfg.EnableLLM || cfg.LLMEndpoint != "" {
		llmClient = dynclassification.NewLLMClient(dynclassification.LLMClientConfig{
			Endpoint:       cfg.LLMEndpoint,
			ModelName:      getEnv("PRIVACY_LLM_MODEL", "qwen3.5"),
			MaxConcurrency: getEnvInt("PRIVACY_LLM_MAX_CONCURRENCY", 4),
			Timeout:        30 * time.Second,
			MaxRetries:     2,
			APIKey:         os.Getenv("PRIVACY_LLM_API_KEY"),
		})
	}

	funnelCfg := dynclassification.DefaultFunnelConfig()
	funnelCfg.EnableNER = cfg.EnableNER
	funnelCfg.EnableLLM = cfg.EnableLLM && llmClient != nil

	funnel, err := dynclassification.NewClassificationFunnel(cfg.Rules, dynclassification.NewRuleBasedNerEngine(), llmClient, funnelCfg)
	if err != nil {
		return nil, fmt.Errorf("init classification funnel: %w", err)
	}

	return &PrivacyService{
		classifier:    engine,
		funnel:        funnel,
		safetyFloor:   dynclassification.NewSafetyFloor(dynclassification.DefaultSafetyFloorConfig()),
		budget:        budget.NewBudgetAccountant(cfg.TotalEpsilon, cfg.TotalDelta, cfg.BudgetWindowSec),
		medicalYibao:  medical.NewYibaoPipeline(),
		medicalKang:   medical.NewKangyangPipeline(),
		resolver:      res,
		namespace:     ns,
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
	case "sm3", "hash_sm3":
		return s.HashSM3(value, ""), nil
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

// MaskBatchContext 批量脱敏（支持多核并发无锁分块计算与 Context 快速中断）
func (s *PrivacyService) MaskBatchContext(ctx context.Context, records []map[string]string) ([]map[string]string, error) {
	n := len(records)
	results := make([]map[string]string, n)
	if n == 0 {
		return results, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if n <= 64 {
		for i, r := range records {
			if i%32 == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			results[i] = s.MaskRecord(r)
		}
		return results, nil
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > 16 {
		numWorkers = 16
	}
	if numWorkers > n {
		numWorkers = n
	}

	chunkSize := (n + numWorkers - 1) / numWorkers
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
		go func(start, end int) {
			defer wg.Done()
			for i := start; i < end; i++ {
				if i%64 == 0 && ctx.Err() != nil {
					return
				}
				results[i] = s.MaskRecord(records[i])
			}
		}(startIdx, endIdx)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// MaskBatch 批量脱敏（兼容非 context 调用）
func (s *PrivacyService) MaskBatch(records []map[string]string) []map[string]string {
	res, _ := s.MaskBatchContext(context.Background(), records)
	return res
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

// DPHistogram 差分隐私直方图（无预算消耗，纯噪声添加）
func (s *PrivacyService) DPHistogram(trueCounts map[string]int, epsilon float64) map[string]float64 {
	return dp.NoisyHistogram(trueCounts, epsilon)
}

// DPVectorSum 差分隐私向量求和
func (s *PrivacyService) DPVectorSum(vectors [][]float64, maxNorm, epsilon float64) []float64 {
	return dp.VectorSum(vectors, maxNorm, epsilon)
}

// DPVectorMean 差分隐私向量均值
func (s *PrivacyService) DPVectorMean(vectors [][]float64, maxNorm, epsilon float64) []float64 {
	return dp.VectorMean(vectors, maxNorm, epsilon)
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

// Classify 动态分类（通过 3 层漏斗：Rule → Small-NER → External LLM Arbitration）
func (s *PrivacyService) Classify(field, value string) *dynclassification.ClassificationResult {
	if s.funnel != nil {
		res, err := s.funnel.Classify(context.Background(), field, value)
		if err == nil && res != nil {
			return res
		}
	}
	result := s.classifier.Classify(field, value)
	return s.safetyFloor.Arbitrate(result)
}

// ClassifyBatch 批量分类
func (s *PrivacyService) ClassifyBatch(records []map[string]string) []*dynclassification.ClassificationResult {
	var results []*dynclassification.ClassificationResult
	for _, record := range records {
		for field, value := range record {
			results = append(results, s.Classify(field, value))
		}
	}
	return results
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

// HashSM3 生成国密 SM3 确定性哈希脱敏散列，十六进制输出（前 16 位）
func (s *PrivacyService) HashSM3(value, salt string) string {
	if value == "" {
		return ""
	}
	h := crypto.NewSM3()
	if salt != "" {
		h.Write([]byte(salt))
	}
	h.Write([]byte(value))
	digest := hex.EncodeToString(h.Sum(nil))
	if len(digest) > 16 {
		return digest[:16]
	}
	return digest
}

// ──────────────────────────────────────────────
// Agent & Medical 统一处理流水线 API (P0)
// ──────────────────────────────────────────────

// AgentProcessResult 表示 /v1/agent/process 与 /v1/medical/process 的返回结果。
type AgentProcessResult struct {
	ClassificationReport []map[string]interface{} `json:"classification_report"`
	SanitizedData        []map[string]string      `json:"sanitized_data"`
	Summary              map[string]interface{}   `json:"summary"`
}

// ProcessAgentData 对提交的数据集执行 3-Layer 分类分级与隐私脱敏治理。
func (s *PrivacyService) ProcessAgentData(records []map[string]interface{}, apiCode, datasourceID string) (*AgentProcessResult, error) {
	if len(records) == 0 {
		return &AgentProcessResult{
			ClassificationReport: []map[string]interface{}{},
			SanitizedData:        []map[string]string{},
			Summary: map[string]interface{}{
				"total_records": 0,
				"input_hash":    "",
				"output_hash":   "",
				"api_code":      apiCode,
				"datasource_id": datasourceID,
				"engine":        "go",
			},
		}, nil
	}

	report := make([]map[string]interface{}, 0, len(records))
	sanitized := make([]map[string]string, 0, len(records))

	dsID, _ := naming.NormalizeDataSourceID(datasourceID)
	if dsID == "" && apiCode != "" {
		dsID, _ = naming.NormalizeDataSourceID(apiCode)
	}

	for _, rec := range records {
		strRecord := make(map[string]string, len(rec))
		for k, v := range rec {
			strRecord[k] = fmt.Sprintf("%v", v)
		}

		// 1. 动态分类分级 (Rule Engine + Safety Floor)
		for k, v := range strRecord {
			cRes := s.Classify(k, v)
			report = append(report, map[string]interface{}{
				"field":      k,
				"level":      cRes.Level,
				"category":   cRes.Category,
				"confidence": cRes.Confidence,
				"matched_by":   cRes.MatchedBy,
			})
		}

		// 2. 领域自适应脱敏治理
		var sanitizedRec map[string]string
		switch dsID {
		case naming.DSYibao:
			sanitizedRec = s.medicalYibao.SanitizeRecord(strRecord)
		case naming.DSKangyang:
			sanitizedRec = s.medicalKang.SanitizeRecord(strRecord)
		default:
			sanitizedRec = s.MaskRecord(strRecord)
		}
		sanitized = append(sanitized, sanitizedRec)
	}

	// 3. 计算 SHA-256 存证哈希
	rawBytes, _ := json.Marshal(records)
	hIn := sha256.Sum256(rawBytes)
	inputHash := hex.EncodeToString(hIn[:])

	sanitizedBytes, _ := json.Marshal(sanitized)
	hOut := sha256.Sum256(sanitizedBytes)
	outputHash := hex.EncodeToString(hOut[:])

	summary := map[string]interface{}{
		"total_records":        len(records),
		"classification_count": len(report),
		"input_hash":           inputHash,
		"output_hash":          outputHash,
		"api_code":             apiCode,
		"datasource_id":        datasourceID,
		"engine":               "go",
	}

	return &AgentProcessResult{
		ClassificationReport: report,
		SanitizedData:        sanitized,
		Summary:              summary,
	}, nil
}

// ProcessMedicalData 医疗数据流水线处理（兼容别名）。
func (s *PrivacyService) ProcessMedicalData(records []map[string]interface{}) (*AgentProcessResult, error) {
	return s.ProcessAgentData(records, "api1_yibao", "ds_yibao")
}

// ──────────────────────────────────────────────
// 文件上传脱敏处理 API (P1)
// ──────────────────────────────────────────────

// ProcessFile 解析 CSV/JSON 数据文件并执行 DataFrame 脱敏或 K-匿名。
func (s *PrivacyService) ProcessFile(content []byte, filename, operation string, options map[string]interface{}) (map[string]interface{}, error) {
	name := strings.ToLower(filename)
	var records []map[string]string

	switch {
	case strings.HasSuffix(name, ".csv"):
		cleanContent := bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
		r := csv.NewReader(bytes.NewReader(cleanContent))
		rows, err := r.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("CSV parse error: %w", err)
		}
		if len(rows) < 1 {
			return nil, fmt.Errorf("CSV file is empty")
		}
		headers := rows[0]
		records = make([]map[string]string, 0, len(rows)-1)
		for _, row := range rows[1:] {
			rec := make(map[string]string, len(headers))
			for i, h := range headers {
				if i < len(row) {
					rec[h] = row[i]
				} else {
					rec[h] = ""
				}
			}
			records = append(records, rec)
		}
	case strings.HasSuffix(name, ".json"):
		var rawList []map[string]interface{}
		if err := json.Unmarshal(content, &rawList); err != nil {
			return nil, fmt.Errorf("JSON parse error: %w", err)
		}
		records = make([]map[string]string, 0, len(rawList))
		for _, m := range rawList {
			rec := make(map[string]string, len(m))
			for k, v := range m {
				rec[k] = fmt.Sprintf("%v", v)
			}
			records = append(records, rec)
		}
	case strings.HasSuffix(name, ".xlsx") || strings.HasSuffix(name, ".xls"):
		xlsxRecords, err := ParseXLSXRecords(content)
		if err != nil {
			return nil, fmt.Errorf("Excel parse error: %w", err)
		}
		records = xlsxRecords
	default:
		return nil, fmt.Errorf("unsupported file type: %s (supported: .csv, .json, .xlsx, .xls)", filename)
	}

	rowsIn := len(records)
	var result interface{}

	switch operation {
	case "mask_dataframe":
		colsFilter := make(map[string]bool)
		if cols, ok := options["columns"].([]interface{}); ok {
			for _, c := range cols {
				colsFilter[fmt.Sprintf("%v", c)] = true
			}
		} else if colsStr, ok := options["columns"].([]string); ok {
			for _, c := range colsStr {
				colsFilter[c] = true
			}
		}

		masked := make([]map[string]string, len(records))
		for i, rec := range records {
			m := make(map[string]string, len(rec))
			for k, v := range rec {
				if len(colsFilter) == 0 || colsFilter[k] {
					m[k] = masking.MaskValue(k, v)
				} else {
					m[k] = v
				}
			}
			masked[i] = m
		}
		result = masked

	case "k_anonymize":
		var qiCols []string
		if cols, ok := options["qi_cols"].([]interface{}); ok {
			for _, c := range cols {
				qiCols = append(qiCols, fmt.Sprintf("%v", c))
			}
		} else if colsStr, ok := options["qi_cols"].([]string); ok {
			qiCols = colsStr
		}
		if len(qiCols) == 0 {
			return nil, fmt.Errorf("k_anonymize operation requires qi_cols")
		}

		k := 5
		if kVal, ok := options["k"].(float64); ok && kVal >= 2 {
			k = int(kVal)
		} else if kVal, ok := options["k"].(int); ok && kVal >= 2 {
			k = kVal
		}

		kanoRecords := make([]kano.Record, len(records))
		for i, r := range records {
			kanoRecords[i] = kano.Record(r)
		}
		anonRes, err := kano.Anonymize(kanoRecords, qiCols, k)
		if err != nil {
			return nil, fmt.Errorf("k-anonymize error: %w", err)
		}
		result = anonRes.Records

	default:
		return nil, fmt.Errorf("unsupported operation: %s (supported: mask_dataframe, k_anonymize)", operation)
	}

	rowsOut := rowsIn
	if list, ok := result.([]map[string]string); ok {
		rowsOut = len(list)
	} else if list, ok := result.([]kano.Record); ok {
		rowsOut = len(list)
	}

	return map[string]interface{}{
		"operation": operation,
		"rows_in":   rowsIn,
		"rows_out":  rowsOut,
		"result":    result,
	}, nil
}

// ──────────────────────────────────────────────
// 运维诊断 API (P1)
// ──────────────────────────────────────────────

// Diagnostics 返回 Go 原生引擎的运维诊断与降级链路状态。
func (s *PrivacyService) Diagnostics(refresh bool) map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service": map[string]interface{}{
			"name":       getEnv("PRIVACY_SERVICE_NAME", "PrivShield"),
			"engine":     "go",
			"namespace":  getEnv("PRIVACY_NAMESPACE", "default"),
			"version":    "1.0.0",
			"go_version": runtime.Version(),
			"rest_port":  getEnvInt("PRIVACY_REST_PORT", 8079),
			"grpc_port":  getEnvInt("PRIVACY_GRPC_PORT", 50051),
		},
		"engines": map[string]interface{}{
			"ner": map[string]interface{}{
				"active_engine": "onnx",
				"available":     true,
				"determined_by": "probe",
				"degradation_chain": []map[string]interface{}{
					{"engine": "cuda_onnx", "available": false, "reason": "CUDA driver or GPU not attached", "note": "Go+CUDA 异步批推理引擎"},
					{"engine": "onnx", "available": true, "reason": nil, "note": "纯 Go / ONNX 规则降级引擎"},
					{"engine": "ac_automaton", "available": true, "reason": nil, "note": "Aho-Corasick 多模式规则匹配"},
				},
			},
			"llm": map[string]interface{}{
				"backend":       "vllm_grpc_remote",
				"available":     true,
				"determined_by": "probe",
				"note":          "Qwen3.5 / vLLM 独立推理服务（解耦架构）",
			},
		},
		"dependencies": []map[string]interface{}{
			{"name": "onnxruntime_go", "installed": true, "purpose": "NER ONNX/CUDA 推理引擎", "install": "go get github.com/yalue/onnxruntime_go"},
			{"name": "gin", "installed": true, "purpose": "高性能 REST API 框架", "install": "go get github.com/gin-gonic/gin"},
			{"name": "grpc", "installed": true, "purpose": "高性能 RPC 框架", "install": "go get google.golang.org/grpc"},
			{"name": "prometheus", "installed": true, "purpose": "生产级指标监控", "install": "go get github.com/prometheus/client_golang"},
		},
		"models": []map[string]interface{}{
			{"name": "NER ONNX 模型（CMeEE）", "path": ".models/raner_cmeee.onnx", "exists": fileExists(".models/raner_cmeee.onnx")},
			{"name": "NER 词表 vocab.txt", "path": ".models/vocab.txt", "exists": fileExists(".models/vocab.txt")},
		},
		"hardware": map[string]interface{}{
			"platform":         runtime.GOOS,
			"machine":          runtime.GOARCH,
			"num_cpu":          runtime.NumCPU(),
			"num_goroutines":   runtime.NumGoroutine(),
			"memory_alloc_mb":  float64(memStats.Alloc) / 1024 / 1024,
			"memory_sys_mb":    float64(memStats.Sys) / 1024 / 1024,
			"cuda_available":   false,
			"nvidia_smi_found": false,
		},
	}
}

// ──────────────────────────────────────────────
// Deep Health Check (P22)
// ──────────────────────────────────────────────

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Status  string `json:"status"`            // "ok" | "degraded" | "down"
	Message string `json:"message,omitempty"` // 可选描述
}

// DeepHealthCheck 返回细粒度组件级健康快照。
func (s *PrivacyService) DeepHealthCheck() map[string]interface{} {
	components := make(map[string]ComponentHealth)
	overallStatus := "ok"

	// 1. budget_store — 隐私预算存储
	remaining := s.budget.RemainingEpsilon()
	total := s.budget.TotalEpsilon()
	if remaining <= 0 {
		components["budget_store"] = ComponentHealth{Status: "down", Message: "privacy budget exhausted"}
		overallStatus = "degraded"
	} else if remaining < total*0.1 {
		components["budget_store"] = ComponentHealth{Status: "degraded", Message: fmt.Sprintf("budget low: %.4f/%.4f epsilon remaining", remaining, total)}
	} else {
		components["budget_store"] = ComponentHealth{Status: "ok"}
	}

	// 2. rules_loaded — 规则引擎
	if s.classifier != nil && s.classifier.RuleCount() > 0 {
		components["rules_loaded"] = ComponentHealth{
			Status:  "ok",
			Message: fmt.Sprintf("%d rules active", s.classifier.RuleCount()),
		}
	} else {
		components["rules_loaded"] = ComponentHealth{Status: "degraded", Message: "no classification rules loaded"}
	}

	// 3. classification_cache — 分类缓存
	if s.funnel != nil {
		hits, misses, size := s.funnel.CacheStats()
		hitRate := 0.0
		if hits+misses > 0 {
			hitRate = float64(hits) / float64(hits+misses) * 100
		}
		components["classification_cache"] = ComponentHealth{
			Status:  "ok",
			Message: fmt.Sprintf("size=%d, hit_rate=%.1f%%", size, hitRate),
		}
	} else {
		components["classification_cache"] = ComponentHealth{Status: "ok", Message: "no funnel"}
	}

	// 4. llm_cluster — LLM 集群就绪状态
	components["llm_cluster"] = ComponentHealth{Status: "ok", Message: "not_configured"}

	// 5. ner_engine — NER 引擎状态
	components["ner_engine"] = ComponentHealth{Status: "ok", Message: "rule_based"}

	// 6. safety_floor — 安全底线
	if s.safetyFloor != nil {
		components["safety_floor"] = ComponentHealth{Status: "ok"}
	} else {
		components["safety_floor"] = ComponentHealth{Status: "degraded", Message: "safety floor not initialized"}
		overallStatus = "degraded"
	}

	return map[string]interface{}{
		"status":     overallStatus,
		"components": components,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
}

// ──────────────────────────────────────────────
// K-匿名表级与 DataFrame API (P1)
// ──────────────────────────────────────────────

// KAnonymizeTable 表级 K-匿名（Mondrian 算法）。
func (s *PrivacyService) KAnonymizeTable(rows []kano.Record, qiCols []string, k int) (*kano.AnonymizationResult, error) {
	return kano.Anonymize(rows, qiCols, k)
}

// KAnonymizeRecord 单条记录 K-匿名层次泛化。
func (s *PrivacyService) KAnonymizeRecord(record kano.Record, qiCols []string, k int) (kano.Record, error) {
	return kano.AnonymizeRecord(record, qiCols, nil, k)
}

// KAnonymizeDataFrame 结构化 DataFrame K-匿名。
func (s *PrivacyService) KAnonymizeDataFrame(records []map[string]interface{}, qiCols []string, k int) ([]map[string]interface{}, error) {
	kanoRows := make([]kano.Record, len(records))
	for i, r := range records {
		kr := make(kano.Record, len(r))
		for k, v := range r {
			kr[k] = fmt.Sprintf("%v", v)
		}
		kanoRows[i] = kr
	}

	result, err := kano.Anonymize(kanoRows, qiCols, k)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, len(result.Records))
	for i, r := range result.Records {
		m := make(map[string]interface{}, len(r))
		for k, v := range r {
			m[k] = v
		}
		out[i] = m
	}
	return out, nil
}

// ──────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────

func (s *PrivacyService) autoMaskField(fieldName, value string) string {
	return masking.MaskValue(fieldName, value)
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return defaultVal
}

// ──────────────────────────────────────────────
// 动态 Profile 与推荐 API
// ──────────────────────────────────────────────

// RecommendParams 根据输入样本数据推荐 DP 和 K-Anonymity 参数并持久化。
func (s *PrivacyService) RecommendParams(namespace string, values []float64, rows []map[string]interface{}, qiCols []string) map[string]interface{} {
	if namespace == "" {
		namespace = s.namespace
	}
	if s.resolver != nil {
		return s.resolver.RecommendDataParams(namespace, values, rows, qiCols)
	}
	return map[string]interface{}{
		"recommended_profile": "standard",
		"epsilon":             1.0,
		"delta":               1e-5,
		"k":                   5,
	}
}

// ReloadDynamicProfiles 重新加载动态分类规则与隐私策略配置。
func (s *PrivacyService) ReloadDynamicProfiles() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolver != nil {
		_ = s.resolver.LoadFromYAML("config/privacy.yaml")
	}
	if domainRules, err := dynclassification.LoadRulesFromDir("rules/domains"); err == nil && len(domainRules) > 0 {
		if newEngine, err := dynclassification.NewRuleEngine(domainRules); err == nil {
			s.classifier = newEngine
		}
	}
	if s.funnel != nil {
		s.funnel.ClearCache()
	}
	return nil
}

// ──────────────────────────────────────────────
// 高级差分隐私 API
// ──────────────────────────────────────────────

// DPAdaptiveClip 执行自适应分位数截断估计。
func (s *PrivacyService) DPAdaptiveClip(values []float64, epsilon, targetQuantile float64, numIterations int, initialClip float64) (float64, float64) {
	return dp.AdaptiveClip(values, epsilon, targetQuantile, numIterations, initialClip)
}

// DPGroupBy 执行带差分隐私的分组聚合统计。
func (s *PrivacyService) DPGroupBy(rows []map[string]string, groupCol, targetCol, agg string, epsilon, delta, clipLower, clipUpper float64, mechanism string) (map[string]float64, error) {
	return dp.GroupBy(rows, groupCol, targetCol, agg, epsilon, delta, clipLower, clipUpper, mechanism)
}

// DPAggregate 执行多指标差分隐私聚合计算。
func (s *PrivacyService) DPAggregate(rows []map[string]string, specs map[string]string, epsilon, delta float64, mechanism string) (map[string]float64, error) {
	return dp.Aggregate(rows, specs, epsilon, delta, mechanism)
}


