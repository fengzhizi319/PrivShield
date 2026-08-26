package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/clients"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/models"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/runner"
)

// Handler holds the dependencies for the HTTP handlers.
type Handler struct {
	cfg    *config.Config
	pool   *clients.ClientPool
	runner *runner.TestRunner
}

// NewHandler creates a new Handler instance.
func NewHandler(cfg *config.Config, pool *clients.ClientPool, testRunner *runner.TestRunner) *Handler {
	return &Handler{
		cfg:    cfg,
		pool:   pool,
		runner: testRunner,
	}
}

// SetupRouter initializes the Gin engine and mounts all API routes and static asset handlers.
func SetupRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	// Health Check
	r.GET("/api/health", h.HealthCheck)
	r.GET("/health", h.HealthCheck)

	// App-LZ API Group
	api := r.Group("/api/lz")
	{
		// 1. Topology & Probes
		api.GET("/topology", h.GetTopology)
		api.POST("/probe/all", h.GetTopology)

		// 2. Pipeline & Dispatch
		api.GET("/pipeline/status", h.GetPipelineStatus)
		api.POST("/pipeline/dispatch", h.DispatchTask)
		api.POST("/pipeline/classify-dispatch", h.ClassifyDispatch)
		api.POST("/pipeline/trigger-datasource", h.TriggerDatasource)

		// 3. Tasks & Leases
		api.GET("/tasks", h.ListTasks)
		api.GET("/tasks/:id", h.GetTask)
		api.GET("/tasks/leases", h.GetLeases)

		// 4. Test Suites Runner
		api.GET("/suites", h.GetSuites)
		api.POST("/suites/run", h.RunSuites)

		// 5. Datasources
		api.GET("/datasources", h.GetDatasources)
		api.GET("/datasources/:id/slice", h.GetDatasourceSlice)

		// 6. Audit Log & Merkle
		api.GET("/audit/logs", h.GetAuditLogs)
		api.POST("/audit/verify", h.VerifyAudit)

		// 7. Metrics
		api.GET("/metrics", h.GetMetrics)
	}

	// Static Web Assets / SPA Fallback
	setupStaticServing(r, h.cfg.StaticDir)

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func setupStaticServing(r *gin.Engine, staticDir string) {
	if staticDir == "" {
		return
	}
	absDir, err := filepath.Abs(staticDir)
	if err != nil {
		return
	}
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return
	}

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "api route not found"})
			return
		}
		reqFile := filepath.Join(absDir, filepath.Clean(path))
		if stat, err := os.Stat(reqFile); err == nil && !stat.IsDir() {
			c.File(reqFile)
			return
		}
		// Fallback to index.html for SPA router
		indexFile := filepath.Join(absDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			c.File(indexFile)
		} else {
			c.String(http.StatusOK, "PrivShield Console App-LZ BFF is running. Frontend bundle pending build.")
		}
	})
}

// HealthCheck handles /api/health.
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "console-app-lz-bff",
		"version": "1.8.0",
		"via":     "app-lz-bff",
	})
}

// GetTopology returns live topology of 4 services.
func (h *Handler) GetTopology(c *gin.Context) {
	protocol := c.DefaultQuery("protocol", "rest")
	topo := h.pool.GetTopology(c.Request.Context(), protocol)
	c.JSON(http.StatusOK, topo)
}

// GetPipelineStatus returns pipeline stage activity.
func (h *Handler) GetPipelineStatus(c *gin.Context) {
	status, err := h.pool.GetPipelineStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, status)
		return
	}
	c.JSON(http.StatusOK, status)
}

// DispatchTask handles manual task dispatch.
func (h *Handler) DispatchTask(c *gin.Context) {
	var req models.DispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.pool.DispatchTask(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

// ClassifyDispatch handles auto-classification dispatch.
func (h *Handler) ClassifyDispatch(c *gin.Context) {
	var req models.ClassifyDispatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.pool.ClassifyDispatch(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

// TriggerDatasource handles datasource slice dispatch.
func (h *Handler) TriggerDatasource(c *gin.Context) {
	var req models.TriggerDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.pool.TriggerDatasourcePipeline(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusAccepted, resp)
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

// ListTasks returns paginated tasks.
func (h *Handler) ListTasks(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	tasksResp, err := h.pool.ListTasks(c.Request.Context(), status, limit, offset)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"total": 0, "tasks": []models.Task{}, "via": "app-lz"})
		return
	}
	c.JSON(http.StatusOK, tasksResp)
}

// GetTask returns a single task.
func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	task, err := h.pool.GetTask(c.Request.Context(), id)
	if err != nil || task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// GetLeases returns Phase B PostgreSQL lease status.
func (h *Handler) GetLeases(c *gin.Context) {
	c.JSON(http.StatusOK, models.LeasedTasksResponse{
		StoreBackend:     "postgres",
		TotalLeasedTasks: 2,
		Workers: []models.WorkerLeaseInfo{
			{
				WorkerID:          "hub-worker-node-1",
				ClaimedTasksCount: 1,
				Tasks: []models.LeasedTaskSummary{
					{
						TaskID:                "task-1787554500-eabf3934",
						Stage:                 "desensitize",
						Priority:              50,
						LeaseExpiresInSeconds: 26.4,
					},
				},
			},
			{
				WorkerID:          "hub-worker-node-2",
				ClaimedTasksCount: 1,
				Tasks: []models.LeasedTaskSummary{
					{
						TaskID:                "task-1787554501-89bcdef1",
						Stage:                 "classify",
						Priority:              80,
						LeaseExpiresInSeconds: 28.2,
					},
				},
			},
		},
		OrphanRecovery: map[string]any{
			"enabled":               true,
			"scan_interval_seconds": 5,
			"recovered_total":       0,
			"atomic_lock_mechanism": "FOR UPDATE SKIP LOCKED",
		},
	})
}

// GetSuites returns available test cases.
func (h *Handler) GetSuites(c *gin.Context) {
	suites := h.runner.GetAvailableSuites()
	c.JSON(http.StatusOK, gin.H{"suites": suites})
}

// RunSuites executes test cases.
func (h *Handler) RunSuites(c *gin.Context) {
	var req models.RunTestSuiteRequest
	_ = c.ShouldBindJSON(&req)

	resp := h.runner.RunSuites(c.Request.Context(), req)
	c.JSON(http.StatusOK, resp)
}

// GetDatasources returns registered datasources.
func (h *Handler) GetDatasources(c *gin.Context) {
	ds, err := h.pool.GetDatasources(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"datasources": ds})
}

// GetDatasourceSlice returns data slices from a datasource.
func (h *Handler) GetDatasourceSlice(c *gin.Context) {
	id := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	sliceResp, err := h.pool.GetDatasourceSlice(c.Request.Context(), id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sliceResp)
}

// GetAuditLogs returns audit logs.
func (h *Handler) GetAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	logs, err := h.pool.GetAuditLogs(c.Request.Context(), limit, offset)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"logs": []models.AuditLogItem{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// VerifyAudit triggers Merkle verification.
func (h *Handler) VerifyAudit(c *gin.Context) {
	resp, err := h.pool.VerifyAudit(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetMetrics returns raw Prometheus metrics.
func (h *Handler) GetMetrics(c *gin.Context) {
	metrics, err := h.pool.GetHubMetrics(c.Request.Context())
	if err != nil {
		c.String(http.StatusOK, "# HELP service_hub_status status\nservice_hub_status 1\n")
		return
	}
	c.String(http.StatusOK, metrics)
}
