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
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pkgconfig "github.com/fengzhizi319/PrivShield/pkg/config"
	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/pkg/store/sqlite"
	"github.com/fengzhizi319/PrivShield/pkg/tlsutil"

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
	//
	// 3.1 SQLite Integrity Check / SQLite 完整性校验
	// 启动时先校验数据库完整性，检测损坏并阻止服务启动，防止带病运行。
	if cfg.DBPath != "" {
		if err := sqlite.ValidateIntegrity(cfg.DBPath); err != nil {
			log.Fatalf("sqlite integrity check failed: %v", err)
		}
		logger.Info("database integrity check passed", "path", cfg.DBPath)
	}

	taskStore, err := initTaskStore(cfg.DBPath, logger)
	if err != nil {
		log.Fatalf("failed to initialize task store: %v", err)
	}

	// =========================================================================
	// 4. Prometheus Metrics Collector / Prometheus 监控指标收集器
	// =========================================================================
	// 注册 service-hub 命名空间的 Prometheus 监控指标（QPS、延迟、流水线各阶段状态等）。
	// 注意：mc 必须在崩溃恢复/重试之前初始化，以便记录恢复/重试指标。
	mc := metrics.NewCollector("service-hub")

	// =========================================================================
	// 3.5 Crash Recovery / 崩溃恢复机制
	// =========================================================================
	// 启动时自动扫描并恢复孤立任务：
	// - pending 任务：直接保留在队列中（尚未执行，无需标记失败）；
	// - running 任务：标记为 failed（可能已部分执行，需重新提交）。
	recoverOrphanedTasks(taskStore, mc, logger)

	// =========================================================================
	// 3.6 Automatic Task Retry / 失败任务自动重试
	// =========================================================================
	// 启动时自动重试因临时错误（网络超时、连接失败等）而失败的任务。
	// 最多重试 3 次，使用结构化 RetryCount 字段（替代脆弱的字符串匹配）。
	// 重试采用指数退避延迟，避免下游服务仍不可用时立即再次失败。
	retryFailedTasks(taskStore, mc, logger)

	// =========================================================================
	// 3.7 Periodic Background Retry / 周期性后台重试协程
	// =========================================================================
	// 启动后台协程，每 60 秒扫描一次 failed 任务并自动重试。
	// 解决“运行时失败的任务必须等到下次服务重启才能重试”的问题。
	retryCtx, retryCancel := context.WithCancel(context.Background())
	go periodicRetryLoop(retryCtx, taskStore, mc, logger, 60*time.Second)

	// =========================================================================
	// 3.8 Periodic Data Retention Cleanup / 周期性数据保留清理协程
	// =========================================================================
	// 启动后台协程，每 6 小时扫描并清理超过保留期的终态任务，防止 SQLite 无限膨胀。
	// RetentionDays=0 时禁用清理（适用于调试或短期部署）。
	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	if cfg.RetentionDays > 0 {
		go dataRetentionLoop(retentionCtx, taskStore, logger, cfg.RetentionDays)
	}

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
	// 6.5 HTTP TLS/mTLS Configuration / HTTP TLS 双向认证配置
	// =========================================================================
	// 当启用 TLS 时，为 HTTP 服务器构建 TLS 配置，支持 mTLS 双向认证：
	// - TLS 1.3 强制最低版本；
	// - 可选客户端证书认证（require/verify/request）；
	// - 可选公钥固定（SPKI Pinning）。
	var httpTLSConfig *tls.Config
	if cfg.TLSEnabled {
		tlsCfg := &tlsutil.ServerTLSConfig{
			Enabled:          cfg.TLSEnabled,
			CertFile:         cfg.TLSCertFile,
			KeyFile:          cfg.TLSKeyFile,
			CAFile:           cfg.TLSCAFile,
			ClientAuth:       cfg.TLSClientAuth,
			PinnedPubKeyFile: cfg.TLSPinnedPubKeyFile,
		}
		var tlsErr error
		httpTLSConfig, tlsErr = tlsutil.BuildServerTLSConfig(tlsCfg)
		if tlsErr != nil {
			log.Fatalf("failed to build HTTP TLS config: %v", tlsErr)
		}
		httpSrv.TLSConfig = httpTLSConfig
		logger.Info("HTTP REST server configured with mTLS",
			"client_auth", cfg.TLSClientAuth,
			"tls_cert", cfg.TLSCertFile,
		)
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
		if cfg.TLSEnabled {
			logger.Info("service-hub HTTPS REST server started (mTLS enabled)",
				"addr", cfg.Address(),
				"grpc_addr", cfg.GRPCAddress(),
				"agent_rest", cfg.AgentBaseURL(),
				"datasource_rest", cfg.DatasourceBaseURL(),
				"db_path", cfg.DBPath,
				"auth_enabled", cfg.APIKey != "",
				"mtls_client_auth", cfg.TLSClientAuth,
			)
			// ListenAndServeTLS 使用 httpSrv.TLSConfig 中的证书，空字符串表示从 TLSConfig 读取
			if err := httpSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				logger.Error("HTTPS server error", "error", err.Error())
			}
		} else {
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
		}
	}()

	// =========================================================================
	// 10. Graceful Shutdown Workflow / 优雅停机收敛流程
	// =========================================================================
	// 1) 阻塞等待退出信号（SIGINT / SIGTERM）
	sig := <-sigChan
	logger.Info("shutting down service-hub servers...", "signal", sig.String())

	// 2) 停止周期性重试协程与数据保留清理协程
	retryCancel()
	retentionCancel()

	// 3) 优先向内部异步流水线任务发送取消信号，平滑等待在途处理协程完成
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

// recoverOrphanedTasks scans for tasks stuck in "running" or "pending" state
// after a crash/restart and handles them appropriately:
// - pending tasks: kept in queue (not yet executed, safe to requeue);
// - running tasks: marked as failed (may have partially executed).
// 崩溃恢复：区分处理 running 和 pending 状态的孤立任务。
//
// 当服务突然崩溃（kill -9、OOM Kill、断电）时，优雅停机代码不会执行，
// 导致 running/pending 状态的任务永远卡在数据库中。此函数在启动时自动恢复这些孤立任务。
//
// 改进点（#1）：pending 任务直接保留在队列中（它们尚未执行，无需标记失败）；
// running 任务标记为 failed（可能已部分执行，需要重新提交）。
func recoverOrphanedTasks(taskStore store.TaskStore, mc *metrics.Collector, logger *slog.Logger) {
	// 1. 扫描所有 "running" 状态的任务 → 标记为 failed（可能已部分执行）
	runningTasks, _, err := taskStore.List(store.TaskFilter{Status: "running", Limit: 10000})
	if err != nil {
		logger.Error("failed to list running tasks for recovery", "error", err.Error())
		return
	}

	for i := range runningTasks {
		runningTasks[i].Status = "failed"
		runningTasks[i].Error = "server crashed or restarted (recovered on startup)"
		now := time.Now()
		runningTasks[i].CompletedAt = &now
		runningTasks[i].DurationMs = now.Sub(runningTasks[i].CreatedAt).Milliseconds()
		_ = taskStore.Update(&runningTasks[i])
		if mc != nil {
			mc.RecordOrphanedRecovery("running")
		}
	}

	// 2. 扫描所有 "pending" 状态的任务 → 直接保留在队列中（尚未执行，无需标记失败）
	pendingTasks, _, err := taskStore.List(store.TaskFilter{Status: "pending", Limit: 10000})
	if err != nil {
		logger.Error("failed to list pending tasks for recovery", "error", err.Error())
		return
	}

	// pending 任务无需修改状态，仅记录指标
	for range pendingTasks {
		if mc != nil {
			mc.RecordOrphanedRecovery("pending")
		}
	}

	// 3. 记录恢复日志
	if len(runningTasks) > 0 || len(pendingTasks) > 0 {
		logger.Warn("recovered orphaned tasks after crash/restart",
			"running_marked_failed", len(runningTasks),
			"pending_kept_in_queue", len(pendingTasks),
			"total_recovered", len(runningTasks)+len(pendingTasks))
	} else {
		logger.Info("no orphaned tasks found, all tasks are in terminal state")
	}
}

// maxRetryCount is the maximum number of retry attempts for a failed task.
const maxRetryCount = 3

// retryFailedTasks automatically retries failed tasks that are marked as retryable.
// 自动重试机制：扫描所有因临时错误而失败的任务，重新提交执行。
//
// 改进点（#3）：使用结构化 RetryCount 字段替代脆弱的字符串匹配；
// 改进点（#10）：重试采用指数退避延迟（5s → 10s → 20s），避免下游仍不可用时立即再次失败。
func retryFailedTasks(taskStore store.TaskStore, mc *metrics.Collector, logger *slog.Logger) {
	// 扫描所有 "failed" 状态的任务
	failedTasks, _, err := taskStore.List(store.TaskFilter{Status: "failed", Limit: 100})
	if err != nil {
		logger.Error("failed to list failed tasks for retry", "error", err.Error())
		return
	}

	retryCount := 0
	for i := range failedTasks {
		// 只重试特定类型的失败（如网络超时、临时错误）
		if !isRetryableError(failedTasks[i].Error) {
			continue
		}

		// 使用结构化 RetryCount 字段检查重试次数（替代脆弱的 strings.Count）
		if failedTasks[i].RetryCount >= maxRetryCount {
			logger.Warn("task exceeded max retry attempts, skipping",
				"task_id", failedTasks[i].ID,
				"retry_count", failedTasks[i].RetryCount,
				"max_retry", maxRetryCount)
			if mc != nil {
				mc.RecordTaskRetry("exhausted")
			}
			continue
		}

		// 检查退避延迟（#10）：如果 RetryAfter 尚未到期，跳过
		if failedTasks[i].RetryAfter != nil && time.Now().Before(*failedTasks[i].RetryAfter) {
			continue
		}

		// 计算指数退避延迟：5s * 2^(retryCount)
		newRetryCount := failedTasks[i].RetryCount + 1
		backoffDuration := 5 * time.Second * time.Duration(1<<uint(failedTasks[i].RetryCount))
		retryAfter := time.Now().Add(backoffDuration)

		// 重置任务状态为 pending
		failedTasks[i].Status = "pending"
		failedTasks[i].Stage = "queued"
		failedTasks[i].Error = fmt.Sprintf("retrying (attempt %d/%d)", newRetryCount, maxRetryCount)
		failedTasks[i].StartedAt = nil
		failedTasks[i].CompletedAt = nil
		failedTasks[i].DurationMs = 0
		failedTasks[i].RetryCount = newRetryCount
		failedTasks[i].RetryAfter = &retryAfter

		if err := taskStore.Update(&failedTasks[i]); err != nil {
			logger.Error("failed to reset task for retry", "task_id", failedTasks[i].ID, "error", err.Error())
			continue
		}

		retryCount++
		if mc != nil {
			mc.RecordTaskRetry("queued")
		}
		logger.Info("task queued for retry",
			"task_id", failedTasks[i].ID,
			"attempt", newRetryCount,
			"backoff_seconds", backoffDuration.Seconds())
	}

	if retryCount > 0 {
		logger.Info("queued tasks for retry", "count", retryCount)
	} else {
		logger.Debug("no retryable failed tasks found")
	}
}

// periodicRetryLoop runs retryFailedTasks periodically until the context is cancelled.
// 周期性后台重试循环：每隔 interval 扫描一次 failed 任务并自动重试。
// 解决“运行时失败的任务必须等到下次服务重启才能重试”的问题（#2）。
func periodicRetryLoop(ctx context.Context, taskStore store.TaskStore, mc *metrics.Collector, logger *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("periodic background retry started", "interval_seconds", interval.Seconds())

	for {
		select {
		case <-ctx.Done():
			logger.Info("periodic background retry stopped")
			return
		case <-ticker.C:
			retryFailedTasks(taskStore, mc, logger)
		}
	}
}

// isRetryableError checks if an error is retryable (network timeout, temporary failure, etc.)
// isRetryableError 检查错误是否可重试（网络超时、临时故障等）。
//
// 可重试的错误类型：
// - timeout（超时）
// - connection refused（连接拒绝）
// - temporary failure（临时故障）
// - network unreachable（网络不可达）
// - context deadline exceeded（上下文超时）
// - server crashed or restarted（服务崩溃或重启）
func isRetryableError(errMsg string) bool {
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"temporary failure",
		"network unreachable",
		"context deadline exceeded",
		"server crashed or restarted",
	}

	errMsgLower := strings.ToLower(errMsg)
	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsgLower, pattern) {
			return true
		}
	}
	return false
}

// dataRetentionLoop periodically deletes terminal tasks older than retentionDays.
// dataRetentionLoop 周期性删除超过保留期的终态任务，防止 SQLite 无限膨胀。
//
// 每 6 小时执行一次清理，仅删除 completed/failed 状态的任务，
// 保留 pending/running 状态的任务不受影响。
func dataRetentionLoop(ctx context.Context, taskStore store.TaskStore, logger *slog.Logger, retentionDays int) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	logger.Info("data retention cleanup started", "retention_days", retentionDays, "interval_hours", 6)

	// Run once immediately on startup / 启动时立即执行一次
	runRetentionCleanup(taskStore, logger, retentionDays)

	for {
		select {
		case <-ctx.Done():
			logger.Info("data retention cleanup stopped")
			return
		case <-ticker.C:
			runRetentionCleanup(taskStore, logger, retentionDays)
		}
	}
}

// runRetentionCleanup performs a single cleanup pass.
func runRetentionCleanup(taskStore store.TaskStore, logger *slog.Logger, retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted, err := taskStore.CleanupOld(cutoff)
	if err != nil {
		logger.Error("data retention cleanup failed", "error", err.Error())
		return
	}
	if deleted > 0 {
		logger.Info("data retention cleanup completed",
			"deleted_tasks", deleted,
			"cutoff", cutoff.Format(time.RFC3339),
			"retention_days", retentionDays)
	}
}
