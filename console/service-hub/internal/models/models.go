// Package models defines shared data structures for the service-hub module.
// Package models 定义数据服务调度中枢模块的共享数据结构。
package models

import "time"

// HubStatus represents the scheduling hub's current status.
// HubStatus 表示调度中枢的当前状态。
type HubStatus struct {
	Status        string `json:"status"`         // "running" | "degraded" | "stopped"
	Uptime        string `json:"uptime"`         // Human-readable uptime / 可读运行时长
	ActiveTasks   int    `json:"active_tasks"`   // Currently running tasks / 当前运行任务数
	QueuedTasks   int    `json:"queued_tasks"`   // Tasks waiting in queue / 排队等待任务数
	CompletedTotal int   `json:"completed_total"` // Total completed tasks / 已完成任务总数
	FailedTotal   int    `json:"failed_total"`   // Total failed tasks / 已失败任务总数
	AgentURL      string `json:"agent_url"`      // Upstream agent URL / 上游 agent 地址
}

// Task represents a scheduling task in the pipeline.
// Task 表示流水线中的一个调度任务。
type Task struct {
	ID          string    `json:"id"`           // Unique task ID / 唯一任务 ID
	Status      string    `json:"status"`       // "pending" | "running" | "completed" | "failed"
	Stage       string    `json:"stage"`        // Current pipeline stage / 当前流水线阶段
	Source      string    `json:"source"`       // Data source name / 数据源名称
	Operation   string    `json:"operation"`    // Operation type (mask/k_anon/dp) / 操作类型
	CreatedAt   time.Time `json:"created_at"`   // Creation time / 创建时间
	StartedAt   *time.Time `json:"started_at"`  // Start time / 开始时间
	CompletedAt *time.Time `json:"completed_at"` // Completion time / 完成时间
	DurationMs  int64     `json:"duration_ms"`  // Duration in ms / 耗时（毫秒）
	Error       string    `json:"error,omitempty"` // Error message if failed / 失败时的错误信息
}

// TaskListResponse is the response for listing tasks.
// TaskListResponse 是任务列表查询的响应。
type TaskListResponse struct {
	Total  int    `json:"total"`   // Total task count / 任务总数
	Tasks  []Task `json:"tasks"`   // Task list / 任务列表
	Via    string `json:"via"`     // Module identifier / 模块标识
}

// DispatchRequest is the request body for dispatching a new task.
// DispatchRequest 是分发新任务的请求体。
type DispatchRequest struct {
	Source    string `json:"source" binding:"required"`    // Data source name / 数据源名称
	Operation string `json:"operation" binding:"required"` // Operation type / 操作类型
	Payload   any    `json:"payload"`                       // Task payload / 任务数据
	Priority  int    `json:"priority"`                      // Priority (higher = sooner) / 优先级
}

// DispatchResponse is the response after dispatching a task.
// DispatchResponse 是任务分发后的响应。
type DispatchResponse struct {
	TaskID string `json:"task_id"` // Assigned task ID / 分配的任务 ID
	Status string `json:"status"`  // "accepted" | "queued" | "rejected"
	Via    string `json:"via"`
}

// PipelineStage represents one stage in the scheduling pipeline.
// PipelineStage 表示调度流水线中的一个阶段。
type PipelineStage struct {
	Name       string `json:"name"`        // Stage name / 阶段名称
	Status     string `json:"status"`      // "idle" | "processing" | "error"
	ActiveCount int   `json:"active_count"` // Active tasks in this stage / 当前阶段活跃任务数
	AvgLatencyMs int64 `json:"avg_latency_ms"` // Average latency / 平均延迟
	Throughput  int   `json:"throughput"`   // Tasks per minute / 每分钟任务数
}

// PipelineStatus represents the full pipeline status.
// PipelineStatus 表示完整流水线状态。
type PipelineStatus struct {
	Stages    []PipelineStage `json:"stages"`     // Pipeline stages / 流水线各阶段
	TotalRPS  float64         `json:"total_rps"`  // Total requests per second / 总每秒请求数
	AgentOK   bool            `json:"agent_ok"`   // Upstream agent reachable / 上游 agent 可达
}

// ProxyResponse is the unified response wrapper.
// ProxyResponse 是统一响应包装结构。
type ProxyResponse struct {
	Status     int    `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Data       any    `json:"data"`
	Via        string `json:"via"`
}
