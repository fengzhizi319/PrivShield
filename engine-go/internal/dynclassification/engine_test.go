package dynclassification

import (
	"testing"
)

func TestNewRuleEngine(t *testing.T) {
	rules := []RuleDef{
		{
			ID:            "test_rule",
			Level:         LevelConfidential,
			Category:      "test",
			FieldPatterns: []string{`(?i)test_field`},
		},
	}

	engine, err := NewRuleEngine(rules)
	if err != nil {
		t.Fatalf("NewRuleEngine failed: %v", err)
	}

	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestClassifyByFieldPattern(t *testing.T) {
	rules := []RuleDef{
		{
			ID:            "id_card",
			Level:         LevelSecret,
			Category:      "pii.identity",
			FieldPatterns: []string{`(?i)(id_?card|身份证)`},
		},
		{
			ID:            "phone",
			Level:         LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)(phone|mobile|手机)`},
		},
	}

	engine, err := NewRuleEngine(rules)
	if err != nil {
		t.Fatalf("NewRuleEngine failed: %v", err)
	}

	tests := []struct {
		field          string
		expectedLevel  SecurityLevel
		expectedCategory string
	}{
		{"id_card", LevelSecret, "pii.identity"},
		{"ID_CARD", LevelSecret, "pii.identity"},
		{"身份证号码", LevelSecret, "pii.identity"},
		{"phone", LevelConfidential, "pii.contact"},
		{"手机号", LevelConfidential, "pii.contact"},
		{"unknown_field", LevelPublic, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result := engine.Classify(tt.field, "some_value")
			if result.Level != tt.expectedLevel {
				t.Errorf("Level = %v, want %v", result.Level, tt.expectedLevel)
			}
			if result.Category != tt.expectedCategory {
				t.Errorf("Category = %v, want %v", result.Category, tt.expectedCategory)
			}
		})
	}
}

func TestClassifyBatch(t *testing.T) {
	rules := []RuleDef{
		{
			ID:            "email",
			Level:         LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)email`},
		},
	}

	engine, err := NewRuleEngine(rules)
	if err != nil {
		t.Fatalf("NewRuleEngine failed: %v", err)
	}

	records := []map[string]string{
		{"email": "test@example.com", "name": "John"},
		{"phone": "1234567890"},
	}

	results := engine.ClassifyBatch(records)
	if len(results) == 0 {
		t.Error("ClassifyBatch should return results")
	}

	// 验证至少有一个 email 字段被分类
	foundEmail := false
	for _, r := range results {
		if r.Field == "email" && r.Level == LevelConfidential {
			foundEmail = true
			break
		}
	}
	if !foundEmail {
		t.Error("Should classify email field")
	}
}

func TestACAutomaton(t *testing.T) {
	ac := NewACAutomaton()

	// 添加模式
	if err := ac.AddPattern("pattern1", "test"); err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	ac.Build()

	// 搜索
	matches := ac.Search("this is a test string")
	if len(matches) == 0 {
		t.Error("AC automaton should find matches")
	}
}

func TestSecurityLevels(t *testing.T) {
	levels := []SecurityLevel{
		LevelPublic,
		LevelInternal,
		LevelConfidential,
		LevelSecret,
		LevelTopSecret,
	}

	for _, level := range levels {
		if level == "" {
			t.Error("Security level should not be empty")
		}
	}
}
