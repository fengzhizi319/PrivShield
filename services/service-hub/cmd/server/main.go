// Command server is the entry point for the service-hub module.
// Command server 是数据服务调度中枢模块（service-hub）的程序主入口。
//
// ==============================================================================
// Architecture & Traffic Flow / 系统架构与流量拓扑：
// ==============================================================================
//
//	┌────────────────────────┐         HTTP / JSON (:8082)
//	│  React Web UI / BFF-Go │ ──────────────────────────────────┐
//	└────────────────────────┘                                   │
//	                                                             ▼
//	┌────────────────────────┐   gRPC + mTLS 双向加密 (:50052)   ┌───────────────────────────────┐
//	│ 上游业务系统 / 客户端   │ ───────────────────────────────▶ │ service-hub 数据服务调度中枢  │
//	└────────────────────────┘                                   │ - HTTP REST: :8082            │
//	                                                             │ - gRPC (mTLS/Plain): :50052   │
//	                                                             │ - 6 阶段流水线调度引擎        │
//	                                                             └──────────────┬────────────────┘
//	                                                                            │
//	                         ┌──────────────────────────────────────────────────┴──────────────────────────────────┐
//	                         │ HTTP REST                                                                           │ HTTP REST / gRPC
//	                         ▼                                                                                     ▼
//	        ┌──────────────────────────────────┐                                                  ┌──────────────────────────────────┐
//	        │ PrivShield Agent 隐私脱敏引擎      │                                                  │ datasource-mgr 模拟数据源服务     │
//	        │ - 动态分类分级 /v1/dynclassificatio │                                                  │ - 医保/康养模拟数据 :8083 / :50053 │
//	        │ - 隐私脱敏与K匿名 /v1/privacy      │                                                  └──────────────────────────────────┘
//	        └──────────────────────────────────┘
//
// ==============================================================================
// Key Responsibilities / 核心职责：
// ==============================================================================
// 1. 配置与日志加载：从环境变量读取配置并初始化基于 slog 的结构化日志记录器；
// 2. 任务持久化存储初始化：支持纯内存存储（测试/轻量）与 SQLite 持久化存储（生产容灾）；
// 3. Prometheus 指标收集器：初始化请求计数、耗时分布与流水线执行指标；
// 4. 下游客户端组件实例化：创建与 PrivShield Agent 及 datasource-mgr 通信的客户端；
// 5. 双协议并发服务监听：在独立协程中启动 HTTP REST (Gin) 与 gRPC (支持零信任 mTLS 与公钥固定)；
// 6. 优雅停机收敛：拦截 SIGINT/SIGTERM，先向异步任务协程发送取消信号，再顺序关闭 gRPC 与 HTTP 服务器。
// ==============================================================================

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
	// =========================================================================
	// 1. Configuration Loading / 配置解析与加载
	// =========================================================================
	// 从环境变量中读取运行配置（如 SERVICE_HUB_PORT, AGENT_REST_HOST, DB_PATH, TLS 配置等），
	// 未设置时采用安全合理的回退默认值（默认 HTTP :8082, gRPC :50052）。
	cfg := config.Load()

	// =========================================================================
	// 2. Structured Logger Setup / 结构化日志系统初始化
	// =========================================================================
	// 使用共享库 pkgconfig.SetupLogger 初始化基于 slog 的全局日志记录器（支持 json/text 格式）。
	logger := pkgconfig.SetupLogger(cfg.LogFormat, cfg.LogLevel)

	// =========================================================================
	// 3. Task Store Initialization / 任务持久化存储初始化
	// =========================================================================
	// 若配置了 DBPath（如 "/app/data/service-hub.db"），则初始化 SQLite 持久化任务库；
	// 若 DBPath 为空，则回退为进程内内存任务存储（memory.NewTaskStore），确保轻量与无外部依赖。
	taskStore, err := initTaskStore(cfg.DBPath, logger)
	if err != nil {
		log.Fatalf("failed to initialize task store: %v", err)
	}

	// =========================================================================
	// 4. Prometheus Metrics Collector / Prometheus 监控指标收集器
	// =========================================================================
	// 注册 service-hub 命名空间的 Prometheus 监控指标（QPS、延迟、流水线各阶段状态等）。
	mc := metrics.NewCollector("service-hub")

	// =========================================================================
	// 5. Upstream & Downstream Clients Setup / 下游依赖客户端实例化
	// =========================================================================
	// 1) AgentClient: 负责与 PrivShield Python Core Sidecar（:8079）通信，调用分类分级与脱敏算子；
	// 2) DatasourceClient: 负责与 datasource-mgr 模拟数据源服务（:8083/:50053）交互，采样抽取数据。
	agentClient := agent.New(cfg)
	dsClient := datasource.New(cfg)

	// =========================================================================
	// 6. HTTP REST Server Setup / HTTP REST 路由与服务器构建
	// =========================================================================
	// 1) 锁定 Gin 为生产发布模式（ReleaseMode）；
	// 2) 实例化 HTTP 处理器集合，装配任务分发调度、流水线查询、数据源代理等端点；
	// 3) 初始化无默认中间件的 Gin 引擎，并通过 RegisterRoutes 挂载通用中间件链（RequestID、Logger、Recovery、CORS、Auth）；
	// 4) 显式配置 http.Server 网络超时参数，防范 Slowloris 慢速连接拒绝服务攻击。
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, dsClient, cfg, taskStore, logger, mc)
	router := gin.New()
	server.RegisterRoutes(router)

	httpSrv := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,   // 限制读取 HTTP Header 的最大时间，防御 Slowloris
		ReadTimeout:       30 * time.Second,  // 读取请求体的超时时间
		WriteTimeout:      60 * time.Second,  // 响应写入的超时时间
		IdleTimeout:       120 * time.Second, // Keep-Alive 空闲连接保活上限
		MaxHeaderBytes:    1 << 20,           // 1 MiB 单请求 Header 最大字节限制
	}

	// =========================================================================
	// 7. gRPC Server Setup (with optional mTLS) / gRPC 服务构建（支持可选 mTLS）
	// =========================================================================
	// 根据配置判断是否启用 mTLS 双向认证：
	// - 启用 TLS: 加载服务端证书/私钥，挂载 CA 证书校验客户端身份，注册服务桩并开启 TLS 1.3 强加密；
	// - 未启用 TLS: 启动标准明文 gRPC Server 实例，适用于本地开发或 Service Mesh 代理。
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

	// =========================================================================
	// 8. Operating System Signal Registration / 系统中断信号监听
	// =========================================================================
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// =========================================================================
	// 9. Dual-Protocol Concurrent Listeners / 双协议并发监听启动
	// =========================================================================
	// 1) 启动 gRPC TCP 监听端口（默认 :50052）并在后台协程中运行事件循环
	grpcLis, err := net.Listen("tcp", cfg.GRPCAddress())
	if err != nil {
		log.Fatalf("failed to listen on gRPC address %s: %v", cfg.GRPCAddress(), err)
	}

	go func() {
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC server error", "error", err.Error())
		}
	}()

	// 2) 启动 HTTP REST 服务并在后台独立协程中监听请求
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

	// =========================================================================
	// 10. Graceful Shutdown Workflow / 优雅停机收敛流程
	// =========================================================================
	// 1) 阻塞等待退出信号（SIGINT / SIGTERM）
	sig := <-sigChan
	logger.Info("shutting down service-hub servers...", "signal", sig.String())

	// 2) 优先向内部异步流水线任务发送取消信号，平滑等待在途处理协程完成
	serviceImpl.Shutdown()
	server.Shutdown()

	// 3) 优雅停止 gRPC 服务器，拒绝新连接并等待当前 RPC 调用返回
	grpcServer.GracefulStop()
	logger.Info("gRPC server stopped")

	// 4) 优雅关闭 HTTP 服务器，设定 5 秒硬上限等待正在处理中的 HTTP 请求
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err.Error())
	} else {
		logger.Info("HTTP server stopped")
	}
}

// initTaskStore initializes either an in-memory task store or a persistent SQLite database.
// initTaskStore 根据配置的 dbPath 初始化任务存储介质：
// - dbPath 为空：使用轻量内存存储（memory.NewTaskStore()）；
// - dbPath 非空：打开并初始化 SQLite 数据库连接（sqlite.NewTaskStore(db)）。
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
