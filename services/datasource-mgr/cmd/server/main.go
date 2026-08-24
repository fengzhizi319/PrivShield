// Command server is the entry point for the mock datasource-mgr module.
// Command server 是模拟数据源模块的程序入口（用于开发与调试模拟数据通信）。
//
// Architecture / 架构：
//
//	React 前端 / BFF  ──HTTP/JSON──▶  datasource-mgr(Go) :8083 (提供 yibao/kangyang 模拟数据)
//	                                └─gRPC(mTLS)───────▶ :50053 (API 1, 2, 3, 4 模拟数据接口)
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
)

func main() {
	cfg := config.Load()

	// ── Structured logger / 结构化日志 ────────────────────────
	logger := pkgconfig.SetupLogger(cfg.LogFormat, cfg.LogLevel)

	// ── HTTP REST Server / HTTP REST 服务器 ──────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(cfg, logger)
	router := gin.New()
	server.RegisterRoutes(router)

	httpSrv := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
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
		serviceImpl = grpcserver.New(cfg, logger)
		pb.RegisterDataSourceManagerServiceServer(grpcServer, serviceImpl)
		logger.Info("mock datasource-mgr gRPC server started with mTLS",
			"addr", cfg.GRPCAddress(),
			"tls_cert", cfg.TLSCertFile,
			"tls_key", cfg.TLSKeyFile,
		)
	} else {
		grpcServer = grpc.NewServer()
		serviceImpl = grpcserver.New(cfg, logger)
		pb.RegisterDataSourceManagerServiceServer(grpcServer, serviceImpl)
		logger.Info("mock datasource-mgr gRPC server started (insecure)", "addr", cfg.GRPCAddress())
	}

	// ── Signal handling / 信号处理 ───────────────────────────────
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start gRPC listener / 启动 gRPC 监听
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		log.Fatalf("failed to listen on gRPC address %s: %v", cfg.GRPCAddress(), err)
	}

	go func() {
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC server error", "error", err.Error())
		}
	}()

	// Start HTTP server / 启动 HTTP 监听
	go func() {
		logger.Info("mock datasource-mgr HTTP REST server started",
			"addr", cfg.Address(),
			"grpc_addr", cfg.GRPCAddress(),
			"mode", "mock_development_and_debugging",
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err.Error())
		}
	}()

	// Wait for shutdown signal / 等待优雅停机信号
	sig := <-sigChan
	logger.Info("shutting down mock datasource-mgr servers...", "signal", sig.String())

	// Graceful shutdown gRPC
	serviceImpl.Shutdown()
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

	// Graceful shutdown HTTP
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err.Error())
	} else {
		logger.Info("HTTP server stopped")
	}
}
