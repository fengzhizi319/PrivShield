// Package handlers implements the HTTP REST interface for the service-hub module.
// Package handlers 实现数据服务调度中枢模块的 HTTP REST 接口层。
//
// Route list / 路由清单：
//
//	GET  /health              → Health check (self + upstream agent)
//	GET  /api/health          → Health check (self + upstream agent)
//	GET  /api/hub/status      → Scheduling hub status overview
//	GET  /api/hub/tasks       → List all tasks (with optional status filter)
//	POST /api/hub/dispatch    → Dispatch a new task to the pipeline
//	GET  /api/hub/pipeline    → Pipeline stages status
//	POST /api/hub/classify    → Classify + dispatch based on data sensitivity
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/validation"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/models"
)

const moduleVia = "service-hub"

// dispatchRequest is the common request shape used across Dispatch / ClassifyAndDispatch / processTask.
type dispatchRequest struct {
	Source    string `json:"source"`
	Operation string `json:"operation"`
	Payload   any    `json:"payload"`
	Priority  int    `json:"priority"`
}

// Server aggregates HTTP handler dependencies.
// Server 聚合 HTTP 处理器所需的全部依赖。
type Server struct {
	agent     *agent.Client
	cfg       *config.Config
	startTime time.Time
	tasks     store.TaskStore
	logger    *slog.Logger
	mc        *metrics.Collector
	taskSem   chan struct{} // P29 fix: semaphore to limit concurrent task processing goroutines
	ctx       context.Context    // P51 fix: parent context for graceful shutdown of task goroutines
	cancel    context.CancelFunc // P51 fix: cancel function to signal all task goroutines to stop
	wg        sync.WaitGroup     // P51 fix: wait group to track active task goroutines
}

// New creates a new Server instance.
// New 创建新的 Server 实例，tasks 为 nil 时自动回退到内存实现。
func New(ag *agent.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger, mc *metrics.Collector) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		agent:     ag,
		cfg:       cfg,
		startTime: time.Now(),
		tasks:     tasks,
		logger:    logger,
		mc:        mc,
		taskSem:   make(chan struct{}, 10), // P29: max 10 concurrent task goroutines
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Shutdown gracefully stops all in-flight task goroutines.
// P51 fix: call during server shutdown to cancel running processTask goroutines.
func (s *Server) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
// RegisterRoutes 在 Gin 引擎上注册全部 HTTP 路由，并注入共享中间件。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "service-hub"))
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/hub/status", s.HubStatus)
	r.GET("/api/hub/tasks", s.ListTasks)
	r.GET("/api/hub/tasks/:id", s.GetTask)
	r.POST("/api/hub/dispatch", s.Dispatch)
	r.GET("/api/hub/pipeline", s.Pipeline)
	r.POST("/api/hub/classify", s.ClassifyAndDispatch)
	r.GET("/metrics", s.mc.Handler())
}

// Health checks self + upstream agent connectivity.
// Health 检查自身与上游 agent 的连通性。
func (s *Server) Health(c *gin.Context) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agentData, err := s.agent.Health(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"backend":    "ok",
			"agent":      "unreachable",
			"agent_url":  s.cfg.AgentBaseURL(),
			"latency_ms": latency,
			"error":      err.Error(),
			"via":        moduleVia,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"backend":    "ok",
		"agent":      agentData,
		"agent_url":  s.cfg.AgentBaseURL(),
		"latency_ms": latency,
		"via":        moduleVia,
	})
}

// HubStatus returns the scheduling hub's current status.
// HubStatus 返回调度中枢的当前状态概览。
func (s *Server) HubStatus(c *gin.Context) {
	counts, err := s.tasks.Counts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "running",
		"uptime":          time.Since(s.startTime).Round(time.Second).String(),
		"active_tasks":    counts.Running,
		"queued_tasks":    counts.Pending,
		"completed_total": counts.Completed,
		"failed_total":    counts.Failed,
		"agent_url":       s.cfg.AgentBaseURL(),
	})
}

// GetTask returns a single task by ID.
// GetTask 根据 ID 返回单个任务的详情。
func (s *Server) GetTask(c *gin.Context) {
	id := c.Param("id")
	task, err := s.tasks.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"detail": fmt.Sprintf("task %s not found", id),
			"via":    moduleVia,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task": task,
		"via":  moduleVia,
	})
}

// ListTasks returns all tasks, optionally filtered by status query param.
// P17 fix: added pagination via limit/offset query params with safe defaults.
func (s *Server) ListTasks(c *gin.Context) {
	statusFilter := c.Query("status")

	// P52 fix: validate status filter to prevent meaningless queries.
	if statusFilter != "" {
		if err := validation.AllowedValues("status", statusFilter, validation.TaskStatuses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
	}

	// P61 fix: use shared ParsePagination helper instead of duplicated parsing logic.
	limit, offset := validation.ParsePagination(c, 100, 1000)

	tasks, total, err := s.tasks.List(store.TaskFilter{Status: statusFilter, Limit: limit, Offset: offset})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"tasks":  tasks,
		"via":    moduleVia,
	})
}

// Dispatch creates a new task and simulates pipeline processing.
// Dispatch 创建新任务并模拟流水线处理。
//
// Input validation / 输入校验：
//   - operation 必须在白名单内: mask / k_anon / dp / classify / none
//   - source 不得为空且长度不超过 1024 字符
func (s *Server) Dispatch(c *gin.Context) {
	var req dispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// Input validation / 输入校验
	if err := validation.NonEmpty("source", req.Source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validation.AllowedValues("operation", req.Operation, validation.HubOperations); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validation.MaxLength("source", req.Source, 1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	taskID := validation.GenerateID("task")
	now := time.Now()

	payloadJSON, _ := json.Marshal(req.Payload)
	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   req.Operation,
		Priority:    req.Priority,
		CreatedAt:   now,
		PayloadJSON: string(payloadJSON),
	}

	if err := s.tasks.Save(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	// P51 fix: track goroutine via WaitGroup for graceful shutdown.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, req)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"task_id": taskID,
		"status":  "accepted",
		"via":     moduleVia,
	})
}

// processTask simulates the scheduling pipeline stages.
// processTask 模拟调度流水线的各阶段处理。
//
// P19 fix: recover from panics to prevent goroutine crash from killing the process.
// 从 panic 中恢复，防止 goroutine 崩溃导致整个进程退出。
//
// Pipeline stages / 流水线阶段：
//
//	① 请求接入 → ② 申请原数 → ③ 分类分级 → ④ 下发脱敏 → ⑤ 返回结果 → ⑥ 存证写日志
func (s *Server) processTask(task *store.Task, req dispatchRequest) {
	// P29 fix: acquire semaphore slot to limit concurrent task goroutines
	// 获取信号量槽位以限制并发任务 goroutine 数量
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
		if stage == "classify" && req.Operation == "classify" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Classify(ctx, req.Payload)
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
		if stage == "desensitize" && (req.Operation == "mask" || req.Operation == "k_anon" || req.Operation == "dp") {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			record := toStringMap(req.Payload)
			var err error
			if len(record) > 0 {
				_, err = s.agent.MaskRecord(ctx, record)
			} else {
				_, err = s.agent.Mask(ctx, req.Payload)
			}
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

// Pipeline returns the status of each pipeline stage.
// Pipeline 返回调度流水线各阶段的状态。
func (s *Server) Pipeline(c *gin.Context) {
	// P36 fix: cap running tasks query to prevent OOM under high concurrency
	runningTasks, _, err := s.tasks.List(store.TaskFilter{Status: "running", Limit: 1000})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	stageNames := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}
	stageCounts := make(map[string]int)
	for _, t := range runningTasks {
		stageCounts[t.Stage]++
	}

	stages := make([]gin.H, 0, len(stageNames))
	for _, name := range stageNames {
		status := "idle"
		if stageCounts[name] > 0 {
			status = "processing"
		}
		stages = append(stages, gin.H{
			"name":         name,
			"status":       status,
			"active_count": stageCounts[name],
		})
	}

	// Check agent connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, agentErr := s.agent.Health(ctx)
	cancel()

	c.JSON(http.StatusOK, gin.H{
		"stages":   stages,
		"agent_ok": agentErr == nil,
	})
}

// ClassifyAndDispatch performs classification first, then auto-dispatches
// the appropriate desensitization operation based on the sensitivity level.
// ClassifyAndDispatch 先执行分类分级，再根据敏感度等级自动分发对应的脱敏操作。
//
// This is the key integration point with the desensitization module / 这是与分级脱敏模块的关键集成点：
//   - L1 (public): no masking needed
//   - L2 (internal): field-level masking
//   - L3 (confidential): record-level masking + K-anonymity
//   - L4 (secret): differential privacy
//   - L5 (top-secret): full anonymization + query obfuscation
func (s *Server) ClassifyAndDispatch(c *gin.Context) {
	var req struct {
		Source  string `json:"source" binding:"required"`
		Payload any    `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	if err := validation.MaxLength("source", req.Source, 1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	// Step 1: Call agent classification
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	classifyResult, err := s.agent.Classify(ctx, req.Payload)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"detail": fmt.Sprintf("classification failed: %v", err),
			"via":    moduleVia,
		})
		return
	}

	// Step 2: Determine operation based on classification level
	level := "L2"
	if lvl, ok := classifyResult["level"].(string); ok {
		level = lvl
	}

	operation := levelToOperation(level)

	// Step 3: Auto-dispatch the appropriate task
	taskID := validation.GenerateID("task")
	now := time.Now()

	payloadJSON, _ := json.Marshal(req.Payload)
	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      req.Source,
		Operation:   operation,
		Priority:    levelToPriority(level),
		CreatedAt:   now,
		PayloadJSON: string(payloadJSON),
	}

	if err := s.tasks.Save(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	dispatchReq := dispatchRequest{
		Source:    req.Source,
		Operation: operation,
		Payload:   req.Payload,
		Priority:  levelToPriority(level),
	}

	// P51 fix: track goroutine via WaitGroup for graceful shutdown.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, dispatchReq)
	}()

	c.JSON(http.StatusOK, gin.H{
		"task_id":         taskID,
		"classify_result": classifyResult,
		"auto_operation":  operation,
		"level":           level,
		"via":             moduleVia,
	})
}

// levelToOperation maps sensitivity level to desensitization operation.
// P50 fix: delegates to shared models.LevelToOperation to eliminate duplication.
func levelToOperation(level string) string {
	return models.LevelToOperation(level)
}

// levelToPriority maps sensitivity level to task priority.
// levelToPriority 将敏感度等级映射为任务优先级。
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

// toStringMap converts an arbitrary payload to map[string]string for the mask_record endpoint.
// Returns an empty map if the conversion is not possible.
// toStringMap 将任意 payload 转换为 map[string]string，供 mask_record 端点使用。
// 转换失败时返回空 map。
func toStringMap(payload any) map[string]string {
	result := make(map[string]string)
	m, ok := payload.(map[string]any)
	if !ok {
		return result
	}
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			result[k] = fmt.Sprintf("%v", val)
		case bool:
			result[k] = fmt.Sprintf("%v", val)
		default:
			b, err := json.Marshal(v)
			if err == nil {
				result[k] = string(b)
			}
		}
	}
	return result
}
