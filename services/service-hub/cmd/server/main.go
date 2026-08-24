// Command server is the entry point for the service-hub module.
// Command server 是数据服务调度中枢模块的程序入口。
//
// Architecture / 架构：
//
//	React 前端  ──HTTP/JSON──▶  service-hub(Go)  ──HTTP──▶  PrivShield Agent
//	                          └─gRPC(mTLS)──▶  外部客户端
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pkgconfig "github.com/fengzhizi319/PrivShield/console/pkg/config"
	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/console/pkg/store/sqlite"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/console/service-hub/proto"
)

func main() {
	cfg := config.Load()

	// ── Structured logger / 结构化日志 ────────────────────────
	logger := pkgconfig.SetupLogger(cfg.LogFormat, cfg.LogLevel)

	// ── Task store / 任务存储 ──────────────────────────────────
	taskStore, err := initTaskStore(cfg.DBPath, logger)
	if err != nil {
		log.Fatalf("failed to initialize task store: %v", err)
	}

	// ── Prometheus metrics / Prometheus 指标 ───────────────────
	mc := metrics.NewCollector("service-hub")

	// ── Agent client / Agent 客户端 ────────────────────────────
	agentClient := agent.New(cfg)

	// ── HTTP REST Server / HTTP REST 服务器 ──────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, cfg, taskStore, logger, mc)
	router := gin.New()
	server.RegisterRoutes(router)

	httpSrv := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// ── gRPC Server (with optional mTLS) / gRPC 服务器（可选 mTLS）──
	var grpcServer *grpc.Server
	// P53 fix: save serviceImpl reference for graceful shutdown of task goroutines.
	// 保存 serviceImpl 引用，以便优雅关停时通知 processTask goroutine 停止。
	var serviceImpl *grpcserver.GRPCServer

	// P53 fix: both TLS and non-TLS branches manually create serviceImpl so we
	// hold the registered instance reference for Shutdown(). Previously the TLS
	// branch used StartGRPCServer() which created an internal instance we couldn't
	// reach, then created a second unregistered one — Shutdown() was a no-op.
	// TLS 和非 TLS 分支均手动创建 serviceImpl，确保持有已注册实例的引用用于 Shutdown()。
	if cfg.TLSEnabled {
		creds, credErr := grpcserver.BuildServerCredentials(cfg)
		if credErr != nil {
			log.Fatalf("failed to build TLS credentials: %v", credErr)
		}
		grpcServer = grpc.NewServer(grpc.Creds(creds))
		serviceImpl = grpcserver.New(agentClient, cfg, taskStore, logger)
		pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)
		logger.Info("gRPC server started with mTLS",
			"addr", cfg.GRPCAddress(),
			"tls_cert", cfg.TLSCertFile,
			"tls_key", cfg.TLSKeyFile,
		)
	} else {
		grpcServer = grpc.NewServer()
		serviceImpl = grpcserver.New(agentClient, cfg, taskStore, logger)
		pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)
		logger.Info("gRPC server started (insecure)", "addr", cfg.GRPCAddress())
	}

	// ── Signal handling / 信号处理 ───────────────────────────────
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start gRPC listener
	// 启动 gRPC 监听
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		log.Fatalf("failed to listen on gRPC address %s: %v", cfg.GRPCAddress(), err)
	}

	go func() {
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC server error", "error", err.Error())
		}
	}()

	// Start HTTP server
	go func() {
		logger.Info("service-hub started",
			"http_addr", cfg.Address(),
			"grpc_addr", cfg.GRPCAddress(),
			"agent_rest", cfg.AgentBaseURL(),
			"db_path", cfg.DBPath,
			"auth_enabled", cfg.APIKey != "",
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	logger.Info("shutting down servers...")

	// P53 fix: cancel in-flight task goroutines via Server.Shutdown() before
	// stopping the gRPC/HTTP listeners, so processTask goroutines receive
	// context cancellation and exit cleanly instead of being killed mid-flight.
	server.Shutdown()   // HTTP handler: cancel processTask goroutines + wait
	serviceImpl.Shutdown() // gRPC handler: cancel processTask goroutines + wait

	// Graceful shutdown of listeners
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	grpcServer.GracefulStop()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err.Error())
	}

	logger.Info("servers stopped gracefully")
}

// initTaskStore creates the task store based on configuration.
// initTaskStore 根据配置创建任务存储：SQLite 或内存回退。
func initTaskStore(dbPath string, logger *slog.Logger) (store.TaskStore, error) {
	if dbPath == "" {
		// Fall back to in-memory store
		// 回退到内存存储
		logger.Info("using in-memory task store (no persistence)")
		return memory.NewTaskStore(), nil
	}

	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	ts, err := sqlite.NewTaskStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create task store: %w", err)
	}

	logger.Info("sqlite task store initialized", "path", dbPath)
	return ts, nil
}
