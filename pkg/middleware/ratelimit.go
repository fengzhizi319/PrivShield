package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// MaxBodySize returns a middleware that limits the maximum allowed request body size.
// MaxBodySize 返回一个限制最大请求体字节数的 Gin 中间件，防止大包拒绝服务攻击（Payload DDoS）。
//
// 当请求体大小超过 maxBytes 时，读取请求体会返回错误并向客户端响应 413 Payload Too Large。
func MaxBodySize(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = 32 << 20 // 默认 32 MiB
	}
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// MaxConcurrent returns a middleware that caps the total in-flight requests on the server.
// MaxConcurrent 返回限制服务器最大并发处理请求数的中间件，防止突发流量耗尽系统线程/内存资源（Concurrency DDoS）。
//
// 当并发请求超过 limit 时，立即快速失败返回 503 Service Unavailable，并提示客户端重试。
func MaxConcurrent(limit int) gin.HandlerFunc {
	if limit <= 0 {
		limit = 1000 // 默认最多 1000 并发
	}
	sem := make(chan struct{}, limit)

	return func(c *gin.Context) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			c.Next()
		default:
			AbortWithError(c, http.StatusServiceUnavailable,
				"UPSTREAM_UNAVAILABLE",
				"Server is overloaded: concurrent request limit reached, please retry later",
				nil,
			)
		}
	}
}

// ipBucket tracks token bucket rate limit for an individual IP.
type ipBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter is an in-memory per-IP token-bucket rate limiter with automatic stale IP cleanup.
type IPRateLimiter struct {
	mu      sync.Mutex
	rps     float64
	burst   float64
	buckets map[string]*ipBucket
	stopCh  chan struct{}
}

// NewIPRateLimiter creates a new IPRateLimiter with background garbage collection.
func NewIPRateLimiter(rps int, burst int) *IPRateLimiter {
	if rps <= 0 {
		rps = 100
	}
	if burst <= 0 {
		burst = rps * 2
	}

	limiter := &IPRateLimiter{
		rps:     float64(rps),
		burst:   float64(burst),
		buckets: make(map[string]*ipBucket),
		stopCh:  make(chan struct{}),
	}

	// 启动后台定时器清理 10 分钟未活动的 IP 桶，防止内存泄露
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				limiter.cleanup(10 * time.Minute)
			case <-limiter.stopCh:
				return
			}
		}
	}()

	return limiter
}

// Close stops the background cleanup goroutine.
func (l *IPRateLimiter) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-l.stopCh:
	default:
		close(l.stopCh)
	}
}

func (l *IPRateLimiter) cleanup(maxIdle time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for ip, b := range l.buckets {
		if now.Sub(b.lastRefill) > maxIdle {
			delete(l.buckets, ip)
		}
	}
}

// Allow checks whether a request from the given IP is permitted under the rate limit.
func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[ip]
	if !exists {
		l.buckets[ip] = &ipBucket{
			tokens:     l.burst - 1.0,
			lastRefill: now,
		}
		return true
	}

	// 计算自上次请求以来新生成的令牌数
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.rps
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// RateLimit returns a Gin middleware that enforces per-client-IP token bucket rate limiting.
// RateLimit 返回一个基于客户端 IP 令牌桶算法的限流中间件，抵御针对单 IP 或分布式代理的 L7 HTTP Flood DDoS 攻击。
//
// 豁免路径：/health 与 /api/health。
func RateLimit(rps int, burst int) gin.HandlerFunc {
	limiter := NewIPRateLimiter(rps, burst)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/api/health" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		if !limiter.Allow(clientIP) {
			c.Writer.Header().Set("Retry-After", "1")
			c.Writer.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rps))
			AbortWithError(c, http.StatusTooManyRequests,
				"RATE_LIMITED",
				"Too Many Requests: rate limit exceeded, please retry later",
				nil,
			)
			return
		}

		c.Next()
	}
}
