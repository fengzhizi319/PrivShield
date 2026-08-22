// Package handlers implements the HTTP REST interface for the audit-log module.
package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/config"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/models"
)

const moduleVia = "audit-log"

// Server aggregates HTTP handler dependencies.
type Server struct {
	agent     *agent.Client
	cfg       *config.Config
	startTime time.Time

	mu        sync.RWMutex
	logs      []models.AuditLog
	snapshots []models.SnapshotRecord
	logSeq    int
}

// New creates a new Server instance.
func New(ag *agent.Client, cfg *config.Config) *Server {
	return &Server{
		agent:     ag,
		cfg:       cfg,
		startTime: time.Now(),
		logs:      make([]models.AuditLog, 0),
		snapshots: make([]models.SnapshotRecord, 0),
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(corsMiddleware())
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/audit/logs", s.ListLogs)
	r.POST("/api/audit/logs", s.CreateLog)
	r.GET("/api/audit/logs/:id", s.GetLog)
	r.GET("/api/audit/stats", s.GetStats)
	r.GET("/api/audit/snapshots", s.ListSnapshots)
	r.POST("/api/audit/snapshots/verify", s.VerifyIntegrity)
	r.POST("/api/audit/report", s.GenerateReport)
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

// ListLogs returns audit logs with optional filtering.
func (s *Server) ListLogs(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	operation := c.Query("operation")
	datasource := c.Query("datasource")
	user := c.Query("user")
	status := c.Query("status")
	level := c.Query("security_level")
	limitStr := c.DefaultQuery("limit", "100")

	limit := 100
	if l, err := fmt.Sscanf(limitStr, "%d", &limit); l == 0 || err != nil {
		limit = 100
	}

	filtered := make([]models.AuditLog, 0)
	for _, log := range s.logs {
		if operation != "" && log.Operation != operation {
			continue
		}
		if datasource != "" && log.DataSource != datasource {
			continue
		}
		if user != "" && log.User != user {
			continue
		}
		if status != "" && log.Status != status {
			continue
		}
		if level != "" && log.SecurityLevel != level {
			continue
		}
		filtered = append(filtered, log)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	c.JSON(http.StatusOK, models.AuditLogListResponse{
		Total: len(filtered),
		Logs:  filtered,
		Via:   moduleVia,
	})
}

// CreateLog creates a new audit log entry.
// Integration with desensitization: automatically generates snapshot and integrity hash.
func (s *Server) CreateLog(c *gin.Context) {
	var log models.AuditLog
	if err := c.ShouldBindJSON(&log); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	s.mu.Lock()
	s.logSeq++
	log.ID = fmt.Sprintf("audit-%d-%d", s.startTime.Unix(), s.logSeq)
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}
	s.logs = append(s.logs, log)

	// Auto-generate snapshot for evidence
	snapshot := models.SnapshotRecord{
		ID:         fmt.Sprintf("snap-%d", s.logSeq),
		AuditLogID: log.ID,
		Timestamp:  log.Timestamp,
		Algorithm:  log.Algorithm,
		Parameters: log.Parameters,
		IntegrityHash: computeIntegrityHash(log.ID, log.Timestamp, log.Algorithm),
	}
	s.snapshots = append(s.snapshots, snapshot)

	// Enforce max log entries
	if len(s.logs) > s.cfg.MaxLogEntries {
		s.logs = s.logs[len(s.logs)-s.cfg.MaxLogEntries:]
	}
	s.mu.Unlock()

	c.JSON(http.StatusCreated, gin.H{
		"id":   log.ID,
		"via":  moduleVia,
	})
}

// GetLog returns a specific audit log by ID.
func (s *Server) GetLog(c *gin.Context) {
	id := c.Param("id")
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, log := range s.logs {
		if log.ID == id {
			c.JSON(http.StatusOK, log)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"detail": "audit log not found"})
}

// GetStats returns aggregated audit statistics.
func (s *Server) GetStats(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	period := c.DefaultQuery("period", "24h")

	stats := models.AuditStats{
		TotalOperations: len(s.logs),
		ByOperation:     make(map[string]int),
		ByStatus:        make(map[string]int),
		BySecurityLevel: make(map[string]int),
		Period:          period,
	}

	var totalDuration int64
	for _, log := range s.logs {
		stats.ByOperation[log.Operation]++
		stats.ByStatus[log.Status]++
		if log.SecurityLevel != "" {
			stats.BySecurityLevel[log.SecurityLevel]++
		}
		totalDuration += log.DurationMs
	}

	if len(s.logs) > 0 {
		stats.AvgDurationMs = float64(totalDuration) / float64(len(s.logs))
	}

	c.JSON(http.StatusOK, stats)
}

// ListSnapshots returns desensitization snapshots.
func (s *Server) ListSnapshots(c *gin.Context) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limitStr := c.DefaultQuery("limit", "50")
	limit := 50
	if l, err := fmt.Sscanf(limitStr, "%d", &limit); l == 0 || err != nil {
		limit = 50
	}

	sorted := make([]models.SnapshotRecord, len(s.snapshots))
	copy(sorted, s.snapshots)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})

	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}

	c.JSON(http.StatusOK, models.SnapshotListResponse{
		Total:     len(sorted),
		Snapshots: sorted,
		Via:       moduleVia,
	})
}

// VerifyIntegrity verifies the integrity of a snapshot using its hash.
func (s *Server) VerifyIntegrity(c *gin.Context) {
	var req struct {
		SnapshotID string `json:"snapshot_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, snap := range s.snapshots {
		if snap.ID == req.SnapshotID {
			// Recompute hash and compare
			expectedHash := computeIntegrityHash(snap.AuditLogID, snap.Timestamp, snap.Algorithm)
			valid := snap.IntegrityHash == expectedHash

			c.JSON(http.StatusOK, gin.H{
				"snapshot_id": req.SnapshotID,
				"valid":       valid,
				"expected":    expectedHash,
				"actual":      snap.IntegrityHash,
				"via":         moduleVia,
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"detail": "snapshot not found"})
}

// GenerateReport generates a compliance audit report.
func (s *Server) GenerateReport(c *gin.Context) {
	var req struct {
		Period string `json:"period"` // "1h" | "24h" | "7d" | "30d"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Period = "24h"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter logs by period
	now := time.Now()
	var periodDuration time.Duration
	switch req.Period {
	case "1h":
		periodDuration = time.Hour
	case "7d":
		periodDuration = 7 * 24 * time.Hour
	case "30d":
		periodDuration = 30 * 24 * time.Hour
	default:
		periodDuration = 24 * time.Hour
	}

	cutoff := now.Add(-periodDuration)
	filtered := make([]models.AuditLog, 0)
	for _, log := range s.logs {
		if log.Timestamp.After(cutoff) {
			filtered = append(filtered, log)
		}
	}

	// Compute statistics
	byLevel := make(map[string]int)
	byOp := make(map[string]int)
	successCount := 0
	for _, log := range filtered {
		if log.SecurityLevel != "" {
			byLevel[log.SecurityLevel]++
		}
		byOp[log.Operation]++
		if log.Status == "success" {
			successCount++
		}
	}

	successRate := 0.0
	if len(filtered) > 0 {
		successRate = float64(successCount) / float64(len(filtered)) * 100
	}

	// Generate recommendations
	recommendations := generateRecommendations(byLevel, successRate)

	report := models.ComplianceReport{
		ID:              fmt.Sprintf("report-%d", time.Now().Unix()),
		GeneratedAt:     time.Now(),
		Period:          req.Period,
		TotalOps:        len(filtered),
		SuccessRate:     successRate,
		ByLevel:         byLevel,
		TopOperations:   getTopOperations(byOp),
		Recommendations: recommendations,
	}

	c.JSON(http.StatusOK, report)
}

func computeIntegrityHash(logID string, timestamp time.Time, algorithm string) string {
	data := fmt.Sprintf("%s|%s|%s", logID, timestamp.Format(time.RFC3339Nano), algorithm)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func generateRecommendations(byLevel map[string]int, successRate float64) []string {
	recs := make([]string, 0)

	if l4 := byLevel["L4"]; l4 > 100 {
		recs = append(recs, "L4 级别操作频繁，建议审查差分隐私预算消耗")
	}
	if l5 := byLevel["L5"]; l5 > 50 {
		recs = append(recs, "L5 绝密数据操作较多，建议加强访问控制审计")
	}
	if successRate < 95 {
		recs = append(recs, fmt.Sprintf("成功率 %.1f%% 低于 95%%，建议排查失败原因", successRate))
	}
	if len(recs) == 0 {
		recs = append(recs, "审计指标正常，无需特别关注")
	}

	return recs
}

func getTopOperations(byOp map[string]int) []string {
	type kv struct {
		Key   string
		Value int
	}
	sorted := make([]kv, 0, len(byOp))
	for k, v := range byOp {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	top := make([]string, 0, 5)
	for i, kv := range sorted {
		if i >= 5 {
			break
		}
		top = append(top, fmt.Sprintf("%s (%d)", kv.Key, kv.Value))
	}
	return top
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
