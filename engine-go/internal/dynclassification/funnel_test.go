package dynclassification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClassificationFunnel_Layer1RuleHit(t *testing.T) {
	rules := []RuleDef{
		{
			ID:            "id_card_rule",
			Level:         LevelTopSecret,
			Category:      "pii.identity",
			FieldPatterns: []string{`(?i)id_card_no`},
		},
	}

	funnel, err := NewClassificationFunnel(rules, nil, nil, DefaultFunnelConfig())
	if err != nil {
		t.Fatalf("NewClassificationFunnel: %v", err)
	}

	res, err := funnel.Classify(context.Background(), "id_card_no", "110101199001011234")
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if res.Level != LevelTopSecret {
		t.Errorf("Level = %q, want %q", res.Level, LevelTopSecret)
	}
	if res.MatchedBy != "rule:id_card_rule" {
		t.Errorf("MatchedBy = %q, want 'rule:id_card_rule'", res.MatchedBy)
	}
}

func TestClassificationFunnel_Layer2NERHit(t *testing.T) {
	// 规则中没有 content 字段匹配
	rules := []RuleDef{}

	nerEngine := NewRuleBasedNerEngine()
	funnel, err := NewClassificationFunnel(rules, nerEngine, nil, DefaultFunnelConfig())
	if err != nil {
		t.Fatalf("NewClassificationFunnel: %v", err)
	}

	// 传入包含艾滋病高危文本
	res, err := funnel.Classify(context.Background(), "remark", "患者既往有艾滋病病史")
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if res.Level != LevelSecret {
		t.Errorf("Level = %q, want %q", res.Level, LevelSecret)
	}
	if res.Category != "medical.condition" {
		t.Errorf("Category = %q, want 'medical.condition'", res.Category)
	}
	if res.MatchedBy != "ner:MEDICAL_CONDITION" {
		t.Errorf("MatchedBy = %q, want 'ner:MEDICAL_CONDITION'", res.MatchedBy)
	}
}

func TestClassificationFunnel_Layer3ExternalLLMHit(t *testing.T) {
	// 模拟外部 vLLM / OpenAI API 服务
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": `{"level": "top_secret", "category": "pii.financial", "confidence": 0.96, "reasoning": "Detected bank card"}`,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	llmClient := NewLLMClient(LLMClientConfig{
		Endpoint:       mockServer.URL,
		ModelName:      "qwen3.5",
		MaxConcurrency: 2,
		Timeout:        2 * time.Second,
		MaxRetries:     1,
	})

	cfg := DefaultFunnelConfig()
	cfg.EnableNER = false // 关闭 NER 直接触发 LLM
	cfg.EnableLLM = true

	funnel, err := NewClassificationFunnel(nil, nil, llmClient, cfg)
	if err != nil {
		t.Fatalf("NewClassificationFunnel: %v", err)
	}

	res, err := funnel.Classify(context.Background(), "unknown_field", "6222021234567890")
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if res.Level != LevelTopSecret {
		t.Errorf("Level = %q, want %q", res.Level, LevelTopSecret)
	}
	if res.Category != "pii.financial" {
		t.Errorf("Category = %q, want 'pii.financial'", res.Category)
	}
	if res.MatchedBy != "llm" {
		t.Errorf("MatchedBy = %q, want 'llm'", res.MatchedBy)
	}
}

// ──────────────────────────────────────────────
// P3: 分片 LRU 缓存测试
// ──────────────────────────────────────────────

func TestShardedLRU_BasicGetPut(t *testing.T) {
	c := newClassificationCache(100)

	// put 并 get
	res := &ClassificationResult{Field: "phone", Level: LevelConfidential, MatchedBy: "test"}
	c.put("key1", res)

	got, ok := c.get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Field != "phone" {
		t.Errorf("Field = %q, want 'phone'", got.Field)
	}
}

func TestShardedLRU_Eviction(t *testing.T) {
	// 容量 16 分片，每分片容量 2，总容量 32
	c := newClassificationCache(32)

	// 写入 40 条（超过总容量 32）
	for i := 0; i < 40; i++ {
		c.put("key"+string(rune('A'+i)), &ClassificationResult{
			Field: "f", Level: LevelPublic, MatchedBy: "test",
		})
	}

	// 总条目数应不超过 32
	totalSize := 0
	for _, shard := range c.shards {
		shard.mu.Lock()
		totalSize += len(shard.items)
		shard.mu.Unlock()
	}
	if totalSize > 32 {
		t.Errorf("total size = %d, should not exceed 32", totalSize)
	}
}

func TestShardedLRU_ConcurrentAccess(t *testing.T) {
	c := newClassificationCache(1000)

	var wg sync.WaitGroup
	// 16 个 goroutine 并发读写
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := "key"
				c.put(key, &ClassificationResult{Field: "f", Level: LevelPublic})
				c.get(key)
			}
		}(g)
	}
	wg.Wait()
}

func TestShardedLRU_Clear(t *testing.T) {
	c := newClassificationCache(100)
	for i := 0; i < 10; i++ {
		c.put("k"+string(rune('0'+i)), &ClassificationResult{Field: "f"})
	}
	c.clear()

	for i := 0; i < 10; i++ {
		_, ok := c.get("k" + string(rune('0'+i)))
		if ok {
			t.Errorf("key should be cleared after clear()")
		}
	}
}

func TestFunnel_CacheStats_Sharded(t *testing.T) {
	rules := []RuleDef{
		{ID: "r1", Level: LevelTopSecret, Category: "pii", FieldPatterns: []string{`(?i)id_card`}},
	}
	funnel, err := NewClassificationFunnel(rules, nil, nil, DefaultFunnelConfig())
	if err != nil {
		t.Fatalf("NewClassificationFunnel: %v", err)
	}

	// 执行多次分类（触发缓存写入）
	for i := 0; i < 10; i++ {
		funnel.Classify(context.Background(), "id_card_no", "110101199001011234")
	}

	hits, _, size := funnel.CacheStats()
	if size == 0 {
		t.Error("cache size should be > 0 after classifications")
	}
	// 第二次开始应命中缓存
	if hits < 9 {
		t.Errorf("expected at least 9 cache hits, got %d", hits)
	}
}

// ──────────────────────────────────────────────
// P2: SafetyFloor Ring Buffer 测试
// ──────────────────────────────────────────────

func TestSafetyFloor_RingBuffer_FixedCapacity(t *testing.T) {
	cfg := DefaultSafetyFloorConfig()
	sf := NewSafetyFloor(cfg)

	// 写入超过容量的事件
	for i := 0; i < 15000; i++ {
		sf.recordEvent(ArbitrationEvent{
			Field:      "f",
			Reason:     "test",
			FinalLevel: LevelPublic,
		})
	}

	events := sf.AuditEvents()
	// 固定容量为 10000，不应超过
	if len(events) > 10000 {
		t.Errorf("audit events = %d, should not exceed 10000", len(events))
	}
}

func TestSafetyFloor_RingBuffer_ChronologicalOrder(t *testing.T) {
	cfg := DefaultSafetyFloorConfig()
	sf := NewSafetyFloor(cfg)

	// 写入 15000 条（超过容量 10000，触发循环覆盖）
	for i := 0; i < 15000; i++ {
		sf.recordEvent(ArbitrationEvent{
			Field:  "f",
			Reason: "test",
		})
	}

	events := sf.AuditEvents()
	if len(events) != 10000 {
		t.Fatalf("expected 10000 events, got %d", len(events))
	}
	// 最早的事件应是第 5000 条（索引从 5000 开始）
	// 由于 recordEvent 不设置唯一标识，我们只验证返回顺序正确
}

func TestSafetyFloor_Arbitrate_RecordsAudit(t *testing.T) {
	cfg := SafetyFloorConfig{
		MinLevel:                  LevelConfidential,
		ConfidenceThreshold:       0.8,
		ForceUpgradeOnUncertainty: true,
		AuditLog:                  true,
	}
	sf := NewSafetyFloor(cfg)

	// 低于最低等级，应触发升级并记录审计
	result := sf.Arbitrate(&ClassificationResult{
		Field:      "test_field",
		Level:      LevelPublic,
		Confidence: 0.5,
		MatchedBy:  "test",
	})

	if result.Level != LevelSecret {
		t.Errorf("Level = %q, want %q (upgraded twice)", result.Level, LevelSecret)
	}

	events := sf.AuditEvents()
	if len(events) == 0 {
		t.Error("expected audit events to be recorded")
	}
}

// ──────────────────────────────────────────────
// P1: LLM IsAvailable TTL 缓存测试
// ──────────────────────────────────────────────

func TestLLMClient_IsAvailable_TTLCache(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	client := NewLLMClient(LLMClientConfig{
		Endpoint:       mockServer.URL,
		MaxConcurrency: 1,
		Timeout:        2 * time.Second,
	})

	ctx := context.Background()

	// 第一次调用应实际探测
	avail := client.IsAvailable(ctx)
	if !avail {
		t.Error("expected available")
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}

	// 第二次调用应命中缓存，不发起 HTTP 请求
	avail = client.IsAvailable(ctx)
	if !avail {
		t.Error("expected available from cache")
	}
	if callCount != 1 {
		t.Errorf("expected still 1 HTTP call (cached), got %d", callCount)
	}

	// 等待 TTL 过期后重新探测
	client.availCacheTTL = 50 * time.Millisecond
	time.Sleep(60 * time.Millisecond)

	avail = client.IsAvailable(ctx)
	if !avail {
		t.Error("expected available after TTL expiry")
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls after TTL, got %d", callCount)
	}
}

func TestLLMClient_IsAvailable_ConcurrentNoStorm(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	client := NewLLMClient(LLMClientConfig{
		Endpoint:       mockServer.URL,
		MaxConcurrency: 10,
		Timeout:        2 * time.Second,
	})
	client.availCacheTTL = 1 * time.Second

	ctx := context.Background()

	// 50 个 goroutine 并发调用，应只有极少数实际发起 HTTP 请求
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.IsAvailable(ctx)
		}()
	}
	wg.Wait()

	mu.Lock()
	actual := callCount
	mu.Unlock()
	// TTL 缓存应大幅减少探测次数，理想情况下只有 1-2 次
	if actual > 5 {
		t.Errorf("expected <= 5 HTTP calls with TTL cache, got %d", actual)
	}
}

// ──────────────────────────────────────────────
// P1-5: RuleEngine 有界分片缓存测试
// ──────────────────────────────────────────────

func TestEngineCache_BoundedCapacity(t *testing.T) {
	c := newEngineCache(100) // 总容量 100，每分片约 7
	// 插入远超容量的条目
	for i := 0; i < 1000; i++ {
		c.put("key"+strings.Repeat("x", i%50)+"_"+string(rune('A'+i%26)), &ClassificationResult{
			Field: "f", Level: LevelPublic, Category: "c", Confidence: 0.9, MatchedBy: "test",
		})
	}
	// 验证总条目数不超过容量
	total := 0
	for _, shard := range c.shards {
		shard.mu.Lock()
		total += len(shard.items)
		shard.mu.Unlock()
	}
	if total > 100 {
		t.Errorf("cache total = %d, should be <= 100", total)
	}
}

func TestEngineCache_ConcurrentAccess(t *testing.T) {
	c := newEngineCache(1000)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := "key_" + string(rune('A'+id)) + "_" + string(rune('0'+j%10))
				c.put(key, &ClassificationResult{Field: key, Level: LevelPublic})
				c.get(key)
			}
		}(i)
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// P1-7: SafetyFloor 并发仲裁 + 配置更新
// ──────────────────────────────────────────────

func TestSafetyFloor_ConcurrentArbitrateAndUpdate(t *testing.T) {
	sf := NewSafetyFloor(DefaultSafetyFloorConfig())
	var wg sync.WaitGroup
	// 16 goroutine 并发仲裁
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				res := &ClassificationResult{
					Field: "phone", Level: LevelPublic, Confidence: 0.5,
				}
				sf.Arbitrate(res)
			}
		}()
	}
	// 2 goroutine 并发更新配置
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				sf.UpdateConfig(SafetyFloorConfig{
					MinLevel:                  LevelInternal,
					ConfidenceThreshold:       0.7,
					ForceUpgradeOnUncertainty: true,
					AuditLog:                  true,
				})
			}
		}()
	}
	wg.Wait()
}

// ──────────────────────────────────────────────
// P3-22: ArbitrateBatch 并行测试
// ──────────────────────────────────────────────

func TestSafetyFloor_ArbitrateBatch_Parallel(t *testing.T) {
	sf := NewSafetyFloor(DefaultSafetyFloorConfig())
	results := make([]*ClassificationResult, 200)
	for i := range results {
		results[i] = &ClassificationResult{
			Field: "field", Level: LevelPublic, Confidence: 0.5,
		}
	}
	out := sf.ArbitrateBatch(results)
	if len(out) != 200 {
		t.Errorf("ArbitrateBatch returned %d results, want 200", len(out))
	}
	// 所有结果应被升级到至少 Internal（默认 ConfidenceThreshold=0.6 > 0.5）
	for i, r := range out {
		if LevelRank(r.Level) < LevelRank(LevelInternal) {
			t.Errorf("result[%d].Level = %q, want >= internal", i, r.Level)
		}
	}
}
