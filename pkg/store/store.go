// Package store defines storage interfaces for console Go modules.
// Package store 为控制台各 Go 模块定义存储接口。
//
// 三个接口（TaskStore / DataSourceStore / AuditStore）分别对应
// service-hub / datasource-mgr / audit-log 的核心数据模型。
// 各模块可独立选择内存实现（开发）或 SQLite 实现（生产）。
package store

import "time"

// ─────────────────────────────────────────────────────────────
// Task Store (service-hub) / 任务存储
// ─────────────────────────────────────────────────────────────

// Task represents a scheduling task in the pipeline.
type Task struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`       // "pending" | "running" | "completed" | "failed"
	Stage       string     `json:"stage"`         // Current pipeline stage
	Source      string     `json:"source"`        // Data source name
	Operation   string     `json:"operation"`     // "mask" | "k_anon" | "dp" | "classify" | "none"
	Priority    int        `json:"priority"`      // Higher = sooner
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	DurationMs  int64      `json:"duration_ms"`
	Error       string     `json:"error,omitempty"`
	PayloadJSON string     `json:"-"`             // Raw payload (not exposed in JSON)
	RetryCount  int        `json:"retry_count"`   // Number of retry attempts (replaces fragile string matching)
	RetryAfter  *time.Time `json:"retry_after,omitempty"` // Earliest time for next retry (backoff delay)
}

// TaskFilter specifies filtering criteria for listing tasks.
type TaskFilter struct {
	Status string // Filter by status (empty = all)
	Limit  int    // Max results (0 = unlimited)
	Offset int    // Pagination offset
}

// TaskCounts holds aggregated task counts by status.
type TaskCounts struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// TaskStore defines the persistence interface for scheduling tasks.
type TaskStore interface {
	Save(task *Task) error
	Get(id string) (*Task, error)
	List(filter TaskFilter) ([]Task, int, error) // returns tasks, total count, error
	Update(task *Task) error
	Counts() (TaskCounts, error)
}

// ─────────────────────────────────────────────────────────────
// DataSource Store (datasource-mgr) / 数据源存储
// ─────────────────────────────────────────────────────────────

// DataSource represents a registered data source.
type DataSource struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`            // "database" | "api" | "file"
	Host          string     `json:"host"`
	Port          int        `json:"port"`
	Database      string     `json:"database"`
	SecurityLevel string     `json:"security_level"`  // "high" | "medium" | "low"
	Status        string     `json:"status"`          // "connected" | "disconnected" | "error"
	CreatedAt     time.Time  `json:"created_at"`
	LastCheckAt   *time.Time `json:"last_check_at"`
	TagsJSON      string     `json:"-"`               // JSON-encoded tags
	Tags          []string   `json:"tags"`            // Business tags
}

// AccessAuditRecord represents an access audit log entry.
type AccessAuditRecord struct {
	ID             string    `json:"id"`
	DataSourceID   string    `json:"datasource_id"`
	DataSourceName string    `json:"datasource_name"`
	Operation      string    `json:"operation"`
	User           string    `json:"user"`
	Timestamp      time.Time `json:"timestamp"`
	RecordsCount   int       `json:"records_count"`
	Status         string    `json:"status"`
}

// DataSourceFilter specifies filtering/pagination criteria for listing data sources.
// P28 fix: push pagination to SQL level instead of in-memory slicing.
type DataSourceFilter struct {
	Limit  int // Max results (0 = unlimited)
	Offset int // Pagination offset
}

// DataSourceStore defines the persistence interface for data sources.
type DataSourceStore interface {
	SaveDS(ds *DataSource) error
	GetDS(id string) (*DataSource, error)
	ListDS(filter DataSourceFilter) ([]DataSource, int, error) // returns datasources, total count, error
	DeleteDS(id string) error
	UpdateDS(ds *DataSource) error

	SaveAudit(rec *AccessAuditRecord) error
	ListAudit(dsID string, limit, offset int) ([]AccessAuditRecord, int, error) // returns records, total count, error
}

// ─────────────────────────────────────────────────────────────
// Audit Store (audit-log) / 审计日志存储
// ─────────────────────────────────────────────────────────────

// AuditLog represents a single audit log entry.
type AuditLog struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Operation     string    `json:"operation"`
	DataSource    string    `json:"datasource"`
	InputHash     string    `json:"input_hash"`
	OutputHash    string    `json:"output_hash"`
	Algorithm     string    `json:"algorithm"`
	ParametersJSON string   `json:"-"`
	Parameters    any       `json:"parameters"`
	InputRows     int       `json:"input_rows"`
	OutputRows    int       `json:"output_rows"`
	DurationMs    int64     `json:"duration_ms"`
	User          string    `json:"user"`
	Status        string    `json:"status"`
	ErrorMessage  string    `json:"error,omitempty"`
	SecurityLevel string    `json:"security_level"`
}

// AuditFilter specifies filtering criteria for listing audit logs.
type AuditFilter struct {
	Operation     string
	DataSource    string
	User          string
	Status        string
	SecurityLevel string
	Limit         int
	Offset        int
}

// SnapshotRecord represents a desensitization snapshot for evidence.
type SnapshotRecord struct {
	ID            string    `json:"id"`
	AuditLogID    string    `json:"audit_log_id"`
	Timestamp     time.Time `json:"timestamp"`
	InputSample   string    `json:"input_sample"`
	OutputSample  string    `json:"output_sample"`
	Algorithm     string    `json:"algorithm"`
	ParametersJSON string   `json:"-"`
	Parameters    any       `json:"parameters"`
	IntegrityHash string    `json:"integrity_hash"`
}

// AuditStats holds aggregated audit statistics.
// P31 fix: SQL-level aggregation instead of loading 10k records into memory.
type AuditStats struct {
	TotalOperations  int            `json:"total_operations"`
	ByOperation      map[string]int `json:"by_operation"`
	ByStatus         map[string]int `json:"by_status"`
	BySecurityLevel  map[string]int `json:"by_security_level"`
	AvgDurationMs    float64        `json:"avg_duration_ms"`
}

// AuditReport holds compliance audit report data.
// P33 fix: SQL-level filtering and aggregation instead of loading 10k records.
type AuditReport struct {
	TotalOperations   int            `json:"total_operations"`
	SuccessRate       float64        `json:"success_rate"`
	BySecurityLevel   map[string]int `json:"by_security_level"`
	TopOperations     []string       `json:"top_operations"`
	Recommendations   []string       `json:"recommendations"`
}

// AuditStore defines the persistence interface for audit logs and snapshots.
type AuditStore interface {
	SaveLog(log *AuditLog) error
	GetLog(id string) (*AuditLog, error)
	ListLogs(filter AuditFilter) ([]AuditLog, int, error)
	GetStats() (*AuditStats, error) // P31: SQL-level aggregation
	GenerateReport(period string) (*AuditReport, error) // P33: SQL-level filtering + aggregation

	SaveSnapshot(snap *SnapshotRecord) error
	ListSnapshots(limit, offset int) ([]SnapshotRecord, int, error) // P35: return total count for pagination
	GetSnapshot(id string) (*SnapshotRecord, error)
}
