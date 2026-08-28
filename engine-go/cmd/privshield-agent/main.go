// Package main 提供 PrivShield Go 引擎的双协议（REST + gRPC）服务端入口。
//
// 架构：
//   - REST API (Gin)：面向外部调用方，端口 8079
//   - gRPC Server：面向内部微服务，端口 50051
//   - 信号处理：SIGINT/SIGTERM 优雅停机
//
// 环境变量：
//   - PRIVACY_REST_HOST / PRIVACY_REST_PORT：REST 监听地址
//   - PRIVACY_GRPC_HOST / PRIVACY_GRPC_PORT：gRPC 监听地址
//   - PRIVACY_LOG_LEVEL：日志级别（DEBUG/INFO/WARN/ERROR）
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/observability"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/rest"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/pkg/tlsutil"
)

// ──────────────────────────────────────────────
// 版本信息（编译时注入）
// ──────────────────────────────────────────────

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// ──────────────────────────────────────────────
// 配置
// ──────────────────────────────────────────────

type Config struct {
	RESTHost    string
	RESTPort    int
	GRPCPort    int
	LogLevel    string
	RateLimitRPS int
	RateLimitBurst int
}

func loadConfig() Config {
	return Config{
		RESTHost:    getEnv("PRIVACY_REST_HOST", "0.0.0.0"),
		RESTPort:    getEnvInt("PRIVACY_REST_PORT", 8079),
		GRPCPort:    getEnvInt("PRIVACY_GRPC_PORT", 50051),
		LogLevel:    getEnv("PRIVACY_LOG_LEVEL", "INFO"),
		RateLimitRPS:   getEnvInt("PRIVACY_RATE_LIMIT_RPS", 1000),
		RateLimitBurst: getEnvInt("PRIVACY_RATE_LIMIT_BURST", 2000),
	}
}

// ──────────────────────────────────────────────
// 主入口
// ──────────────────────────────────────────────

func main() {
	cfg := loadConfig()

	// 初始化日志
	observability.InitLogger(cfg.LogLevel)

	slog.Info("Starting PrivShield Go Engine",
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit,
	)

	// 初始化 PrivacyService 统一编排层
	svcCfg := service.DefaultConfig()
	svc, err := service.NewPrivacyService(svcCfg)
	if err != nil {
		slog.Error("Failed to init PrivacyService", "err", err)
		os.Exit(1)
	}

	// 初始化 Prometheus 指标收集器（设计文档 §11.1）
	engineMetrics := observability.NewEngineMetrics()

	// ── REST API (Gin) ──
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.TraceMiddleware()) // 全链路分布式追踪 (X-Request-ID + X-Trace-ID)

	// 可选限流中间件（设计文档 §12.7 / §13.4）
	if cfg.RateLimitRPS > 0 {
		router.Use(middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))
		slog.Info("Rate limiting enabled", "rps", cfg.RateLimitRPS, "burst", cfg.RateLimitBurst)
	}

	router.Use(observability.RequestLogger())
	router.Use(engineMetrics.PrometheusMiddleware()) // Prometheus 实际指标注册（替代旧 TODO 桩）

	// 注册全部 REST API 路由（17 个端点）
	rest.RegisterRoutes(router, svc)

	// Prometheus /metrics 端点（设计文档 §11.1）
	router.GET("/metrics", engineMetrics.Handler())

	restAddr := fmt.Sprintf("%s:%d", cfg.RESTHost, cfg.RESTPort)
	restServer := &http.Server{
		Addr:         restAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "addr", restAddr)
		if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("REST server error", "err", err)
			os.Exit(1)
		}
	}()

	// ── gRPC Server ──
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", cfg.GRPCPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("gRPC listen failed", "err", err)
		os.Exit(1)
	}

	// mTLS CN 白名单拦截器（设计文档 §13.4）
	var grpcOpts []grpc.ServerOption
	whitelistPath := getEnv("PRIVACY_AUTH_MTLS_WHITELIST_FILE", "")
	if whitelistPath != "" {
		unaryInter, streamInter, _, err := tlsutil.NewWhitelistInterceptor(whitelistPath)
		if err != nil {
			slog.Error("Failed to init mTLS whitelist interceptor", "err", err)
			os.Exit(1)
		}
		if unaryInter != nil {
			grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(unaryInter))
		}
		if streamInter != nil {
			grpcOpts = append(grpcOpts, grpc.StreamInterceptor(streamInter))
		}
		slog.Info("mTLS CN whitelist interceptor enabled", "path", whitelistPath)
	}

	grpcSrv := grpcserver.NewServer(svc, grpcOpts...)
	go func() {
		slog.Info("gRPC server starting", "addr", grpcAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "err", err)
		}
	}()

	// ── 启动配置摘要 ──
	budgetStatus := svc.BudgetStatus()
	slog.Info("Configuration summary",
		"rest_addr", restAddr,
		"grpc_addr", grpcAddr,
		"log_level", cfg.LogLevel,
		"budget_total_epsilon", budgetStatus["total_epsilon"],
		"budget_remaining_epsilon", budgetStatus["remaining_epsilon"],
	)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Shutdown signal received", "signal", sig)

	// 优雅停机
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := restServer.Shutdown(ctx); err != nil {
		slog.Error("REST server shutdown error", "err", err)
	}
	grpcSrv.GracefulStop()

	slog.Info("Server stopped gracefully")
}

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return defaultVal
}
