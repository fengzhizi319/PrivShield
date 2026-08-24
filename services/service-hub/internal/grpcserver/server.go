// Package grpcserver implements the gRPC service interface for service-hub with mTLS support.
// Package grpcserver 实现 service-hub 的 gRPC 服务接口，支持 mTLS 双向认证与公钥固定。
package grpcserver

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/validation"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/models"
	pb "github.com/fengzhizi319/PrivShield/console/service-hub/proto"
)

const moduleVia = "service-hub-grpc"

// GRPCServer implements the ServiceHubService gRPC service.
// GRPCServer 实现 ServiceHubService gRPC 服务。
type GRPCServer struct {
	pb.UnimplementedServiceHubServiceServer

	agent     *agent.Client
	cfg       *config.Config
	startTime time.Time
	tasks     store.TaskStore
	logger    *slog.Logger
	taskSem   chan struct{} // P29 fix: semaphore to limit concurrent task processing goroutines
	ctx       context.Context    // P51 fix: parent context for graceful shutdown of task goroutines
	cancel    context.CancelFunc // P51 fix: cancel function to signal all task goroutines to stop
	wg        sync.WaitGroup     // P51 fix: wait group to track active task goroutines
}

// New creates a new GRPCServer instance.
// New 创建一个新的 GRPCServer 实例。
func New(ag *agent.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger) *GRPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GRPCServer{
		agent:     ag,
		cfg:       cfg,
		startTime: time.Now(),
		tasks:     tasks,
		logger:    logger,
		taskSem:   make(chan struct{}, 10), // P29: max 10 concurrent task goroutines
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Shutdown gracefully stops all in-flight task goroutines.
// P51 fix: call during server shutdown to cancel running processTask goroutines.
func (s *GRPCServer) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// ─────────────────────────────────────────────────────────────
// gRPC Service Methods / gRPC 服务方法
// ─────────────────────────────────────────────────────────────

// Health checks self + upstream agent connectivity.
// Health 检查自身与上游 agent 的连通性。
func (s *GRPCServer) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	start := time.Now()
	healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	agentData, err := s.agent.Health(healthCtx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return &pb.HealthResponse{
			Backend:    "ok",
			Agent:      "unreachable",
			AgentUrl:   s.cfg.AgentBaseURL(),
			LatencyMs:  latency,
			Error:      err.Error(),
			Via:        moduleVia,
		}, nil
	}

	agentStr := fmt.Sprintf("%v", agentData["status"])
	return &pb.HealthResponse{
		Backend:    "ok",
		Agent:      agentStr,
		AgentUrl:   s.cfg.AgentBaseURL(),
		LatencyMs:  latency,
		Via:        moduleVia,
	}, nil
}

// HubStatus returns the scheduling hub's current status.
// HubStatus 返回调度中枢的当前状态概览。
func (s *GRPCServer) HubStatus(ctx context.Context, req *pb.HubStatusRequest) (*pb.HubStatusResponse, error) {
	counts, err := s.tasks.Counts()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get task counts: %v", err)
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

// Dispatch creates a new task and processes it through the pipeline.
// Dispatch 创建新任务并通过流水线处理。
func (s *GRPCServer) Dispatch(ctx context.Context, req *pb.DispatchRequest) (*pb.DispatchResponse, error) {
	if req.Source == "" || req.Operation == "" {
		return nil, status.Error(codes.InvalidArgument, "source and operation are required")
	}
	// P54 fix: validate source length to prevent storage exhaustion, aligned with HTTP handler.
	if len(req.Source) > 1024 {
		return nil, status.Error(codes.InvalidArgument, "source must not exceed 1024 characters")
	}

	taskID := validation.GenerateID("grpc-task")
	now := time.Now()
	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   req.Operation,
		CreatedAt:   now,
		PayloadJSON: req.PayloadJson,
	}

	if err := s.tasks.Save(task); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save task: %v", err)
	}

	// Process task asynchronously
	// P51 fix: track goroutine via WaitGroup for graceful shutdown.
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

// ClassifyAndDispatch performs classification first, then auto-dispatches.
// ClassifyAndDispatch 先执行分类分级，再根据敏感度自动分发。
func (s *GRPCServer) ClassifyAndDispatch(ctx context.Context, req *pb.ClassifyAndDispatchRequest) (*pb.ClassifyAndDispatchResponse, error) {
	if req.Source == "" {
		return nil, status.Error(codes.InvalidArgument, "source is required")
	}
	// P49 fix: validate source length to prevent storage exhaustion, aligned with HTTP handler.
	if len(req.Source) > 1024 {
		return nil, status.Error(codes.InvalidArgument, "source must not exceed 1024 characters")
	}

	// Call agent classification
	classifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	classifyResult, err := s.agent.Classify(classifyCtx, req.PayloadJson)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "classification failed: %v", err)
	}

	// Determine operation based on classification level
	level := "L2"
	if lvl, ok := classifyResult["level"].(string); ok {
		level = lvl
	}

	operation := levelToOperation(level)

	// Auto-dispatch the appropriate task
	taskID := validation.GenerateID("grpc-task")
	now := time.Now()
	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   operation,
		CreatedAt:   now,
		PayloadJSON: req.PayloadJson,
	}

	if err := s.tasks.Save(task); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save task: %v", err)
	}

	// P51 fix: track goroutine via WaitGroup for graceful shutdown.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, operation, req.PayloadJson)
	}()

	// Serialize classify result
	resultJSON := fmt.Sprintf("%v", classifyResult)

	return &pb.ClassifyAndDispatchResponse{
		TaskId:               taskID,
		Level:                level,
		AutoOperation:        operation,
		ClassifyResultJson:   resultJSON,
		Via:                  moduleVia,
	}, nil
}

// GetTask returns a single task by ID.
// GetTask 根据 ID 返回单个任务。
func (s *GRPCServer) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.TaskProto, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}

	task, err := s.tasks.Get(req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "task %s not found", req.TaskId)
	}

	return taskToProto(task), nil
}

// ListTasks returns all tasks, optionally filtered by status.
// P20 fix: added pagination via limit/offset with safe defaults, aligned with REST API.
// ListTasks 返回所有任务，可选按状态过滤。已添加分页保护，与 REST API 对齐。
func (s *GRPCServer) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	// P52 fix: validate status filter to prevent meaningless queries.
	if req.StatusFilter != "" {
		if err := validation.AllowedValues("status", req.StatusFilter, validation.TaskStatuses); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	// P20 fix: apply safe pagination limits (server-side defaults, proto fields not yet available)
	// 应用安全分页限制（服务端默认值，proto 字段待 protoc 可用后补齐）
	limit := 100
	offset := 0

	tasks, total, err := s.tasks.List(store.TaskFilter{Status: req.StatusFilter, Limit: limit, Offset: offset})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	protos := make([]*pb.TaskProto, 0, len(tasks))
	for i := range tasks {
		protos = append(protos, taskToProto(&tasks[i]))
	}

	// Sort by creation time descending (newest first)
	sort.Slice(protos, func(i, j int) bool {
		return protos[i].CreatedAt > protos[j].CreatedAt
	})

	return &pb.ListTasksResponse{
		Total: int32(total),
		Tasks: protos,
		Via:   moduleVia,
	}, nil
}

// PipelineStatus returns the status of each pipeline stage.
// PipelineStatus 返回调度流水线各阶段的状态。
func (s *GRPCServer) PipelineStatus(ctx context.Context, req *pb.PipelineStatusRequest) (*pb.PipelineStatusResponse, error) {
	// P36 fix: cap running tasks query to prevent OOM under high concurrency
	runningTasks, _, err := s.tasks.List(store.TaskFilter{Status: "running", Limit: 1000})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	stageNames := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}
	stageCounts := make(map[string]int32)
	for _, t := range runningTasks {
		stageCounts[t.Stage]++
	}

	stages := make([]*pb.PipelineStageProto, 0, len(stageNames))
	for _, name := range stageNames {
		statusStr := "idle"
		if stageCounts[name] > 0 {
			statusStr = "processing"
		}
		stages = append(stages, &pb.PipelineStageProto{
			Name:        name,
			Status:      statusStr,
			ActiveCount: stageCounts[name],
		})
	}

	// Check agent connectivity
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
// processTask 模拟调度流水线的各阶段处理。
//
// P19 fix: recover from panics to prevent goroutine crash from killing the process.
// 从 panic 中恢复，防止 goroutine 崩溃导致整个进程退出。
func (s *GRPCServer) processTask(task *store.Task, operation, payloadJSON string) {
	// P29 fix: acquire semaphore slot to limit concurrent task goroutines
	s.taskSem <- struct{}{}
	defer func() { <-s.taskSem }()

	// P19 fix: panic recovery — mark task as failed instead of crashing the process
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

		// P51 fix: use select with context instead of plain time.Sleep,
		// so goroutine can be cancelled during shutdown.
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

		// Stage 3: classify → call agent if operation is classify
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

		// Stage 4: desensitize → call agent masking
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

// taskToProto converts a store.Task to pb.TaskProto.
// taskToProto 将 store.Task 转换为 pb.TaskProto。
func taskToProto(t *store.Task) *pb.TaskProto {
	proto := &pb.TaskProto{
		Id:         t.ID,
		Status:     t.Status,
		Stage:      t.Stage,
		Source:     t.Source,
		Operation:  t.Operation,
		CreatedAt:  t.CreatedAt.Format(time.RFC3339),
		DurationMs: t.DurationMs,
		Error:      t.Error,
	}
	if t.StartedAt != nil {
		proto.StartedAt = t.StartedAt.Format(time.RFC3339)
	}
	if t.CompletedAt != nil {
		proto.CompletedAt = t.CompletedAt.Format(time.RFC3339)
	}
	return proto
}

// levelToOperation maps sensitivity level to desensitization operation.
// P50 fix: delegates to shared models.LevelToOperation to eliminate duplication.
func levelToOperation(level string) string {
	return models.LevelToOperation(level)
}

// ─────────────────────────────────────────────────────────────
// mTLS Credential Builder / mTLS 凭证构建
// ─────────────────────────────────────────────────────────────

// BuildServerCredentials constructs gRPC server credentials with mTLS.
// BuildServerCredentials 构建带 mTLS 双向认证的 gRPC 服务端凭证。
//
// Security layers / 安全层级：
//  1. TLS encryption: server cert + key for encrypted transport
//  2. Client certificate verification: CA cert to verify client identity
//  3. Public key pinning: optional pinned public key for extra security
func BuildServerCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if !cfg.TLSEnabled {
		return nil, fmt.Errorf("TLS is not enabled")
	}

	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("server cert and key are required when TLS is enabled")
	}

	serverCert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
	}

	switch strings.ToLower(cfg.TLSClientAuth) {
	case "require", "verify":
		if cfg.TLSCAFile == "" {
			return nil, fmt.Errorf("CA certificate is required for client verification")
		}

		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert

		if cfg.TLSPinnedPubKeyFile != "" {
			tlsConfig.VerifyPeerCertificate = buildPublicKeyVerifier(cfg.TLSPinnedPubKeyFile)
		}

	case "":
		tlsConfig.ClientAuth = tls.NoClientCert

	default:
		return nil, fmt.Errorf("unknown client auth mode: %s", cfg.TLSClientAuth)
	}

	return credentials.NewTLS(tlsConfig), nil
}

// buildPublicKeyVerifier creates a VerifyPeerCertificate callback that pins a specific public key.
func buildPublicKeyVerifier(pinnedPubKeyFile string) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	pinnedKey, err := loadPublicKey(pinnedPubKeyFile)
	if err != nil {
		return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return fmt.Errorf("failed to load pinned public key: %v", err)
		}
	}

	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("no client certificate provided")
		}

		cert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("failed to parse client certificate: %w", err)
		}

		if !publicKeysEqual(cert.PublicKey, pinnedKey) {
			return fmt.Errorf("client public key does not match pinned key")
		}

		return nil
	}
}

// loadPublicKey loads a public key from a PEM file.
func loadPublicKey(path string) (crypto.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		return pub, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		return cert.PublicKey, nil
	}

	return nil, fmt.Errorf("failed to parse public key: not a valid PKIX public key or certificate")
}

// publicKeysEqual compares two public keys for equality.
func publicKeysEqual(a, b crypto.PublicKey) bool {
	switch aKey := a.(type) {
	case *rsa.PublicKey:
		bKey, ok := b.(*rsa.PublicKey)
		if !ok {
			return false
		}
		return aKey.N.Cmp(bKey.N) == 0 && aKey.E == bKey.E

	case *ecdsa.PublicKey:
		bKey, ok := b.(*ecdsa.PublicKey)
		if !ok {
			return false
		}
		return aKey.Curve == bKey.Curve && aKey.X.Cmp(bKey.X) == 0 && aKey.Y.Cmp(bKey.Y) == 0

	case ed25519.PublicKey:
		bKey, ok := b.(ed25519.PublicKey)
		if !ok {
			return false
		}
		return aKey.Equal(bKey)

	default:
		return false
	}
}

// ─────────────────────────────────────────────────────────────
// Server Lifecycle / 服务器生命周期
// ─────────────────────────────────────────────────────────────

// StartGRPCServer creates and starts the gRPC server with optional mTLS.
// StartGRPCServer 创建并启动 gRPC 服务器，可选 mTLS。
//
// The shared taskStore is used for persistence (replaces in-memory map).
// 共享 taskStore 用于持久化（替代原内存 map）。
func StartGRPCServer(ag *agent.Client, cfg *config.Config, taskStore store.TaskStore, logger *slog.Logger) (*grpc.Server, error) {
	var opts []grpc.ServerOption

	if cfg.TLSEnabled {
		creds, err := BuildServerCredentials(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)
	serviceImpl := New(ag, cfg, taskStore, logger)
	pb.RegisterServiceHubServiceServer(grpcServer, serviceImpl)

	return grpcServer, nil
}

// GetPeerCertificate extracts the client certificate from gRPC context (for logging/auditing).
// GetPeerCertificate 从 gRPC 上下文中提取客户端证书（用于日志/审计）。
func GetPeerCertificate(ctx context.Context) *x509.Certificate {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 {
		return nil
	}

	return tlsInfo.State.VerifiedChains[0][0]
}
