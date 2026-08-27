package tlsutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testWhitelistYAML = `version: "1.0"
clients:
  - cn: "bff-go.privshield.internal"
    allowed_scopes: ["*"]
    role: "gateway"
    description: "BFF gateway"
    enabled: true

  - cn: "service-hub.privshield.internal"
    allowed_scopes: ["/PrivacyService/Process", "/AuditLog/*"]
    role: "orchestrator"
    description: "Service hub"
    enabled: true

  - cn: "disabled-service"
    allowed_scopes: ["*"]
    description: "Disabled entry"
    enabled: false
`

func createTempWhitelist(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Clean(filepath.Join(dir, "mtls-whitelist.yaml"))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp whitelist: %v", err)
	}
	return path
}

func TestNewDynamicWhitelist_Load(t *testing.T) {
	path := createTempWhitelist(t, testWhitelistYAML)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	// bff-go should be loaded
	if !dw.IsAuthorized("bff-go.privshield.internal") {
		t.Error("expected bff-go to be authorized")
	}

	// service-hub should be loaded
	if !dw.IsAuthorized("service-hub.privshield.internal") {
		t.Error("expected service-hub to be authorized")
	}

	// disabled-service should NOT be loaded
	if dw.IsAuthorized("disabled-service") {
		t.Error("expected disabled-service to NOT be authorized")
	}

	// unknown CN should fail
	if dw.IsAuthorized("unknown-client") {
		t.Error("expected unknown-client to NOT be authorized")
	}
}

func TestDynamicWhitelist_CheckScope(t *testing.T) {
	path := createTempWhitelist(t, testWhitelistYAML)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	// bff-go has wildcard scope
	ok, scopes := dw.CheckScope("bff-go.privshield.internal", "/AnyMethod/Anything")
	if !ok {
		t.Error("expected bff-go to be authorized for any method")
	}
	if len(scopes) != 1 || scopes[0] != "*" {
		t.Errorf("expected scopes [*], got %v", scopes)
	}

	// service-hub has specific scopes
	ok, _ = dw.CheckScope("service-hub.privshield.internal", "/PrivacyService/Process")
	if !ok {
		t.Error("expected service-hub to be authorized for /PrivacyService/Process")
	}

	// service-hub wildcard pattern /AuditLog/*
	ok, _ = dw.CheckScope("service-hub.privshield.internal", "/AuditLog/RecordAudit")
	if !ok {
		t.Error("expected service-hub to be authorized for /AuditLog/RecordAudit via wildcard")
	}

	// service-hub should NOT have access to unauthorized method
	ok, _ = dw.CheckScope("service-hub.privshield.internal", "/DatasourceMgr/FetchSlice")
	if ok {
		t.Error("expected service-hub to NOT be authorized for /DatasourceMgr/FetchSlice")
	}

	// unknown CN
	ok, _ = dw.CheckScope("unknown", "/AnyMethod")
	if ok {
		t.Error("expected unknown CN to fail scope check")
	}
}

func TestDynamicWhitelist_HotReload(t *testing.T) {
	path := createTempWhitelist(t, testWhitelistYAML)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	// Initially bff-go is authorized
	if !dw.IsAuthorized("bff-go.privshield.internal") {
		t.Fatal("expected bff-go to be authorized initially")
	}

	// Rewrite the file with different content
	newYAML := `version: "1.0"
clients:
  - cn: "new-client"
    allowed_scopes: ["*"]
    enabled: true
`
	// Wait a bit to ensure mod time changes (filesystem granularity)
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte(newYAML), 0644); err != nil {
		t.Fatalf("failed to rewrite whitelist: %v", err)
	}

	// Manually trigger reload (polling would take 5s)
	if err := dw.reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Old CN should no longer be authorized
	if dw.IsAuthorized("bff-go.privshield.internal") {
		t.Error("expected bff-go to NOT be authorized after reload")
	}

	// New CN should be authorized
	if !dw.IsAuthorized("new-client") {
		t.Error("expected new-client to be authorized after reload")
	}
}

func TestDynamicWhitelist_GetScopes(t *testing.T) {
	path := createTempWhitelist(t, testWhitelistYAML)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	scopes, ok := dw.GetScopes("service-hub.privshield.internal")
	if !ok {
		t.Fatal("expected service-hub to exist")
	}
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d: %v", len(scopes), scopes)
	}

	_, ok = dw.GetScopes("nonexistent")
	if ok {
		t.Error("expected nonexistent CN to return false")
	}
}

func TestDynamicWhitelist_InvalidFile(t *testing.T) {
	_, err := NewDynamicWhitelist("/nonexistent/path/whitelist.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestDynamicWhitelist_InvalidYAML(t *testing.T) {
	path := createTempWhitelist(t, "invalid: yaml: [broken")
	_, err := NewDynamicWhitelist(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestDynamicWhitelist_LegacyEntriesFormat(t *testing.T) {
	legacyYAML := `version: "1.0"
entries:
  - cn: "legacy-client"
    scopes: ["*"]
    description: "Legacy format entry"
    enabled: true
  - cn: "legacy-disabled"
    scopes: ["*"]
    enabled: false
`
	path := createTempWhitelist(t, legacyYAML)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	if !dw.IsAuthorized("legacy-client") {
		t.Error("expected legacy-client to be authorized")
	}
	if dw.IsAuthorized("legacy-disabled") {
		t.Error("expected legacy-disabled to NOT be authorized")
	}
}

func TestMatchScopePattern(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*", "/Any/Method", true},
		{"/ServiceHub/*", "/ServiceHub/DispatchTask", true},
		{"/ServiceHub/*", "/ServiceHub/", true},
		{"/ServiceHub/*", "/AuditLog/Record", false},
		{"/PrivacyService/Process", "/PrivacyService/Process", true},
		{"/PrivacyService/Process", "/PrivacyService/Other", false},
	}

	for _, tt := range tests {
		got := matchScopePattern(tt.pattern, tt.value)
		if got != tt.want {
			t.Errorf("matchScopePattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}
