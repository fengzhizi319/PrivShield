package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAbortWithError_Format(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/test", nil)
	c.Request.Header.Set("X-Request-ID", "req-test-envelope-001")

	AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "参数校验失败", "field 'name' is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var env ErrorEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if env.Code != "INVALID_ARGUMENT" {
		t.Errorf("expected code INVALID_ARGUMENT, got %s", env.Code)
	}
	if env.Message != "参数校验失败" {
		t.Errorf("expected message '参数校验失败', got %s", env.Message)
	}
	if env.TraceID != "req-test-envelope-001" {
		t.Errorf("expected trace_id 'req-test-envelope-001', got %s", env.TraceID)
	}
	if env.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}

	// Verify headers
	if got := w.Header().Get("X-Request-ID"); got != "req-test-envelope-001" {
		t.Errorf("expected X-Request-ID header 'req-test-envelope-001', got %s", got)
	}
	if got := w.Header().Get("X-Trace-ID"); got != "req-test-envelope-001" {
		t.Errorf("expected X-Trace-ID header 'req-test-envelope-001', got %s", got)
	}
}

func TestRespondWithSuccess_Format(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/test", nil)
	c.Request.Header.Set("X-Request-ID", "req-success-001")

	RespondWithSuccess(c, http.StatusOK, "操作成功", map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var env SuccessEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if env.Code != "OK" {
		t.Errorf("expected code 'OK', got %s", env.Code)
	}
	if env.TraceID != "req-success-001" {
		t.Errorf("expected trace_id 'req-success-001', got %s", env.TraceID)
	}
}

func TestErrorCodeFromStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected string
	}{
		{http.StatusBadRequest, "INVALID_ARGUMENT"},
		{http.StatusUnauthorized, "UNAUTHORIZED"},
		{http.StatusForbidden, "FORBIDDEN"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusConflict, "CONFLICT"},
		{http.StatusTooManyRequests, "RATE_LIMITED"},
		{http.StatusInternalServerError, "INTERNAL_ERROR"},
		{http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
		{http.StatusTeapot, "UNKNOWN_ERROR"}, // unmapped
	}

	for _, tt := range tests {
		got := ErrorCodeFromStatus(tt.status)
		if got != tt.expected {
			t.Errorf("ErrorCodeFromStatus(%d) = %s, want %s", tt.status, got, tt.expected)
		}
	}
}

func TestAbortWithError_GeneratesTraceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// No X-Request-ID header set
	c.Request = httptest.NewRequest("GET", "/api/test", nil)

	AbortWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误", nil)

	var env ErrorEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)

	if env.TraceID == "" {
		t.Error("expected auto-generated trace_id, got empty string")
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if w.Header().Get("X-Trace-ID") == "" {
		t.Error("expected X-Trace-ID header to be set")
	}
}
