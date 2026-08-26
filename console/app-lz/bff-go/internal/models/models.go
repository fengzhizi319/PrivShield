package models

import "time"

// ServiceNode represents health and connectivity information for a single microservice.
type ServiceNode struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	HTTPURL    string         `json:"http_url"`
	GRPCAddr   string         `json:"grpc_addr"`
	Status     string         `json:"status"` // "ready" | "unhealthy" | "unreachable"
	RTTMs      float64        `json:"rtt_ms"`
	RESTStatus string         `json:"rest_status"`
	RESTRTTMs  float64        `json:"rest_rtt_ms"`
	GRPCStatus string         `json:"grpc_status"`
	GRPCRTTMs  float64        `json:"grpc_rtt_ms"`
	Protocol   string         `json:"protocol"` // "rest" | "grpc"
	Version    string         `json:"version"`
	Details    map[string]any `json:"details,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// TopologyResponse returns the live mesh topology for all 4 microservices.
type TopologyResponse struct {
	Status         string        `json:"status"`
	ActiveProtocol string        `json:"active_protocol"` // "rest" | "grpc"
	Timestamp      string        `json:"timestamp"`
	Services       []ServiceNode `json:"services"`
}

// DispatchRequest is the payload for manual task dispatch.
type DispatchRequest struct {
	Source    string         `json:"source" binding:"required"`
	Operation string         `json:"operation"` // "mask" | "dp" | "k_anon" | "qol" | "none"
	Payload   map[string]any `json:"payload" binding:"required"`
	Priority  int            `json:"priority"`
}

// DispatchResponse is returned after task dispatch.
type DispatchResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Via    string `json:"via,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Task represents a service-hub task with full lifecycle and lease metadata.
type Task struct {
	ID             string    `json:"id"`
	Status         string    `json:"status"` // "pending" | "running" | "completed" | "failed"
	Stage          string    `json:"stage"`
	Source         string    `json:"source"`
	Operation      string    `json:"operation"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DurationMs     int64     `json:"duration_ms"`
	Error          string    `json:"error,omitempty"`
	PayloadJSON    string    `json:"payload_json,omitempty"`
	ResultJSON     string    `json:"result_json,omitempty"`
	RetryCount     int       `json:"retry_count"`
	LeaseOwner     string    `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	Via            string    `json:"via,omitempty"`
}

// TasksResponse represents the paginated tasks response.
type TasksResponse struct {
	Total int    `json:"total"`
	Tasks []Task `json:"tasks"`
	Via   string `json:"via,omitempty"`
}

// LeasedTaskSummary holds lease information for UI display.
type LeasedTaskSummary struct {
	TaskID                string  `json:"task_id"`
	Stage                 string  `json:"stage"`
	Priority              int     `json:"priority"`
	LeaseExpiresInSeconds float64 `json:"lease_expires_in_seconds"`
}

// WorkerLeaseInfo holds tasks claimed by a specific worker.
type WorkerLeaseInfo struct {
	WorkerID          string              `json:"worker_id"`
	ClaimedTasksCount int                 `json:"claimed_tasks_count"`
	Tasks             []LeasedTaskSummary `json:"tasks"`
}

// LeasedTasksResponse represents Phase B PostgreSQL lease inspection details.
type LeasedTasksResponse struct {
	StoreBackend     string            `json:"store_backend"`
	TotalLeasedTasks int               `json:"total_leased_tasks"`
	Workers          []WorkerLeaseInfo `json:"workers"`
	OrphanRecovery   map[string]any    `json:"orphan_recovery"`
}

// TestSuiteAssertion represents an individual assertion result within a test case.
type TestSuiteAssertion struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}

// TestSuiteCase represents an E2E test scenario (TS-01 ~ TS-07).
type TestSuiteCase struct {
	ID          string               `json:"id"` // "TS-01" ~ "TS-07"
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Category    string               `json:"category"`
	Status      string               `json:"status"` // "pending" | "running" | "passed" | "failed" | "skipped"
	DurationMs  float64              `json:"duration_ms"`
	Error       string               `json:"error,omitempty"`
	Assertions  []TestSuiteAssertion `json:"assertions"`
	Logs        []string             `json:"logs"`
}

// RunTestSuiteRequest is the request to execute test suites.
type RunTestSuiteRequest struct {
	SuiteIDs          []string `json:"suite_ids"`
	Concurrency       int      `json:"concurrency"`
	BenchmarkRequests int      `json:"benchmark_requests"`
}

// RunTestSuiteResponse is returned after initiating test execution.
type RunTestSuiteResponse struct {
	RunID       string          `json:"run_id"`
	Status      string          `json:"status"` // "running" | "completed" | "failed"
	TotalCases  int             `json:"total_cases"`
	PassedCases int             `json:"passed_cases"`
	FailedCases int             `json:"failed_cases"`
	StartedAt   string          `json:"started_at"`
	CompletedAt string          `json:"completed_at,omitempty"`
	Results     []TestSuiteCase `json:"results"`
	Summary     map[string]any  `json:"summary,omitempty"`
}

// Datasource represents a registered simulated datasource in datasource-mgr.
type Datasource struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	RecordsCount int      `json:"records_count"`
	Fields       []string `json:"fields,omitempty"`
}

// DatasourceSliceResponse contains sampled rows from a datasource.
type DatasourceSliceResponse struct {
	DatasourceID string           `json:"datasource_id"`
	Count        int              `json:"count"`
	Total        int              `json:"total"`
	Records      []map[string]any `json:"records"`
}

// AuditLogItem represents an audit entry from audit-log.
type AuditLogItem struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	TaskID     string `json:"task_id"`
	Source     string `json:"source"`
	Operation  string `json:"operation"`
	DataHash   string `json:"data_hash"`
	Operator   string `json:"operator"`
	Encryption string `json:"encryption"`
	Result     string `json:"result"`
}

// AuditVerifyResponse represents Merkle tree verification output.
type AuditVerifyResponse struct {
	MerkleValid  bool   `json:"merkle_valid"`
	RootHash     string `json:"root_hash"`
	TotalEntries int    `json:"total_entries"`
	Timestamp    string `json:"timestamp"`
	Signature    string `json:"signature,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Preset Data API Session Models (4 预设数据 API)
// ---------------------------------------------------------------------------

// DataApiDef describes one of the 4 preset data APIs between service-hub and datasource-mgr.
type DataApiDef struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	DatasourceID string   `json:"datasource_id"`
	Category     string   `json:"category"`
	Description  string   `json:"description"`
	Fields       []string `json:"fields"`
	Status       string   `json:"status"` // "active" | "reserved"
}

// DataApiInvokeRequest is the payload to invoke a preset data API session.
type DataApiInvokeRequest struct {
	ApiID int `json:"api_id" binding:"required,min=1,max=4"`
	Limit int `json:"limit"`
}

// DataApiSessionStage records one step in the full session lifecycle.
type DataApiSessionStage struct {
	Name       string `json:"name"`        // "fetch" | "classify" | "desensitize" | "audit"
	Title      string `json:"title"`
	Status     string `json:"status"`      // "success" | "error" | "skipped"
	DurationMs int64  `json:"duration_ms"`
	Detail     string `json:"detail,omitempty"`
}

// DataApiSessionResponse is the full session result returned to the frontend.
type DataApiSessionResponse struct {
	SessionID     string                `json:"session_id"`
	ApiID         int                   `json:"api_id"`
	ApiName       string                `json:"api_name"`
	Status        string                `json:"status"` // "completed" | "partial" | "failed"
	RawRecords    []map[string]any      `json:"raw_records"`
	SanitizedData []map[string]any      `json:"sanitized_data"`
	Stages        []DataApiSessionStage `json:"stages"`
	AuditEntryID  string                `json:"audit_entry_id,omitempty"`
	TotalDuration int64                 `json:"total_duration_ms"`
	Error         string                `json:"error,omitempty"`
}
