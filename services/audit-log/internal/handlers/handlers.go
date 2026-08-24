// Package handlers implements the HTTP REST interface for the audit-log module.
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/validation"

	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/config"
)

const moduleVia = "audit-log"

// Server aggregates HTTP handler dependencies.
type Server struct {
	agent  *agent.Client
	cfg    *config.Config
	audit  store.AuditStore
	logger *slog.Logger
	mc     *metrics.Collector
}

// New creates a new Server instance.
func New(ag *agent.Client, cfg *config.Config, audit store.AuditStore, logger *slog.Logger, mc *metrics.Collector) *Server {
	return &Server{
		agent:  ag,
		cfg:    cfg,
		audit:  audit,
		logger: logger,
		mc:     mc,
	}
}

// RegisterRoutes registers all HTTP routes on the Gin engine.
func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "audit-log"))
	r.Use(middleware.Recovery(s.logger, "audit-log"))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(32 << 20)) // 32 MiB max payload protection
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/audit/logs", s.ListLogs)
	r.POST("/api/audit/logs", s.CreateLog)
	r.GET("/api/audit/logs/:id", s.GetLog)
	r.GET("/api/audit/stats", s.GetStats)
	r.GET("/api/audit/snapshots", s.ListSnapshots)
	r.POST("/api/audit/snapshots/verify", s.VerifyIntegrity)
	r.POST("/api/audit/report", s.GenerateReport)
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

// ListLogs returns audit logs with optional filtering.
// P21 fix: added offset support for proper pagination.
func (s *Server) ListLogs(c *gin.Context) {
	// P61 fix: use shared ParsePagination helper instead of duplicated parsing logic.
	limit, offset := validation.ParsePagination(c, 100, 1000)

	filter := store.AuditFilter{
		Operation:     c.Query("operation"),
		DataSource:    c.Query("datasource"),
		User:          c.Query("user"),
		Status:        c.Query("status"),
		SecurityLevel: c.Query("security_level"),
		Limit:         limit,
		Offset:        offset,
	}

	logs, total, err := s.audit.ListLogs(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"logs":   logs,
		"via":    moduleVia,
	})
}

// CreateLog creates a new audit log entry.
//
// Input validation / 输入校验：
//   - operation 白名单: mask / classify / k_anon / dp / qol
//   - status 白名单: success / failed
//   - security_level 白名单: L1-L5（如果提供）
//
// Enhanced integrity hash / 增强完整性哈希：
//   - 将 input_hash + output_hash + parameters + user + security_level 全部纳入哈希计算
//   - 防止攻击者篡改输入/输出数据而不被检测
func (s *Server) CreateLog(c *gin.Context) {
	var req struct {
		Operation     string `json:"operation" binding:"required"`
		DataSource    string `json:"datasource"`
		InputHash     string `json:"input_hash"`
		OutputHash    string `json:"output_hash"`
		InputSample   string `json:"input_sample"`
		OutputSample  string `json:"output_sample"`
		Algorithm     string `json:"algorithm"`
		Parameters    any    `json:"parameters"`
		InputRows     int    `json:"input_rows"`
		OutputRows    int    `json:"output_rows"`
		DurationMs    int64  `json:"duration_ms"`
		User          string `json:"user"`
		Status        string `json:"status" binding:"required"`
		ErrorMessage  string `json:"error"`
		SecurityLevel string `json:"security_level"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// Input validation / 输入校验
	if err := validation.AllowedValues("operation", req.Operation, validation.AuditOperations); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := validation.AllowedValues("status", req.Status, validation.AuditStatuses); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.SecurityLevel != "" {
		if err := validation.AllowedValues("security_level", req.SecurityLevel, validation.SensitivityLevels); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
	}

	logID := validation.GenerateID("audit")
	now := time.Now()

	paramsJSON, _ := json.Marshal(req.Parameters)

	// P44 fix: 限制 parameters JSON 序列化后的大小，防止超大 JSON 对象耗尽存储空间。
	// 1 MB 上限足以覆盖正常审计参数（算法配置、字段列表等）。
	const maxParamsSize = 1 << 20 // 1 MB
	if len(paramsJSON) > maxParamsSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"detail": fmt.Sprintf("parameters too large: %d bytes (max %d bytes)", len(paramsJSON), maxParamsSize),
		})
		return
	}

	log := &store.AuditLog{
		ID:             logID,
		Timestamp:      now,
		Operation:      req.Operation,
		DataSource:     req.DataSource,
		InputHash:      req.InputHash,
		OutputHash:     req.OutputHash,
		Algorithm:      req.Algorithm,
		Parameters:     req.Parameters,
		ParametersJSON: string(paramsJSON),
		InputRows:      req.InputRows,
		OutputRows:     req.OutputRows,
		DurationMs:     req.DurationMs,
		User:           req.User,
		Status:         req.Status,
		ErrorMessage:   req.ErrorMessage,
		SecurityLevel:  req.SecurityLevel,
	}

	if err := s.audit.SaveLog(log); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	// Auto-generate snapshot with ENHANCED integrity hash
	// 自动生成快照，使用增强完整性哈希（包含 input_hash/output_hash/parameters/user/security_level）
	snapshot := &store.SnapshotRecord{
		ID:             validation.GenerateID("snap"),
		AuditLogID:     logID,
		Timestamp:      now,
		InputSample:    req.InputSample,
		OutputSample:   req.OutputSample,
		Algorithm:      req.Algorithm,
		Parameters:     req.Parameters,
		ParametersJSON: string(paramsJSON),
		IntegrityHash:  computeIntegrityHash(logID, now, req.Algorithm, req.InputHash, req.OutputHash, req.User, req.SecurityLevel, string(paramsJSON)),
	}

	if err := s.audit.SaveSnapshot(snapshot); err != nil {
		s.logger.Error("failed to save snapshot", "error", err.Error())
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":  logID,
		"via": moduleVia,
	})
}

// GetLog returns a specific audit log by ID.
func (s *Server) GetLog(c *gin.Context) {
	id := c.Param("id")
	log, err := s.audit.GetLog(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "audit log not found"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetStats returns aggregated audit statistics.
// P31 fix: use SQL-level aggregation instead of loading 10k records into memory.
func (s *Server) GetStats(c *gin.Context) {
	stats, err := s.audit.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_operations":  stats.TotalOperations,
		"by_operation":      stats.ByOperation,
		"by_status":         stats.ByStatus,
		"by_security_level": stats.BySecurityLevel,
		"avg_duration_ms":   stats.AvgDurationMs,
		"period":            c.DefaultQuery("period", "24h"),
	})
}

// ListSnapshots returns desensitization snapshots.
// P30 fix: added offset support for proper pagination.
func (s *Server) ListSnapshots(c *gin.Context) {
	// P61 fix: use shared ParsePagination helper instead of duplicated parsing logic.
	limit, offset := validation.ParsePagination(c, 50, 500)

	// P35 fix: use SQL-level total count instead of len(snaps) for proper pagination
	snaps, total, err := s.audit.ListSnapshots(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"limit":     limit,
		"offset":    offset,
		"snapshots": snaps,
		"via":       moduleVia,
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

	snap, err := s.audit.GetSnapshot(req.SnapshotID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "snapshot not found"})
		return
	}

	// Get the associated audit log for full hash computation
	log, err := s.audit.GetLog(snap.AuditLogID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "associated audit log not found"})
		return
	}

	// Recompute hash with enhanced fields and compare
	expectedHash := computeIntegrityHash(
		snap.AuditLogID, snap.Timestamp, snap.Algorithm,
		log.InputHash, log.OutputHash, log.User, log.SecurityLevel, snap.ParametersJSON,
	)
	valid := snap.IntegrityHash == expectedHash

	c.JSON(http.StatusOK, gin.H{
		"snapshot_id": req.SnapshotID,
		"valid":       valid,
		"expected":    expectedHash,
		"actual":      snap.IntegrityHash,
		"via":         moduleVia,
	})
}

// GenerateReport generates a compliance audit report.
// P33 fix: use SQL-level filtering and aggregation instead of loading 10k records.
func (s *Server) GenerateReport(c *gin.Context) {
	var req struct {
		Period string `json:"period"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Period = "24h"
	}

	// Use SQL-level filtering and aggregation
	report, err := s.audit.GenerateReport(req.Period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                fmt.Sprintf("report-%d", time.Now().Unix()),
		"generated_at":      time.Now(),
		"period":            req.Period,
		"total_operations":  report.TotalOperations,
		"success_rate":      report.SuccessRate,
		"by_security_level": report.BySecurityLevel,
		"top_operations":    report.TopOperations,
		"recommendations":   report.Recommendations,
	})
}

// computeIntegrityHash computes an enhanced SHA-256 integrity hash.
//
// Security fix / 安全修复：
// 原实现仅哈希 3 个字段（logID, timestamp, algorithm），攻击者可篡改
// 输入/输出数据而不被检测。增强版将 input_hash + output_hash + parameters
// + user + security_level 全部纳入哈希计算。
func computeIntegrityHash(logID string, timestamp time.Time, algorithm, inputHash, outputHash, user, securityLevel, paramsJSON string) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%v",
		logID, timestamp.Format(time.RFC3339Nano), algorithm,
		inputHash, outputHash, user, securityLevel, paramsJSON)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
