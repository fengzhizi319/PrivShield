package service

import (
	"strings"
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
