// Package handlers implements the HTTP REST interface for the mock datasource-mgr module.
package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/models"
)

const moduleVia = "datasource-mgr"

// Server aggregates HTTP handler dependencies.
type Server struct {
	cfg    *config.Config
	logger *slog.Logger
}

// New creates a new Server instance.
func New(cfg *config.Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "datasource-mgr"))
	r.Use(middleware.Recovery(s.logger, "datasource-mgr"))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	// Health
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)

	// API 1, 2, 3, 4: Dedicated Mock Data Endpoints
	r.GET("/api/v1/yibao", s.GetYibaoData)
	r.GET("/api/v1/kangyang", s.GetKangyangData)
	r.GET("/api/v1/mock3", s.GetMock3Data)
	r.GET("/api/v1/mock4", s.GetMock4Data)

	// General Datasource & Sample Endpoints
	r.GET("/api/datasources", s.ListDataSources)
	r.GET("/api/datasources/:id", s.GetDataSource)
	r.GET("/api/datasources/:id/records", s.GetDataSourceRecords)
	r.GET("/api/datasources/:id/sample", s.GetDataSourceRecords)
	r.POST("/api/datasources/:id/test", s.TestConnection)
	r.GET("/api/datasources/:id/metadata", s.GetMetadata)
	r.GET("/api/datasources/:id/audit", s.GetAccessAudit)
	r.POST("/api/datasources/seed", s.SeedDataSourcesEndpoint)
}

func parsePagination(c *gin.Context, defaultLimit, maxLimit int) (int, int) {
	limitStr := c.Query("limit")
	limit := defaultLimit
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offsetStr := c.Query("offset")
	offset := 0
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}
	return limit, offset
}

// Health returns mock service status.
func (s *Server) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"backend":    "ok",
		"status":     "ok",
		"mode":       "mock_datasource_provider",
		"latency_ms": 0,
		"via":        moduleVia,
	})
}

// API 1: 医保就医与结算模拟数据
func (s *Server) GetYibaoData(c *gin.Context) {
	limit, offset := parsePagination(c, 20, 500)
	records, total, err := GetYibaoRecords(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, models.DataQueryResponse{
		SourceID:   "ds_yibao",
		SourceName: "医保就医与结算模拟数据库 (yibao.csv)",
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Records:    records,
		Via:        moduleVia,
	})
}

// API 2: 康养体检与慢病模拟数据
func (s *Server) GetKangyangData(c *gin.Context) {
	limit, offset := parsePagination(c, 20, 500)
	records, total, err := GetKangyangRecords(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, models.DataQueryResponse{
		SourceID:   "ds_kangyang",
		SourceName: "康养体检与慢病模拟数据库 (kangyang.csv)",
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Records:    records,
		Via:        moduleVia,
	})
}

// API 3: 预留模拟数据源 3
func (s *Server) GetMock3Data(c *gin.Context) {
	limit, offset := parsePagination(c, 20, 500)
	records, total, err := GetMock3Records(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, models.DataQueryResponse{
		SourceID:   "ds_mock3",
		SourceName: "预留政务数据源 3",
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Records:    records,
		Via:        moduleVia,
	})
}

// API 4: 预留模拟数据源 4
func (s *Server) GetMock4Data(c *gin.Context) {
	limit, offset := parsePagination(c, 20, 500)
	records, total, err := GetMock4Records(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, models.DataQueryResponse{
		SourceID:   "ds_mock4",
		SourceName: "预留政务数据源 4",
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Records:    records,
		Via:        moduleVia,
	})
}

// ListDataSources returns list of all mock sources.
func (s *Server) ListDataSources(c *gin.Context) {
	list := ListMockDataSources()
	c.JSON(http.StatusOK, gin.H{
		"total":       len(list),
		"datasources": list,
		"via":         moduleVia,
	})
}

// GetDataSource returns single mock datasource info.
func (s *Server) GetDataSource(c *gin.Context) {
	id := c.Param("id")
	ds, err := GetMockDataSource(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, ds)
}

// GetDataSourceRecords returns records for a given datasource ID.
func (s *Server) GetDataSourceRecords(c *gin.Context) {
	id := c.Param("id")
	limit, offset := parsePagination(c, 20, 500)

	records, total, sourceName, err := GetDataBySource(id, limit, offset)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"datasource_id": id,
		"name":          sourceName,
		"total":         total,
		"limit":         limit,
		"offset":        offset,
		"records":       records,
		"via":           moduleVia,
	})
}

// TestConnection tests mock source connectivity.
func (s *Server) TestConnection(c *gin.Context) {
	id := c.Param("id")
	_, err := GetMockDataSource(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, models.ConnectionTestResult{
		DataSourceID: id,
		Success:      true,
		LatencyMs:    2,
		Via:          moduleVia,
	})
}

// GetMetadata returns schema metadata for a mock source.
func (s *Server) GetMetadata(c *gin.Context) {
	id := c.Param("id")
	meta, err := GetMetadata(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, meta)
}

// GetAccessAudit returns mock audit records.
func (s *Server) GetAccessAudit(c *gin.Context) {
	id := c.Param("id")
	c.JSON(http.StatusOK, gin.H{
		"datasource_id": id,
		"total":         1,
		"records": []gin.H{
			{
				"id":        "audit_mock_1",
				"operation": "query_sample",
				"user":      "dev_user",
				"timestamp": time.Now().Format(time.RFC3339),
				"status":    "success",
			},
		},
		"via": moduleVia,
	})
}

// SeedDataSourcesEndpoint returns mock seed confirmation.
func (s *Server) SeedDataSourcesEndpoint(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "mock datasources initialized (yibao, kangyang, mock3, mock4)",
		"via":     moduleVia,
	})
}
