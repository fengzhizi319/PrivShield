package dynclassification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
