package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"

	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testDeps bundles shared test dependencies.
type testDeps struct {
	ds     *memory.DataSourceStore
	logger *slog.Logger
	mc     *metrics.Collector
}

func newTestServer() *Server {
	cfg := &config.Config{
		Host:          "127.0.0.1",
		Port:          0,
		AgentRESTHost: "127.0.0.1",
		AgentRESTPort: 19999, // unreachable
		AgentAPIKey:   "",
	}
	d := &testDeps{
		ds:     memory.NewDataSourceStore(),
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		mc:     metrics.NewCollector("datasource-mgr-test"),
	}
	ag := agent.New(cfg)
	return New(ag, cfg, d.ds, d.logger, d.mc)
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

// TestCreateDataSourceNameTooLong 验证 name 超过 1024 字符时返回 400。
// P43 fix: name length validation.
func TestCreateDataSourceNameTooLong(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	longName := strings.Repeat("x", 1025)
	body := map[string]any{
		"name": longName,
		"type": "database",
		"host": "127.0.0.1",
		"port": 3306,
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for name > 1024 chars, got %d", w.Code)
	}
}

func TestUpdateDataSource(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// Create a datasource first
	body := map[string]any{
		"name":           "初始名称",
		"type":           "database",
		"host":           "127.0.0.1",
		"port":           3306,
		"security_level": "medium",
		"tags":           []string{"tag1"},
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	var createResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &createResp)
	id := createResp["id"].(string)

	// Update datasource
	updateBody := map[string]any{
		"name":           "修改后名称",
		"security_level": "high",
		"tags":           []string{"tag1", "tag2"},
	}
	ub, _ := json.Marshal(updateBody)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/datasources/"+id, bytes.NewReader(ub))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify get reflects updated values
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/datasources/"+id, nil)
	router.ServeHTTP(w3, req3)

	var ds map[string]any
	_ = json.Unmarshal(w3.Body.Bytes(), &ds)
	if ds["name"] != "修改后名称" {
		t.Errorf("expected name=修改后名称, got %v", ds["name"])
	}
	if ds["security_level"] != "high" {
		t.Errorf("expected security_level=high, got %v", ds["security_level"])
	}
}

func TestSeedAndFetchRecords(t *testing.T) {
	s := newTestServer()
	router := newTestRouter(s)

	// 1. Trigger Seed
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/datasources/seed", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on seed, got %d: %s", w.Code, w.Body.String())
	}

	// 2. List DataSources - should contain ds_yibao and ds_kangyang
	wList := httptest.NewRecorder()
	reqList, _ := http.NewRequest("GET", "/api/datasources", nil)
	router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", wList.Code)
	}
	var listResp map[string]any
	_ = json.Unmarshal(wList.Body.Bytes(), &listResp)
	total := int(listResp["total"].(float64))
	if total < 2 {
		t.Fatalf("expected at least 2 seeded datasources, got %d", total)
	}

	// 3. Fetch Records from ds_yibao
	wRec := httptest.NewRecorder()
	reqRec, _ := http.NewRequest("GET", "/api/datasources/ds_yibao/records?limit=5&offset=0", nil)
	router.ServeHTTP(wRec, reqRec)
	if wRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on records, got %d: %s", wRec.Code, wRec.Body.String())
	}
	var recResp map[string]any
	_ = json.Unmarshal(wRec.Body.Bytes(), &recResp)
	records := recResp["records"].([]any)
	if len(records) == 0 {
		t.Fatalf("expected records from yibao.csv, got 0")
	}

	// 4. Get Metadata from ds_kangyang
	wMeta := httptest.NewRecorder()
	reqMeta, _ := http.NewRequest("GET", "/api/datasources/ds_kangyang/metadata", nil)
	router.ServeHTTP(wMeta, reqMeta)
	if wMeta.Code != http.StatusOK {
		t.Fatalf("expected 200 on metadata, got %d: %s", wMeta.Code, wMeta.Body.String())
	}
	var metaResp map[string]any
	_ = json.Unmarshal(wMeta.Body.Bytes(), &metaResp)
	tables := metaResp["tables"].([]any)
	if len(tables) == 0 {
		t.Fatalf("expected tables from kangyang.csv metadata, got 0")
	}
}

func TestLoadCSVRecords_PathTraversal(t *testing.T) {
	// Attempt to access non-csv and traversal paths
	traversalPaths := []string{
		"../../../../etc/passwd",
		"/etc/shadow",
		"test.txt",
		"../../../main.go",
	}

	for _, p := range traversalPaths {
		_, _, err := LoadCSVRecords(p, 10, 0)
		if err == nil {
			t.Errorf("expected error for traversal path %q, got nil", p)
		}
	}
}


