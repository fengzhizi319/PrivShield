package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/fengzhizi319/PrivShield/pkg/naming"
)

func newTestService(t *testing.T) *PrivacyService {
	t.Helper()
	svc, err := NewPrivacyService(DefaultConfig())
	if err != nil {
		t.Fatalf("NewPrivacyService: %v", err)
	}
	return svc
}

// ──────────────────────────────────────────────
// SSOT 数据源归一化 — SanitizeMedicalRecord
// ──────────────────────────────────────────────

func TestSanitizeMedicalRecord_CanonicalDSID(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "张三", "id_card_no": "110101199003072345"}

	// canonical datasource_id
	result, err := svc.SanitizeMedicalRecord(record, naming.DSYibao)
	if err != nil {
		t.Fatalf("unexpected error for %s: %v", naming.DSYibao, err)
	}
	if result["name"] == "张三" {
		t.Error("name should be masked")
	}
	if result["id_card_no"] == "110101199003072345" {
		t.Error("id_card_no should be masked")
	}
}

func TestSanitizeMedicalRecord_APICode(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "李四"}

	// api_code alias → should resolve to DSYibao
	result, err := svc.SanitizeMedicalRecord(record, naming.API1Yibao)
	if err != nil {
		t.Fatalf("unexpected error for %s: %v", naming.API1Yibao, err)
	}
	if result["name"] == "李四" {
		t.Error("name should be masked via api_code resolution")
	}
}

func TestSanitizeMedicalRecord_SlugAlias(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "王五"}

	// slug alias "yibao" → should resolve to DSYibao
	result, err := svc.SanitizeMedicalRecord(record, "yibao")
	if err != nil {
		t.Fatalf("unexpected error for slug 'yibao': %v", err)
	}
	if result["name"] == "王五" {
		t.Error("name should be masked via slug resolution")
	}
}

func TestSanitizeMedicalRecord_ChineseAlias(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "赵六"}

	// Chinese alias "医保" → should resolve to DSYibao
	result, err := svc.SanitizeMedicalRecord(record, "医保")
	if err != nil {
		t.Fatalf("unexpected error for alias '医保': %v", err)
	}
	if result["name"] == "赵六" {
		t.Error("name should be masked via Chinese alias resolution")
	}
}

func TestSanitizeMedicalRecord_Kangyang_AllForms(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "孙七", "phone": "13800138000"}

	aliases := []string{naming.DSKangyang, naming.API2Kangyang, "kangyang", "康养"}
	for _, alias := range aliases {
		result, err := svc.SanitizeMedicalRecord(record, alias)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", alias, err)
			continue
		}
		if result["name"] == "孙七" {
			t.Errorf("name should be masked for alias %q", alias)
		}
	}
}

func TestSanitizeMedicalRecord_UnknownDomain_FailClosed(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "张三"}

	_, err := svc.SanitizeMedicalRecord(record, "unknown_source")
	if err == nil {
		t.Fatal("expected error for unknown domain, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_DATASOURCE_ID") {
		t.Errorf("error should contain INVALID_DATASOURCE_ID, got: %v", err)
	}
}

func TestSanitizeMedicalRecord_ReservedDomain_FailClosed(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "张三"}

	// DSMock3 is registered but reserved → should fail
	_, err := svc.SanitizeMedicalRecord(record, naming.DSMock3)
	if err == nil {
		t.Fatal("expected error for reserved domain, got nil")
	}
}

func TestSanitizeMedicalRecord_EmptyDomain_FailClosed(t *testing.T) {
	svc := newTestService(t)
	record := map[string]string{"name": "张三"}

	_, err := svc.SanitizeMedicalRecord(record, "")
	if err == nil {
		t.Fatal("expected error for empty domain, got nil")
	}
}

// ──────────────────────────────────────────────
// SSOT 数据源归一化 — SanitizeMedicalBatch
// ──────────────────────────────────────────────

func TestSanitizeMedicalBatch_CanonicalDSID(t *testing.T) {
	svc := newTestService(t)
	records := []map[string]string{
		{"name": "张三", "phone": "13800138000"},
		{"name": "李四", "phone": "13900139000"},
	}

	results, err := svc.SanitizeMedicalBatch(records, naming.DSYibao)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0]["name"] == "张三" {
		t.Error("first record name should be masked")
	}
}

func TestSanitizeMedicalBatch_UnknownDomain_FailClosed(t *testing.T) {
	svc := newTestService(t)
	records := []map[string]string{{"name": "张三"}}

	_, err := svc.SanitizeMedicalBatch(records, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown domain, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_DATASOURCE_ID") {
		t.Errorf("error should contain INVALID_DATASOURCE_ID, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// P0: ClassifyBatch 并行化测试
// ──────────────────────────────────────────────

func TestClassifyBatch_Parallel_Correctness(t *testing.T) {
	svc := newTestService(t)

	// 100 条记录，每条 3 个字段 → 展平后 300 个 (field, value) 对
	records := make([]map[string]string, 100)
	for i := 0; i < 100; i++ {
		records[i] = map[string]string{
			"id_card_no": "110101199003072345",
			"phone":      "13812345678",
			"name":       "张三",
		}
	}

	results := svc.ClassifyBatch(records)
	// ClassifyBatch 展平所有字段，100 × 3 = 300
	if len(results) != 300 {
		t.Fatalf("expected 300 results (100 records × 3 fields), got %d", len(results))
	}
	for i, r := range results {
		if r == nil {
			t.Errorf("result[%d] is nil", i)
		}
	}
}

func TestClassifyBatch_Parallel_ConcurrentSafety(t *testing.T) {
	svc := newTestService(t)

	// 并发执行多次 ClassifyBatch，验证无 data race
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			records := make([]map[string]string, 20)
			for i := range records {
				records[i] = map[string]string{"phone": "13800138000"}
			}
			results := svc.ClassifyBatch(records)
			if len(results) != 20 {
				t.Errorf("expected 20 results, got %d", len(results))
			}
		}()
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// P0: DP 预算检查测试
// ──────────────────────────────────────────────

func TestNoisyCount_BudgetExhaustion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TotalEpsilon = 1.0 // 极低预算
	svc, err := NewPrivacyService(cfg)
	if err != nil {
		t.Fatalf("NewPrivacyService: %v", err)
	}
	ctx := context.Background()

	// 第一次消耗 0.6
	_, err = svc.NoisyCount(ctx, 100, 0.6)
	if err != nil {
		t.Fatalf("first NoisyCount should succeed: %v", err)
	}

	// 第二次消耗 0.6，累计 1.2 > 1.0，应被拒绝
	_, err = svc.NoisyCount(ctx, 100, 0.6)
	if err == nil {
		t.Fatal("expected budget exhaustion error, got nil")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("error should mention budget, got: %v", err)
	}
}

func TestDPVectorSum_BudgetExhaustion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TotalEpsilon = 0.5
	svc, err := NewPrivacyService(cfg)
	if err != nil {
		t.Fatalf("NewPrivacyService: %v", err)
	}
	ctx := context.Background()

	vectors := [][]float64{{1.0, 2.0}, {3.0, 4.0}}
	_, err = svc.DPVectorSum(ctx, vectors, 1.0, 0.6)
	if err == nil {
		t.Fatal("expected budget exhaustion, got nil")
	}
}

// ──────────────────────────────────────────────
// P0: ObfuscateQueryBatch 并行化测试
// ──────────────────────────────────────────────

func TestObfuscateQueryBatch_Parallel_LargeBatch(t *testing.T) {
	svc := newTestService(t)

	// 200 条查询（超过 32 阈值，触发并行路径）
	queries := make([]string, 200)
	for i := range queries {
		queries[i] = "肺癌早期症状"
	}

	results := svc.ObfuscateQueryBatch(queries, 3, "medical")
	if len(results) != 200 {
		t.Fatalf("expected 200 results, got %d", len(results))
	}
	for i, r := range results {
		// 原始查询 + 3 个混淆 = 4 条
		if len(r) != 4 {
			t.Errorf("result[%d] has %d queries, want 4", i, len(r))
		}
	}
}

func TestObfuscateQueryBatch_Parallel_ConcurrentSafety(t *testing.T) {
	svc := newTestService(t)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queries := make([]string, 50)
			for i := range queries {
				queries[i] = "糖尿病治疗"
			}
			results := svc.ObfuscateQueryBatch(queries, 2, "medical")
			if len(results) != 50 {
				t.Errorf("expected 50 results, got %d", len(results))
			}
		}()
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// P2: Config 去硬编码路径测试
// ──────────────────────────────────────────────

func TestDefaultConfig_HasConfigurablePaths(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RulesDir == "" {
		t.Error("RulesDir should not be empty")
	}
	if cfg.PrivacyYAML == "" {
		t.Error("PrivacyYAML should not be empty")
	}
	if cfg.RulesDir != "rules/domains" {
		t.Errorf("RulesDir = %q, want %q", cfg.RulesDir, "rules/domains")
	}
	if cfg.PrivacyYAML != "config/privacy.yaml" {
		t.Errorf("PrivacyYAML = %q, want %q", cfg.PrivacyYAML, "config/privacy.yaml")
	}
}

// ──────────────────────────────────────────────
// P0: 热重载并发安全（atomic.Pointer + RLock）
// ──────────────────────────────────────────────

func TestClassify_ConcurrentWithReload(t *testing.T) {
	svc := newTestService(t)
	var wg sync.WaitGroup
	// 32 个 goroutine 并发分类
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				res := svc.Classify("phone", "13812345678")
				if res == nil {
					t.Error("Classify returned nil")
					return
				}
			}
		}()
	}
	// 2 个 goroutine 并发重载
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = svc.ReloadDynamicProfiles()
			}
		}()
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// P0: atomic.Pointer 初始化验证
// ──────────────────────────────────────────────

func TestNewPrivacyService_ClassifierInitialized(t *testing.T) {
	svc := newTestService(t)
	// classifier 应该在构造后立即可用
	res := svc.Classify("phone", "13812345678")
	if res == nil {
		t.Fatal("Classify returned nil after initialization")
	}
	if res.Level == "" {
		t.Fatal("Classify returned empty level")
	}
}
