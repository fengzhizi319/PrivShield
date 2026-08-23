package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector("test-module")
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.module != "test-module" {
		t.Errorf("module = %q, want %q", c.module, "test-module")
	}
	if c.registry == nil {
		t.Error("registry is nil")
	}
	if c.HTTPRequestsTotal == nil {
		t.Error("HTTPRequestsTotal is nil")
	}
	if c.HTTPRequestDuration == nil {
		t.Error("HTTPRequestDuration is nil")
	}
	if c.AgentRequestsTotal == nil {
		t.Error("AgentRequestsTotal is nil")
	}
	if c.AgentRequestDuration == nil {
		t.Error("AgentRequestDuration is nil")
	}
}

func TestNewCollector_IndependentRegistries(t *testing.T) {
	c1 := NewCollector("module-a")
	c2 := NewCollector("module-b")
	if c1.registry == c2.registry {
		t.Error("two collectors should have independent registries")
	}
}

func TestRecordHTTP(t *testing.T) {
	c := NewCollector("test")
	// Should not panic
	c.RecordHTTP("GET", "/api/health", 200, 0.05)
	c.RecordHTTP("POST", "/api/dispatch", 201, 0.12)
	c.RecordHTTP("GET", "/api/health", 500, 0.01)
}

func TestRecordAgentCall(t *testing.T) {
	c := NewCollector("test")
	// Should not panic
	c.RecordAgentCall("/health", "200", 0.03)
	c.RecordAgentCall("/v1/privacy/mask", "500", 0.5)
}

func TestHandler_ReturnsMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewCollector("handler-test")

	// Record some metrics first
	c.RecordHTTP("GET", "/health", 200, 0.01)
	c.RecordAgentCall("/health", "200", 0.02)

	r := gin.New()
	r.GET("/metrics", c.Handler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	// Verify key metrics are present
	if !strings.Contains(body, "http_requests_total") {
		t.Error("response missing http_requests_total")
	}
	if !strings.Contains(body, "agent_requests_total") {
		t.Error("response missing agent_requests_total")
	}
	if !strings.Contains(body, `module="handler-test"`) {
		t.Error("response missing module label")
	}
}

func TestHandler_ContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewCollector("content-type-test")

	r := gin.New()
	r.GET("/metrics", c.Handler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "openmetrics") {
		t.Errorf("content-type = %q, want prometheus format", ct)
	}
}
