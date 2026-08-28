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

// LLMClient LLM 连接池客户端
type LLMClient struct {
	config  LLMClientConfig
	client  *http.Client
	sem     chan struct{} // 并发信号量
	mu      sync.Mutex
}

// NewLLMClient 创建 LLM 客户端
func NewLLMClient(config LLMClientConfig) *LLMClient {
	return &LLMClient{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		sem: make(chan struct{}, config.MaxConcurrency),
	}
}

// Classify 使用 LLM 对字段执行分类
func (c *LLMClient) Classify(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
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
		resp, err := c.callLLM(ctx, prompt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < c.config.MaxRetries {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

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
		respBody, _ := io.ReadAll(resp.Body)
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

// IsAvailable 检查 LLM 服务是否可用
func (c *LLMClient) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.Endpoint, nil)
	if err != nil {
		return false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// Close 关闭客户端
func (c *LLMClient) Close() {
	c.client.CloseIdleConnections()
}
