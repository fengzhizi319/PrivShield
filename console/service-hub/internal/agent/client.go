// Package agent provides an HTTP client to the upstream PrivShield agent.
// Package agent 封装到上游 PrivShield agent 的 HTTP 客户端。
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
)

// Client wraps HTTP calls to the upstream PrivShield agent REST API.
// Client 封装对上游 PrivShield agent REST API 的 HTTP 调用。
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new agent client from the given config.
// New 根据配置创建新的 agent 客户端。
func New(cfg *config.Config) *Client {
	return &Client{
		baseURL: cfg.AgentBaseURL(),
		apiKey:  cfg.AgentAPIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Health checks the upstream agent health.
// Health 检查上游 agent 健康状态。
func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	return c.get(ctx, "/health")
}

// Classify sends data to the dynamic classification endpoint.
// Classify 将数据发送到动态分类分级端点。
//
// 真实 Agent 端点: POST /v1/dynclassification/eval_record
// 请求格式: {"record": {"field1": "value1", ...}, "domain": "...", "standard": "..."}
// 返回格式: {"fields": {"field1": {"level": "L3", "category": "PII", ...}, ...}, ...}
func (c *Client) Classify(ctx context.Context, payload any) (map[string]any, error) {
	// Wrap payload into eval_record format: {"record": payload}
	wrapped := map[string]any{
		"record": payload,
	}
	return c.post(ctx, "/v1/dynclassification/eval_record", wrapped)
}

// Mask sends data to the field-level masking endpoint.
// Mask 将数据发送到字段级脱敏端点。
//
// 真实 Agent 端点: POST /v1/privacy/mask
// 请求格式: {"field_name": "name", "value": "张三", "context": ""}
// 返回格式: {"result": "张*"}
func (c *Client) Mask(ctx context.Context, payload any) (map[string]any, error) {
	return c.post(ctx, "/v1/privacy/mask", payload)
}

// MaskRecord sends a full record to the record-level masking endpoint.
// MaskRecord 将整条记录发送到记录级脱敏端点。
//
// 真实 Agent 端点: POST /v1/privacy/mask_record
// 请求格式: {"record": {"name": "张三", "id_card": "110..."}, "context": ""}
// 返回格式: {"result": {"name": "张*", "id_card": "110***"}}
func (c *Client) MaskRecord(ctx context.Context, record map[string]string) (map[string]any, error) {
	payload := map[string]any{
		"record":  record,
		"context": "",
	}
	return c.post(ctx, "/v1/privacy/mask_record", payload)
}

// get performs a GET request to the agent and returns parsed JSON.
func (c *Client) get(ctx context.Context, path string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)
	return c.do(req)
}

// post performs a POST request to the agent and returns parsed JSON.
func (c *Client) post(ctx context.Context, path string, payload any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
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
		return nil, fmt.Errorf("agent request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read agent response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parse agent response: %w", err)
		}
	}
	return result, nil
}
