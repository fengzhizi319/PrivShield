// Package agent provides an HTTP client to the upstream PrivShield agent.
// Package agent 封装到上游 PrivShield agent 的 HTTP 客户端。
//
// 本模块的 agent 客户端已精简为 thin wrapper，
// 通用 HTTP 逻辑（GET/POST/Health/熔断器）由 console/pkg/agent 共享库提供。
package agent

import (
	"context"

	pkgagent "github.com/fengzhizi319/PrivShield/console/pkg/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
)

// Client wraps the shared agent client with service-hub-specific endpoints.
// Client 在共享 agent 客户端基础上封装 service-hub 特有的端点调用。
type Client struct {
	*pkgagent.Client
}

// New creates a new agent client from the given config.
// New 根据配置创建新的 agent 客户端。
func New(cfg *config.Config) *Client {
	shared := pkgagent.New(pkgagent.Config{
		BaseURLs: cfg.AgentBaseURLs(),
		APIKey:   cfg.AgentAPIKey,
	})
	return &Client{Client: shared}
}

// Classify sends data to the dynamic classification endpoint.
// Classify 将数据发送到动态分类分级端点。
//
// 真实 Agent 端点: POST /v1/dynclassification/eval_record
// 请求格式: {"record": {"field1": "value1", ...}, "domain": "...", "standard": "..."}
// 返回格式: {"fields": {"field1": {"level": "L3", "category": "PII", ...}, ...}, ...}
func (c *Client) Classify(ctx context.Context, payload any) (map[string]any, error) {
	wrapped := map[string]any{
		"record": payload,
	}
	return c.Post(ctx, "/v1/dynclassification/eval_record", wrapped)
}

// Mask sends data to the field-level masking endpoint.
// Mask 将数据发送到字段级脱敏端点。
//
// 真实 Agent 端点: POST /v1/privacy/mask
func (c *Client) Mask(ctx context.Context, payload any) (map[string]any, error) {
	return c.Post(ctx, "/v1/privacy/mask", payload)
}

// MaskRecord sends a full record to the record-level masking endpoint.
// MaskRecord 将整条记录发送到记录级脱敏端点。
//
// 真实 Agent 端点: POST /v1/privacy/mask_record
func (c *Client) MaskRecord(ctx context.Context, record map[string]string) (map[string]any, error) {
	payload := map[string]any{
		"record":  record,
		"context": "",
	}
	return c.Post(ctx, "/v1/privacy/mask_record", payload)
}
