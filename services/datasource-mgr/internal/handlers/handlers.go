// Package handlers implements the HTTP REST interface for the mock datasource-mgr module.
// Package handlers 实现了模拟数据源模块（datasource-mgr）的 HTTP REST 服务端接口。
//
// 该文件通过 Gin 框架暴露了一系列 RESTful API 端点：
// 1. 健康检查与探针（/health, /api/health）；
// 2. 专用模拟数据集抽取端点（/api/v1/yibao, /api/v1/kangyang, /api/v1/mock3, /api/v1/mock4）；
// 3. 通用数据源资产查询、记录采样、Schema 元数据探测与连通性测试接口（/api/datasources/*）。
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

// moduleVia 是响应体中的服务标识常量，用于全链路追踪定位请求处理节点。
const moduleVia = "datasource-mgr"

// Server aggregates HTTP handler dependencies.
// Server 结构体聚合了 HTTP 处理器层所需的运行配置和结构化日志组件。
type Server struct {
	cfg    *config.Config // 全局运行配置
	logger *slog.Logger   // 结构化日志记录器
}

// New creates a new Server instance.
// New 创建并返回一个新的 Server 实例。
func New(cfg *config.Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

// RegisterRoutes registers all HTTP routes and middleware on the Gin engine.
// RegisterRoutes 向 Gin 引擎装配通用安全中间件链并注册全部业务路由端点，执行逻辑如下：
// 1. 中间件装配链（Middleware Chain）：
//    - RequestID: 生成并注入全链路追踪 X-Request-ID；
//    - StructuredLogger: 请求访问日志记录；
//    - Recovery: Panic 拦截保护，保障进程高可用；
//    - SecurityHeaders: 注入安全响应头（X-Frame-Options, X-Content-Type-Options 等）；
//    - CORS: 跨域策略配置；
//    - Auth: 基于 Header API Key 的身份认证（如果配置了 APIKey）。
// 2. 路由分组注册：
//    - 存活健康探针（Health Check）；
//    - 专用模拟数据集端点（API 1 ~ 4）；
//    - 数据源管理与元数据探测端点。
func (s *Server) RegisterRoutes(r *gin.Engine) {
	// 装配中间件栈
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "datasource-mgr"))
	r.Use(middleware.Recovery(s.logger, "datasource-mgr"))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(s.cfg.CORSOrigins))
	r.Use(middleware.Auth(s.cfg.APIKey))

	// 健康探针路由
	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)

	// API 1, 2, 3, 4: 专用模拟数据源访问端点
	r.GET("/api/v1/yibao", s.GetYibaoData)         // API 1: 医保就医与结算
	r.GET("/api/v1/kangyang", s.GetKangyangData)   // API 2: 康养体检与慢病
	r.GET("/api/v1/mock3", s.GetMock3Data)         // API 3: 预留政务数据源 3
	r.GET("/api/v1/mock4", s.GetMock4Data)         // API 4: 预留政务数据源 4

	// 通用数据源资产与采样端点
	r.GET("/api/datasources", s.ListDataSources)               // 数据源目录列表
	r.GET("/api/datasources/:id", s.GetDataSource)             // 单个数据源详情
	r.GET("/api/datasources/:id/records", s.GetDataSourceRecords) // 动态分页查询记录
	r.GET("/api/datasources/:id/sample", s.GetDataSourceRecords)  // 兼容样本数据接口别名
	r.POST("/api/datasources/:id/test", s.TestConnection)      // 数据源连通性测试
	r.GET("/api/datasources/:id/metadata", s.GetMetadata)      // Schema 元数据查询
	r.GET("/api/datasources/:id/audit", s.GetAccessAudit)      // 数据访问审计日志查询
	r.POST("/api/datasources/seed", s.SeedDataSourcesEndpoint) // 初始化/重置模拟数据源
}

// parsePagination parses limit and offset query parameters with safety bounds.
// parsePagination 从 HTTP GET 请求的 URL Query 参数中解析 limit 与 offset 分页参数：
// 1. 若 limit 缺省或非法，使用 defaultLimit；若超过 maxLimit 则截断至 maxLimit；
// 2. 若 offset 缺省或非法，重置为 0；
// 3. 返回安全校验后的 limit 与 offset。
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
// Health 返回模拟数据源服务的健康状态与元数据，用于负载均衡器和容器编排存活探针。
func (s *Server) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"backend":    "ok",
		"status":     "ok",
		"mode":       "mock_datasource_provider",
		"latency_ms": 0,
		"via":        moduleVia,
	})
}

// GetYibaoData implements API 1: queries mock healthcare and settlement records.
// GetYibaoData 处理 API 1 请求：分页读取并返回医保就医与结算模拟数据（yibao.csv）。
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

// GetKangyangData implements API 2: queries mock elderly care and chronic disease records.
// GetKangyangData 处理 API 2 请求：分页读取并返回康养体检与慢病管理模拟数据（kangyang.csv）。
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

// GetMock3Data implements API 3: queries reserved municipal dataset 3.
// GetMock3Data 处理 API 3 请求：分页读取并返回预留政务模拟数据源 3 的记录。
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

// GetMock4Data implements API 4: queries reserved municipal dataset 4.
// GetMock4Data 处理 API 4 请求：分页读取并返回预留政务模拟数据源 4 的记录。
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

// ListDataSources returns list of all registered mock sources.
// ListDataSources 返回系统中已注册的所有模拟数据源元数据列表。
func (s *Server) ListDataSources(c *gin.Context) {
	list := ListMockDataSources()
	c.JSON(http.StatusOK, gin.H{
		"total":       len(list),
		"datasources": list,
		"via":         moduleVia,
	})
}

// GetDataSource returns single mock datasource info by its ID.
// GetDataSource 根据 URL 路径参数 :id 查询单个模拟数据源的元数据，未找到时返回 HTTP 404。
func (s *Server) GetDataSource(c *gin.Context) {
	id := c.Param("id")
	ds, err := GetMockDataSource(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, ds)
}

// GetDataSourceRecords returns records for a given datasource ID with pagination.
// GetDataSourceRecords 根据 URL 路径参数 :id 动态路由并分页查询对应数据源的数据记录。
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
// TestConnection 测试指定数据源的连通性并返回模拟延迟。
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
// GetMetadata 根据数据源 ID 返回表结构与字段类型定义（Schema Metadata）。
func (s *Server) GetMetadata(c *gin.Context) {
	id := c.Param("id")
	meta, err := GetMetadata(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "via": moduleVia})
		return
	}
	c.JSON(http.StatusOK, meta)
}

// GetAccessAudit returns mock audit records for a given datasource ID.
// GetAccessAudit 返回指定数据源的模拟访问审计存证记录。
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
// SeedDataSourcesEndpoint 提供模拟数据源初始化/重新播种的触发端点。
func (s *Server) SeedDataSourcesEndpoint(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "mock datasources initialized (yibao, kangyang, mock3, mock4)",
		"via":     moduleVia,
	})
}
