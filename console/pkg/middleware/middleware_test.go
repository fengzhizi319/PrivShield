package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─────────────────────────────────────────────────────────────
// CORS / 跨域中间件测试
// ─────────────────────────────────────────────────────────────

func TestCORS_AllowAll(t *testing.T) {
	r := gin.New()
	r.Use(CORS(nil))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
}

func TestCORS_AllowAllWildcard(t *testing.T) {
	r := gin.New()
	r.Use(CORS([]string{"*"}))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
}

func TestCORS_SpecificOrigins(t *testing.T) {
	allowed := []string{"http://localhost:5173", "http://localhost:3000"}
	r := gin.New()
	r.Use(CORS(allowed))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// Matching origin
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q, want http://localhost:5173", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}

	// Non-matching origin
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w2, req2)

	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty (non-matching origin)", got)
	}
}

func TestCORS_PreflightOptions(t *testing.T) {
	r := gin.New()
	r.Use(CORS(nil))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Errorf("Allow-Methods = %q, should contain GET", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Request-ID") {
		t.Errorf("Allow-Headers = %q, should contain X-Request-ID", got)
	}
}

// ─────────────────────────────────────────────────────────────
// Auth / 鉴权中间件测试
// ─────────────────────────────────────────────────────────────

func TestAuth_EmptyKey_SkipsAuth(t *testing.T) {
	r := gin.New()
	r.Use(Auth(""))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no auth required)", w.Code)
	}
}

func TestAuth_HealthExempt(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret-key"))
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for _, path := range []string{"/health", "/api/health"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (health exempt)", path, w.Code)
		}
	}
}

func TestAuth_ValidKey(t *testing.T) {
	r := gin.New()
	r.Use(Auth("my-secret"))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAuth_InvalidKey(t *testing.T) {
	r := gin.New()
	r.Use(Auth("my-secret"))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuth_MissingToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("my-secret"))
	r.GET("/api/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuth_NonApiPath_Exempt(t *testing.T) {
	r := gin.New()
	r.Use(Auth("my-secret"))
	r.GET("/metrics", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (non-api path exempt)", w.Code)
	}
}

func TestExtractBearer(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer token123", "token123"},
		{"bearer token123", "token123"},
		{"BEARER token123", "token123"},
		{"Basic dXNlcjpwYXNz", ""},
		{"", ""},
		{"Bearer", ""},
		{"Bearer a b", ""},
	}
	for _, tt := range tests {
		if got := extractBearer(tt.header); got != tt.want {
			t.Errorf("extractBearer(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

// ─────────────────────────────────────────────────────────────
// RequestID / 请求 ID 中间件测试
// ─────────────────────────────────────────────────────────────

func TestRequestID_Passthrough(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(200, gin.H{"request_id": rid})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "incoming-rid-42")
	r.ServeHTTP(w, req)

	// Response should echo the incoming request ID
	if got := w.Header().Get("X-Request-ID"); got != "incoming-rid-42" {
		t.Errorf("response X-Request-ID = %q, want incoming-rid-42", got)
	}
}

func TestRequestID_Generated(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		rid, _ := c.Get("request_id")
		c.JSON(200, gin.H{"request_id": rid})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	// No X-Request-ID header → should be generated
	r.ServeHTTP(w, req)

	got := w.Header().Get("X-Request-ID")
	if got == "" {
		t.Error("X-Request-ID should be generated when not provided")
	}
	if !strings.HasPrefix(got, "req-") && len(got) < 10 {
		t.Errorf("generated request ID looks unexpected: %q", got)
	}
}

// ─────────────────────────────────────────────────────────────
// StructuredLogger / 结构化日志中间件测试
// ─────────────────────────────────────────────────────────────

func TestStructuredLogger_NoPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := gin.New()
	r.Use(StructuredLogger(logger, "test-module"))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestStructuredLogger_NilLogger(t *testing.T) {
	r := gin.New()
	r.Use(StructuredLogger(nil, "test-module"))
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ─────────────────────────────────────────────────────────────
// Recovery / 异常恢复中间件测试
// ─────────────────────────────────────────────────────────────

func TestRecovery_CatchesPanic(t *testing.T) {
	r := gin.New()
	r.Use(RequestID())
	r.Use(Recovery(nil, "test-module"))
	r.GET("/panic", func(c *gin.Context) {
		panic("unexpected runtime error")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// ─────────────────────────────────────────────────────────────
// SecurityHeaders / 安全头中间件测试
// ─────────────────────────────────────────────────────────────

func TestSecurityHeaders(t *testing.T) {
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/test", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if got := w.Header().Get("X-XSS-Protection"); got != "1; mode=block" {
		t.Errorf("X-XSS-Protection = %q, want 1; mode=block", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q, want strict-origin-when-cross-origin", got)
	}
}
