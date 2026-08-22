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

	mu      sync.RWMutex
	tasks   map[string]*models.Task
	taskSeq int
}

// New creates a new GRPCServer instance.
// New 创建一个新的 GRPCServer 实例。
func New(ag *agent.Client, cfg *config.Config) *GRPCServer {
	return &GRPCServer{
		agent:     ag,
		cfg:       cfg,
		startTime: time.Now(),
		tasks:     make(map[string]*models.Task),
	}
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active, queued, completed, failed int32
	for _, t := range s.tasks {
		switch t.Status {
		case "running":
			active++
		case "pending":
			queued++
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}

	return &pb.HubStatusResponse{
		Status:         "running",
		Uptime:         time.Since(s.startTime).Round(time.Second).String(),
		ActiveTasks:    active,
		QueuedTasks:    queued,
		CompletedTotal: completed,
		FailedTotal:    failed,
		AgentUrl:       s.cfg.AgentBaseURL(),
	}, nil
}

// Dispatch creates a new task and processes it through the pipeline.
// Dispatch 创建新任务并通过流水线处理。
func (s *GRPCServer) Dispatch(ctx context.Context, req *pb.DispatchRequest) (*pb.DispatchResponse, error) {
	if req.Source == "" || req.Operation == "" {
		return nil, status.Error(codes.InvalidArgument, "source and operation are required")
	}

	s.mu.Lock()
	s.taskSeq++
	taskID := fmt.Sprintf("grpc-task-%d-%d", s.startTime.Unix(), s.taskSeq)
	now := time.Now()
	task := &models.Task{
		ID:        taskID,
		Status:    "pending",
		Stage:     "queued",
		Source:    req.Source,
		Operation: req.Operation,
		CreatedAt: now,
	}
	s.tasks[taskID] = task
	s.mu.Unlock()

	// Process task asynchronously
	go s.processTask(task, req.Operation, req.PayloadJson)

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
	s.mu.Lock()
	s.taskSeq++
	taskID := fmt.Sprintf("grpc-task-%d-%d", s.startTime.Unix(), s.taskSeq)
	now := time.Now()
	task := &models.Task{
		ID:        taskID,
		Status:    "pending",
		Stage:     "queued",
		Source:    req.Source,
		Operation: operation,
		CreatedAt: now,
	}
	s.tasks[taskID] = task
	s.mu.Unlock()

	go s.processTask(task, operation, req.PayloadJson)

	// Serialize classify result
	resultJSON := fmt.Sprintf("%v", classifyResult)

	return &pb.ClassifyAndDispatchResponse{
		TaskId:            taskID,
		Level:             level,
		AutoOperation:     operation,
		ClassifyResultJson: resultJSON,
		Via:               moduleVia,
	}, nil
}

// GetTask returns a single task by ID.
// GetTask 根据 ID 返回单个任务。
func (s *GRPCServer) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.TaskProto, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}

	s.mu.RLock()
	task, ok := s.tasks[req.TaskId]
	s.mu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "task %s not found", req.TaskId)
	}

	return taskToProto(task), nil
}

// ListTasks returns all tasks, optionally filtered by status.
// ListTasks 返回所有任务，可选按状态过滤。
func (s *GRPCServer) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*pb.TaskProto, 0, len(s.tasks))
	for _, t := range s.tasks {
		if req.StatusFilter != "" && t.Status != req.StatusFilter {
			continue
		}
		tasks = append(tasks, taskToProto(t))
	}

	// Sort by creation time descending (newest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt > tasks[j].CreatedAt
	})

	return &pb.ListTasksResponse{
		Total: int32(len(tasks)),
		Tasks: tasks,
		Via:   moduleVia,
	}, nil
}

// PipelineStatus returns the status of each pipeline stage.
// PipelineStatus 返回调度流水线各阶段的状态。
func (s *GRPCServer) PipelineStatus(ctx context.Context, req *pb.PipelineStatusRequest) (*pb.PipelineStatusResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stageNames := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}
	stageCounts := make(map[string]int32)
	for _, t := range s.tasks {
		if t.Status == "running" {
			stageCounts[t.Stage]++
		}
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
func (s *GRPCServer) processTask(task *models.Task, operation, payloadJSON string) {
	stages := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}

	for _, stage := range stages {
		s.mu.Lock()
		task.Stage = stage
		task.Status = "running"
		now := time.Now()
		task.StartedAt = &now
		s.mu.Unlock()

		// Simulate stage processing time
		time.Sleep(100 * time.Millisecond)

		// Stage 3: classify → call agent if operation is classify
		if stage == "classify" && operation == "classify" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Classify(ctx, payloadJSON)
			cancel()
			if err != nil {
				s.mu.Lock()
				task.Status = "failed"
				task.Error = fmt.Sprintf("classify failed at stage %s: %v", stage, err)
				s.mu.Unlock()
				return
			}
		}

		// Stage 4: desensitize → call agent masking
		if stage == "desensitize" && (operation == "mask" || operation == "k_anon" || operation == "dp") {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Mask(ctx, payloadJSON)
			cancel()
			if err != nil {
				s.mu.Lock()
				task.Status = "failed"
				task.Error = fmt.Sprintf("desensitize failed at stage %s: %v", stage, err)
				s.mu.Unlock()
				return
			}
		}
	}

	s.mu.Lock()
	task.Status = "completed"
	task.Stage = "done"
	now := time.Now()
	task.CompletedAt = &now
	task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
	s.mu.Unlock()
}

// taskToProto converts a models.Task to pb.TaskProto.
// taskToProto 将 models.Task 转换为 pb.TaskProto。
func taskToProto(t *models.Task) *pb.TaskProto {
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
// levelToOperation 将敏感度等级映射为对应的脱敏操作。
func levelToOperation(level string) string {
	switch level {
	case "L1":
		return "none"
	case "L2":
		return "mask"
	case "L3":
		return "k_anon"
	case "L4":
		return "dp"
	case "L5":
		return "dp"
	default:
		return "mask"
	}
}

// ─────────────────────────────────────────────────────────────
// mTLS Credential Builder / mTLS 凭证构建
// ─────────────────────────────────────────────────────────────

// BuildServerCredentials constructs gRPC server credentials with mTLS.
// BuildServerCredentials 构建带 mTLS 双向认证的 gRPC 服务端凭证。
//
// Security layers / 安全层级：
//   1. TLS encryption: server cert + key for encrypted transport
//      TLS 加密：服务端证书 + 私钥，确保传输加密
//   2. Client certificate verification: CA cert to verify client identity
//      客户端证书校验：CA 证书校验客户端身份（mTLS 双向认证）
//   3. Public key pinning: optional pinned public key for extra security
//      公钥固定：可选的固定公钥，提供额外安全层
func BuildServerCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if !cfg.TLSEnabled {
		return nil, fmt.Errorf("TLS is not enabled")
	}

	// Load server certificate and key
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("server cert and key are required when TLS is enabled")
	}

	serverCert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	// Build TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
	}

	// Configure client authentication based on mode
	switch strings.ToLower(cfg.TLSClientAuth) {
	case "require", "verify":
		// mTLS: require and verify client certificate
		// mTLS：要求并验证客户端证书
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

		// Add public key pinning if configured
		// 如果配置了公钥固定，添加额外的公钥校验
		if cfg.TLSPinnedPubKeyFile != "" {
			tlsConfig.VerifyPeerCertificate = buildPublicKeyVerifier(cfg.TLSPinnedPubKeyFile)
		}

	case "":
		// No client authentication (server-only TLS)
		// 无客户端认证（仅服务端 TLS）
		tlsConfig.ClientAuth = tls.NoClientCert

	default:
		return nil, fmt.Errorf("unknown client auth mode: %s", cfg.TLSClientAuth)
	}

	return credentials.NewTLS(tlsConfig), nil
}

// buildPublicKeyVerifier creates a VerifyPeerCertificate callback that pins a specific public key.
// buildPublicKeyVerifier 创建一个 VerifyPeerCertificate 回调函数，用于固定特定的公钥。
//
// This provides an extra security layer beyond CA verification:
// even if a client has a valid certificate signed by the CA,
// it will be rejected unless its public key matches the pinned key.
//
// 这提供了超越 CA 验证的额外安全层：
// 即使客户端拥有由 CA 签发的有效证书，
// 除非其公钥与固定的公钥匹配，否则将被拒绝。
func buildPublicKeyVerifier(pinnedPubKeyFile string) func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// Load pinneded public key at startup
	// 在启动时加载固定的公钥
	pinnedKey, err := loadPublicKey(pinnedPubKeyFile)
	if err != nil {
		// If we can't load the key at startup, return a verifier that always fails
		// 如果启动时无法加载公钥，返回一个总是失败的验证器
		return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return fmt.Errorf("failed to load pinned public key: %v", err)
		}
	}

	// Return the verification callback
	// 返回验证回调函数
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("no client certificate provided")
		}

		// Parse the leaf certificate (first in chain)
		// 解析叶子证书（证书链中的第一个）
		cert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("failed to parse client certificate: %w", err)
		}

		// Compare public keys
		// 比较公钥
		if !publicKeysEqual(cert.PublicKey, pinnedKey) {
			return fmt.Errorf("client public key does not match pinned key")
		}

		return nil
	}
}

// loadPublicKey loads a public key from a PEM file.
// loadPublicKey 从 PEM 文件加载公钥。
func loadPublicKey(path string) (crypto.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try parsing as PKIX public key first
	// 首先尝试解析为 PKIX 公钥
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err == nil {
		return pub, nil
	}

	// Try parsing as certificate and extract public key
	// 尝试解析为证书并提取公钥
	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		return cert.PublicKey, nil
	}

	return nil, fmt.Errorf("failed to parse public key: not a valid PKIX public key or certificate")
}

// publicKeysEqual compares two public keys for equality.
// publicKeysEqual 比较两个公钥是否相等。
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
// Returns the server instance and a function to gracefully stop it.
// 返回服务器实例和优雅停止函数。
func StartGRPCServer(ag *agent.Client, cfg *config.Config) (*grpc.Server, error) {
	var opts []grpc.ServerOption

	// Configure TLS/mTLS if enabled
	// 如果启用则配置 TLS/mTLS
	if cfg.TLSEnabled {
		creds, err := BuildServerCredentials(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	// Create gRPC server
	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer(opts...)

	// Register service implementation
	// 注册服务实现
	serviceImpl := New(ag, cfg)
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
