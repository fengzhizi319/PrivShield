// Package middleware provides shared Gin middleware for console Go modules.
// Package middleware 为控制台各 Go 模块提供共享的 Gin 中间件。
//
// 包含：可配置 CORS、API Key 鉴权、请求 ID 注入、结构化访问日志。
// 三个模块（service-hub / datasource-mgr / audit-log）原先各自维护近乎相同的
// corsMiddleware 实现，现统一抽取至本包。
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORS returns a CORS middleware that allows requests from the specified origins.
// CORS 返回可配置来源的 CORS 中间件。
//
// Security / 安全说明：
//   - origins 为空或仅含 "*" → 允许任意来源（开发模式兼容）
//   - origins 非空 → 精确匹配 Origin 头，不匹配则不设置 CORS 头
//   - 生产部署时应显式配置允许的来源列表，避免使用 "*"
//
// 本控制台为内部工具，不依赖 cookie/凭证，故 Allow-Origin: * 不携带
// Allow-Credentials，避免"任意来源 + 凭证"组合的跨域凭证泄露风险。
func CORS(origins []string) gin.HandlerFunc {
	allowAll := len(origins) == 0 || (len(origins) == 1 && origins[0] == "*")
	originSet := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		originSet[strings.TrimRight(o, "/")] = struct{}{}
	}

	return func(c *gin.Context) {
		reqOrigin := c.GetHeader("Origin")

		if allowAll {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else if reqOrigin != "" {
			normalized := strings.TrimRight(reqOrigin, "/")
			if _, ok := originSet[normalized]; ok {
				c.Writer.Header().Set("Access-Control-Allow-Origin", reqOrigin)
				c.Writer.Header().Set("Vary", "Origin")
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequestID returns a middleware that extracts or generates a unique request ID.
// RequestID 返回一个中间件，提取或生成唯一请求 ID。
//
// 逻辑：
//  1. 读取入站 X-Request-ID 头（上游网关/负载均衡器可能已注入）
//  2. 不存在则生成 req-<timestamp>-<random> 格式 ID
//  3. 写入 gin.Context（Key: "request_id"）供后续 handler/日志使用
//  4. 写入响应头 X-Request-ID 供客户端关联
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = generateRequestID()
		}
		c.Set("request_id", rid)
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	}
}

// StructuredLogger returns a Gin middleware that logs each request in structured JSON.
// StructuredLogger 返回以结构化 JSON 格式记录每个请求的 Gin 中间件。
//
// 输出字段：time / level / msg / request_id / method / path / status / latency_ms / module
// 使用 Go 标准 log/slog 包，零额外依赖。
func StructuredLogger(logger *slog.Logger, module string) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			path = path + "?" + c.Request.URL.RawQuery
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		rid, _ := c.Get("request_id")
		requestID, _ := rid.(string)

		logger.Info("request completed",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
			"module", module,
		)
	}
}

// Recovery returns a Gin middleware that recovers from panics and logs structured errors.
// Recovery 返回一个 Gin 中间件，捕获 panic 并记录结构化日志与返回 500 JSON。
func Recovery(logger *slog.Logger, module string) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				rid, _ := c.Get("request_id")
				requestID, _ := rid.(string)
				logger.Error("panic recovered in handler",
					"request_id", requestID,
					"module", module,
					"panic", fmt.Sprintf("%v", r),
					"path", c.Request.URL.Path,
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"detail":     fmt.Sprintf("Internal Server Error: %v", r),
					"request_id": requestID,
				})
			}
		}()
		c.Next()
	}
}

// SecurityHeaders returns a middleware that sets recommended HTTP security response headers.
// SecurityHeaders 返回一个设置推荐 HTTP 安全响应头的中间件。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// generateRequestID generates a unique request ID using crypto/rand.
// P25 fix: replaces predictable timestamp-based ID with cryptographic random suffix.
// Format: req-<unix_seconds>-<8_random_hex_chars>
func generateRequestID() string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return "req-" + strings.Replace(
		time.Unix(0, time.Now().UnixNano()).Format("20060102150405.000000000"),
		".", "-", 1,
	) + "-" + hex.EncodeToString(buf[:])
}
