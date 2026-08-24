// Package handlers implements the HTTP REST interface for the service-hub module.
// Package handlers 实现数据服务调度中枢模块（service-hub）的 HTTP REST 控制器与 6 阶段流水线调度逻辑。
//
// ==============================================================================
// 6 阶段数据流通与安全治理调度流水线 (6-Stage Governance Pipeline)：
// ==============================================================================
//
//	① 请求接入 (ingest)   ──▶ ② 申请原数 (fetch)    ──▶ ③ 分类分级 (classify)
//	       │                           │                           │
//	       ▼                           ▼                           ▼
//	④ 下发脱敏 (desensitize) ──▶ ⑤ 结果返回 (return)   ──▶ ⑥ 审计存证 (audit / done)
//
// 路由清单 (Route List)：
//   GET  /health                         → 自身与上下游健康检查探针 (Self + Upstream Agent + Datasource-Mgr)
//   GET  /api/health                     → 标准健康检查探针
//   GET  /api/hub/status                 → 调度中枢运行状态与队列深度概览
//   GET  /api/hub/tasks                  → 分页查询任务列表 (支持 status 状态过滤)
//   GET  /api/hub/tasks/:id              → 根据 TaskID 查询单个任务详情
//   POST /api/hub/dispatch               → 直接分发指定算子的隐私处理任务
//   GET  /api/hub/pipeline               → 6 阶段流水线监控遥测与 QPS 统计
//   POST /api/hub/classify               → 智能探查分类分级并根据等级（L1~L5）自动下发对应算子
//   POST /api/hub/pipeline/trigger-ds    → 从 datasource-mgr 自动抓取样本并全自动触发流水线
//   GET  /api/hub/datasources            → 代理查询 datasource-mgr 的全部数据源元数据
//   GET  /metrics                        → Prometheus 格式监控指标导出端点
// ==============================================================================

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

	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/validation"

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/datasource"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/models"
)

const moduleVia = "service-hub"

// dispatchRequest is the common request shape used across Dispatch / ClassifyAndDispatch / processTask.
// dispatchRequest 结构体表示任务提交与内部异步流转的标准入参载荷。
type dispatchRequest struct {
	Source    string `json:"source"`    // 数据源标识
	Operation string `json:"operation"` // 脱敏算子（mask/k_anon/dp/none）
	Payload   any    `json:"payload"`   // 原始记录数据
	Priority  int    `json:"priority"`  // 执行优先级
}

// Server aggregates HTTP handler dependencies.
// Server 结构体聚合 HTTP REST 控制器所需的全部核心依赖与并发控制资源。
type Server struct {
	agent      *agent.Client      // 上游 PrivShield Python Agent 客户端
	datasource *datasource.Client // 下游 datasource-mgr 数据源服务客户端
	cfg        *config.Config     // 模块全局运行配置
	startTime  time.Time          // 服务启动时间戳（用于计算 Uptime）
	tasks      store.TaskStore    // 任务持久化存储介质（SQLite 或内存实现）
	logger     *slog.Logger       // 结构化日志记录器
	mc         *metrics.Collector // Prometheus 监控指标收集器
	taskSem    chan struct{}      // 信号量通道，限制后台最大并发任务协程数（默认 10）
	ctx        context.Context    // 用于在服务停机时向所有在途任务协程广播取消信号的父 Context
	cancel     context.CancelFunc // 触发优雅停机 Context 取消的回调函数
	wg         sync.WaitGroup     // 等待组，跟踪记录正在执行的后台任务协程
}

// New creates a new Server instance.
// New 构造函数初始化 Server 实例，并默认分配容量为 10 的并发任务信号量与优雅停机上下文。
func New(ag *agent.Client, ds *datasource.Client, cfg *config.Config, tasks store.TaskStore, logger *slog.Logger, mc *metrics.Collector) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		agent:      ag,
		datasource: ds,
		cfg:        cfg,
		startTime:  time.Now(),
		tasks:      tasks,
		logger:     logger,
		mc:         mc,
		taskSem:    make(chan struct{}, 10), // 最大允许 10 个流水线任务并发异步执行
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Shutdown gracefully stops all in-flight task goroutines.
// Shutdown 优雅停机方法：通知所有正在执行的 processTask 任务协程安全退出，并阻塞等待全部协程完成。
func (s *Server) Shutdown() {
	s.cancel()
	s.wg.Wait()
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
// RegisterRoutes 在 Gin 路由引擎上挂载完整的中间件链与 REST API 端点。
// 中间件装配顺序：
// 1. RequestID: 自动注入链路追踪 X-Request-ID
// 2. StructuredLogger: 输出包含延迟、状态码、IP 的结构化 JSON/Text 日志
// 3. Recovery: 拦截 Handler Panic 并返回 500 JSON
// 4. SecurityHeaders: 注入 CSP、HSTS、X-Content-Type-Options 等安全防护头
// 5. MaxBodySize: 限制请求体最大 32 MiB，防御超大 Body 内存溢出
// 6. CORS: 跨域来源校验与预检放行
// 7. Auth: 基于 Authorization Bearer 的 API Key 鉴权校验
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "service-hub"))
	r.Use(middleware.Recovery(s.logger, "service-hub"))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(32 << 20)) // 32 MiB 请求体最大保护
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	// 基础健康检查与服务概览
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/hub/status", s.HubStatus)

	// 任务生命周期管理
	r.GET("/api/hub/tasks", s.ListTasks)
	r.GET("/api/hub/tasks/:id", s.GetTask)
	r.POST("/api/hub/dispatch", s.Dispatch)

	// 流水线与自适应分类调度
	r.GET("/api/hub/pipeline", s.Pipeline)
	r.POST("/api/hub/classify", s.ClassifyAndDispatch)

	// 模拟数据源联动与代理端点
	r.POST("/api/hub/pipeline/trigger-datasource", s.TriggerDataSourcePipeline)
	r.GET("/api/hub/datasources", s.ListDataSources)

	// Prometheus 监控指标导出
	r.GET("/metrics", s.mc.Handler())
}

// Health checks self + upstream agent + datasource-mgr connectivity.
// Health 组合健康检查探针：并发或快速校验自身、上游 PrivShield Agent 与下游 datasource-mgr 的健康状态。
func (s *Server) Health(c *gin.Context) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agentData, agentErr := s.agent.Health(ctx)
	latency := time.Since(start).Milliseconds()

	var dsStatus any = "ok"
	if s.datasource != nil {
		if dsData, dsErr := s.datasource.Health(ctx); dsErr != nil {
			dsStatus = "unreachable"
		} else if st, ok := dsData["status"]; ok {
			dsStatus = st
		}
	}

	if agentErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"backend":        "ok",
			"agent":          "unreachable",
			"agent_url":      s.cfg.AgentBaseURL(),
			"datasource":     dsStatus,
			"datasource_url": s.cfg.DatasourceBaseURL(),
			"latency_ms":     latency,
			"error":          agentErr.Error(),
			"via":            moduleVia,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"backend":        "ok",
		"agent":          agentData,
		"agent_url":      s.cfg.AgentBaseURL(),
		"datasource":     dsStatus,
		"datasource_url": s.cfg.DatasourceBaseURL(),
		"latency_ms":     latency,
		"via":            moduleVia,
	})
}

// HubStatus returns the scheduling hub's current status.
// HubStatus 返回调度中枢当前运行概览（Uptime、排队任务数、活跃任务数、累计成功/失败数）。
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
		"datasource_url":  s.cfg.DatasourceBaseURL(),
	})
}

// GetTask returns a single task by ID.
// GetTask 根据 TaskID 查询单个任务详情，若不存在则返回 404 Not Found。
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
// ListTasks 分页获取任务列表：
// 1. 校验 status 查询参数是否在有效值集合（pending/running/completed/failed）内；
// 2. 解析 limit/offset 分页参数并执行数据库查询；
// 3. 返回包含分页元数据和任务切片的响应。
func (s *Server) ListTasks(c *gin.Context) {
	statusFilter := c.Query("status")

	// 状态枚举白名单校验
	if statusFilter != "" {
		if err := validation.AllowedValues("status", statusFilter, validation.TaskStatuses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
	}

	// 解析分页安全边界（默认 100，最大 1000）
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
// Dispatch 接收用户显式提交的数据处理任务：
// 1. 绑定并校验 JSON 请求体（source 非空且限长、operation 在合法操作集合内）；
// 2. 生成全局唯一 TaskID，初始化为 pending/queued 状态并写入 TaskStore；
// 3. 异步拉起后台协程执行 6 阶段流水线调度（processTask）；
// 4. 立即返回 202 Accepted 包含 TaskID。
func (s *Server) Dispatch(c *gin.Context) {
	var req dispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// 字段合法性安全校验
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

	// 加入 WaitGroup 跟踪并拉起异步协程
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

// processTask executes the full 6-stage scheduling pipeline.
// processTask 完整驱动 6 阶段数据安全流通流水线执行：
//
// 流水线 6 阶段执行逻辑：
// ① 请求接入 (ingest)：更新状态为 running，初始化任务元数据；
// ② 申请原数 (fetch)：若 Payload 为空，自动向 datasource-mgr 发起远程抽样获取数据；
// ③ 分类分级 (classify)：若为动态分类任务，调用 PrivShield Agent /v1/dynclassification/eval_record 进行三层漏斗定级；
// ④ 下发脱敏 (desensitize)：根据算子类型（mask/k_anon/dp）调用 Agent 执行字段/记录级脱敏保护；
// ⑤ 结果返回 (return)：组装脱敏后的数据对象；
// ⑥ 审计存证 (audit/done)：记录执行耗时与完成状态并落盘存证。
func (s *Server) processTask(task *store.Task, req dispatchRequest) {
	// 并发信号量限流控制（最多 10 个并发任务）
	s.taskSem <- struct{}{}
	defer func() { <-s.taskSem }()

	// Panic 安全恢复，确保异常时任务被正确标记为 failed 并持久化
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

		// 检查优雅停机信号
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

		// 阶段 ②：申请原数 (fetch) ── 若载荷为空，自动向数据源微服务拉取数据
		if stage == "fetch" && s.datasource != nil {
			if req.Payload == nil || isEmptyPayload(req.Payload) {
				ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
				if res, err := s.datasource.FetchDataBySource(ctx, req.Source, 10, 0); err == nil && len(res.Records) > 0 {
					req.Payload = res.Records
					payloadBytes, _ := json.Marshal(req.Payload)
					task.PayloadJSON = string(payloadBytes)
					_ = s.tasks.Update(task)
				}
				cancel()
			}
		}

		// 阶段 ③：分类分级 (classify) ── 调用 Agent 三层漏斗评估
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

		// 阶段 ④：下发脱敏 (desensitize) ── 调用 Agent 执行脱敏算子
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

	// 阶段 ⑤/⑥ 顺利完成：标记任务为 completed 并计算端到端总耗时
	task.Status = "completed"
	task.Stage = "done"
	now := time.Now()
	task.CompletedAt = &now
	task.DurationMs = now.Sub(task.CreatedAt).Milliseconds()
	_ = s.tasks.Update(task)
}

// Pipeline returns the status of each pipeline stage.
// Pipeline 端点聚合当前 6 个阶段各自的活跃任务并发数、处理状态以及上游 Agent 连通性。
func (s *Server) Pipeline(c *gin.Context) {
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

	// 检测 Agent 连通性
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, agentErr := s.agent.Health(ctx)
	cancel()

	c.JSON(http.StatusOK, gin.H{
		"stages":   stages,
		"agent_ok": agentErr == nil,
	})
}

// ClassifyAndDispatch performs classification first, then auto-dispatches.
// ClassifyAndDispatch 智能分类定级分发端点：
// 1. 若 Payload 为空，自动从 datasource-mgr 抓取样本；
// 2. 调用 Agent Classify 接口完成动态分类定级；
// 3. 根据识别出的等级（如 L1/L2/L3/L4/L5）自动决策脱敏算子（none/mask/k_anon/dp）；
// 4. 生成任务并异步启动流水线，返回分类分级与自动决标决策结果。
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

	// 若载荷为空，自动从数据源服务拉取样本
	if (req.Payload == nil || isEmptyPayload(req.Payload)) && s.datasource != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if res, err := s.datasource.FetchDataBySource(ctx, req.Source, 5, 0); err == nil && len(res.Records) > 0 {
			req.Payload = res.Records[0]
		}
		cancel()
	}

	// 步骤 1：调用 Agent 进行动态分类定级
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

	// 步骤 2：根据分类级别映射脱敏算子
	level := "L2"
	if lvl, ok := classifyResult["level"].(string); ok {
		level = lvl
	}

	operation := levelToOperation(level)

	// 步骤 3：根据判定结果自动分发新任务
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

// TriggerDataSourcePipeline requests mock data from datasource-mgr, classifies and executes desensitization.
// TriggerDataSourcePipeline 联动触发端点：
// 指定 DataSourceID 后，service-hub 自动从 datasource-mgr 调取指定条数的数据，并以预设或自动算子触发 6 阶段流水线。
func (s *Server) TriggerDataSourcePipeline(c *gin.Context) {
	var req struct {
		DataSourceID string `json:"datasource_id" binding:"required"`
		Limit        int    `json:"limit"`
		Operation    string `json:"operation"` // 可选，默认为 "auto"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	if s.datasource == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "datasource client not configured", "via": moduleVia})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dsResp, err := s.datasource.FetchDataBySource(ctx, req.DataSourceID, req.Limit, 0)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"detail": fmt.Sprintf("fetch data from datasource-mgr failed: %v", err),
			"via":    moduleVia,
		})
		return
	}

	operation := req.Operation
	if operation == "" || operation == "auto" {
		operation = "mask"
	}

	taskID := validation.GenerateID("task")
	now := time.Now()
	payloadJSON, _ := json.Marshal(dsResp.Records)

	task := &store.Task{
		ID:          taskID,
		Status:      "pending",
		Stage:       "queued",
		Source:      dsResp.SourceName,
		Operation:   operation,
		Priority:    50,
		CreatedAt:   now,
		PayloadJSON: string(payloadJSON),
	}

	if err := s.tasks.Save(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	dispatchReq := dispatchRequest{
		Source:    dsResp.SourceName,
		Operation: operation,
		Payload:   dsResp.Records,
		Priority:  50,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.processTask(task, dispatchReq)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"task_id":       taskID,
		"datasource_id": req.DataSourceID,
		"records_count": len(dsResp.Records),
		"operation":     operation,
		"status":        "accepted",
		"via":           moduleVia,
	})
}

// ListDataSources proxies datasource list from datasource-mgr.
// ListDataSources 代理转发端点：从 datasource-mgr 透明拉取所有数据源资产清单。
func (s *Server) ListDataSources(c *gin.Context) {
	if s.datasource == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "datasource client not configured", "via": moduleVia})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	list, err := s.datasource.ListDataSources(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"detail": fmt.Sprintf("list datasources failed: %v", err), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, list)
}

// isEmptyPayload checks whether a generic payload is nil or empty.
// isEmptyPayload 检查通用 Payload 是否为空（nil、空字符串、空 JSON 对象或空切片）。
func isEmptyPayload(p any) bool {
	if p == nil {
		return true
	}
	switch v := p.(type) {
	case string:
		return v == "" || v == "{}" || v == "[]"
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case []map[string]any:
		return len(v) == 0
	}
	return false
}

// levelToOperation delegates to the shared models.LevelToOperation helper.
// levelToOperation 将敏感等级转换为脱敏操作算子。
func levelToOperation(level string) string {
	return models.LevelToOperation(level)
}

// levelToPriority calculates task execution priority based on sensitivity level.
// levelToPriority 根据敏感级别计算任务调度优先级（L5=100, L4=80, L3=60, L2=40, L1=10）。
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

// toStringMap converts a generic map[string]any or slice of maps to map[string]string for Agent masking calls.
// toStringMap 辅助函数：将通用的 map[string]any 或切片转换为单行记录的 map[string]string 字符串键值对。
func toStringMap(payload any) map[string]string {
	result := make(map[string]string)
	m, ok := payload.(map[string]any)
	if !ok {
		// 若 payload 为 map 切片，则提取第一条记录作为样本
		if slice, ok := payload.([]map[string]any); ok && len(slice) > 0 {
			m = slice[0]
		} else {
			return result
		}
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
