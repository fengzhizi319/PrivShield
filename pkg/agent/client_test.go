package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// ─────────────────────────────────────────────────────────────
// Client basics / 客户端基础测试
// ─────────────────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	c := New(Config{BaseURL: "http://localhost:8079"})
	if c.BaseURL() != "http://localhost:8079" {
		t.Errorf("BaseURL() = %q, want %q", c.BaseURL(), "http://localhost:8079")
	}
	if c.cbThreshold != 5 {
		t.Errorf("cbThreshold = %d, want 5", c.cbThreshold)
	}
	if c.cbCooldown != 30*time.Second {
		t.Errorf("cbCooldown = %v, want 30s", c.cbCooldown)
	}
}

func TestNew_CustomConfig(t *testing.T) {
	c := New(Config{
		BaseURL:     "http://example.com",
		APIKey:      "secret",
		Timeout:     10 * time.Second,
		CBThreshold: 3,
		CBCooldown:  15 * time.Second,
		Logger:      newTestLogger(),
	})
	if c.apiKey != "secret" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "secret")
	}
	if c.cbThreshold != 3 {
		t.Errorf("cbThreshold = %d, want 3", c.cbThreshold)
	}
	if c.cbCooldown != 15*time.Second {
		t.Errorf("cbCooldown = %v, want 15s", c.cbCooldown)
	}
}

func TestBaseURL(t *testing.T) {
	c := New(Config{BaseURL: "http://test:9090"})
	if got := c.BaseURL(); got != "http://test:9090" {
		t.Errorf("BaseURL() = %q, want %q", got, "http://test:9090")
	}
}

// ─────────────────────────────────────────────────────────────
// GET / POST / Health / 请求测试
// ─────────────────────────────────────────────────────────────

func TestGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/health" {
			t.Errorf("path = %s, want /health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Logger: newTestLogger()})
	result, err := c.Get(context.Background(), "/health")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}
}

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %s, want application/json", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": body["input"]})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Logger: newTestLogger()})
	result, err := c.Post(context.Background(), "/v1/privacy/mask", map[string]any{"input": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "test" {
		t.Errorf("result = %v, want test", result["result"])
	}
}

func TestPostWithRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"request_id": rid})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Logger: newTestLogger()})
	result, err := c.PostWithRequestID(context.Background(), "/test", nil, "req-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", result["request_id"])
	}
}

func TestBearerTokenInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"auth": auth})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, APIKey: "my-key", Logger: newTestLogger()})
	result, err := c.Get(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["auth"] != "Bearer my-key" {
		t.Errorf("auth = %v, want Bearer my-key", result["auth"])
	}
}

func TestGet_AgentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Logger: newTestLogger()})
	_, err := c.Get(context.Background(), "/fail")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestHealth_DelegatesToGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path = %s, want /health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Logger: newTestLogger()})
	result, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}
}

// ─────────────────────────────────────────────────────────────
// Circuit Breaker / 熔断器测试
// ─────────────────────────────────────────────────────────────

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	// MaxRetries=1 + RetryBaseDelay=1ms: each c.Get() makes 2 attempts (1 initial + 1 retry),
	// each recording a failure. CBThreshold=2 means 1 c.Get() call (2 failures) trips the circuit.
	// 每个 c.Get() 调用产生 2 次尝试（1 次初始 + 1 次重试），每次均记录失败。
	// CBThreshold=2 表示 1 次 c.Get() 调用（2 次失败）即触发熔断。
	c := New(Config{
		BaseURL:       srv.URL,
		CBThreshold:   2,
		CBCooldown:    1 * time.Second,
		MaxRetries:    1,
		RetryBaseDelay: time.Millisecond,
		Logger:        newTestLogger(),
	})

	// 1 request with 2 attempts → 2 failures >= threshold(2) → circuit opens
	// 1 次请求含 2 次尝试 → 2 次失败 >= 阈值(2) → 熔断器打开
	c.Get(context.Background(), "/test")

	// Circuit should now be open
	if state := c.CircuitStateString(); state != "open" {
		t.Errorf("state = %s, want open", state)
	}

	// Next request should be rejected immediately (circuit open)
	_, err := c.Get(context.Background(), "/test")
	if err == nil {
		t.Fatal("expected circuit breaker error, got nil")
	}
	if got := err.Error(); got != "circuit breaker open (cooldown remaining)" {
		t.Errorf("error = %q, want circuit breaker open", got)
	}
}

func TestCircuitBreaker_HalfOpenAfterCooldown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	c := New(Config{
		BaseURL:     srv.URL,
		CBThreshold: 2,
		CBCooldown:  50 * time.Millisecond,
		Logger:      newTestLogger(),
	})

	// Trip the circuit breaker
	for i := 0; i < 2; i++ {
		c.Get(context.Background(), "/test")
	}
	if state := c.CircuitStateString(); state != "open" {
		t.Fatalf("state = %s, want open", state)
	}

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one probe
	// (will fail since server still returns 500, but it should be allowed through)
	c.Get(context.Background(), "/test")

	// After failed probe, should re-open
	if state := c.CircuitStateString(); state != "open" {
		t.Errorf("state after failed probe = %s, want open", state)
	}
}

func TestCircuitBreaker_RecoveryOnSuccess(t *testing.T) {
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	c := New(Config{
		BaseURL:     srv.URL,
		CBThreshold: 2,
		CBCooldown:  50 * time.Millisecond,
		Logger:      newTestLogger(),
	})

	// Trip the circuit breaker
	for i := 0; i < 2; i++ {
		c.Get(context.Background(), "/test")
	}

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)

	// Make server healthy
	shouldFail.Store(false)

	// Probe request should succeed → circuit closes
	result, err := c.Get(context.Background(), "/test")
	if err != nil {
		t.Fatalf("unexpected error after recovery: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}

	if state := c.CircuitStateString(); state != "closed" {
		t.Errorf("state = %s, want closed", state)
	}
}

func TestCircuitBreaker_IntermittentSuccessResetsFailureCount(t *testing.T) {
	var shouldFail atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	// MaxRetries=1 + RetryBaseDelay=1ms: each c.Get() makes 2 attempts.
	// CBThreshold=5: need 5 consecutive failures to trip.
	// 每次 c.Get() 调用产生 2 次尝试。CBThreshold=5：需要 5 次连续失败才触发熔断。
	c := New(Config{
		BaseURL:       srv.URL,
		CBThreshold:   5,
		MaxRetries:    1,
		RetryBaseDelay: time.Millisecond,
		Logger:        newTestLogger(),
	})

	// 2 failures (2 calls × 2 attempts = 4 consecutive failures)
	// 2 次失败（2 次调用 × 2 次尝试 = 4 次连续失败）
	shouldFail.Store(true)
	c.Get(context.Background(), "/test")
	c.Get(context.Background(), "/test")
	if c.CircuitStateString() != "closed" {
		t.Fatalf("expected closed after 4 consecutive failures, got %s", c.CircuitStateString())
	}

	// 1 success -> resets consecutive failure counter to 0
	// 1 次成功 → 重置连续失败计数器为 0
	shouldFail.Store(false)
	c.Get(context.Background(), "/test")

	// 2 more failures (2 calls × 2 attempts = 4 consecutive failures)
	// Should NOT trip since threshold is 5 and failures were reset by the success
	// 不应触发熔断，因为阈值为 5 且失败计数已被成功重置
	shouldFail.Store(true)
	c.Get(context.Background(), "/test")
	c.Get(context.Background(), "/test")

	if c.CircuitStateString() != "closed" {
		t.Errorf("expected circuit to stay closed because failures were non-consecutive, got %s", c.CircuitStateString())
	}
}

func TestMultiNode_RoundRobin(t *testing.T) {
	var count1, count2 atomic.Int64

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"node": 1})
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"node": 2})
	}))
	defer srv2.Close()

	c := New(Config{
		BaseURLs: []string{srv1.URL, srv2.URL},
		Logger:   newTestLogger(),
	})

	if len(c.BaseURLs()) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(c.BaseURLs()))
	}

	// Make 6 requests, should be evenly distributed (3 to srv1, 3 to srv2)
	for i := 0; i < 6; i++ {
		_, err := c.Get(context.Background(), "/node")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	if count1.Load() != 3 || count2.Load() != 3 {
		t.Errorf("expected 3 requests each, got srv1=%d, srv2=%d", count1.Load(), count2.Load())
	}
}

func TestCircuitBreaker_ClientError4xx_NoTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"detail":"invalid argument"}`))
	}))
	defer srv.Close()

	c := New(Config{
		BaseURL:     srv.URL,
		CBThreshold: 3,
		Logger:      newTestLogger(),
	})

	// Send 5 400 Bad Request calls
	for i := 0; i < 5; i++ {
		_, err := c.Get(context.Background(), "/bad-request")
		if err == nil {
			t.Errorf("expected error for 400, got nil")
		}
	}

	// Circuit should still be CLOSED because 4xx does not indicate an agent server outage
	if state := c.CircuitStateString(); state != "closed" {
		t.Errorf("state = %s, want closed (4xx errors must not trip circuit breaker)", state)
	}
}


