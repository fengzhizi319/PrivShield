package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestServer() *Server {
	cfg := &config.Config{
		Host:          "127.0.0.1",
		Port:          0,
		AgentRESTHost: "127.0.0.1",
		AgentRESTPort: 19999, // unreachable
		AgentAPIKey:   "",
		MaxLogEntries: 1000,
	}
	ag := agent.New(cfg)
	return New(ag, cfg)
}

func newTestRouter(s *Server) *gin.Engine {
	r := gin.New()
	s.RegisterRoutes(r)
	return r
}

func TestHealth(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["backend"] != "ok" {
		t.Errorf("expected backend=ok, got %v", resp["backend"])
	}
	if resp["via"] != "audit-log" {
		t.Errorf("expected via=audit-log, got %v", resp["via"])
	}
}

func TestListLogsEmpty(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/audit/logs", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected 0 logs, got %v", resp["total"])
	}
}

func TestCreateLog(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	body := map[string]any{
		"operation":      "mask",
		"datasource":     "卫健数据库",
		"algorithm":      "field_mask",
		"parameters":     map[string]any{"fields": []string{"name", "id_card"}},
		"input_rows":     1000,
		"output_rows":    1000,
		"duration_ms":    45,
		"user":           "admin",
		"status":         "success",
		"security_level": "L3",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty id")
	}

	// Verify it appears in list
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/audit/logs", nil)
	router.ServeHTTP(w2, req2)

	var resp2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["total"].(float64) != 1 {
		t.Errorf("expected 1 log, got %v", resp2["total"])
	}
}

func TestCreateLogInvalidBody(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Empty body should still succeed (no required fields in model)
	// but the log will have empty fields
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetLogNotFound(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/audit/logs/nonexistent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetLog(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a log
	body := map[string]any{
		"operation":      "k_anon",
		"datasource":     "医保数据库",
		"algorithm":      "k_anonymity",
		"input_rows":     5000,
		"output_rows":    5000,
		"duration_ms":    120,
		"user":           "data_scientist",
		"status":         "success",
		"security_level": "L4",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp["id"].(string)

	// Get the log
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/audit/logs/"+id, nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var log map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &log)
	if log["operation"] != "k_anon" {
		t.Errorf("expected operation=k_anon, got %v", log["operation"])
	}
	if log["security_level"] != "L4" {
		t.Errorf("expected security_level=L4, got %v", log["security_level"])
	}
}

func TestGetStats(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create some logs
	for _, op := range []string{"mask", "mask", "k_anon", "dp"} {
		body := map[string]any{
			"operation":      op,
			"datasource":     "test",
			"status":         "success",
			"security_level": "L3",
			"duration_ms":    50,
		}
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
	}

	// Get stats
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/audit/stats", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &stats)
	if stats["total_operations"].(float64) != 4 {
		t.Errorf("expected 4 total ops, got %v", stats["total_operations"])
	}
	byOp := stats["by_operation"].(map[string]any)
	if byOp["mask"].(float64) != 2 {
		t.Errorf("expected 2 mask ops, got %v", byOp["mask"])
	}
}

func TestListSnapshots(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a log (which auto-generates a snapshot)
	body := map[string]any{
		"operation": "mask",
		"algorithm": "field_mask",
		"status":    "success",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// List snapshots
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/audit/snapshots", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["total"].(float64) != 1 {
		t.Errorf("expected 1 snapshot, got %v", resp["total"])
	}
}

func TestVerifyIntegrity(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a log (auto-generates snapshot)
	body := map[string]any{
		"operation": "mask",
		"algorithm": "field_mask",
		"status":    "success",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Verify the snapshot
	verifyBody := map[string]any{"snapshot_id": "snap-1"}
	vb, _ := json.Marshal(verifyBody)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/audit/snapshots/verify", bytes.NewReader(vb))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var result map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &result)
	if result["valid"] != true {
		t.Errorf("expected valid=true, got %v", result["valid"])
	}
}

func TestVerifyIntegrityNotFound(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	body := map[string]any{"snapshot_id": "nonexistent"}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/snapshots/verify", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGenerateReport(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create some logs
	for i := 0; i < 5; i++ {
		body := map[string]any{
			"operation":      "mask",
			"datasource":     "test",
			"status":         "success",
			"security_level": "L3",
			"duration_ms":    50,
		}
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
	}

	// Generate report
	reportBody := map[string]any{"period": "24h"}
	rb, _ := json.Marshal(reportBody)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/audit/report", bytes.NewReader(rb))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var report map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &report)
	if report["total_operations"].(float64) != 5 {
		t.Errorf("expected 5 total ops, got %v", report["total_operations"])
	}
	if report["success_rate"].(float64) != 100 {
		t.Errorf("expected 100%% success rate, got %v", report["success_rate"])
	}
	recs := report["recommendations"].([]any)
	if len(recs) == 0 {
		t.Error("expected at least 1 recommendation")
	}
}

func TestListLogsWithFilter(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create logs with different operations
	for _, op := range []string{"mask", "k_anon", "dp"} {
		body := map[string]any{
			"operation":      op,
			"datasource":     "test",
			"status":         "success",
			"security_level": "L3",
		}
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/audit/logs", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
	}

	// Filter by operation
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/audit/logs?operation=mask", nil)
	router.ServeHTTP(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) != 1 {
		t.Errorf("expected 1 mask log, got %v", resp["total"])
	}
}

func TestComputeIntegrityHash(t *testing.T) {
	ts := time.Now()
	hash1 := computeIntegrityHash("log-1", ts, "field_mask")
	hash2 := computeIntegrityHash("log-1", ts, "field_mask")
	hash3 := computeIntegrityHash("log-2", ts, "field_mask")

	if hash1 != hash2 {
		t.Error("same inputs should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different inputs should produce different hash")
	}
	if len(hash1) != 64 { // SHA256 hex = 64 chars
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash1))
	}
}
