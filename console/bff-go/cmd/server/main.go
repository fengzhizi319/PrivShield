// Command server 是 Go gRPC 代理后端的程序入口。
// Command server is the entry point for the Go gRPC proxy backend.
//
// 执行流程 / Execution flow:
//   1. 从环境变量加载配置（agent 地址、监听端口、API Key 等）
//      Load configuration from env vars (agent address, listen port, API Key, etc.)
//   2. 创建到 PrivShield Python gRPC 服务的客户端连接
//      Create gRPC client connection to PrivShield Python service
//   3. 初始化 Gin HTTP 路由，注册所有 REST 代理接口与静态 UI 托管
//      Initialize Gin HTTP routes, register all REST proxy endpoints and static UI hosting
//   4. 启动 HTTP 服务器，监听前端请求
//      Start HTTP server, listen for frontend requests
//   5. 监听系统信号（SIGINT/SIGTERM），收到后执行优雅关闭
//      Listen for system signals (SIGINT/SIGTERM), perform graceful shutdown on receipt
//
// 整体架构 / Overall architecture:
//   React 前端  ──HTTP/JSON──▶  本程序(Go)  ──gRPC──▶  PrivShield(Python)
//   React frontend  ──HTTP/JSON──▶  This program(Go)  ──gRPC──▶  PrivShield(Python)
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/pkg/metrics"

	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/handlers"
)

// main 是程序入口函数，按以下步骤顺序执行：
// main is the program entry point, executing in the following order:
//   加载配置 → 创建 gRPC 客户端 → 初始化 HTTP 路由 → 启动服务器 → 等待关闭信号
//   Load config → Create gRPC client → Init HTTP routes → Start server → Wait for shutdown signal
func main() {
	// ── 步骤 1：加载配置 ──────────────────────────────────────────────
	// 从环境变量读取所有配置项，包括：
	//   - PRIVACY_AGENT_HOST / PRIVACY_AGENT_PORT：上游 gRPC agent 地址
	//   - PRIVACY_CONSOLE_HOST / PRIVACY_CONSOLE_PORT：本代理 HTTP 监听地址
	//   - PRIVACY_AGENT_API_KEY：可选的认证 API Key
	//   - PRIVACY_CONSOLE_STATIC_DIR：可选的前端静态文件目录
	cfg := config.Load()

	// Validate configuration consistency (fail-fast with clear error messages).
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// ── 步骤 1.5：结构化日志 + Prometheus 指标 ─────────────────────
	logger := pkgconfig.SetupLogger(
		pkgconfig.EnvString("CONSOLE_LOG_FORMAT", "json"),
		pkgconfig.EnvString("CONSOLE_LOG_LEVEL", "info"),
	)
	mc := metrics.NewCollector("backend-go")

	// ── 步骤 2：创建 gRPC 客户端 ─────────────────────────────────────
	// 根据配置建立到 PrivShield 的 gRPC 连接。
	// 如果配置了 API Key，会自动附加 authorization 元数据。
	// 连接失败时打印错误并立即退出进程（log.Fatalf）。
	client, err := agent.New(cfg)
	if err != nil {
		log.Fatalf("failed to create agent client: %v", err) // 致命错误：无法连接上游 agent
	}
	// 注册 defer：main 函数退出前自动关闭 gRPC 连接，释放底层 TCP 连接与 HTTP/2 流
	defer func() { _ = client.Close() }()

	// ── 步骤 3：初始化 HTTP 路由 ─────────────────────────────────────
	// 将 Gin 设置为发布模式，关闭调试日志输出，提升性能
	gin.SetMode(gin.ReleaseMode)
	// 创建 HTTP 处理器实例，持有 gRPC 客户端引用与配置信息，
	// 内部实现了 /api/health、/api/samples、/api/proxy、/api/batch 等接口
	server := handlers.New(client, cfg, logger, mc)
	// 创建一个新的 Gin 引擎实例（包含默认的 Logger + Recovery 中间件）
	router := gin.New()
	// 将所有 REST 代理路由与可选的静态 UI 托管路由注册到 Gin 引擎
	// 包括 CORS 中间件、健康检查、代理转发、批量测试、静态文件服务等
	server.RegisterRoutes(router)

	// ── 步骤 4：配置并启动 HTTP 服务器 ───────────────────────────────
	// 创建标准库 HTTP 服务器实例，将 Gin 引擎作为 Handler
	srv := &http.Server{
		Addr:              cfg.ConsoleAddress(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,  // Slowloris header timeout
		ReadTimeout:       30 * time.Second, // Slow request body timeout
		WriteTimeout:      60 * time.Second, // Slow client response timeout
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB max header size
	}

	// ── 步骤 5：启动优雅关闭协程 ─────────────────────────────────────
	// 在独立 goroutine 中监听系统信号，主协程继续执行到 ListenAndServe
	go func() {
		// 创建一个带缓冲的信号通道，容量为 1 避免信号丢失
		sigChan := make(chan os.Signal, 1)
		// 将 SIGINT（Ctrl+C）和 SIGTERM（kill/容器停止）信号注册到通道
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		// 阻塞等待，直到收到任意一个系统信号
		<-sigChan

		// P57 fix: stop securityMiddleware ticker goroutine before stopping HTTP server.
		server.Shutdown()

		// 收到关闭信号后，创建带 5 秒超时的上下文，
		// 确保优雅关闭不会无限阻塞（如存在未完成的长连接）
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// 注册 defer：函数退出时释放上下文资源
		defer cancel()
		// 调用 Shutdown：停止接收新连接，等待所有活跃请求处理完毕或超时
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// 超时或关闭异常时仅打印日志（此时主协程可能已退出）
			logger.Error("http server shutdown error", "error", err.Error())
		}
	}()

	// ── 步骤 6：启动服务 ──────────────────────────────────────────────
	logger.Info("backend-go started",
		"http_addr", cfg.ConsoleAddress(),
		"agent_grpc", cfg.AgentAddress(),
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}
