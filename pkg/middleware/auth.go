package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Auth returns an optional API Key authentication middleware.
// Auth 返回可选的 API Key 鉴权中间件。
//
// Behavior / 行为：
//   - apiKey 为空 → 中间件直接放行（开发模式，无需鉴权）
//   - apiKey 非空 → /api/* 路径需携带 Authorization: Bearer <key>
//   - /api/health 与 /health 豁免鉴权（健康检查端点始终可访问）
//   - 非 /api 路径（如静态资源、/metrics）豁免
//
// Security / 安全：
//   - 使用 crypto/subtle.ConstantTimeCompare 防止时序攻击
//   - Token 提取遵循 RFC 6750 Bearer 规范
func Auth(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// apiKey 为空 → 跳过鉴权（开发模式）
		if apiKey == "" {
			c.Next()
			return
		}

		path := c.Request.URL.Path

		// 健康检查端点豁免
		if path == "/health" || path == "/api/health" {
			c.Next()
			return
		}

		// 仅对 /api/* 路径生效
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		// 提取 Bearer token
		token := extractBearer(c.GetHeader("Authorization"))
		if token == "" {
			AbortWithError(c, http.StatusUnauthorized,
				"UNAUTHORIZED",
				"Unauthorized: missing or invalid bearer token",
				nil,
			)
			return
		}

		// 常量时间比较，防止时序攻击
		if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			AbortWithError(c, http.StatusUnauthorized,
				"UNAUTHORIZED",
				"Unauthorized: invalid api key",
				nil,
			)
			return
		}

		c.Next()
	}
}

// extractBearer extracts the Bearer token from the Authorization header.
// Returns empty string if the header format is invalid.
// extractBearer 从 Authorization 头提取 Bearer token。
// 格式不符时返回空字符串。
func extractBearer(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}
