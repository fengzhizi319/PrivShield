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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/handlers"
	pb "github.com/fengzhizi319/PrivShield/console/service-hub/proto"
)

func main() {
	cfg := config.Load()
	agentClient := agent.New(cfg)

	// ── HTTP REST Server / HTTP REST 服务器 ──────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, cfg)
	router := gin.New()
	server.RegisterRoutes(router)

	httpSrv := &http.Server{
		Addr:    cfg.Address(),
		Handler: router,
	}

	// ── gRPC Server (with optional mTLS) / gRPC 服务器（可选 mTLS）──
	var grpcServer *grpc.Server
	if cfg.TLSEnabled {
		var err error
		grpcServer, err = grpcserver.StartGRPCServer(agentClient, cfg)
		if err != nil {
			log.Fatalf("failed to start gRPC server: %v", err)
		}
		fmt.Printf("gRPC server started with mTLS on %s\n", cfg.GRPCAddress())
		fmt.Printf("  TLS cert: %s\n", cfg.TLSCertFile)
		fmt.Printf("  TLS key:  %s\n", cfg.TLSKeyFile)
		if cfg.TLSCAFile != "" {
			fmt.Printf("  CA cert:  %s (client verification enabled)\n", cfg.TLSCAFile)
		}
		if cfg.TLSPinnedPubKeyFile != "" {
			fmt.Printf("  Pinned public key: %s\n", cfg.TLSPinnedPubKeyFile)
		}
	} else {
		// Start gRPC without TLS (development mode)
		// 无 TLS 启动 gRPC（开发模式）
		grpcServer = grpc.NewServer()
		serviceImpl := grpcserver.New(agentClient, cfg)
		pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)
		fmt.Printf("gRPC server started (insecure) on %s\n", cfg.GRPCAddress())
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
			log.Printf("gRPC server error: %v", err)
		}
	}()

	// Start HTTP server
	// 启动 HTTP 服务器
	go func() {
		fmt.Printf("Service Hub (HTTP) listening on http://%s\n", cfg.Address())
		fmt.Printf("Service Hub (gRPC) listening on %s\n", cfg.GRPCAddress())
		fmt.Printf("Upstream agent REST: %s\n", cfg.AgentBaseURL())

		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	// 等待关闭信号
	<-sigChan
	fmt.Println("\nShutting down servers...")

	// Graceful shutdown
	// 优雅关闭
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop gRPC server
	grpcServer.GracefulStop()

	// Stop HTTP server
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown error: %v", err)
	}

	fmt.Println("Servers stopped gracefully")
}
