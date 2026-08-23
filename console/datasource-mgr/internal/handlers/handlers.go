// Package handlers implements the HTTP REST interface for the datasource-mgr module.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/validation"

	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
)

const moduleVia = "datasource-mgr"

// Server aggregates HTTP handler dependencies.
type Server struct {
	agent  *agent.Client
	cfg    *config.Config
	ds     store.DataSourceStore
	logger *slog.Logger
	mc     *metrics.Collector
}

// New creates a new Server instance.
func New(ag *agent.Client, cfg *config.Config, ds store.DataSourceStore, logger *slog.Logger, mc *metrics.Collector) *Server {
	return &Server{
		agent:  ag,
		cfg:    cfg,
		ds:     ds,
		logger: logger,
		mc:     mc,
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "datasource-mgr"))
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/datasources", s.ListDataSources)
	r.POST("/api/datasources", s.CreateDataSource)
	r.GET("/api/datasources/:id", s.GetDataSource)
	r.DELETE("/api/datasources/:id", s.DeleteDataSource)
	r.POST("/api/datasources/:id/test", s.TestConnection)
	r.GET("/api/datasources/:id/metadata", s.GetMetadata)
	r.GET("/api/datasources/:id/audit", s.GetAccessAudit)
	r.GET("/metrics", s.mc.Handler())
}

// Health checks self + upstream agent connectivity.
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

// ListDataSources returns all registered data sources.
// P28 fix: SQL-level pagination instead of in-memory slicing.
func (s *Server) ListDataSources(c *gin.Context) {
	// P61 fix: use shared ParsePagination helper instead of duplicated parsing logic.
	limit, offset := validation.ParsePagination(c, 100, 1000)

	list, total, err := s.ds.ListDS(store.DataSourceFilter{Limit: limit, Offset: offset})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":       total,
		"limit":       limit,
		"offset":      offset,
		"datasources": list,
		"via":         moduleVia,
	})
}

// CreateDataSource registers a new data source.
//
// Input validation / 输入校验：
//   - type 白名单: database / api / file
//   - port 范围: 1-65535
//   - host 非空
//   - security_level 白名单: high / medium / low
func (s *Server) CreateDataSource(c *gin.Context) {
	var req struct {
		Name          string   `json:"name" binding:"required"`
		Type          string   `json:"type" binding:"required"`
		Host          string   `json:"host" binding:"required"`
		Port          int      `json:"port" binding:"required"`
		Database      string   `json:"database"`
		SecurityLevel string   `json:"security_level"`
		Tags          []string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// Input validation / 输入校验
	if err := validation.AllowedValues("type", req.Type, validation.DataSourceTypes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validation.PortRange(req.Port); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validation.NonEmpty("host", req.Host); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	// P41 fix: 校验 name 长度，防止超大名称耗尽存储空间或导致展示异常
	if err := validation.MaxLength("name", req.Name, 1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.SecurityLevel != "" {
		if err := validation.AllowedValues("security_level", req.SecurityLevel, validation.SecurityLevels); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
	}

	id := validation.GenerateID("ds")
	now := time.Now()
	ds := &store.DataSource{
		ID:            id,
		Name:          req.Name,
		Type:          req.Type,
		Host:          req.Host,
		Port:          req.Port,
		Database:      req.Database,
		SecurityLevel: req.SecurityLevel,
		Status:        "disconnected",
		CreatedAt:     now,
		Tags:          req.Tags,
	}

	if err := s.ds.SaveDS(ds); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	// Log the creation
	s.addAuditRecord(id, req.Name, "create", "system", 0, "success")

	c.JSON(http.StatusCreated, gin.H{
		"id":  id,
		"via": moduleVia,
	})
}

// GetDataSource returns a specific data source by ID.
func (s *Server) GetDataSource(c *gin.Context) {
	id := c.Param("id")
	ds, err := s.ds.GetDS(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	c.JSON(http.StatusOK, ds)
}

// DeleteDataSource removes a data source.
func (s *Server) DeleteDataSource(c *gin.Context) {
	id := c.Param("id")
	ds, err := s.ds.GetDS(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	if err := s.ds.DeleteDS(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	s.addAuditRecord(id, ds.Name, "delete", "system", 0, "success")
	c.JSON(http.StatusOK, gin.H{"deleted": id, "via": moduleVia})
}

// TestConnection tests connectivity to the data source.
func (s *Server) TestConnection(c *gin.Context) {
	id := c.Param("id")
	ds, err := s.ds.GetDS(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	start := time.Now()

	// Simulate connection test by calling agent health
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, agentErr := s.agent.Health(ctx)
	cancel()

	latency := time.Since(start).Milliseconds()
	success := agentErr == nil

	if success {
		ds.Status = "connected"
	} else {
		ds.Status = "error"
	}
	now := time.Now()
	ds.LastCheckAt = &now
	_ = s.ds.UpdateDS(ds)

	s.addAuditRecord(id, ds.Name, "test_connection", "system", 0, map[bool]string{true: "success", false: "failed"}[success])

	c.JSON(http.StatusOK, gin.H{
		"datasource_id": id,
		"success":       success,
		"latency_ms":    latency,
		"via":           moduleVia,
	})
}

// GetMetadata returns metadata for a data source.
func (s *Server) GetMetadata(c *gin.Context) {
	id := c.Param("id")
	ds, err := s.ds.GetDS(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	// Simulate metadata retrieval with auto-classification
	tables := []gin.H{
		{
			"name":      "patients",
			"row_count": 10000,
			"fields": []gin.H{
				{"name": "id", "type": "integer", "security_level": "L1", "sensitive": false},
				{"name": "name", "type": "string", "security_level": "L3", "classification": "PII", "sensitive": true},
				{"name": "id_card", "type": "string", "security_level": "L4", "classification": "PII", "sensitive": true},
				{"name": "diagnosis", "type": "string", "security_level": "L3", "classification": "medical", "sensitive": true},
			},
		},
	}

	s.addAuditRecord(id, ds.Name, "query_metadata", "system", 0, "success")

	c.JSON(http.StatusOK, gin.H{
		"datasource_id": id,
		"tables":        tables,
		"via":           moduleVia,
	})
}

// GetAccessAudit returns access audit records for a data source.
// P28 fix: SQL-level pagination instead of in-memory slicing.
func (s *Server) GetAccessAudit(c *gin.Context) {
	id := c.Param("id")

	// P61 fix: use shared ParsePagination helper instead of duplicated parsing logic.
	limit, offset := validation.ParsePagination(c, 100, 1000)

	records, total, err := s.ds.ListAudit(id, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   total,
		"limit":   limit,
		"offset":  offset,
		"records": records,
		"via":     moduleVia,
	})
}

func (s *Server) addAuditRecord(dsID, dsName, operation, user string, count int, status string) {
	rec := &store.AccessAuditRecord{
		ID:             validation.GenerateID("audit"),
		DataSourceID:   dsID,
		DataSourceName: dsName,
		Operation:      operation,
		User:           user,
		Timestamp:      time.Now(),
		RecordsCount:   count,
		Status:         status,
	}
	_ = s.ds.SaveAudit(rec)
}
