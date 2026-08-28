package medical

import (
	"testing"
)

func TestNewYibaoPipeline(t *testing.T) {
	p := NewYibaoPipeline()
	if p.FieldCount() != 18 {
		t.Errorf("YibaoPipeline field count = %d, want 18", p.FieldCount())
	}
}

func TestNewKangyangPipeline(t *testing.T) {
	p := NewKangyangPipeline()
	if p.FieldCount() != 27 {
		t.Errorf("KangyangPipeline field count = %d, want 27", p.FieldCount())
	}
}

func TestSanitizeIdCard(t *testing.T) {
	p := NewYibaoPipeline()
	result := p.SanitizeField("id_card_no", "110101199001011234")
	expected := "110101********1234"
	if result != expected {
		t.Errorf("SanitizeField(id_card_no) = %q, want %q", result, expected)
	}
}

func TestSanitizePhone(t *testing.T) {
	p := NewYibaoPipeline()
	result := p.SanitizeField("phone", "13812345678")
	expected := "138****5678"
	if result != expected {
		t.Errorf("SanitizeField(phone) = %q, want %q", result, expected)
	}
}

func TestSanitizeName(t *testing.T) {
	p := NewYibaoPipeline()
	result := p.SanitizeField("name", "张三丰")
	expected := "张**丰" // 与 Python mask_name 对齐：3字→首+**+尾
	if result != expected {
		t.Errorf("SanitizeField(name) = %q, want %q", result, expected)
	}
}

func TestSanitizeAddress(t *testing.T) {
	p := NewYibaoPipeline()
	result := p.SanitizeField("address", "北京市朝阳区建国路88号")
	if len(result) == 0 {
		t.Error("SanitizeField(address) should not be empty")
	}
	// 前 6 个 rune 应保留（MaskAddress 保留前 6 个字符）
	runes := []rune(result)
	if string(runes[:6]) != "北京市朝阳区" {
		t.Errorf("SanitizeField(address) prefix = %q, want '北京市朝阳区'", string(runes[:6]))
	}
}

func TestSanitizeRecord(t *testing.T) {
	p := NewYibaoPipeline()
	record := map[string]string{
		"name":       "张三",
		"id_card_no": "110101199001011234",
		"phone":      "13812345678",
		"gender":     "男",
		"age":        "35",
	}

	result := p.SanitizeRecord(record)

	if result["name"] != "张*" {
		t.Errorf("name = %q, want '张*'", result["name"])
	}
	if result["id_card_no"] != "110101********1234" {
		t.Errorf("id_card_no = %q, want '110101********1234'", result["id_card_no"])
	}
	if result["phone"] != "138****5678" {
		t.Errorf("phone = %q, want '138****5678'", result["phone"])
	}
	if result["gender"] != "男" {
		t.Errorf("gender should be preserved, got %q", result["gender"])
	}
	if result["age"] != "35" {
		t.Errorf("age should be preserved, got %q", result["age"])
	}
}

func TestSanitizeBatch(t *testing.T) {
	p := NewYibaoPipeline()
	records := []map[string]string{
		{"name": "张三", "phone": "13812345678"},
		{"name": "李四", "phone": "13987654321"},
	}

	results := p.SanitizeBatch(records)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSanitizeEmpty(t *testing.T) {
	p := NewYibaoPipeline()
	result := p.SanitizeField("name", "")
	if result != "" {
		t.Errorf("SanitizeField(name, '') = %q, want ''", result)
	}
}

func TestSanitizeUnknownField(t *testing.T) {
	p := NewYibaoPipeline()
	// 未知字段名但含 phone 关键词
	result := p.SanitizeField("contact_phone", "13812345678")
	expected := "138****5678"
	if result != expected {
		t.Errorf("SanitizeField(contact_phone) = %q, want %q", result, expected)
	}
}

func TestMaskClinicalText(t *testing.T) {
	text := "患者因持续性头痛伴恶心三天入院"
	result := maskClinicalText(text)
	runes := []rune(result)
	// 首 2 后 2 应保留
	if string(runes[:2]) != "患者" {
		t.Errorf("prefix = %q, want '患者'", string(runes[:2]))
	}
	if string(runes[len(runes)-2:]) != "入院" {
		t.Errorf("suffix = %q, want '入院'", string(runes[len(runes)-2:]))
	}
}

func TestGetFieldSpec(t *testing.T) {
	p := NewYibaoPipeline()
	spec := p.GetFieldSpec("id_card_no")
	if spec == nil {
		t.Fatal("GetFieldSpec(id_card_no) should not be nil")
	}
	if spec.Level != 5 {
		t.Errorf("id_card_no level = %d, want 5", spec.Level)
	}
	if spec.Category != CategoryIdentity {
		t.Errorf("id_card_no category = %q, want 'identity'", spec.Category)
	}
}
