// Package handlers implements the HTTP REST interface for the datasource-mgr module.
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/models"
)

const moduleVia = "datasource-mgr"

// Server aggregates HTTP handler dependencies.
type Server struct {
	agent     *agent.Client
	cfg       *config.Config
	startTime time.Time

	mu          sync.RWMutex
	datasources map[string]*models.DataSource
	auditLog    []models.AccessAuditRecord
	dsSeq       int
}

// New creates a new Server instance.
func New(ag *agent.Client, cfg *config.Config) *Server {
	return &Server{
		agent:       ag,
		cfg:         cfg,
		startTime:   time.Now(),
		datasources: make(map[string]*models.DataSource),
		auditLog:    make([]models.AccessAuditRecord, 0),
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(corsMiddleware())
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/datasources", s.ListDataSources)
	r.POST("/api/datasources", s.CreateDataSource)
	r.GET("/api/datasources/:id", s.GetDataSource)
	r.DELETE("/api/datasources/:id", s.DeleteDataSource)
	r.POST("/api/datasources/:id/test", s.TestConnection)
	r.GET("/api/datasources/:id/metadata", s.GetMetadata)
	r.GET("/api/datasources/:id/audit", s.GetAccessAudit)
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
func (s *Server) ListDataSources(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]models.DataSource, 0, len(s.datasources))
	for _, ds := range s.datasources {
		list = append(list, *ds)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	c.JSON(http.StatusOK, models.DataSourceListResponse{
		Total:       len(list),
		DataSources: list,
		Via:         moduleVia,
	})
}

// CreateDataSource registers a new data source.
func (s *Server) CreateDataSource(c *gin.Context) {
	var req models.DataSourceCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	s.mu.Lock()
	s.dsSeq++
	id := fmt.Sprintf("ds-%d-%d", s.startTime.Unix(), s.dsSeq)
	now := time.Now()
	ds := &models.DataSource{
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
	s.datasources[id] = ds
	s.mu.Unlock()

	// Log the creation
	s.addAuditRecord(id, req.Name, "create", "system", 0, "success")

	c.JSON(http.StatusCreated, gin.H{
		"id":   id,
		"via":  moduleVia,
	})
}

// GetDataSource returns a specific data source by ID.
func (s *Server) GetDataSource(c *gin.Context) {
	id := c.Param("id")
	s.mu.RLock()
	ds, ok := s.datasources[id]
	s.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	c.JSON(http.StatusOK, ds)
}

// DeleteDataSource removes a data source.
func (s *Server) DeleteDataSource(c *gin.Context) {
	id := c.Param("id")
	s.mu.Lock()
	ds, ok := s.datasources[id]
	if ok {
		delete(s.datasources, id)
	}
	s.mu.Unlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	s.addAuditRecord(id, ds.Name, "delete", "system", 0, "success")
	c.JSON(http.StatusOK, gin.H{"deleted": id, "via": moduleVia})
}

// TestConnection tests connectivity to the data source.
// Integration with desensitization: calls agent classify to verify data accessibility.
func (s *Server) TestConnection(c *gin.Context) {
	id := c.Param("id")
	s.mu.RLock()
	ds, ok := s.datasources[id]
	s.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	start := time.Now()

	// Simulate connection test by calling agent health
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := s.agent.Health(ctx)
	cancel()

	latency := time.Since(start).Milliseconds()
	success := err == nil

	s.mu.Lock()
	if success {
		ds.Status = "connected"
	} else {
		ds.Status = "error"
	}
	now := time.Now()
	ds.LastCheckAt = &now
	s.mu.Unlock()

	s.addAuditRecord(id, ds.Name, "test_connection", "system", 0, map[bool]string{true: "success", false: "failed"}[success])

	c.JSON(http.StatusOK, models.ConnectionTestResult{
		DataSourceID: id,
		Success:      success,
		LatencyMs:    latency,
		Via:          moduleVia,
	})
}

// GetMetadata returns metadata for a data source.
// Integration with desensitization: calls agent classify to auto-tag fields.
func (s *Server) GetMetadata(c *gin.Context) {
	id := c.Param("id")
	s.mu.RLock()
	ds, ok := s.datasources[id]
	s.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"detail": "data source not found"})
		return
	}

	// Simulate metadata retrieval with auto-classification
	// In production, this would query the actual data source schema
	// and call agent's classify endpoint for each field
	tables := []models.TableMetadata{
		{
			Name:     "patients",
			RowCount: 10000,
			Fields: []models.MetadataField{
				{Name: "id", Type: "integer", SecurityLevel: "L1", Sensitive: false},
				{Name: "name", Type: "string", SecurityLevel: "L3", Classification: "PII", Sensitive: true},
				{Name: "id_card", Type: "string", SecurityLevel: "L4", Classification: "PII", Sensitive: true},
				{Name: "diagnosis", Type: "string", SecurityLevel: "L3", Classification: "medical", Sensitive: true},
			},
		},
	}

	s.addAuditRecord(id, ds.Name, "query_metadata", "system", 0, "success")

	c.JSON(http.StatusOK, models.MetadataResponse{
		DataSourceID: id,
		Tables:       tables,
		Via:          moduleVia,
	})
}

// GetAccessAudit returns access audit records for a data source.
func (s *Server) GetAccessAudit(c *gin.Context) {
	id := c.Param("id")
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]models.AccessAuditRecord, 0)
	for _, r := range s.auditLog {
		if r.DataSourceID == id {
			records = append(records, r)
		}
	}

	c.JSON(http.StatusOK, models.AccessAuditResponse{
		Total:   len(records),
		Records: records,
		Via:     moduleVia,
	})
}

func (s *Server) addAuditRecord(dsID, dsName, operation, user string, count int, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := models.AccessAuditRecord{
		ID:             fmt.Sprintf("audit-%d", len(s.auditLog)+1),
		DataSourceID:   dsID,
		DataSourceName: dsName,
		Operation:      operation,
		User:           user,
		Timestamp:      time.Now(),
		RecordsCount:   count,
		Status:         status,
	}
	s.auditLog = append(s.auditLog, record)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
