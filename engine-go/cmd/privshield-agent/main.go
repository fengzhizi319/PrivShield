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
//   - PRIVACY_TLS_ENABLED：是否启用 TLS (HTTPS / gRPC TLS)
//   - PRIVACY_TLS_CERT_FILE / PRIVACY_TLS_KEY_FILE / PRIVACY_TLS_CA_FILE：证书路径
//   - PRIVACY_AUTH_INTERNAL_MTLS_ENABLED：是否启用 mTLS 客户端双向认证
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/observability"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/rest"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/security"
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
	RESTHost       string
	RESTPort       int
	GRPCPort       int
	LogLevel       string
	RateLimitRPS   int
	RateLimitBurst int
	TLSEnabled     bool
	TLSCertFile    string
	TLSKeyFile     string
	TLSCAFile      string
	MTLSEnabled    bool
}

func loadConfig() Config {
	return Config{
		RESTHost:       getEnv("PRIVACY_REST_HOST", "0.0.0.0"),
		RESTPort:       getEnvInt("PRIVACY_REST_PORT", 8079),
		GRPCPort:       getEnvInt("PRIVACY_GRPC_PORT", 50051),
		LogLevel:       getEnv("PRIVACY_LOG_LEVEL", "INFO"),
		RateLimitRPS:   getEnvInt("PRIVACY_RATE_LIMIT_RPS", 1000),
		RateLimitBurst: getEnvInt("PRIVACY_RATE_LIMIT_BURST", 2000),
		TLSEnabled:     getEnvBool("PRIVACY_TLS_ENABLED", false),
		TLSCertFile:    getEnv("PRIVACY_TLS_CERT_FILE", ""),
		TLSKeyFile:     getEnv("PRIVACY_TLS_KEY_FILE", ""),
		TLSCAFile:      getEnv("PRIVACY_TLS_CA_FILE", ""),
		MTLSEnabled:    getEnvBool("PRIVACY_AUTH_INTERNAL_MTLS_ENABLED", false),
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
	router.Use(security.SecurityHeadersMiddleware())
	router.Use(security.AuthMiddleware())
	router.Use(security.RateLimitMiddleware())

	// 可选限流中间件（设计文档 §12.7 / §13.4）
	if cfg.RateLimitRPS > 0 {
		router.Use(middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))
		slog.Info("Rate limiting enabled", "rps", cfg.RateLimitRPS, "burst", cfg.RateLimitBurst)
	}

	router.Use(observability.RequestLogger())
	router.Use(engineMetrics.PrometheusMiddleware()) // Prometheus 实际指标注册

	// 注册全部 REST API 路由
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
		if cfg.TLSEnabled && cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			slog.Info("REST HTTPS server starting", "addr", restAddr, "cert", cfg.TLSCertFile)
			if err := restServer.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				slog.Error("REST HTTPS server error", "err", err)
				os.Exit(1)
			}
		} else {
			slog.Info("REST server starting", "addr", restAddr)
			if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("REST server error", "err", err)
				os.Exit(1)
			}
		}
	}()

	// ── gRPC Server ──
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", cfg.GRPCPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("gRPC listen failed", "err", err)
		os.Exit(1)
	}

	// 生产级 gRPC Keepalive 保活策略配置
	var grpcOpts = []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			MaxConnectionAge:  2 * time.Hour,
			Time:              2 * time.Minute,
			Timeout:           20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	if cfg.TLSEnabled && cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		clientAuth := ""
		if cfg.MTLSEnabled {
			clientAuth = "require"
		}
		tlsCfg, err := tlsutil.BuildServerTLSConfig(&tlsutil.ServerTLSConfig{
			Enabled:    true,
			CertFile:   cfg.TLSCertFile,
			KeyFile:    cfg.TLSKeyFile,
			CAFile:     cfg.TLSCAFile,
			ClientAuth: clientAuth,
		})
		if err != nil {
			slog.Error("Failed to build gRPC TLS credentials", "err", err)
			os.Exit(1)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("gRPC TLS credentials enabled", "mtls", cfg.MTLSEnabled)
	}

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
		"tls_enabled", cfg.TLSEnabled,
		"mtls_enabled", cfg.MTLSEnabled,
		"log_level", cfg.LogLevel,
		"budget_total_epsilon", budgetStatus["total_epsilon"],
		"budget_remaining_epsilon", budgetStatus["remaining_epsilon"],
	)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Shutdown signal received, starting graceful draining", "signal", sig)

	// 1. 标记 K8s 就绪探针为 unready
	rest.SetReady(false)

	// 2. 流量排空等待窗口
	drainSec := getEnvInt("PRIVACY_SHUTDOWN_DRAIN_SECONDS", 5)
	if drainSec > 0 {
		slog.Info("Draining in-flight traffic", "seconds", drainSec)
		time.Sleep(time.Duration(drainSec) * time.Second)
	}

	// 3. 优雅停止 REST 与 gRPC Server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := restServer.Shutdown(ctx); err != nil {
		slog.Error("REST server shutdown error", "err", err)
	}

	// gRPC GracefulStop 带超时回退：若 RPC 不结束则强制停止，防止挂死
	grpcDone := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcDone)
	}()
	grpcGraceSec := getEnvInt("PRIVACY_GRPC_GRACEFUL_STOP_SECONDS", 15)
	select {
	case <-grpcDone:
		slog.Info("gRPC server stopped gracefully")
	case <-time.After(time.Duration(grpcGraceSec) * time.Second):
		slog.Warn("gRPC graceful stop timed out, forcing stop", "timeout_sec", grpcGraceSec)
		grpcSrv.Stop()
	}

	// 4. 停止后台 goroutine（限流清理等），与生命周期绑定
	security.StopRateLimiter()

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
		n, err := strconv.Atoi(v)
		if err != nil {
			return defaultVal
		}
		return n
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		return strings.EqualFold(v, "true") || v == "1"
	}
	return defaultVal
}
