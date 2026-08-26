package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/clients"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/models"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/runner"
)

func setupTestRouter() *Handler {
	cfg := &config.Config{
		Host:          "127.0.0.1",
		Port:          "8085",
		HubURL:        "http://127.0.0.1:8082",
		DatasourceURL: "http://127.0.0.1:8083",
		AuditURL:      "http://127.0.0.1:8084",
		AgentURL:      "http://127.0.0.1:8079",
	}
	pool := clients.NewClientPool(cfg)
	testRunner := runner.NewTestRunner(pool)
	return NewHandler(cfg, pool, testRunner)
}

func TestHealthCheck(t *testing.T) {
	h := setupTestRouter()
	router := SetupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestGetTopology(t *testing.T) {
	h := setupTestRouter()
	router := SetupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/lz/topology?protocol=rest", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var topo models.TopologyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &topo); err != nil {
		t.Fatalf("failed to parse topology: %v", err)
	}
	if len(topo.Services) != 4 {
		t.Fatalf("expected 4 services, got %d", len(topo.Services))
	}

	// Verify strictly fixed order: 1. Hub, 2. Agent, 3. Datasource, 4. Audit
	expectedOrder := []string{"service-hub", "engine", "datasource-mgr", "audit-log"}
	for i, exp := range expectedOrder {
		if topo.Services[i].ID != exp {
			t.Errorf("expected service[%d] = %s, got %s", i, exp, topo.Services[i].ID)
		}
	}

	// Test gRPC protocol query
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/lz/topology?protocol=grpc", nil)
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200 for grpc query, got %d", w2.Code)
	}
}

func TestGetSuitesAndRun(t *testing.T) {
	h := setupTestRouter()
	router := SetupRouter(h)

	// 1. Get Suites
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/lz/suites", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// 2. Run Suites (TS-01, TS-02, TS-03)
	runPayload := models.RunTestSuiteRequest{
		SuiteIDs:          []string{"TS-01", "TS-02", "TS-03"},
		Concurrency:       5,
		BenchmarkRequests: 10,
	}
	data, _ := json.Marshal(runPayload)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/lz/suites/run", bytes.NewReader(data))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w2.Code)
	}

	var runResp models.RunTestSuiteResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("failed to parse run response: %v", err)
	}
	if runResp.TotalCases != 3 {
		t.Errorf("expected 3 total suite items, got %d", runResp.TotalCases)
	}
}

func TestGetLeases(t *testing.T) {
	h := setupTestRouter()
	router := SetupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/lz/tasks/leases", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var leases models.LeasedTasksResponse
	if err := json.Unmarshal(w.Body.Bytes(), &leases); err != nil {
		t.Fatalf("failed to parse leases: %v", err)
	}
	if leases.StoreBackend != "sqlite" {
		t.Errorf("expected sqlite store, got %s", leases.StoreBackend)
	}
}
