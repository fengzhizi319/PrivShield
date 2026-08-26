// Package models 定义 App-LZ BFF 层所有数据结构的统一集合。
//
// 这些结构体在 BFF 的三个核心层之间传递：
//   - clients 层：从上游微服务获取原始数据，反序列化为这些模型
//   - handlers 层：将模型序列化为 JSON 返回给前端
//   - runner 层：E2E 测试执行时构造和校验这些模型
//
// 模型分组：
//   - 拓扑探测：ServiceNode, TopologyResponse
//   - 任务调度：DispatchRequest/Response, Task, TasksResponse
//   - Phase B 租约：LeasedTaskSummary, WorkerLeaseInfo, LeasedTasksResponse
//   - E2E 测试：TestSuiteAssertion, TestSuiteCase, RunTestSuiteRequest/Response
//   - 数据源：Datasource, DatasourceSliceResponse
//   - 审计：AuditLogItem, AuditVerifyResponse
//   - 预设数据 API：DataApiDef, DataApiInvokeRequest, DataApiSessionStage, DataApiSessionResponse
package models

import "time"

// ---------------------------------------------------------------------------
// 拓扑探测模型 —— 用于前端「服务拓扑大屏」展示 4 个上游微服务的实时状态
// ---------------------------------------------------------------------------

// ServiceNode 描述单个微服务的健康状态和连接信息。
// 每个节点同时探测 REST 和 gRPC 两种协议，前端可切换查看。
type ServiceNode struct {
	// 服务标识符，如 "service-hub", "engine", "datasource-mgr", "audit-log"
	ID       string `json:"id"`
	Name     string `json:"name"`      // 服务显示名称
	HTTPURL  string `json:"http_url"`  // REST 探测地址
	GRPCAddr string `json:"grpc_addr"` // gRPC 探测地址

	// 综合状态（取当前活跃协议的结果）
	Status    string  `json:"status"`     // "ready" | "unhealthy" | "unreachable"
	RTTMs     float64 `json:"rtt_ms"`     // 当前协议的往返延迟（毫秒）
	Protocol  string  `json:"protocol"`   // 当前活跃协议："rest" | "grpc"

	// REST 协议独立探测结果
	RESTStatus string  `json:"rest_status"` // REST 健康状态
	RESTRTTMs  float64 `json:"rest_rtt_ms"` // REST 往返延迟

	// gRPC 协议独立探测结果
	GRPCStatus string  `json:"grpc_status"` // gRPC 健康状态
	GRPCRTTMs  float64 `json:"grpc_rtt_ms"` // gRPC 往返延迟

	Version string         `json:"version"`          // 服务版本号
	Details map[string]any `json:"details,omitempty"` // 额外元数据（如 upstream_count）
	Error   string         `json:"error,omitempty"`   // 错误信息（仅异常时填充）
}

// TopologyResponse 是前端拓扑大屏的完整响应。
// 包含 4 个微服务节点的实时状态快照。
type TopologyResponse struct {
	Status         string        `json:"status"`          // 整体状态："healthy" | "degraded"
	ActiveProtocol string        `json:"active_protocol"` // 当前查看的协议视角
	Timestamp      string        `json:"timestamp"`       // 探测时间戳
	Services       []ServiceNode `json:"services"`        // 固定 4 个服务节点（Hub→Engine→Datasource→Audit）
}

// ---------------------------------------------------------------------------
// 任务调度模型 —— 用于前端「任务生命周期大屏」
// ---------------------------------------------------------------------------

// DispatchRequest 是手动派发任务的请求体。
// 前端通过此结构向 Service Hub 提交新的数据处理任务。
type DispatchRequest struct {
	Source    string         `json:"source" binding:"required"`    // 数据来源标识（如 "yibao", "kangyang"）
	Operation string         `json:"operation"`                    // 隐私操作类型："mask" | "dp" | "k_anon" | "qol" | "none"
	Payload   map[string]any `json:"payload" binding:"required"`   // 任务负载数据（原始记录）
	Priority  int            `json:"priority"`                     // 优先级（数值越小越优先）
}

// DispatchResponse 是任务派发后的响应。
type DispatchResponse struct {
	TaskID string `json:"task_id"`         // 新创建的任务 ID
	Status string `json:"status"`          // 任务初始状态："pending" | "accepted"
	Via    string `json:"via,omitempty"`   // 派发路径（如 "service-hub"）
	Error  string `json:"error,omitempty"` // 错误信息
}

// Task 表示 Service Hub 中的一个完整生命周期任务。
// 包含从创建到完成的所有元数据，以及 Phase B 的租约信息。
type Task struct {
	ID          string     `json:"id"`                     // 任务唯一标识
	Status      string     `json:"status"`                 // 状态机："pending" → "running" → "completed"/"failed"
	Stage       string     `json:"stage"`                  // 当前处理阶段（如 "masking", "dp", "audit"）
	Source      string     `json:"source"`                 // 数据来源
	Operation   string     `json:"operation"`              // 隐私操作类型
	Priority    int        `json:"priority"`               // 优先级
	CreatedAt   time.Time  `json:"created_at"`             // 任务创建时间
	StartedAt   *time.Time `json:"started_at,omitempty"`   // 开始执行时间（nullable）
	CompletedAt *time.Time `json:"completed_at,omitempty"` // 完成时间（nullable）
	DurationMs  int64      `json:"duration_ms"`            // 总耗时（毫秒）
	Error       string     `json:"error,omitempty"`        // 错误信息（仅 failed 时）
	PayloadJSON string     `json:"payload_json,omitempty"` // 原始负载的 JSON 字符串
	ResultJSON  string     `json:"result_json,omitempty"`  // 执行结果的 JSON 字符串
	RetryCount  int        `json:"retry_count"`            // 已重试次数

	// Phase B PostgreSQL 租约字段
	LeaseOwner     string     `json:"lease_owner,omitempty"`      // 租约持有者 Worker ID
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"` // 租约过期时间

	Via string `json:"via,omitempty"` // 数据来源标识（"live" / "fallback"）
}

// TasksResponse 是任务列表查询的响应，支持分页。
type TasksResponse struct {
	Total int    `json:"total"`         // 任务总数
	Tasks []Task `json:"tasks"`         // 当前页的任务列表
	Via   string `json:"via,omitempty"` // 数据来源标识
}

// ---------------------------------------------------------------------------
// Phase B 租约模型 —— 用于前端「租约检查器」展示 PostgreSQL 原子租约状态
// ---------------------------------------------------------------------------

// LeasedTaskSummary 保存单个任务的租约信息，用于 UI 展示。
type LeasedTaskSummary struct {
	TaskID                string  `json:"task_id"`                   // 任务 ID
	Stage                 string  `json:"stage"`                     // 当前处理阶段
	Priority              int     `json:"priority"`                  // 优先级
	LeaseExpiresInSeconds float64 `json:"lease_expires_in_seconds"`  // 租约剩余秒数（负数表示已过期）
}

// WorkerLeaseInfo 描述单个 Worker 当前持有的所有租约。
type WorkerLeaseInfo struct {
	WorkerID          string              `json:"worker_id"`           // Worker 唯一标识
	ClaimedTasksCount int                 `json:"claimed_tasks_count"` // 该 Worker 持有的任务数
	Tasks             []LeasedTaskSummary `json:"tasks"`               // 具体任务列表
}

// LeasedTasksResponse 是 Phase B PostgreSQL 租约检查的完整响应。
// 包含存储后端类型、所有租约任务、Worker 分组和孤儿任务恢复信息。
type LeasedTasksResponse struct {
	StoreBackend     string            `json:"store_backend"`      // 存储后端："postgresql" | "sqlite" | "memory"
	TotalLeasedTasks int               `json:"total_leased_tasks"` // 当前活跃租约总数
	Workers          []WorkerLeaseInfo `json:"workers"`            // 按 Worker 分组的租约信息
	OrphanRecovery   map[string]any    `json:"orphan_recovery"`    // 孤儿任务恢复状态（过期租约的自动回收）
}

// ---------------------------------------------------------------------------
// E2E 测试套件模型 —— 用于前端「测试运行器大屏」
// ---------------------------------------------------------------------------

// TestSuiteAssertion 表示测试用例中的单个断言结果。
type TestSuiteAssertion struct {
	Name     string `json:"name"`     // 断言名称（如 "审计链完整"）
	Expected string `json:"expected"` // 期望值
	Actual   string `json:"actual"`   // 实际值
	Passed   bool   `json:"passed"`   // 是否通过
}

// TestSuiteCase 表示一个完整的 E2E 测试场景。
// 当前实现 3 个套件：TS-01（审计验真）、TS-02（高并发压测）、TS-03（原子租约争抢）。
type TestSuiteCase struct {
	ID          string               `json:"id"`          // 套件编号："TS-01" | "TS-02" | "TS-03"
	Title       string               `json:"title"`       // 套件标题
	Description string               `json:"description"` // 详细描述
	Category    string               `json:"category"`    // 分类（如 "audit", "performance", "lease"）
	Status      string               `json:"status"`      // 执行状态："pending" | "running" | "passed" | "failed" | "skipped"
	DurationMs  float64              `json:"duration_ms"` // 执行耗时（毫秒）
	Error       string               `json:"error,omitempty"` // 错误信息
	Assertions  []TestSuiteAssertion `json:"assertions"`      // 断言结果列表
	Logs        []string             `json:"logs"`            // 执行日志行
}

// RunTestSuiteRequest 是执行测试套件的请求体。
type RunTestSuiteRequest struct {
	SuiteIDs          []string `json:"suite_ids"`                    // 要执行的套件 ID 列表
	Concurrency       int      `json:"concurrency"`                  // 并发数（TS-02 压测用）
	BenchmarkRequests int      `json:"benchmark_requests"`           // 压测请求数（TS-02 用）
}

// RunTestSuiteResponse 是测试执行完成后的响应。
type RunTestSuiteResponse struct {
	RunID       string          `json:"run_id"`                // 本次运行的唯一标识
	Status      string          `json:"status"`                // 整体状态："running" | "completed" | "failed"
	TotalCases  int             `json:"total_cases"`           // 用例总数
	PassedCases int             `json:"passed_cases"`          // 通过的用例数
	FailedCases int             `json:"failed_cases"`          // 失败的用例数
	StartedAt   string          `json:"started_at"`            // 开始时间
	CompletedAt string          `json:"completed_at,omitempty"` // 完成时间
	Results     []TestSuiteCase `json:"results"`               // 每个套件的详细结果
	Summary     map[string]any  `json:"summary,omitempty"`     // 额外汇总信息（如压测百分位数）
}

// ---------------------------------------------------------------------------
// 数据源模型 —— 用于前端「数据源浏览器」（旧版组件）
// ---------------------------------------------------------------------------

// Datasource 表示 datasource-mgr 中注册的一个模拟数据源。
type Datasource struct {
	ID           string   `json:"id"`                      // 数据源 ID（如 "ds_yibao", "ds_kangyang"）
	Name         string   `json:"name"`                    // 显示名称
	Category     string   `json:"category"`                // 类别（如 "medical", "health"）
	RecordsCount int      `json:"records_count"`           // 记录总数
	Fields       []string `json:"fields,omitempty"`        // 字段名列表
}

// DatasourceSliceResponse 包含从数据源采样的行数据。
type DatasourceSliceResponse struct {
	DatasourceID string           `json:"datasource_id"` // 数据源 ID
	Count        int              `json:"count"`         // 本次返回行数
	Total        int              `json:"total"`         // 数据源总行数
	Records      []map[string]any `json:"records"`       // 采样数据行
}

// ---------------------------------------------------------------------------
// 审计模型 —— 用于前端「审计验证大屏」
// ---------------------------------------------------------------------------

// AuditLogItem 表示 audit-log 服务中的一条审计记录。
type AuditLogItem struct {
	ID         string `json:"id"`         // 审计记录 ID
	Timestamp  string `json:"timestamp"`  // 记录时间戳
	TaskID     string `json:"task_id"`    // 关联的任务 ID
	Source     string `json:"source"`     // 数据来源
	Operation  string `json:"operation"`  // 执行的隐私操作
	DataHash   string `json:"data_hash"`  // 数据哈希（用于完整性校验）
	Operator   string `json:"operator"`   // 操作者标识
	Encryption string `json:"encryption"` // 加密方式
	Result     string `json:"result"`     // 操作结果
}

// AuditVerifyResponse 表示 Merkle 树验证的输出结果。
// 用于前端展示审计日志的不可篡改性验证。
type AuditVerifyResponse struct {
	MerkleValid  bool   `json:"merkle_valid"`            // Merkle 根校验是否通过
	RootHash     string `json:"root_hash"`               // Merkle 根哈希值
	TotalEntries int    `json:"total_entries"`           // 审计条目总数
	Timestamp    string `json:"timestamp"`               // 验证时间戳
	Signature    string `json:"signature,omitempty"`     // HMAC 签名
	Error        string `json:"error,omitempty"`         // 验证错误信息
}

// ---------------------------------------------------------------------------
// 预设数据 API 会话模型 —— 用于前端「预设数据 API 大屏」
// 描述 4 个预设 API 的定义、调用请求和完整的 4 阶段会话生命周期
// ---------------------------------------------------------------------------

// DataApiDef 描述一个预设数据 API 的定义。
// 当前有 4 个预设 API：医保结算、康养体征、隐私分类分级、审计链路验证。
type DataApiDef struct {
	ID           int      `json:"id"`                      // API 编号（1~4）
	Name         string   `json:"name"`                    // API 名称
	DatasourceID string   `json:"datasource_id"`           // 关联的数据源 ID
	Category     string   `json:"category"`                // 类别
	Description  string   `json:"description"`             // 详细描述
	Fields       []string `json:"fields"`                  // 涉及的字段列表
	Status       string   `json:"status"`                  // "active"（已启用）| "reserved"（预留）
}

// DataApiInvokeRequest 是调用预设数据 API 的请求体。
type DataApiInvokeRequest struct {
	ApiID int `json:"api_id" binding:"required,min=1,max=4"` // 目标 API 编号
	Limit int `json:"limit"`                                  // 返回记录数限制
}

// DataApiSessionStage 记录完整会话生命周期中的一个步骤。
// 预设数据 API 的完整流程为：fetch → classify → desensitize → audit。
type DataApiSessionStage struct {
	Name       string `json:"name"`                 // 阶段名："fetch" | "classify" | "desensitize" | "audit"
	Title      string `json:"title"`                // 阶段显示标题
	Status     string `json:"status"`               // 阶段状态："success" | "error" | "skipped"
	DurationMs int64  `json:"duration_ms"`          // 该阶段耗时（毫秒）
	Detail     string `json:"detail,omitempty"`     // 额外详情（如分类结果、脱敏字段数）
}

// DataApiSessionResponse 是预设数据 API 调用的完整会话结果。
// 包含原始数据、脱敏后数据、每个阶段的执行状态和总耗时。
type DataApiSessionResponse struct {
	SessionID     string                `json:"session_id"`               // 会话唯一 ID
	ApiID         int                   `json:"api_id"`                   // API 编号
	ApiName       string                `json:"api_name"`                 // API 名称
	Status        string                `json:"status"`                   // 整体状态："completed" | "partial" | "failed"
	RawRecords    []map[string]any      `json:"raw_records"`              // 从数据源获取的原始记录
	SanitizedData []map[string]any      `json:"sanitized_data"`           // 脱敏后的记录
	Stages        []DataApiSessionStage `json:"stages"`                   // 各阶段执行详情
	AuditEntryID  string                `json:"audit_entry_id,omitempty"` // 写入审计日志的条目 ID
	TotalDuration int64                 `json:"total_duration_ms"`        // 会话总耗时（毫秒）
	Error         string                `json:"error,omitempty"`          // 错误信息
}
