// Package main 提供 L7 自适应负载均衡网关入口。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/gateway"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/observability"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	observability.InitLogger(getEnv("PRIVACY_LOG_LEVEL", "INFO"))

	slog.Info("Starting PrivShield L7 Adaptive Gateway",
		"version", Version,
		"build_time", BuildTime,
	)

	// 解析后端地址
	backends := getEnv("GATEWAY_BACKENDS", "127.0.0.1:8079")
	addresses := strings.Split(backends, ",")

	// 调度策略
	strategy := getEnv("GATEWAY_STRATEGY", "p2c")

	// 创建负载均衡器
	lb := gateway.NewLoadBalancer(addresses, strategy)

	// 创建 Gin 路由
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger())

	// 网关自身健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "component": "gateway"})
	})

	// 后端状态查询
	r.GET("/gateway/backends", gateway.NewHealthCheckHandler(lb))

	// 反向代理：所有未匹配路由转发给后端
	r.NoRoute(gateway.NewHTTPProxyHandler(lb))

	// 启动 HTTP 服务器
	addr := fmt.Sprintf("%s:%s", getEnv("GATEWAY_HOST", "0.0.0.0"), getEnv("GATEWAY_PORT", "8000"))
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Gateway HTTP Proxy listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Gateway server error", "err", err)
			os.Exit(1)
		}
	}()

	slog.Info("Configuration summary",
		"listen_addr", addr,
		"strategy", strategy,
		"backends", addresses,
	)

	// 优雅停机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Shutdown signal received", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Gateway shutdown error", "err", err)
	}
	slog.Info("Gateway stopped gracefully")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
