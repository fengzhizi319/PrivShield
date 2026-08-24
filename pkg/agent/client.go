// Package agent provides a shared HTTP client to the upstream PrivShield agent REST API.
// Package agent 封装到上游 PrivShield agent REST API 的共享 HTTP 客户端。
//
// 三个控制台 Go 模块（service-hub / datasource-mgr / audit-log）均嵌入本 Client，
// 避免重复实现相同的 HTTP 调用、鉴权头注入与错误处理逻辑。
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Client wraps HTTP calls to the upstream PrivShield agent REST API with multi-node load balancing.
// Client 封装对上游 PrivShield agent REST API 的 HTTP 调用，支持多节点客户端负载均衡与故障转移。
type Client struct {
	baseURLs   []string
	apiKey     string
	httpClient *http.Client
	logger     *slog.Logger
	rrIndex    uint64

	// Circuit breaker state / 熔断器状态
	cbMu        sync.Mutex
	cbState     CircuitState
	cbFailures  int
	cbOpenedAt  time.Time
	cbThreshold int           // Consecutive failures before opening / 连续失败熔断阈值
	cbCooldown  time.Duration // Cooldown before half-open / 熔断冷却时间
}

// CircuitState represents the circuit breaker state.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation / 正常运行
	CircuitOpen                         // Tripped, rejecting calls / 熔断中，拒绝调用
	CircuitHalfOpen                     // Testing recovery / 探测恢复中
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds agent client configuration.
type Config struct {
	BaseURL            string        // Upstream agent base URL / 上游 agent 单基础地址
	BaseURLs           []string      // Upstream agent multi-node cluster URLs / 上游 agent 多节点集群地址
	APIKey             string        // Optional Bearer token / 可选 Bearer 令牌
	Timeout            time.Duration // HTTP client timeout / HTTP 客户端超时
	CBThreshold        int           // Consecutive failures before opening / 连续失败熔断阈值
	CBCooldown         time.Duration // Cooldown before half-open / 熔断冷却时间
	Logger             *slog.Logger  // Structured logger / 结构化日志
}

// New creates a new agent client from the given config.
// New 根据配置创建新的 agent 客户端，支持多节点轮询与自动容灾。
func New(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.CBThreshold == 0 {
		cfg.CBThreshold = 5
	}
	if cfg.CBCooldown == 0 {
		cfg.CBCooldown = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	urls := make([]string, 0)
	if len(cfg.BaseURLs) > 0 {
		urls = append(urls, cfg.BaseURLs...)
	} else if cfg.BaseURL != "" {
		urls = append(urls, cfg.BaseURL)
	}

	return &Client{
		baseURLs: urls,
		apiKey:   cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger:      cfg.Logger,
		cbState:     CircuitClosed,
		cbFailures:  0,
		cbThreshold: cfg.CBThreshold,
		cbCooldown:  cfg.CBCooldown,
	}
}

// BaseURL returns the first configured upstream agent base URL.
func (c *Client) BaseURL() string {
	if len(c.baseURLs) == 0 {
		return ""
	}
	return c.baseURLs[0]
}

// BaseURLs returns all configured agent base URLs.
func (c *Client) BaseURLs() []string {
	return c.baseURLs
}

// PickEndpoint returns the next URL in the cluster using round-robin.
func (c *Client) PickEndpoint() string {
	if len(c.baseURLs) == 0 {
		return ""
	}
	if len(c.baseURLs) == 1 {
		return c.baseURLs[0]
	}
	c.cbMu.Lock()
	idx := c.rrIndex
	c.rrIndex++
	c.cbMu.Unlock()
	return c.baseURLs[idx%uint64(len(c.baseURLs))]
}

// Health checks the upstream agent health.
// Health 检查上游 agent 健康状态。
func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	return c.Get(ctx, "/health")
}

// Get performs a GET request to the agent and returns parsed JSON.
// Get 对 agent 执行 GET 请求并返回解析后的 JSON。
func (c *Client) Get(ctx context.Context, path string) (map[string]any, error) {
	if err := c.checkCircuit(); err != nil {
		return nil, err
	}
	endpoint := c.PickEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("no agent endpoint available")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)
	return c.do(req)
}

// Post performs a POST request to the agent and returns parsed JSON.
// Post 对 agent 执行 POST 请求并返回解析后的 JSON。
// P59 fix: delegates to PostWithRequestID to eliminate code duplication.
func (c *Client) Post(ctx context.Context, path string, payload any) (map[string]any, error) {
	return c.PostWithRequestID(ctx, path, payload, "")
}

// PostWithRequestID performs a POST request, injecting X-Request-ID for tracing.
// P59 fix: single implementation that Post delegates to, avoiding duplicated logic.
func (c *Client) PostWithRequestID(ctx context.Context, path string, payload any, requestID string) (map[string]any, error) {
	if err := c.checkCircuit(); err != nil {
		return nil, err
	}
	endpoint := c.PickEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("no agent endpoint available")
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return c.do(req)
}

func (c *Client) setHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func (c *Client) do(req *http.Request) (map[string]any, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.recordFailure()
		c.logger.Warn("agent request failed",
			"method", req.Method,
			"path", req.URL.Path,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("agent request failed: %w", err)
	}
	defer resp.Body.Close()

	// P23 fix: limit response body to 64 MiB to prevent OOM from misbehaving upstream
	// 限制响应体最大 64 MiB，防止上游异常返回超大响应导致 OOM
	const maxBodySize = 64 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("read agent response: %w", err)
	}
	if int64(len(body)) > maxBodySize {
		c.recordFailure()
		return nil, fmt.Errorf("agent response too large: exceeds %d bytes", maxBodySize)
	}

	if resp.StatusCode >= 400 {
		c.recordFailure()
		c.logger.Warn("agent returned error status",
			"method", req.Method,
			"path", req.URL.Path,
			"status", resp.StatusCode,
		)
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	c.recordSuccess()

	var result map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parse agent response: %w", err)
		}
	}
	return result, nil
}

// CircuitStateString returns the current circuit breaker state as a string.
func (c *Client) CircuitStateString() string {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	return c.cbState.String()
}

// ─────────────────────────────────────────────────────────────
// Circuit Breaker / 熔断器
// ─────────────────────────────────────────────────────────────

// checkCircuit returns an error if the circuit is open.
func (c *Client) checkCircuit() error {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()

	switch c.cbState {
	case CircuitClosed:
		return nil
	case CircuitOpen:
		// Check if cooldown has elapsed → transition to half-open
		if time.Since(c.cbOpenedAt) >= c.cooldownDuration() {
			c.cbState = CircuitHalfOpen
			c.logger.Info("circuit breaker half-open, probing recovery")
			return nil
		}
		return fmt.Errorf("circuit breaker open (cooldown remaining)")
	case CircuitHalfOpen:
		// Allow one probe request through
		return nil
	default:
		return nil
	}
}

func (c *Client) cooldownDuration() time.Duration {
	return c.cbCooldown
}

// recordSuccess records a successful call, resetting consecutive failure counter
// and closing the circuit if half-open.
func (c *Client) recordSuccess() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()

	c.cbFailures = 0
	if c.cbState == CircuitHalfOpen {
		c.cbState = CircuitClosed
		c.logger.Info("circuit breaker closed (recovery successful)")
	}
}

// recordFailure records a failed call, potentially opening the circuit.
func (c *Client) recordFailure() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()

	c.cbFailures++
	switch c.cbState {
	case CircuitClosed:
		if c.cbFailures >= c.cbThreshold {
			c.cbState = CircuitOpen
			c.cbOpenedAt = time.Now()
			c.logger.Warn("circuit breaker opened",
				"consecutive_failures", c.cbFailures,
			)
		}
	case CircuitHalfOpen:
		// Probe failed, re-open
		c.cbState = CircuitOpen
		c.cbOpenedAt = time.Now()
		c.logger.Warn("circuit breaker re-opened (probe failed)")
	}
}
