// Command server is the entry point for the service-hub module.
// Command server 是数据服务调度中枢模块的程序入口。
//
// Architecture / 架构：
//
//	React 前端 ──HTTP/JSON──▶ service-hub(Go) ──HTTP──▶ PrivShield Agent
//	                          │               ──HTTP──▶ datasource-mgr (:8083)
//	                          └─gRPC(mTLS)───▶ 调度中枢客户端 (Port: :50052)
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

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/pkg/store/sqlite"

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/datasource"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/services/service-hub/proto"
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

	// ── Clients / 客户端依赖 ───────────────────────────────────
	agentClient := agent.New(cfg)
	dsClient := datasource.New(cfg)

	// ── HTTP REST Server / HTTP REST 服务器 ──────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, dsClient, cfg, taskStore, logger, mc)
	router := gin.New()
	server.RegisterRoutes(router)

	httpSrv := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,  // Slowloris header timeout
		ReadTimeout:       30 * time.Second, // Slow request body timeout
		WriteTimeout:      60 * time.Second, // Slow client response timeout
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB max header size
	}

	// ── gRPC Server (with optional mTLS) / gRPC 服务器（可选 mTLS）──
	var grpcServer *grpc.Server
	var serviceImpl *grpcserver.GRPCServer

	if cfg.TLSEnabled {
		creds, credErr := grpcserver.BuildServerCredentials(cfg)
		if credErr != nil {
			log.Fatalf("failed to build TLS credentials: %v", credErr)
		}
		grpcServer = grpc.NewServer(grpc.Creds(creds))
		serviceImpl = grpcserver.New(agentClient, dsClient, cfg, taskStore, logger)
		pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)
		logger.Info("gRPC server started with mTLS",
			"addr", cfg.GRPCAddress(),
			"tls_cert", cfg.TLSCertFile,
			"tls_key", cfg.TLSKeyFile,
		)
	} else {
		grpcServer = grpc.NewServer()
		serviceImpl = grpcserver.New(agentClient, dsClient, cfg, taskStore, logger)
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
	// 启动 HTTP 监听
	go func() {
		logger.Info("service-hub HTTP REST server started",
			"addr", cfg.Address(),
			"grpc_addr", cfg.GRPCAddress(),
			"agent_rest", cfg.AgentBaseURL(),
			"datasource_rest", cfg.DatasourceBaseURL(),
			"db_path", cfg.DBPath,
			"auth_enabled", cfg.APIKey != "",
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err.Error())
		}
	}()

	// Wait for shutdown signal
	// 等待优雅停机信号
	sig := <-sigChan
	logger.Info("shutting down service-hub servers...", "signal", sig.String())

	// P51 + P53 fix: signal running task goroutines to cancel before stopping servers
	// 先取消正在运行的任务 goroutine，再停止 gRPC 和 HTTP 服务器
	serviceImpl.Shutdown()
	server.Shutdown()

	// Graceful shutdown gRPC
	// 优雅关停 gRPC 服务器
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

	// Graceful shutdown HTTP with 5-second deadline
	// 优雅关停 HTTP 服务器（5 秒超时）
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err.Error())
	} else {
		logger.Info("HTTP server stopped")
	}
}

func initTaskStore(dbPath string, logger *slog.Logger) (store.TaskStore, error) {
	if dbPath == "" {
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
