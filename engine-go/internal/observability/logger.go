// Package observability 提供可观测性基础设施。
//
// 包含结构化日志初始化、Prometheus 指标中间件、请求日志中间件。
package observability

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────
// 日志初始化
// ──────────────────────────────────────────────

// InitLogger 初始化结构化日志
func InitLogger(level string) {
	var logLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

// ──────────────────────────────────────────────
// Prometheus 指标中间件已迁移至 metrics.go
// 使用 EngineMetrics.PrometheusMiddleware() 替代
// ──────────────────────────────────────────────

// ──────────────────────────────────────────────
// 请求日志中间件
// ──────────────────────────────────────────────

// RequestLogger 记录 HTTP 请求日志
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// 生成请求 ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = c.GetHeader("x-request-id")
		}

		slog.Info("HTTP request",
			"method", method,
			"path", path,
			"query", query,
			"status", status,
			"duration", duration,
			"client_ip", clientIP,
			"request_id", requestID,
		)
	}
}

// ──────────────────────────────────────────────
// 健康检查处理器
// ──────────────────────────────────────────────

// HealthHandler 返回健康检查处理器
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}
}
