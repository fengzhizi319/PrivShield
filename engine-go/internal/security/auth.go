package security

import (
	"crypto/hmac"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/pkg/middleware"
)

// contextKey 用于在 gin.Context 中存储认证身份。
const IdentityContextKey = "security_identity"

// extractBearerToken 从 Authorization header 提取 Bearer token。
func extractBearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}

// constantTimeLookup 常量时间查找 token，防止计时攻击。
func constantTimeLookup(keys map[string]*KeyConfig, token string) *KeyConfig {
	tokenBytes := []byte(token)
	var matched *KeyConfig
	for key, value := range keys {
		if hmac.Equal([]byte(key), tokenBytes) {
			matched = value
		}
	}
	return matched
}

// authenticateAPIKey 在内部和外部 key 存储中查找 token。
func authenticateAPIKey(settings *Settings, token string) *Identity {
	if internal := constantTimeLookup(settings.InternalKeys, token); internal != nil {
		return &Identity{ServiceType: "internal", Name: internal.Name, Scopes: internal.Scopes}
	}
	if external := constantTimeLookup(settings.ExternalKeys, token); external != nil {
		return &Identity{ServiceType: "external", Name: external.Name, Scopes: external.Scopes}
	}
	return nil
}

// AuthMiddleware 返回 Gin 中间件，执行 API Key 认证。
// 认证未启用时透传并注入匿名身份。
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		settings := GetSettings()
		path := c.Request.URL.Path

		// 健康端点豁免
		if IsHealthPathOrMethod(path) && settings.HealthNoAuth {
			c.Set(IdentityContextKey, &Identity{ServiceType: "internal", Name: "health-probe", Scopes: []string{"*"}})
			c.Next()
			return
		}

		if !settings.AuthEnabled {
			c.Set(IdentityContextKey, AnonymousIdentity)
			c.Next()
			return
		}

		token := extractBearerToken(c.GetHeader("Authorization"))
		if token == "" {
			middleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Unauthorized: missing credentials", "")
			return
		}

		identity := authenticateAPIKey(settings, token)
		if identity == nil {
			middleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Unauthorized: invalid credentials", "")
			return
		}

		// 接口级权限校验 (PermissionForRESTPath)
		requiredPerm := PermissionForRESTPath(path)
		if requiredPerm != "*" && !identity.HasPermission(requiredPerm) {
			middleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Forbidden: insufficient scope", "")
			return
		}

		c.Set(IdentityContextKey, identity)
		c.Next()
	}
}

// RequirePermission 返回需要指定权限的 Gin 中间件。
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := GetIdentity(c)
		if identity == nil {
			middleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHENTICATED", "No identity in context", "")
			return
		}
		if !identity.HasPermission(permission) {
			middleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Forbidden: insufficient scope", "")
			return
		}
		c.Next()
	}
}

// RequireAnyPermission 返回需要任一指定权限的 Gin 中间件。
func RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := GetIdentity(c)
		if identity == nil {
			middleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHENTICATED", "No identity in context", "")
			return
		}
		for _, p := range permissions {
			if identity.HasPermission(p) {
				c.Next()
				return
			}
		}
		middleware.AbortWithError(c, http.StatusForbidden, "FORBIDDEN", "Forbidden: insufficient scope", "")
	}
}

// GetIdentity 从 gin.Context 提取认证身份。
func GetIdentity(c *gin.Context) *Identity {
	v, exists := c.Get(IdentityContextKey)
	if !exists {
		return nil
	}
	id, ok := v.(*Identity)
	if !ok {
		return nil
	}
	return id
}

// SecurityHeadersMiddleware 注入安全响应头。
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}

// RateLimitMiddleware 返回简单的滑动窗口限流中间件（基于内存）。
func RateLimitMiddleware() gin.HandlerFunc {
	// 简化实现：使用 token bucket per identity+path
	type bucket struct {
		tokens    float64
		lastCheck time.Time
	}
	var (
		buckets   = make(map[string]*bucket)
		bucketsMu sync.Mutex
	)

	return func(c *gin.Context) {
		settings := GetSettings()
		if !settings.RateLimitEnabled {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		if IsHealthPathOrMethod(path) && settings.HealthNoRateLimit {
			c.Next()
			return
		}

		identity := GetIdentity(c)
		if identity == nil {
			identity = &Identity{ServiceType: "external", Name: "anonymous"}
		}

		key := identity.ServiceType + ":" + identity.Name + ":" + path
		rps := settings.RateLimitDefaultRPS
		burst := float64(settings.RateLimitDefaultBurst)

		bucketsMu.Lock()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{tokens: burst, lastCheck: time.Now()}
			buckets[key] = b
		}
		elapsed := time.Since(b.lastCheck).Seconds()
		b.tokens += elapsed * rps
		if b.tokens > burst {
			b.tokens = burst
		}
		b.lastCheck = time.Now()

		if b.tokens < 1 {
			bucketsMu.Unlock()
			middleware.AbortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Rate limit exceeded", "")
			return
		}
		b.tokens--
		bucketsMu.Unlock()

		c.Next()
	}
}
