package security

import (
	"crypto/subtle"
	"net/http"
	"sort"
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
// 对 key 进行排序以确保确定性迭代顺序（Go map 迭代顺序随机），
// 遍历全部 key 且始终比较所有 key，避免时序侧信道泄漏。
func constantTimeLookup(keys map[string]*KeyConfig, token string) *KeyConfig {
	if len(keys) == 0 {
		return nil
	}
	// 排序 key 确保确定性迭代顺序
	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	tokenBytes := []byte(token)
	var matched *KeyConfig
	for _, key := range sortedKeys {
		// subtle.ConstantTimeCompare 确保每次比较耗时恒定
		if subtle.ConstantTimeCompare([]byte(key), tokenBytes) == 1 {
			matched = keys[key]
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

// ──────────────────────────────────────────────
// 分片高并发 Token Bucket 限流器（带 TTL 自动淘汰）
// ──────────────────────────────────────────────

const numRateLimitShards = 32

type rateLimitBucket struct {
	tokens    float64
	lastCheck time.Time
}

type rateLimitShard struct {
	mu      sync.Mutex
	buckets map[string]*rateLimitBucket
}

type shardedRateLimiter struct {
	shards [numRateLimitShards]*rateLimitShard
}

var globalRateLimiter = newShardedRateLimiter()

// rateLimitDone 用于后台清理 goroutine 的优雅退出
var rateLimitDone = make(chan struct{})

// StopRateLimiter 停止限流器后台清理 goroutine
func StopRateLimiter() {
	close(rateLimitDone)
}

func newShardedRateLimiter() *shardedRateLimiter {
	limiter := &shardedRateLimiter{}
	for i := 0; i < numRateLimitShards; i++ {
		limiter.shards[i] = &rateLimitShard{
			buckets: make(map[string]*rateLimitBucket),
		}
	}
	// 后台协程定期清理超过 10 分钟未活动的 Bucket，杜绝内存膨胀
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				limiter.cleanup(10 * time.Minute)
			case <-rateLimitDone:
				return
			}
		}
	}()
	return limiter
}

func (l *shardedRateLimiter) shardFor(key string) *rateLimitShard {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return l.shards[h%numRateLimitShards]
}

func (l *shardedRateLimiter) allow(key string, rps, burst float64) bool {
	shard := l.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	b, ok := shard.buckets[key]
	if !ok {
		b = &rateLimitBucket{tokens: burst, lastCheck: now}
		shard.buckets[key] = b
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * rps
	if b.tokens > burst {
		b.tokens = burst
	}
	b.lastCheck = now

	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (l *shardedRateLimiter) cleanup(ttl time.Duration) {
	now := time.Now()
	for i := 0; i < numRateLimitShards; i++ {
		shard := l.shards[i]
		shard.mu.Lock()
		for k, b := range shard.buckets {
			if now.Sub(b.lastCheck) > ttl {
				delete(shard.buckets, k)
			}
		}
		shard.mu.Unlock()
	}
}

// RateLimitMiddleware 返回分片并发滑动窗口限流中间件（带 TTL 自动淘汰与内存安全）。
func RateLimitMiddleware() gin.HandlerFunc {
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

		// 匿名调用者追加客户端 IP 作为分片因子，防止单 IP 洪泛攻击
		// 对 path 做前缀归一化，去除动态 ID 段，防止高基数路径导致桶爆炸
		normalizedPath := normalizeRateLimitPath(path)
		key := identity.ServiceType + ":" + identity.Name + ":" + normalizedPath
		if identity.Name == "anonymous" {
			clientIP := c.ClientIP()
			if clientIP != "" {
				key += ":" + clientIP
			}
		}
		rps := settings.RateLimitDefaultRPS
		burst := float64(settings.RateLimitDefaultBurst)

		if !globalRateLimiter.allow(key, rps, burst) {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Rate limit exceeded", "")
			return
		}

		c.Next()
	}
}

// normalizeRateLimitPath 将路径中的动态 ID 段替换为 :id 占位符，防止高基数路径导致限流桶爆炸。
// 识别两类动态段：纯数字（如 123）和 UUID 格式（如 550e8400-e29b-...）。
func normalizeRateLimitPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if isAllDigits(part) || isUUIDFormat(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isUUIDFormat(s string) bool {
	// UUID: 8-4-4-4-12 hex digits
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
