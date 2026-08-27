package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTraceMiddleware_SetsBothHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "req-trace-001")
	router.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "req-trace-001" {
		t.Errorf("X-Request-ID = %s, want req-trace-001", got)
	}
	if got := w.Header().Get("X-Trace-ID"); got != "req-trace-001" {
		t.Errorf("X-Trace-ID = %s, want req-trace-001", got)
	}
}

func TestTraceMiddleware_GeneratesID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TraceMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	// No X-Request-ID header
	router.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got == "" {
		t.Error("expected auto-generated X-Request-ID, got empty")
	}
	if got := w.Header().Get("X-Trace-ID"); got == "" {
		t.Error("expected auto-generated X-Trace-ID, got empty")
	}
	// Both headers should be the same
	if w.Header().Get("X-Request-ID") != w.Header().Get("X-Trace-ID") {
		t.Error("X-Request-ID and X-Trace-ID should be identical")
	}
}

func TestTraceMiddleware_BackwardCompatWithRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Use both: RequestID first, then TraceMiddleware
	router.Use(RequestID())
	router.Use(TraceMiddleware())

	var capturedTraceID, capturedRequestID string
	router.GET("/test", func(c *gin.Context) {
		capturedRequestID, _ = c.Get("request_id")
		capturedTraceID = GetTraceID(c)
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "req-compat-001")
	router.ServeHTTP(w, req)

	// Both should be the same value (TraceMiddleware reuses RequestID's value)
	if capturedRequestID != "req-compat-001" {
		t.Errorf("request_id = %s, want req-compat-001", capturedRequestID)
	}
	if capturedTraceID != "req-compat-001" {
		t.Errorf("GetTraceID = %s, want req-compat-001", capturedTraceID)
	}
	if capturedRequestID != capturedTraceID {
		t.Error("request_id and GetTraceID should return the same value")
	}
}

func TestGetTraceID_FallbackOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Case 1: TraceIDContextKey set
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(TraceIDContextKey, "trace-key-val")
	c.Set("request_id", "request-id-val")
	if got := GetTraceID(c); got != "trace-key-val" {
		t.Errorf("expected trace-key-val, got %s", got)
	}

	// Case 2: Only request_id set
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest("GET", "/", nil)
	c2.Set("request_id", "request-id-val")
	if got := GetTraceID(c2); got != "request-id-val" {
		t.Errorf("expected request-id-val, got %s", got)
	}

	// Case 3: Only header
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest("GET", "/", nil)
	c3.Request.Header.Set("X-Request-ID", "header-val")
	if got := GetTraceID(c3); got != "header-val" {
		t.Errorf("expected header-val, got %s", got)
	}

	// Case 4: Nothing set → generates new
	c4, _ := gin.CreateTestContext(httptest.NewRecorder())
	c4.Request = httptest.NewRequest("GET", "/", nil)
	if got := GetTraceID(c4); got == "" {
		t.Error("expected generated trace ID, got empty")
	}
}
