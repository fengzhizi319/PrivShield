// Package grpcserver implements the gRPC service interface for service-hub with mTLS support.
// Package grpcserver 实现 service-hub 的 gRPC 高性能远程调用接口层，支持 TLS 1.3 双向认证（mTLS）与公钥指纹固定（Pinned Public Key）。
//
// 核心能力与安全特性：
// 1. 高性能 RPC：基于 google.golang.org/grpc 提供微秒级内部服务通信；
// 2. 企业级 mTLS：支持 RequireAndVerifyClientCert 双向证书认证与动态 CA 根证书挂载；
// 3. 公钥固定（Public Key Pinning）：支持 RSA、ECDSA、Ed25519 客户端公钥证书比对，防御证书伪造攻击；
// 4. 拦截器链：提供 Unary/Stream 统一 Panic 恢复（Recovery）与结构化访问日志（Logging）拦截器；
// 5. 6 阶段流水线驱动：在后台协程中安全驱动 6 阶段流通治理任务，支持并发信号量限流与优雅停机（Graceful Shutdown）。
package grpcserver

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/validation"

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/datasource"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/models"
	pb "github.com/fengzhizi319/PrivShield/services/service-hub/proto"
)

const moduleVia = "service-hub-grpc"

// GRPCServer implements the ServiceHubService gRPC service.
// GRPCServer 结构体实现 Protobuf 生成的 ServiceHubServiceServer 接口，管理核心客户端与生命周期。
type GRPCServer struct {
	pb.UnimplementedServiceHubServiceServer

	agent      *agent.Client      // 上游 PrivShield Python Agent 客户端
	datasource *datasource.Client // 下游 datasource-mgr 客户端
	cfg        *config.Config     // 模块全局运行配置
	startTime  time.Time          // 服务启动时间戳
	tasks      store.TaskStore    // 任务持久化仓库接口
	logger     *slog.Logger       // 结构化日志记录器
	taskSem    chan struct{}      // 限制后台并发任务协程数的信号量（默认容量 10）
	ctx        context.Context    // 优雅停机广播上下文
	cancel     context.CancelFunc // 触发停机 Context 取消的回调函数
	wg         sync.WaitGroup     // 跟踪记录正在运行的任务协程计数
}

// New creates a new GRPCServer instance.
// New 构造函数初始化 GRPCServer 实例，配置并发信号量与取消上下文。
func New(ag *agent.Client, ds *datasource.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger) *GRPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GRPCServer{
		agent:      ag,
		datasource: ds,
		cfg:        cfg,
		startTime:  time.Now(),
		tasks:      tasks,
		logger:     logger,
		taskSem:    make(chan struct{}, 10), // 最大限制 10 个并发异步任务
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Shutdown gracefully stops all in-flight task goroutines.
// Shutdown 优雅停机方法：通知所有在途 gRPC 任务协程安全退出并阻塞等待收敛。
func (s *GRPCServer) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// ─────────────────────────────────────────────────────────────
// gRPC Service Methods / gRPC 服务方法
// ─────────────────────────────────────────────────────────────

// Health checks self + upstream agent connectivity.
// Health 实现 gRPC 健康检查接口：检测自身并向 Python Agent 发起健康探测。
func (s *GRPCServer) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	start := time.Now()
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	agentData, err := s.agent.Health(healthCtx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &pb.HealthResponse{
			Backend:   "ok",
			Agent:     "unreachable",
			AgentUrl:  s.cfg.AgentBaseURL(),
			LatencyMs: latency,
			Error:     err.Error(),
			Via:       moduleVia,
		}, nil
	}

	agentStr := fmt.Sprintf("%v", agentData["status"])
	return &pb.HealthResponse{
		Backend:   "ok",
		Agent:     agentStr,
		AgentUrl:  s.cfg.AgentBaseURL(),
		LatencyMs: latency,
		Via:       moduleVia,
	}, nil
}

// HubStatus returns the scheduling hub status overview.
// HubStatus 实现调度中枢状态概览 RPC 方法：返回排队、活跃、成功与失败任务计数。
func (s *GRPCServer) HubStatus(ctx context.Context, req *pb.HubStatusRequest) (*pb.HubStatusResponse, error) {
	counts, err := s.tasks.Counts()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get task counts: %v", err)
	}

	return &pb.HubStatusResponse{
		Status:         "running",
		Uptime:         time.Since(s.startTime).Round(time.Second).String(),
		ActiveTasks:    int32(counts.Running),
		QueuedTasks:    int32(counts.Pending),
		CompletedTotal: int32(counts.Completed),
		FailedTotal:    int32(counts.Failed),
		AgentUrl:       s.cfg.AgentBaseURL(),
	}, nil
}

// Dispatch dispatches a new task to the scheduling pipeline.
// Dispatch 实现显式分发任务 RPC 方法：
// 1. 校验 source 非空、限长 1024 字符，operation 属于有效操作集合；
// 2. 持久化任务为 pending 状态；
// 3. 异步拉起 6 阶段流水线协程处理任务并返回 accepted。
func (s *GRPCServer) Dispatch(ctx context.Context, req *pb.DispatchRequest) (*pb.DispatchResponse, error) {
	// 字段合法性校验
	if strings.TrimSpace(req.Source) == "" {
		return nil, status.Error(codes.InvalidArgument, "source must not be empty")
	}
	if len(req.Source) > 1024 {
		return nil, status.Error(codes.InvalidArgument, "source exceeds maximum length of 1024 characters")
	}
	if err := validation.AllowedValues("operation", req.Operation, validation.HubOperations); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	taskID := validation.GenerateID("task")
	now := time.Now()

	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   req.Operation,
		Priority:    int(req.Priority),
		CreatedAt:   now,
		PayloadJSON: req.PayloadJson,
	}

	if err := s.tasks.Save(task); err != nil {
		return nil, status.Errorf(codes.Internal, "save task: %v", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, req.Operation, req.PayloadJson)
	}()

	return &pb.DispatchResponse{
		TaskId: taskID,
		Status: "accepted",
		Via:    moduleVia,
	}, nil
}

// ClassifyAndDispatch performs classification first, then auto-dispatches based on sensitivity.
// ClassifyAndDispatch 动态分类定级并自动分发 RPC 方法：
// 1. 若载荷为空，自动从数据源服务拉取样本；
// 2. 调用 Agent Classify 接口完成敏感等级评估；
// 3. 自适应决策脱敏算子并启动流水线任务。
func (s *GRPCServer) ClassifyAndDispatch(ctx context.Context, req *pb.ClassifyAndDispatchRequest) (*pb.ClassifyAndDispatchResponse, error) {
	if strings.TrimSpace(req.Source) == "" {
		return nil, status.Error(codes.InvalidArgument, "source must not be empty")
	}
	if len(req.Source) > 1024 {
		return nil, status.Error(codes.InvalidArgument, "source exceeds maximum length of 1024 characters")
	}

	payloadJSON := req.PayloadJson
	if (payloadJSON == "" || payloadJSON == "{}" || payloadJSON == "null") && s.datasource != nil {
		dsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if res, err := s.datasource.FetchDataBySource(dsCtx, req.Source, 5, 0); err == nil && len(res.Records) > 0 {
			b, _ := json.Marshal(res.Records[0])
			payloadJSON = string(b)
		}
		cancel()
	}

	classifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	classifyResult, err := s.agent.Classify(classifyCtx, payloadJSON)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "classification failed: %v", err)
	}

	level := "L2"
	if lvl, ok := classifyResult["level"].(string); ok {
		level = lvl
	}

	operation := models.LevelToOperation(level)
	priority := levelToPriority(level)

	taskID := validation.GenerateID("task")
	now := time.Now()

	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   operation,
		Priority:    priority,
		CreatedAt:   now,
		PayloadJSON: payloadJSON,
	}

	if err := s.tasks.Save(task); err != nil {
		return nil, status.Errorf(codes.Internal, "save task: %v", err)
	}

	classifyResultJSON, _ := json.Marshal(classifyResult)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, operation, payloadJSON)
	}()

	return &pb.ClassifyAndDispatchResponse{
		TaskId:             taskID,
		Level:              level,
		AutoOperation:      operation,
		ClassifyResultJson: string(classifyResultJSON),
		Via:                moduleVia,
	}, nil
}

// GetTask returns the details of a single task by ID.
// GetTask 根据 TaskID 查询单个任务详情，若不存在返回 NotFound 错误码。
func (s *GRPCServer) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.TaskProto, error) {
	taskID := strings.TrimSpace(req.GetTaskId())
	if taskID == "" {
		return nil, status.Error(codes.InvalidArgument, "task id must not be empty")
	}

	task, err := s.tasks.Get(taskID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task %s not found", taskID)
	}

	return taskToProto(task), nil
}

// ListTasks returns all tasks, optionally filtered by status.
// ListTasks 分页获取任务列表，支持状态过滤白名单校验。
func (s *GRPCServer) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	statusFilter := req.GetStatusFilter()
	if statusFilter != "" {
		if err := validation.AllowedValues("status", statusFilter, validation.TaskStatuses); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
	}

	tasks, total, err := s.tasks.List(store.TaskFilter{
		Status: statusFilter,
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list tasks: %v", err)
	}

	protos := make([]*pb.TaskProto, len(tasks))
	for i := range tasks {
		protos[i] = taskToProto(&tasks[i])
	}

	return &pb.ListTasksResponse{
		Total: int32(total),
		Tasks: protos,
		Via:   moduleVia,
	}, nil
}

// PipelineStatus returns the current status of each pipeline stage.
// PipelineStatus 获取流水线 6 个阶段的实时任务活跃度与 Agent 连通性。
func (s *GRPCServer) PipelineStatus(ctx context.Context, req *pb.PipelineStatusRequest) (*pb.PipelineStatusResponse, error) {
	runningTasks, _, err := s.tasks.List(store.TaskFilter{Status: "running", Limit: 1000})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list running tasks: %v", err)
	}

	stageNames := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}
	stageCounts := make(map[string]int)
	for _, t := range runningTasks {
		stageCounts[t.Stage]++
	}

	stages := make([]*pb.PipelineStageProto, len(stageNames))
	for i, name := range stageNames {
		st := "idle"
		if stageCounts[name] > 0 {
			st = "processing"
		}
		stages[i] = &pb.PipelineStageProto{
			Name:        name,
			Status:      st,
			ActiveCount: int32(stageCounts[name]),
		}
	}

	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, agentErr := s.agent.Health(healthCtx)

	return &pb.PipelineStatusResponse{
		Stages:  stages,
		AgentOk: agentErr == nil,
	}, nil
}

// ─────────────────────────────────────────────────────────────
// Internal helpers / 内部辅助方法
// ─────────────────────────────────────────────────────────────

// processTask simulates the scheduling pipeline stages.
// processTask 内部异步流水线执行器：
// 顺序流转 ingest ➔ fetch ➔ classify ➔ desensitize ➔ return ➔ audit 6 个阶段。
func (s *GRPCServer) processTask(task *store.Task, operation, payloadJSON string) {
	s.taskSem <- struct{}{}
	defer func() { <-s.taskSem }()

	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("processTask panic recovered",
				"task_id", task.ID, "panic", fmt.Sprintf("%v", r))
			task.Status = "failed"
			task.Error = fmt.Sprintf("internal panic: %v", r)
			now := time.Now()
			task.CompletedAt = &now
			task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
			_ = s.tasks.Update(task)
		}
	}()

	stages := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}

	for _, stage := range stages {
		task.Stage = stage
		task.Status = "running"
		now := time.Now()
		task.StartedAt = &now
		_ = s.tasks.Update(task)

		select {
		case <-time.After(100 * time.Millisecond):
		case <-s.ctx.Done():
			task.Status = "failed"
			task.Error = "server shutting down"
			now := time.Now()
			task.CompletedAt = &now
			task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
			_ = s.tasks.Update(task)
			return
		}

		// Stage 2: fetch → 若载荷为空，从数据源微服务拉取
		if stage == "fetch" && s.datasource != nil {
			if payloadJSON == "" || payloadJSON == "{}" || payloadJSON == "null" {
				ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
				if res, err := s.datasource.FetchDataBySource(ctx, task.Source, 10, 0); err == nil && len(res.Records) > 0 {
					b, _ := json.Marshal(res.Records)
					payloadJSON = string(b)
					task.PayloadJSON = payloadJSON
					_ = s.tasks.Update(task)
				}
				cancel()
			}
		}

		// Stage 3: classify → 调用 Agent 进行分类
		if stage == "classify" && operation == "classify" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Classify(ctx, payloadJSON)
			cancel()
			if err != nil {
				task.Status = "failed"
				task.Error = fmt.Sprintf("classify failed at stage %s: %v", stage, err)
				now := time.Now()
				task.CompletedAt = &now
				task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
				_ = s.tasks.Update(task)
				return
			}
		}

		// Stage 4: desensitize → 调用 Agent 执行脱敏
		if stage == "desensitize" && (operation == "mask" || operation == "k_anon" || operation == "dp") {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Mask(ctx, payloadJSON)
			cancel()
			if err != nil {
				task.Status = "failed"
				task.Error = fmt.Sprintf("desensitize failed at stage %s: %v", stage, err)
				now := time.Now()
				task.CompletedAt = &now
				task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
				_ = s.tasks.Update(task)
				return
			}
		}
	}

	task.Status = "completed"
	task.Stage = "done"
	now := time.Now()
	task.CompletedAt = &now
	task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
	_ = s.tasks.Update(task)
}

// taskToProto converts a store.Task domain model to its Protobuf representation.
// taskToProto 将领域实体 Task 转换为 gRPC Protobuf 传输对象 TaskProto。
func taskToProto(t *store.Task) *pb.TaskProto {
	proto := &pb.TaskProto{
		Id:         t.ID,
		Status:     t.Status,
		Stage:      t.Stage,
		Source:     t.Source,
		Operation:  t.Operation,
		CreatedAt:  t.CreatedAt.Format(time.RFC3339Nano),
		Error:      t.Error,
		DurationMs: t.DurationMs,
	}
	if t.StartedAt != nil {
		proto.StartedAt = t.StartedAt.Format(time.RFC3339Nano)
	}
	if t.CompletedAt != nil {
		proto.CompletedAt = t.CompletedAt.Format(time.RFC3339Nano)
	}
	return proto
}

// levelToPriority maps a sensitivity level (L1~L5) to a priority score.
func levelToPriority(level string) int {
	switch level {
	case "L5":
		return 100
	case "L4":
		return 80
	case "L3":
		return 60
	case "L2":
		return 40
	case "L1":
		return 10
	default:
		return 40
	}
}

// ─────────────────────────────────────────────────────────────
// mTLS Credentials Builder / mTLS 凭证构造
// ─────────────────────────────────────────────────────────────

// BuildServerCredentials constructs gRPC transport credentials supporting TLS 1.3, mTLS client auth, and public key pinning.
// BuildServerCredentials 根据配置构造 gRPC TLS 传输凭证：
// 1. 加载服务端证书与私钥，强制启用 TLS 1.3 最低版本；
// 2. 若配置了 TLSClientAuth，挂载根 CA 证书池，设置 RequireAndVerifyClientCert 双向认证策略；
// 3. 若配置了 TLSPinnedPubKeyFile，注入 VerifyPeerCertificate 验证钩子，严格比对客户端公钥指纹。
func BuildServerCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if !cfg.TLSEnabled {
		return nil, fmt.Errorf("TLS is disabled in configuration")
	}
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("TLS cert file and key file must be configured")
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server x509 key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	clientAuthMode := strings.ToLower(strings.TrimSpace(cfg.TLSClientAuth))
	if clientAuthMode != "" {
		if cfg.TLSCAFile == "" {
			return nil, fmt.Errorf("TLS CA file must be configured when client auth is enabled")
		}
		caPEM, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", cfg.TLSCAFile)
		}
		tlsConfig.ClientCAs = caPool

		switch clientAuthMode {
		case "require", "requireandverify":
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		case "verify":
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		case "request":
			tlsConfig.ClientAuth = tls.RequestClientCert
		default:
			return nil, fmt.Errorf("unknown TLS client auth mode: %s", cfg.TLSClientAuth)
		}
	}

	// 注入公钥固定校验器
	if cfg.TLSPinnedPubKeyFile != "" {
		pinnedKey, err := loadPublicKey(cfg.TLSPinnedPubKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load pinned client public key: %w", err)
		}
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("mTLS: client did not present a certificate")
			}
			peerCert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("mTLS: failed to parse peer certificate: %w", err)
			}
			if !publicKeysEqual(peerCert.PublicKey, pinnedKey) {
				return fmt.Errorf("mTLS: client public key does not match pinned key")
			}
			return nil
		}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// loadPublicKey loads a public key from PEM file (supports PKIX and X.509 Certificate formats).
// loadPublicKey 从 PEM 格式文件中解析并提取公钥对象。
func loadPublicKey(path string) (crypto.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in %s", path)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		cert, certErr := x509.ParseCertificate(block.Bytes)
		if certErr == nil {
			return cert.PublicKey, nil
		}
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return pub, nil
}

// publicKeysEqual checks if two public keys are identical (RSA, ECDSA, Ed25519).
// publicKeysEqual 深度比对两个公钥的数学属性（支持 RSA 模数/指数、ECDSA 椭圆曲线坐标与 Ed25519 字节）。
func publicKeysEqual(a, b crypto.PublicKey) bool {
	switch keyA := a.(type) {
	case *rsa.PublicKey:
		keyB, ok := b.(*rsa.PublicKey)
		if !ok {
			return false
		}
		return keyA.N.Cmp(keyB.N) == 0 && keyA.E == keyB.E
	case *ecdsa.PublicKey:
		keyB, ok := b.(*ecdsa.PublicKey)
		if !ok {
			return false
		}
		return keyA.X.Cmp(keyB.X) == 0 && keyA.Y.Cmp(keyB.Y) == 0 && keyA.Curve == keyB.Curve
	case ed25519.PublicKey:
		keyB, ok := b.(ed25519.PublicKey)
		if !ok {
			return false
		}
		return keyA.Equal(keyB)
	default:
		return false
	}
}

// ─────────────────────────────────────────────────────────────
// Interceptors / 拦截器
// ─────────────────────────────────────────────────────────────

// UnaryLoggingInterceptor logs method name, status code, latency, and client peer address.
// UnaryLoggingInterceptor 记录一元 RPC 调用的方法名、耗时、状态码与客户端来源 IP。
func UnaryLoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		var clientPeer string
		if p, ok := peer.FromContext(ctx); ok {
			clientPeer = p.Addr.String()
		}

		resp, err := handler(ctx, req)
		latency := time.Since(start)

		grpcCode := codes.OK
		if err != nil {
			if s, ok := status.FromError(err); ok {
				grpcCode = s.Code()
			} else {
				grpcCode = codes.Unknown
			}
		}

		logger.Info("gRPC request completed",
			"method", info.FullMethod,
			"code", grpcCode.String(),
			"latency_ms", latency.Milliseconds(),
			"peer", clientPeer,
			"module", "service-hub",
		)
		return resp, err
	}
}

// UnaryRecoveryInterceptor catches panics in unary RPCs and returns an Internal gRPC status error.
// UnaryRecoveryInterceptor 拦截一元 RPC Handler 的 Panic 并安全转换为 Internal gRPC 错误。
func UnaryRecoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("gRPC handler panic recovered",
					"method", info.FullMethod,
					"panic", fmt.Sprintf("%v", r),
					"module", "service-hub",
				)
				err = status.Errorf(codes.Internal, "internal server error: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}

// StreamLoggingInterceptor logs streaming RPC method completions.
// StreamLoggingInterceptor 记录流式 RPC 调用的执行耗时与完成状态。
func StreamLoggingInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		latency := time.Since(start)
		logger.Info("gRPC stream completed",
			"method", info.FullMethod,
			"latency_ms", latency.Milliseconds(),
			"error", err,
			"module", "service-hub",
		)
		return err
	}
}

// StreamRecoveryInterceptor catches panics in stream RPCs and logs the incident.
// StreamRecoveryInterceptor 拦截流式 RPC 中的 Panic 异常。
func StreamRecoveryInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("gRPC stream handler panic recovered",
					"method", info.FullMethod,
					"panic", fmt.Sprintf("%v", r),
					"module", "service-hub",
				)
				err = status.Errorf(codes.Internal, "internal server error: %v", r)
			}
		}()
		return handler(srv, ss)
	}
}

// BuildServerOptions creates server options configuring interceptor chains and transport credentials.
// BuildServerOptions 组装 gRPC 服务端选项链（Recovery -> Logging 拦截器与可选 TLS 凭证）。
func BuildServerOptions(logger *slog.Logger, creds credentials.TransportCredentials) []grpc.ServerOption {
	unaryChain := grpc.ChainUnaryInterceptor(
		UnaryRecoveryInterceptor(logger),
		UnaryLoggingInterceptor(logger),
	)
	streamChain := grpc.ChainStreamInterceptor(
		StreamRecoveryInterceptor(logger),
		StreamLoggingInterceptor(logger),
	)

	opts := []grpc.ServerOption{unaryChain, streamChain}
	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}
	return opts
}

// StartGRPCServer initializes and registers the ServiceHubService gRPC server instance.
// StartGRPCServer 快速装配并返回配置了 TLS 与拦截器的 grpc.Server 与 GRPCServer 实例。
func StartGRPCServer(ag *agent.Client, ds *datasource.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger) (*grpc.Server, *GRPCServer, error) {
	var creds credentials.TransportCredentials
	if cfg.TLSEnabled {
		var err error
		creds, err = BuildServerCredentials(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("build TLS credentials: %w", err)
		}
	}

	opts := BuildServerOptions(logger, creds)
	grpcServer := grpc.NewServer(opts...)
	serviceImpl := New(ag, ds, cfg, tasks, logger)
	pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)

	return grpcServer, serviceImpl, nil
}

// AllowedOperations returns a sorted list of supported hub operations.
// AllowedOperations 返回排序后的有效操作枚举列表。
func AllowedOperations() []string {
	ops := make([]string, len(validation.HubOperations))
	copy(ops, validation.HubOperations)
	sort.Strings(ops)
	return ops
}
