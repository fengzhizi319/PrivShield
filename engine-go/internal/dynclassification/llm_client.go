// Package dynclassification 提供三层动态分类分级引擎扩展。
//
// llm_client.go — Layer 3 Local LLM / vLLM HTTP 连接池客户端
package dynclassification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ──────────────────────────────────────────────
// LLM 客户端配置
// ──────────────────────────────────────────────

// LLMClientConfig LLM 客户端配置
type LLMClientConfig struct {
	// Endpoint LLM 服务地址（如 http://localhost:8000/v1/chat/completions）
	Endpoint string
	// ModelName 模型名称
	ModelName string
	// MaxConcurrency 最大并发推理数
	MaxConcurrency int
	// Timeout 单次请求超时
	Timeout time.Duration
	// MaxRetries 最大重试次数
	MaxRetries int
	// APIKey API 密钥（可选）
	APIKey string
}

// DefaultLLMClientConfig 默认 LLM 客户端配置
func DefaultLLMClientConfig() LLMClientConfig {
	return LLMClientConfig{
		Endpoint:       "http://localhost:8000/v1/chat/completions",
		ModelName:      "qwen3.5",
		MaxConcurrency: 1,
		Timeout:        30 * time.Second,
		MaxRetries:     2,
	}
}

// ──────────────────────────────────────────────
// LLM 请求/响应
// ──────────────────────────────────────────────

// LLMRequest LLM 推理请求
type LLMRequest struct {
	Field    string `json:"field"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Standard string `json:"standard,omitempty"`
}

// LLMResponse LLM 推理响应
type LLMResponse struct {
	Level      string  `json:"level"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// ──────────────────────────────────────────────
// LLM 连接池客户端
// ──────────────────────────────────────────────

// CircuitState 熔断器状态
type CircuitState int32

const (
	CircuitClosed   CircuitState = 0 // 闭合（正常通行）
	CircuitOpen     CircuitState = 1 // 打开（熔断阻断）
	CircuitHalfOpen CircuitState = 2 // 半开（试探自愈）
)

// LLMClient LLM 连接池客户端（内置三态熔断器与并发控制）
type LLMClient struct {
	config      LLMClientConfig
	client      *http.Client
	sem         chan struct{} // 并发信号量
	cbState     CircuitState
	failures    int
	lastFailure time.Time
	cooldown    time.Duration
	cbMu        sync.RWMutex

	// IsAvailable TTL 缓存，防止高并发下探测风暴
	availCache     atomic.Bool
	availCacheTime atomic.Int64 // Unix nano
	availCacheTTL  time.Duration
	availProbeMu   sync.Mutex // 串行化缓存刷新，防止并发探测风暴

	// Half-Open 状态在途试探请求数，防止刚恢复的 LLM 被瞬时并发流量二次打崩
	halfOpenInflight atomic.Int32
}

// maxHalfOpenProbes Half-Open 状态下允许并发通过的试探请求上限，
// 与 gateway.CircuitBreaker 的 halfOpenMax 保持一致的保护语义。
const maxHalfOpenProbes = 3

// NewLLMClient 创建 LLM 客户端
func NewLLMClient(config LLMClientConfig) *LLMClient {
	return &LLMClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		sem:           make(chan struct{}, config.MaxConcurrency),
		cbState:       CircuitClosed,
		cooldown:      15 * time.Second,
		availCacheTTL: 5 * time.Second,
	}
}

// checkCircuit 检查熔断器状态，返回是否允许通行。
// Half-Open 状态下仅允许最多 maxHalfOpenProbes 个并发试探请求通过，
// 并通过 releaseProbe（幂等）在试探结束时释放配额；超额请求直接拒绝走 Safety Floor 降级。
func (c *LLMClient) checkCircuit() (allowed bool, releaseProbe func()) {
	c.cbMu.RLock()
	state := c.cbState
	lastFail := c.lastFailure
	cooldown := c.cooldown
	c.cbMu.RUnlock()

	if state == CircuitClosed {
		return true, nil
	}

	if state == CircuitOpen {
		if time.Since(lastFail) > cooldown {
			c.cbMu.Lock()
			if c.cbState == CircuitOpen && time.Since(c.lastFailure) > c.cooldown {
				c.cbState = CircuitHalfOpen
				// 进入 Half-Open 重置配额，本请求自身占位成为第一个试探请求
				c.halfOpenInflight.Store(1)
				c.cbMu.Unlock()
				var once sync.Once
				return true, func() { once.Do(func() { c.halfOpenInflight.Add(-1) }) }
			}
			c.cbMu.Unlock()
		}
		return false, nil
	}

	// Half-Open: 限制并发试探配额，超额请求拒绝避免二次雪崩
	if c.halfOpenInflight.Add(1) > maxHalfOpenProbes {
		c.halfOpenInflight.Add(-1)
		return false, nil
	}
	var once sync.Once
	return true, func() { once.Do(func() { c.halfOpenInflight.Add(-1) }) }
}

// recordSuccess 记录一次成功调用，自愈重置熔断器
func (c *LLMClient) recordSuccess() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.failures = 0
	c.cbState = CircuitClosed
}

// recordFailure 记录一次失败调用，连续超阈值触发熔断
func (c *LLMClient) recordFailure() {
	c.cbMu.Lock()
	defer c.cbMu.Unlock()
	c.failures++
	c.lastFailure = time.Now()
	if c.failures >= 3 {
		c.cbState = CircuitOpen
	}
}

// Classify 使用 LLM 对字段执行分类（带熔断保护与重试）
func (c *LLMClient) Classify(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	// 熔断器快速拦截（Half-Open 下限制并发试探配额）
	allowed, releaseProbe := c.checkCircuit()
	if !allowed {
		return nil, fmt.Errorf("LLM circuit breaker is OPEN (cooldown active), request rejected")
	}
	if releaseProbe != nil {
		defer releaseProbe()
	}

	// 获取并发槽位
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 构建 prompt
	prompt := c.buildPrompt(req)

	// 调用 LLM
	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// 检查 context 是否已取消，避免无效重试
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, err := c.callLLM(ctx, prompt)
		if err == nil {
			c.recordSuccess()
			return resp, nil
		}
		lastErr = err
		if attempt < c.config.MaxRetries {
			select {
			case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	c.recordFailure()
	return nil, fmt.Errorf("LLM classification failed after %d retries: %w", c.config.MaxRetries+1, lastErr)
}

// buildPrompt 构建分类 prompt
func (c *LLMClient) buildPrompt(req LLMRequest) string {
	return fmt.Sprintf(`你是一个数据安全分类专家。请对以下字段进行分类。

字段名: %s
字段值: %s
领域: %s

请返回 JSON 格式:
{"level": "public|internal|confidential|secret|top_secret", "category": "类别", "confidence": 0.0-1.0}

只返回 JSON，不要其他内容。`, req.Field, req.Value, req.Domain)
}

// callLLM 调用 LLM API
func (c *LLMClient) callLLM(ctx context.Context, prompt string) (*LLMResponse, error) {
	body := map[string]interface{}{
		"model": c.config.ModelName,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个数据安全分类专家，只返回 JSON。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
		"max_tokens":  256,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 限制错误响应体最大 1MB
		return nil, fmt.Errorf("LLM API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty LLM response")
	}

	// 解析 LLM 返回的 JSON
	content := result.Choices[0].Message.Content
	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w (content: %s)", err, content)
	}

	return &llmResp, nil
}

// IsAvailable 检查 LLM 服务是否可用（带 TTL 缓存 + singleflight 串行化探测）。
// 缓存有效期 5 秒，过期后仅一个 goroutine 执行实际探测，其余等待复用结果。
func (c *LLMClient) IsAvailable(ctx context.Context) bool {
	// 快速路径：缓存有效时直接返回
	cachedTime := time.Unix(0, c.availCacheTime.Load())
	if time.Since(cachedTime) < c.availCacheTTL {
		return c.availCache.Load()
	}

	// 慢路径：串行化探测，只有第一个 goroutine 执行 HTTP 请求
	c.availProbeMu.Lock()
	defer c.availProbeMu.Unlock()

	// 双重检查：可能在等锁期间已被其他 goroutine 刷新
	cachedTime = time.Unix(0, c.availCacheTime.Load())
	if time.Since(cachedTime) < c.availCacheTTL {
		return c.availCache.Load()
	}

	// 实际探测（使用 HEAD 请求避免对 POST 端点产生副作用）
	req, err := http.NewRequestWithContext(ctx, "HEAD", c.config.Endpoint, nil)
	if err != nil {
		c.availCache.Store(false)
		c.availCacheTime.Store(time.Now().UnixNano())
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.availCache.Store(false)
		c.availCacheTime.Store(time.Now().UnixNano())
		return false
	}
	resp.Body.Close()
	available := resp.StatusCode < 500
	c.availCache.Store(available)
	c.availCacheTime.Store(time.Now().UnixNano())
	return available
}

// Close 关闭客户端
func (c *LLMClient) Close() {
	c.client.CloseIdleConnections()
}
