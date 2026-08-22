// Package handlers implements the HTTP REST interface for the service-hub module.
// Package handlers 实现数据服务调度中枢模块的 HTTP REST 接口层。
//
// Route list / 路由清单：
//
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
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/models"
)

const moduleVia = "service-hub"

// Server aggregates HTTP handler dependencies.
// Server 聚合 HTTP 处理器所需的全部依赖。
type Server struct {
	agent     *agent.Client
	cfg       *config.Config
	startTime time.Time

	mu       sync.RWMutex
	tasks    map[string]*models.Task
	taskSeq  int
}

// New creates a new Server instance.
func New(ag *agent.Client, cfg *config.Config) *Server {
	return &Server{
		agent:     ag,
		cfg:       cfg,
		startTime: time.Now(),
		tasks:     make(map[string]*models.Task),
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
// RegisterRoutes 在 Gin 引擎上注册全部 HTTP 路由。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(corsMiddleware())
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/hub/status", s.HubStatus)
	r.GET("/api/hub/tasks", s.ListTasks)
	r.POST("/api/hub/dispatch", s.Dispatch)
	r.GET("/api/hub/pipeline", s.Pipeline)
	r.POST("/api/hub/classify", s.ClassifyAndDispatch)
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active, queued, completed, failed int
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

	c.JSON(http.StatusOK, models.HubStatus{
		Status:         "running",
		Uptime:         time.Since(s.startTime).Round(time.Second).String(),
		ActiveTasks:    active,
		QueuedTasks:    queued,
		CompletedTotal: completed,
		FailedTotal:    failed,
		AgentURL:       s.cfg.AgentBaseURL(),
	})
}

// ListTasks returns all tasks, optionally filtered by status query param.
// ListTasks 返回所有任务，可选按 status 查询参数过滤。
func (s *Server) ListTasks(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusFilter := c.Query("status")
	tasks := make([]models.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		tasks = append(tasks, *t)
	}
	// Sort by creation time descending (newest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	c.JSON(http.StatusOK, models.TaskListResponse{
		Total: len(tasks),
		Tasks: tasks,
		Via:   moduleVia,
	})
}

// Dispatch creates a new task and simulates pipeline processing.
// Dispatch 创建新任务并模拟流水线处理。
//
// Integration with desensitization / 与分级脱敏模块集成：
//   1. Accept task with source + operation (mask/k_anon/dp/classify)
//   2. If operation is "classify", first call agent classification
//   3. Based on classification result (L1-L5), auto-select masking strategy
//   4. Forward to agent's masking endpoint
//   5. Record task lifecycle in memory store
func (s *Server) Dispatch(c *gin.Context) {
	var req models.DispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	s.mu.Lock()
	s.taskSeq++
	taskID := fmt.Sprintf("task-%d-%d", s.startTime.Unix(), s.taskSeq)
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

	// Simulate async pipeline processing
	go s.processTask(task, req)

	c.JSON(http.StatusAccepted, models.DispatchResponse{
		TaskID: taskID,
		Status: "accepted",
		Via:    moduleVia,
	})
}

// processTask simulates the scheduling pipeline stages.
// processTask 模拟调度流水线的各阶段处理。
//
// Pipeline stages / 流水线阶段：
//   ① 请求接入 → ② 申请原数 → ③ 分类分级 → ④ 下发脱敏 → ⑤ 返回结果 → ⑥ 存证写日志
func (s *Server) processTask(task *models.Task, req models.DispatchRequest) {
	stages := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}

	for i, stage := range stages {
		s.mu.Lock()
		task.Stage = stage
		task.Status = "running"
		now := time.Now()
		task.StartedAt = &now
		s.mu.Unlock()

		// Simulate stage processing time
		time.Sleep(100 * time.Millisecond)

		// Stage 3: classify → call agent if operation is classify
		if stage == "classify" && req.Operation == "classify" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := s.agent.Classify(ctx, req.Payload)
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
		if stage == "desensitize" && (req.Operation == "mask" || req.Operation == "k_anon" || req.Operation == "dp") {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			// Convert payload to record format for mask_record endpoint
			record := toStringMap(req.Payload)
			var err error
			if len(record) > 0 {
				_, err = s.agent.MaskRecord(ctx, record)
			} else {
				_, err = s.agent.Mask(ctx, req.Payload)
			}
			cancel()
			if err != nil {
				s.mu.Lock()
				task.Status = "failed"
				task.Error = fmt.Sprintf("desensitize failed at stage %s: %v", stage, err)
				s.mu.Unlock()
				return
			}
		}

		_ = i // avoid unused warning
	}

	s.mu.Lock()
	task.Status = "completed"
	task.Stage = "done"
	now := time.Now()
	task.CompletedAt = &now
	task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
	s.mu.Unlock()
}

// Pipeline returns the status of each pipeline stage.
// Pipeline 返回调度流水线各阶段的状态。
func (s *Server) Pipeline(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stageNames := []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"}
	stageCounts := make(map[string]int)
	for _, t := range s.tasks {
		if t.Status == "running" {
			stageCounts[t.Stage]++
		}
	}

	stages := make([]models.PipelineStage, 0, len(stageNames))
	for _, name := range stageNames {
		status := "idle"
		if stageCounts[name] > 0 {
			status = "processing"
		}
		stages = append(stages, models.PipelineStage{
			Name:        name,
			Status:      status,
			ActiveCount: stageCounts[name],
		})
	}

	// Check agent connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, agentErr := s.agent.Health(ctx)
	cancel()

	c.JSON(http.StatusOK, models.PipelineStatus{
		Stages:  stages,
		AgentOK: agentErr == nil,
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
	// Extract level from classification result (default L2 if not found)
	level := "L2"
	if lvl, ok := classifyResult["level"].(string); ok {
		level = lvl
	}

	operation := levelToOperation(level)

	// Step 3: Auto-dispatch the appropriate task
	dispatchReq := models.DispatchRequest{
		Source:    req.Source,
		Operation: operation,
		Payload:   req.Payload,
		Priority:  levelToPriority(level),
	}

	s.mu.Lock()
	s.taskSeq++
	taskID := fmt.Sprintf("task-%d-%d", s.startTime.Unix(), s.taskSeq)
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

	go s.processTask(task, dispatchReq)

	c.JSON(http.StatusOK, gin.H{
		"task_id":         taskID,
		"classify_result": classifyResult,
		"auto_operation":  operation,
		"level":           level,
		"via":             moduleVia,
	})
}

// levelToOperation maps sensitivity level to desensitization operation.
// levelToOperation 将敏感度等级映射为对应的脱敏操作。
func levelToOperation(level string) string {
	switch level {
	case "L1":
		return "none" // public data, no masking
	case "L2":
		return "mask" // field-level masking
	case "L3":
		return "k_anon" // K-anonymity
	case "L4":
		return "dp" // differential privacy
	case "L5":
		return "dp" // full anonymization + DP
	default:
		return "mask"
	}
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

// corsMiddleware returns a permissive CORS middleware.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
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
