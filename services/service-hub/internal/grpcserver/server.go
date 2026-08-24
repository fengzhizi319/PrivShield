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
// GRPCServer 实现 ServiceHubService gRPC 服务。
type GRPCServer struct {
	pb.UnimplementedServiceHubServiceServer

	agent      *agent.Client
	datasource *datasource.Client
	cfg        *config.Config
	startTime  time.Time
	tasks      store.TaskStore
	logger     *slog.Logger
	taskSem    chan struct{}      // P29 fix: semaphore to limit concurrent task processing goroutines
	ctx        context.Context    // P51 fix: parent context for graceful shutdown of task goroutines
	cancel     context.CancelFunc // P51 fix: cancel function to signal all task goroutines to stop
	wg         sync.WaitGroup     // P51 fix: wait group to track active task goroutines
}

// New creates a new GRPCServer instance.
// New 创建一个新的 GRPCServer 实例。
func New(ag *agent.Client, ds *datasource.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger) *GRPCServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &GRPCServer{
		agent:      ag,
		datasource: ds,
		cfg:        cfg,
		startTime:  time.Now(),
		tasks:      tasks,
		logger:     logger,
		taskSem:    make(chan struct{}, 10), // P29: max 10 concurrent task goroutines
		ctx:        ctx,
		cancel:     cancel,
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
// HubStatus 返回调度中枢的状态概览。
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
// Dispatch 将新任务分发到调度流水线。
func (s *GRPCServer) Dispatch(ctx context.Context, req *pb.DispatchRequest) (*pb.DispatchResponse, error) {
	// Input validation / 输入校验
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

		// Stage 2: fetch → if payload is empty, attempt to fetch from datasource-mgr
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

func AllowedOperations() []string {
	ops := make([]string, len(validation.HubOperations))
	copy(ops, validation.HubOperations)
	sort.Strings(ops)
	return ops
}
