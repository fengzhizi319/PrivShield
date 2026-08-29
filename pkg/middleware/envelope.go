// Package middleware — unified API error envelope for cross-language consistency.
// Package middleware — 跨语言统一 API 错误信封。
//
// 所有 Go 微服务（service-hub / datasource-mgr / audit-log / bff-go / app-lz）
// 与 Python 引擎（engine/observability/envelope.py）共享同一错误响应格式：
//
//	{
//	  "code":      "INVALID_ARGUMENT",      // 机器可读错误码枚举
//	  "message":   "请求参数校验失败",        // 人读摘要
//	  "detail":    "...",                   // 兼容原 detail 字段
//	  "trace_id":  "req-1787554500-abc123", // 分布式追踪 ID
//	  "timestamp": "2026-08-27T09:30:00Z"   // UTC 时间戳
//	}
//
// 迁移过渡期双轨兼容：响应体同时包含 code / message / detail，
// 响应头强制下发 X-Request-ID 与 X-Trace-ID。
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ErrorEnvelope is the unified error response body structure.
// ErrorEnvelope 统一错误响应体结构。
//
// All Go services and the Python FastAPI engine produce errors in this format,
// enabling the frontend to use a single interceptor for error parsing.
// 所有 Go 服务与 Python FastAPI 引擎均按此格式产出错误响应，
// 使前端可使用单一拦截器统一解析。
type ErrorEnvelope struct {
	Code      string `json:"code"`             // Machine-readable error code / 机器可读错误码
	Message   string `json:"message"`          // Human-readable summary / 人读摘要
	Detail    any    `json:"detail,omitempty"` // Detailed error info (optional) / 详细错误信息
	TraceID   string `json:"trace_id"`         // Distributed trace ID / 分布式追踪 ID
	Timestamp string `json:"timestamp"`        // UTC timestamp (RFC3339) / UTC 时间戳
}

// AbortWithError aborts the request and responds with a unified error envelope.
// AbortWithError 中断请求并以统一错误信封格式响应。
//
// Automatically injects X-Request-ID and X-Trace-ID response headers for
// distributed tracing correlation.
// 自动注入 X-Request-ID 与 X-Trace-ID 响应头，用于分布式追踪关联。
//
// Parameters / 参数：
//   - c: Gin context / Gin 上下文
//   - httpStatus: HTTP status code / HTTP 状态码
//   - code: Machine-readable error code (e.g. "INVALID_ARGUMENT") / 机器可读错误码
//   - message: Human-readable error summary / 人读错误摘要
//   - detail: Optional detailed error info / 可选的详细错误信息
func AbortWithError(c *gin.Context, httpStatus int, code string, message string, detail any) {
	traceID := GetTraceID(c)
	c.Header("X-Request-ID", traceID)
	c.Header("X-Trace-ID", traceID)
	c.AbortWithStatusJSON(httpStatus, ErrorEnvelope{
		Code:      code,
		Message:   message,
		Detail:    detail,
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// SuccessEnvelope is the unified success response body structure (optional).
// SuccessEnvelope 统一成功响应体结构（可选使用）。
//
// For gradual migration: success responses can optionally wrap data in this
// envelope to maintain format consistency with error responses.
// 渐进迁移用：成功响应可选择性使用此信封包裹数据，保持与错误响应的格式一致性。
type SuccessEnvelope struct {
	Code      string `json:"code"`           // "OK" / 固定为 "OK"
	Message   string `json:"message"`        // Human-readable message / 人读消息
	Data      any    `json:"data,omitempty"` // Response payload / 响应数据
	TraceID   string `json:"trace_id"`       // Distributed trace ID / 分布式追踪 ID
	Timestamp string `json:"timestamp"`      // UTC timestamp / UTC 时间戳
}

// RespondWithSuccess responds with a unified success envelope.
// RespondWithSuccess 以统一成功信封格式响应。
func RespondWithSuccess(c *gin.Context, httpStatus int, message string, data any) {
	traceID := GetTraceID(c)
	c.Header("X-Request-ID", traceID)
	c.Header("X-Trace-ID", traceID)
	c.JSON(httpStatus, SuccessEnvelope{
		Code:      "OK",
		Message:   message,
		Data:      data,
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// ErrorCodeFromStatus maps HTTP status codes to standard error code strings.
// ErrorCodeFromStatus 将 HTTP 状态码映射为标准错误码字串。
//
// Used by both Go services and as reference for the Python engine's code_map.
// Go 服务与 Python 引擎的 code_map 共享同一映射逻辑。
func ErrorCodeFromStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusInternalServerError:
		return "INTERNAL_ERROR"
	case http.StatusServiceUnavailable:
		return "UPSTREAM_UNAVAILABLE"
	default:
		return "UNKNOWN_ERROR"
	}
}

// ExtractErrorMessage extracts the best error message from various response formats.
// ExtractErrorMessage 从多种响应格式中提取最佳错误消息。
//
// Supports: unified envelope (code+message), gin.H (detail/error), and fallback.
// 支持：统一信封（code+message）、gin.H（detail/error）及回退。
func ExtractErrorMessage(c *gin.Context, fallback string) string {
	// Try unified envelope format / 尝试统一信封格式
	if code, exists := c.Get("error_code"); exists {
		if s, ok := code.(string); ok && s != "" {
			return s
		}
	}
	// Try common error fields / 尝试常见错误字段
	for _, key := range []string{"message", "detail", "error"} {
		if val, exists := c.Get(key); exists {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	// Fallback to HTTP status text / 回退到 HTTP 状态文本
	if fallback != "" {
		return fallback
	}
	return http.StatusText(c.Writer.Status())
}
