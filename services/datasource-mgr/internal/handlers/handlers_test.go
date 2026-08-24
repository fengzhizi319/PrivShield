package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/models"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := config.Load()
	logger := pkgconfig.SetupLogger("text", "debug")
	server := New(cfg, logger)

	r := gin.New()
	server.RegisterRoutes(r)
	return r
}

func TestHealth(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["backend"] != "ok" || resp["via"] != "datasource-mgr" {
		t.Errorf("unexpected health response: %+v", resp)
	}
}

func TestAPI1YibaoData(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/v1/yibao?limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.DataQueryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SourceID != "ds_yibao" || resp.Limit != 5 {
		t.Errorf("unexpected yibao response: %+v", resp)
	}
}

func TestAPI2KangyangData(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/v1/kangyang?limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.DataQueryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SourceID != "ds_kangyang" || resp.Limit != 5 {
		t.Errorf("unexpected kangyang response: %+v", resp)
	}
}

func TestAPI3Mock3Data(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/v1/mock3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp models.DataQueryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SourceID != "ds_mock3" || len(resp.Records) == 0 {
		t.Errorf("unexpected mock3 response: %+v", resp)
	}
}

func TestAPI4Mock4Data(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/v1/mock4", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp models.DataQueryResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SourceID != "ds_mock4" || len(resp.Records) == 0 {
		t.Errorf("unexpected mock4 response: %+v", resp)
	}
}

func TestListDataSources(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) < 2 {
		t.Errorf("expected at least 2 datasources, got: %+v", resp)
	}
}

func TestGetDataSource(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources/ds_yibao", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var ds models.MockDataSource
	_ = json.Unmarshal(w.Body.Bytes(), &ds)
	if ds.ID != "ds_yibao" {
		t.Errorf("unexpected datasource: %+v", ds)
	}
}

func TestGetDataSourceNotFound(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources/non_existent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetDataSourceRecords(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources/ds_yibao/records?limit=3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["datasource_id"] != "ds_yibao" {
		t.Errorf("unexpected records response: %+v", resp)
	}
}

func TestTestConnection(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("POST", "/api/datasources/ds_kangyang/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp models.ConnectionTestResult
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success || resp.DataSourceID != "ds_kangyang" {
		t.Errorf("unexpected connection test response: %+v", resp)
	}
}

func TestGetMetadata(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources/ds_yibao/metadata", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp models.MetadataResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.DataSourceID != "ds_yibao" || len(resp.Tables) == 0 {
		t.Errorf("unexpected metadata response: %+v", resp)
	}
}

func TestGetAccessAudit(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("GET", "/api/datasources/ds_yibao/audit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSeedDataSources(t *testing.T) {
	r := newTestRouter()
	req, _ := http.NewRequest("POST", "/api/datasources/seed", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestLoadCSVRecords_PathTraversal(t *testing.T) {
	// Absolute path attempts should be rejected by the allow-list / basename logic.
	malicious := []string{
		"../../../etc/passwd.csv",
		"..\\..\\..\\etc\\passwd.csv",
		"/etc/passwd.csv",
		"yibao.txt",
		"unknown.csv",
		"yibao.csv/../../etc/passwd.csv",
	}

	for _, name := range malicious {
		if _, _, err := LoadCSVRecords(name, 10, 0); err == nil {
			t.Errorf("expected error for path traversal attempt %q, got nil", name)
		}
	}
}

func TestLoadCSVRecords_AllowedFiles(t *testing.T) {
	// The two official mock datasets must continue to load successfully.
	for _, name := range []string{"yibao.csv", "kangyang.csv"} {
		records, total, err := LoadCSVRecords(name, 5, 0)
		if err != nil {
			t.Fatalf("unexpected error loading %s: %v", name, err)
		}
		if total <= 0 || len(records) == 0 {
			t.Errorf("expected records for %s, got total=%d len=%d", name, total, len(records))
		}
	}
}
