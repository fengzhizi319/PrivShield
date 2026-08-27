// Package models defines shared data structures for the audit-log module.
package models

import "time"

// AuditLog represents a single audit log entry.
type AuditLog struct {
	ID            string    `json:"id"`                        // Unique log ID
	TaskID        string    `json:"task_id,omitempty"`         // Associated pipeline task ID
	APICode       string    `json:"api_code,omitempty"`        // Canonical API code (e.g. "api1_yibao")
	DatasourceID  string    `json:"datasource_id,omitempty"`   // Canonical datasource ID (e.g. "ds_yibao")
	Timestamp     time.Time `json:"timestamp"`                 // When the operation occurred
	Operation     string    `json:"operation"`                 // "mask" | "classify" | "k_anon" | "dp" | "qol"
	DataSource    string    `json:"datasource"`                // Source data identifier
	InputHash     string    `json:"input_hash"`                // SHA256 hash of input data
	OutputHash    string    `json:"output_hash"`               // SHA256 hash of output data
	Algorithm     string    `json:"algorithm"`                 // Algorithm used (e.g., "field_mask", "k_anonymity")
	Parameters    any       `json:"parameters"`                // Algorithm parameters
	InputRows     int       `json:"input_rows"`                // Number of input rows
	OutputRows    int       `json:"output_rows"`               // Number of output rows
	DurationMs    int64     `json:"duration_ms"`               // Processing duration
	User          string    `json:"user"`                      // Who performed the operation
	Status        string    `json:"status"`                    // "success" | "failed"
	ErrorMessage  string    `json:"error,omitempty"`           // Error message if failed
	SecurityLevel string    `json:"security_level"`            // L1-L5 classification level
}

// AuditLogListResponse is the response for listing audit logs.
type AuditLogListResponse struct {
	Total int        `json:"total"`
	Logs  []AuditLog `json:"logs"`
	Via   string     `json:"via"`
}

// AuditLogQueryRequest is the request for querying audit logs.
type AuditLogQueryRequest struct {
	TaskID        string     `json:"task_id"`
	APICode       string     `json:"api_code"`
	DatasourceID  string     `json:"datasource_id"`
	StartTime     *time.Time `json:"start_time"`
	EndTime       *time.Time `json:"end_time"`
	Operation     string     `json:"operation"`
	DataSource    string     `json:"datasource"`
	User          string     `json:"user"`
	Status        string     `json:"status"`
	SecurityLevel string     `json:"security_level"`
	Limit         int        `json:"limit"`
}

// AuditStats represents aggregated audit statistics.
type AuditStats struct {
	TotalOperations int            `json:"total_operations"`
	ByOperation     map[string]int `json:"by_operation"`
	ByStatus        map[string]int `json:"by_status"`
	BySecurityLevel map[string]int `json:"by_security_level"`
	AvgDurationMs   float64        `json:"avg_duration_ms"`
	Period          string         `json:"period"` // "1h" | "24h" | "7d" | "30d"
}

// SnapshotRecord represents a desensitization snapshot for evidence.
type SnapshotRecord struct {
	ID            string    `json:"id"`
	AuditLogID    string    `json:"audit_log_id"`
	Timestamp     time.Time `json:"timestamp"`
	InputSample   string    `json:"input_sample"`   // Sample of input data (truncated)
	OutputSample  string    `json:"output_sample"`  // Sample of output data (truncated)
	Algorithm     string    `json:"algorithm"`
	Parameters    any       `json:"parameters"`
	IntegrityHash string    `json:"integrity_hash"` // SHA256 hash for integrity verification
}

// SnapshotListResponse is the response for listing snapshots.
type SnapshotListResponse struct {
	Total     int              `json:"total"`
	Snapshots []SnapshotRecord `json:"snapshots"`
	Via       string           `json:"via"`
}

// ComplianceReport represents a compliance audit report.
type ComplianceReport struct {
	ID            string    `json:"id"`
	GeneratedAt   time.Time `json:"generated_at"`
	Period        string    `json:"period"`
	TotalOps      int       `json:"total_operations"`
	SuccessRate   float64   `json:"success_rate"`
	ByLevel       map[string]int `json:"by_security_level"`
	TopOperations []string  `json:"top_operations"`
	Recommendations []string `json:"recommendations"`
}
