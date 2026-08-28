// Package main 提供 L7 自适应负载均衡网关入口。
//
// 双协议代理：
//   - HTTP 反向代理：REST API 流量 → :8000
//   - gRPC 透明流代理：gRPC 流量 → :50000
//
// 环境变量：
//   - GATEWAY_BACKENDS：后端 Agent 地址（逗号分隔）
//   - GATEWAY_STRATEGY：调度策略（p2c/round_robin/least_conn）
//   - GATEWAY_HOST / GATEWAY_PORT：HTTP 监听地址
//   - GATEWAY_GRPC_PORT：gRPC 监听端口
//   - PRIVACY_LOG_LEVEL：日志级别
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

	// 初始化网关 Prometheus 指标（设计文档 §11.1）
	gwMetrics := observability.NewGatewayMetrics()

	// ── HTTP 反向代理 ──
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger())
	r.Use(gwMetrics.PrometheusMiddleware()) // 网关转发指标

	// 网关自身健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "component": "gateway"})
	})

	// 后端状态查询
	r.GET("/gateway/backends", gateway.NewHealthCheckHandler(lb))

	// Prometheus /metrics 端点（设计文档 §11.1）
	r.GET("/metrics", gwMetrics.Handler())

	// 反向代理：所有未匹配路由转发给后端（传入 metrics 实时上报 Prometheus 指标）
	r.NoRoute(gateway.NewHTTPProxyHandler(lb, gwMetrics))

	// 启动 HTTP 服务器
	httpAddr := fmt.Sprintf("%s:%s", getEnv("GATEWAY_HOST", "0.0.0.0"), getEnv("GATEWAY_PORT", "8000"))
	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Gateway HTTP Proxy listening", "addr", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Gateway HTTP server error", "err", err)
			os.Exit(1)
		}
	}()

	// ── gRPC 透明流代理 ──
	grpcPort := getEnv("GATEWAY_GRPC_PORT", "50000")
	grpcAddr := fmt.Sprintf("0.0.0.0:%s", grpcPort)

	grpcProxyServer, grpcLis, err := gateway.NewGrpcProxyListener(lb, grpcAddr, gwMetrics)
	if err != nil {
		slog.Error("gRPC proxy listener failed", "err", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Gateway gRPC Transparent Proxy listening", "addr", grpcAddr)
		if err := grpcProxyServer.Serve(grpcLis); err != nil {
			slog.Error("Gateway gRPC server error", "err", err)
		}
	}()

	// ── 启动配置摘要 ──
	slog.Info("Configuration summary",
		"http_addr", httpAddr,
		"grpc_addr", grpcAddr,
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
		slog.Error("Gateway HTTP shutdown error", "err", err)
	}
	grpcProxyServer.GracefulStop()

	slog.Info("Gateway stopped gracefully")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
