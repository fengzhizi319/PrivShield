package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
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
	if resp["via"] != "datasource-mgr" {
		t.Errorf("expected via=datasource-mgr, got %v", resp["via"])
	}
}

func TestListDataSourcesEmpty(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/datasources", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected 0 datasources, got %v", resp["total"])
	}
}

func TestCreateDataSource(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	body := map[string]any{
		"name":           "测试数据库",
		"type":           "database",
		"host":           "192.168.1.100",
		"port":           5432,
		"database":       "test_db",
		"security_level": "high",
		"tags":           []string{"卫健", "高密"},
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
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
	if resp["via"] != "datasource-mgr" {
		t.Errorf("expected via=datasource-mgr, got %v", resp["via"])
	}

	// Verify it appears in list
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/datasources", nil)
	router.ServeHTTP(w2, req2)

	var resp2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["total"].(float64) != 1 {
		t.Errorf("expected 1 datasource, got %v", resp2["total"])
	}
}

func TestCreateDataSourceInvalidBody(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetDataSourceNotFound(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/datasources/nonexistent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteDataSourceNotFound(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/datasources/nonexistent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateAndGetAndDelete(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create
	body := map[string]any{
		"name":           "卫健数据库",
		"type":           "database",
		"host":           "192.168.1.100",
		"port":           5432,
		"security_level": "high",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp["id"].(string)

	// Get
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/datasources/"+id, nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w2.Code)
	}

	var ds map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &ds)
	if ds["name"] != "卫健数据库" {
		t.Errorf("expected name=卫健数据库, got %v", ds["name"])
	}

	// Delete
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("DELETE", "/api/datasources/"+id, nil)
	router.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w3.Code)
	}

	// Verify deleted
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("GET", "/api/datasources/"+id, nil)
	router.ServeHTTP(w4, req4)

	if w4.Code != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", w4.Code)
	}
}

func TestGetMetadata(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a datasource first
	body := map[string]any{
		"name": "测试库",
		"type": "database",
		"host": "127.0.0.1",
		"port": 3306,
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp["id"].(string)

	// Get metadata
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/datasources/"+id+"/metadata", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var meta map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &meta)
	tables := meta["tables"].([]any)
	if len(tables) == 0 {
		t.Error("expected at least 1 table in metadata")
	}
}

func TestGetAccessAudit(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a datasource (which generates audit records)
	body := map[string]any{
		"name": "审计测试库",
		"type": "database",
		"host": "127.0.0.1",
		"port": 3306,
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp["id"].(string)

	// Get audit log
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/datasources/"+id+"/audit", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var audit map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &audit)
	// Should have at least the "create" audit record
	if audit["total"].(float64) < 1 {
		t.Errorf("expected at least 1 audit record, got %v", audit["total"])
	}
}
