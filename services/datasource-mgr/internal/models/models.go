// Package models defines data structures for the mock datasource-mgr module.
// Package models 定义模拟数据源模块的数据结构。
package models

// MockDataSource represents a registered mock data source for dev/testing.
type MockDataSource struct {
	ID          string   `json:"id"`          // "ds_yibao" | "ds_kangyang" | "ds_mock3" | "ds_mock4"
	Name        string   `json:"name"`        // Display name
	Type        string   `json:"type"`        // "file" | "mock"
	Description string   `json:"description"` // Description
	Status      string   `json:"status"`      // "connected"
	RowCount    int      `json:"row_count"`   // Total mock rows
	Tags        []string `json:"tags"`        // Tags
}

// DataQueryResponse represents the query result of mock data records.
type DataQueryResponse struct {
	SourceID   string           `json:"source_id"`
	SourceName string           `json:"source_name"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
	Records    []map[string]any `json:"records"`
	Via        string           `json:"via"`
}

// DataSourceListResponse is the response for listing mock datasources.
type DataSourceListResponse struct {
	Total       int              `json:"total"`
	DataSources []MockDataSource `json:"datasources"`
	Via         string           `json:"via"`
}

// MetadataField describes a single column's metadata.
type MetadataField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// TableMetadata describes the schema of a mock data table.
type TableMetadata struct {
	Name     string          `json:"name"`
	RowCount int             `json:"row_count"`
	Fields   []MetadataField `json:"fields"`
}

// MetadataResponse is the response for metadata query.
type MetadataResponse struct {
	DataSourceID string          `json:"datasource_id"`
	Tables       []TableMetadata `json:"tables"`
	Via          string          `json:"via"`
}

// ConnectionTestResult is the response for connection test.
type ConnectionTestResult struct {
	DataSourceID string `json:"datasource_id"`
	Success      bool   `json:"success"`
	LatencyMs    int64  `json:"latency_ms"`
	Via          string `json:"via"`
}
