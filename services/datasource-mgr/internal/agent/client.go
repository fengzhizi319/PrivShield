// Package agent provides an HTTP client to the upstream PrivShield agent.
// Package agent 封装到上游 PrivShield agent 的 HTTP 客户端。
//
// 本模块的 agent 客户端已精简为 thin wrapper，
// 通用 HTTP 逻辑由 console/pkg/agent 共享库提供。
package agent

import (
	"context"

	pkgagent "github.com/fengzhizi319/PrivShield/console/pkg/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
)

// Client wraps the shared agent client with datasource-mgr-specific endpoints.
type Client struct {
	*pkgagent.Client
}

// New creates a new agent client from the given config.
func New(cfg *config.Config) *Client {
	shared := pkgagent.New(pkgagent.Config{
		BaseURLs: cfg.AgentBaseURLs(),
		APIKey:   cfg.AgentAPIKey,
	})
	return &Client{Client: shared}
}

// Classify sends data to the dynamic classification endpoint.
func (c *Client) Classify(ctx context.Context, payload any) (map[string]any, error) {
	return c.Post(ctx, "/v1/dynclassification/classify", payload)
}
