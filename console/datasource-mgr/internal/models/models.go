// Package models defines shared data structures for the datasource-mgr module.
package models

import "time"

// DataSource represents a registered data source.
type DataSource struct {
	ID          string    `json:"id"`           // Unique ID
	Name        string    `json:"name"`         // Display name / 显示名称
	Type        string    `json:"type"`         // "database" | "api" | "file"
	Host        string    `json:"host"`         // Connection host
	Port        int       `json:"port"`         // Connection port
	Database    string    `json:"database"`     // Database name
	SecurityLevel string  `json:"security_level"` // "high" | "medium" | "low"
	Status      string    `json:"status"`       // "connected" | "disconnected" | "error"
	CreatedAt   time.Time `json:"created_at"`
	LastCheckAt *time.Time `json:"last_check_at"`
	Tags        []string  `json:"tags"`         // Business tags (卫健/医保/etc)
}

// DataSourceCreateRequest is the request body for creating a data source.
type DataSourceCreateRequest struct {
	Name          string   `json:"name" binding:"required"`
	Type          string   `json:"type" binding:"required"`
	Host          string   `json:"host" binding:"required"`
	Port          int      `json:"port" binding:"required"`
	Database      string   `json:"database"`
	SecurityLevel string   `json:"security_level"`
	Tags          []string `json:"tags"`
}

// DataSourceListResponse is the response for listing data sources.
type DataSourceListResponse struct {
	Total       int          `json:"total"`
	DataSources []DataSource `json:"datasources"`
	Via         string       `json:"via"`
}

// MetadataField represents a field in the data source metadata.
type MetadataField struct {
	Name          string `json:"name"`           // Field name
	Type          string `json:"type"`           // Data type
	SecurityLevel string `json:"security_level"` // L1-L5
	Classification string `json:"classification"` // Auto-classified category
	Sensitive     bool   `json:"sensitive"`      // Whether contains PII
}

// MetadataResponse is the response for metadata query.
type MetadataResponse struct {
	DataSourceID string          `json:"datasource_id"`
	Tables       []TableMetadata `json:"tables"`
	Via          string          `json:"via"`
}

// TableMetadata represents metadata for a single table.
type TableMetadata struct {
	Name   string          `json:"name"`
	Fields []MetadataField `json:"fields"`
	RowCount int           `json:"row_count"`
}

// AccessAuditRecord represents an access audit log entry.
type AccessAuditRecord struct {
	ID           string    `json:"id"`
	DataSourceID string    `json:"datasource_id"`
	DataSourceName string  `json:"datasource_name"`
	Operation    string    `json:"operation"`     // "query" | "export" | "mask"
	User         string    `json:"user"`
	Timestamp    time.Time `json:"timestamp"`
	RecordsCount int       `json:"records_count"`
	Status       string    `json:"status"`        // "success" | "denied"
}

// AccessAuditResponse is the response for access audit query.
type AccessAuditResponse struct {
	Total   int                  `json:"total"`
	Records []AccessAuditRecord  `json:"records"`
	Via     string               `json:"via"`
}

// ConnectionTestResult is the response for connection test.
type ConnectionTestResult struct {
	DataSourceID string `json:"datasource_id"`
	Success      bool   `json:"success"`
	LatencyMs    int64  `json:"latency_ms"`
	Error        string `json:"error,omitempty"`
	Via          string `json:"via"`
}
