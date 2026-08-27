// Package middleware — full-chain distributed tracing middleware.
// Package middleware — 全链路分布式追踪中间件。
//
// TraceMiddleware 替代 RequestID()，在保留 X-Request-ID 兼容性的基础上
// 额外注入 X-Trace-ID 响应头，确保追踪上下文在 HTTP REST → Go 内部调度 →
// gRPC 跨机调用 → 异步 Goroutine 消费的全生命周期中保持单调且不丢失。
//
// 前端 React UI 生成的 X-Request-ID 经 BFF/Hub 层注入 Context，
// 通过 gRPC Metadata (x-request-id / x-trace-id) 透传至 Engine / Audit 层。
package middleware

import (
	"github.com/gin-gonic/gin"

	pkgagent "github.com/fengzhizi319/PrivShield/pkg/agent"
)

const (
	// TraceIDContextKey is the gin.Context key for storing the trace ID.
	// TraceIDContextKey 是在 gin.Context 中存储追踪 ID 的键名。
	TraceIDContextKey = "PrivShield-Trace-ID"

	// TraceHeader is the primary HTTP header for trace propagation.
	// TraceHeader 是追踪上下文传播的主 HTTP 头。
	TraceHeader = "X-Request-ID"

	// TraceIDHeader is the secondary HTTP header for trace propagation.
	// TraceIDHeader 是追踪上下文传播的辅助 HTTP 头。
	TraceIDHeader = "X-Trace-ID"
)

// TraceMiddleware returns a Gin middleware that propagates distributed tracing context.
// TraceMiddleware 返回传播分布式追踪上下文的 Gin 中间件。
//
// Behavior / 行为：
//  1. Reuses existing request_id from RequestID() middleware if present (backward compatible)
//     若 RequestID() 中间件已设置 request_id，则复用（向后兼容）
//  2. Falls back to X-Request-ID inbound header
//     回退到入站 X-Request-ID 头
//  3. Generates a new ID if neither exists
//     若两者均不存在则生成新 ID
//  4. Sets both X-Request-ID and X-Trace-ID response headers
//     同时设置 X-Request-ID 与 X-Trace-ID 响应头
//  5. Injects trace ID into request context for downstream HTTP client propagation
//     将追踪 ID 注入 request context，使下游 HTTP 客户端自动传播
//
// Migration / 迁移：
//
//	Replace middleware.RequestID() with middleware.TraceMiddleware() in all
//	Go services. The "request_id" context key remains compatible.
//	在所有 Go 服务中将 middleware.RequestID() 替换为 middleware.TraceMiddleware()。
//	"request_id" 上下文键保持兼容。
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var traceID string

		// 1. Reuse existing request_id from RequestID() middleware if present
		// 若 RequestID() 中间件已设置 request_id，则复用（向后兼容）
		if rid, exists := c.Get("request_id"); exists {
			if s, ok := rid.(string); ok && s != "" {
				traceID = s
			}
		}

		// 2. Fall back to inbound header
		// 回退到入站头
		if traceID == "" {
			traceID = c.GetHeader(TraceHeader)
		}

		// 3. Generate if still empty
		// 若仍为空则生成
		if traceID == "" {
			traceID = generateRequestID()
		}

		// 4. Store in context under both keys for backward compatibility
		// 以两个键名存储于上下文中，保持向后兼容
		c.Set("request_id", traceID)
		c.Set(TraceIDContextKey, traceID)

		// 5. Set response headers
		// 设置响应头
		c.Header(TraceHeader, traceID)
		c.Header(TraceIDHeader, traceID)

		// 6. Inject into request context for downstream HTTP client propagation
		// 注入到 request context，使下游 HTTP 客户端（如 pkg/agent）自动传播
		ctx := pkgagent.ContextWithRequestID(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// GetTraceID retrieves the trace ID from gin.Context.
// GetTraceID 从 gin.Context 中获取追踪 ID。
//
// Lookup order / 查找顺序：
//  1. TraceIDContextKey (set by TraceMiddleware)
//  2. "request_id" key (set by RequestID or TraceMiddleware)
//  3. X-Request-ID inbound header
//  4. Generate new ID if none found
func GetTraceID(c *gin.Context) string {
	// 1. Check dedicated trace key / 检查专用追踪键
	if val, ok := c.Get(TraceIDContextKey); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	// 2. Fall back to request_id key (backward compat) / 回退到 request_id 键
	if val, ok := c.Get("request_id"); ok {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	// 3. Fall back to header / 回退到头
	if rid := c.GetHeader(TraceHeader); rid != "" {
		return rid
	}
	// 4. Generate new / 生成新 ID
	return generateRequestID()
}
