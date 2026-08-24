// Package handlers implements the HTTP REST interface layer for the Go gRPC proxy backend.
// Package handlers 实现 Go gRPC 代理后端的 HTTP REST 接口层。
//
// Responsibilities / 职责：
//   - Receive HTTP/JSON requests from the frontend React console
//     接收前端 React 控制台的 HTTP/JSON 请求
//   - Map REST paths to corresponding gRPC calls via mapper
//     通过 mapper 将 REST 路径映射为对应的 gRPC 调用
//   - Convert protobuf responses to JSON format displayable by frontend
//     将 protobuf 响应转换为前端可展示的 JSON 格式
//   - Optionally host frontend static build artifacts, enabling Go backend to serve full Console UI
//     可选托管前端静态构建产物，使 Go 后端可独立提供完整 Console UI
//
// Design goal / 设计目标：
//
//	Maintain fully consistent JSON contract with the Python REST proxy backend;
//	frontend only needs to switch base URL to seamlessly switch between backends.
//	与 Python REST 代理后端保持完全一致的 JSON 契约，
//	前端只需切换 base URL 即可在两种后端之间无缝切换。
//
// Route list / 路由清单：
//
//	GET  /api/health   → Health check (backend self + upstream agent)
//	GET  /api/samples  → Return sample payloads for all endpoints
//	POST /api/proxy    → Single request proxy forwarding (REST → gRPC)
//	POST /api/batch    → Batch request forwarding
//	POST /api/upload   → File upload + privacy processing (masking/K-anonymity/classification)
//	POST /api/lb_test  → Load-balancing strategy test
package handlers

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	// encoding/json：用于 JSON 序列化/反序列化（params 解析、RecordEntry 转换）
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/fileparse"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/lbtest"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/mapper"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/models"
	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/samples"
	pb "github.com/fengzhizi319/PrivShield/console/bff-go/proto"
)

// 本控制台后端的身份标识常量，随每个响应下发给前端。
//
// 用途：前端界面展示"当前请求由哪个后端、以何种协议与 agent 通信"，
// 使 Python REST / Go gRPC 两种通信方式的切换可被直观验证。
const (
	// backendVia：标识响应经由的后端类型，"go-grpc" 表示通过 Go 代理后端转发
	backendVia = "go-grpc"
	// agentProtocol：标识与上游 agent 通信的协议，"gRPC" 表示使用 gRPC 调用
	agentProtocol = "gRPC"
)

// Server 聚合 HTTP 处理器所需的全部依赖。
type Server struct {
	client     *agent.Client
	mapper     *mapper.Mapper
	cfg        *config.Config
	logger     *slog.Logger
	mc         *metrics.Collector
	httpClient *http.Client // Shared HTTP client for REST calls / 共享 HTTP 客户端
	secCleanup func()       // P57 fix: cleanup function for securityMiddleware ticker goroutine
}

func New(client *agent.Client, cfg *config.Config, logger *slog.Logger, mc *metrics.Collector) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		client: client,
		mapper: mapper.New(),
		cfg:    cfg,
		logger: logger,
		mc:     mc,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Shutdown gracefully stops background goroutines.
// P57 fix: stop the securityMiddleware ticker goroutine to prevent goroutine leak.
func (s *Server) Shutdown() {
	if s.secCleanup != nil {
		s.secCleanup()
	}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	// Shared middleware chain / 共享中间件链
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger(s.logger, "backend-go"))
	r.Use(middleware.Recovery(s.logger, "backend-go"))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(64 << 20)) // 64 MiB max payload protection (supports larger CSV uploads)
	r.Use(middleware.CORS(nil))            // backend-go 默认允许所有来源（开发模式）
	// P57 fix: capture cleanup function from securityMiddleware to stop ticker goroutine on shutdown.
	secHandler, secCleanup := securityMiddleware(s.cfg.ConsoleAPIKey, s.cfg.ConsoleRateLimit)
	s.secCleanup = secCleanup
	r.Use(secHandler)

	r.GET("/health", s.Health)
	r.GET("/api/health", s.Health)
	r.GET("/api/samples", s.Samples)
	r.POST("/api/proxy", s.Proxy)
	r.POST("/api/batch", s.Batch)
	r.POST("/api/upload", s.Upload)
	r.POST("/api/lb_test", s.LbTest)
	r.POST("/api/concurrency_test", s.ConcurrencyTest)
	r.POST("/api/medical_pipeline", s.MedicalPipeline)
	r.POST("/api/yibao_pipeline", s.YibaoPipeline)
	r.POST("/api/pipeline/process", s.PipelineProcess)
	r.GET("/metrics", s.mc.Handler())
	s.registerStatic(r)
}

// registerStatic 挂载前端构建产物（SPA），使 Go 后端能独立提供 Console UI，
// 无需依赖 Python 后端。
//
// 执行逻辑：
//  1. 检查配置中的 StaticDistDir 是否为空，空则跳过（纯 API 模式）
//  2. 检查目录是否存在且为合法目录，不存在则跳过
//  3. 检查 index.html 是否存在，不存在则跳过
//  4. 挂载 /assets 静态资源目录
//  5. 注册 SPA 回退路由：非 /api 路由一律返回 index.html
//
// 路由规则与 Python 后端保持一致：
//   - /assets/* → 静态资源（带内容哈希，可强缓存）
//   - 其余非 /api 路由 → 返回 index.html（SPA 回退，禁止缓存）
func (s *Server) registerStatic(r *gin.Engine) {
	// 读取配置中的静态文件目录路径
	distDir := s.cfg.StaticDistDir
	// 目录路径为空时直接返回，仅以 API 模式运行
	if distDir == "" {
		return
	}
	// 检查目录是否存在且为合法目录（非文件）
	info, err := os.Stat(distDir)
	if err != nil || !info.IsDir() {
		// 目录不存在或不是目录时打印日志并跳过，不阻止服务启动
		s.logger.Warn("static dist dir not found", "path", distDir, "serving", "API only")
		return
	}
	// 拼接 index.html 完整路径，检查其是否存在
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		// index.html 不存在说明前端未构建，跳过静态托管
		s.logger.Warn("index.html not found", "dir", distDir, "serving", "API only")
		return
	}

	// 检查 assets 子目录是否存在，存在则挂载为静态资源服务
	// /assets/* 路径下的文件带有内容哈希，浏览器可安全强缓存
	if assetsDir := filepath.Join(distDir, "assets"); dirExists(assetsDir) {
		// r.Static 将 /assets 路径映射到本地 assetsDir 目录，
		// Gin 会自动设置正确的 Content-Type 与 Last-Modified 头
		r.Static("/assets", assetsDir)
	}

	// 注册 NoRoute 处理器：当请求不匹配任何已注册路由时触发。
	// 用于实现 SPA 的前端路由回退：
	//   - /api/* 路径 → 返回 404 JSON 错误（API 路由未匹配说明请求无效）
	//   - 其他路径 → 返回 index.html（让前端 React Router 处理路由）
	r.NoRoute(func(c *gin.Context) {
		// 判断请求路径是否以 /api/ 开头
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			// API 路由未匹配，返回标准 404 JSON 响应
			c.JSON(http.StatusNotFound, gin.H{"detail": "Not Found", "status": http.StatusNotFound})
			return
		}
		// 非 API 路由：设置 no-cache 响应头，防止浏览器缓存 index.html。
		// 必须禁止缓存，否则重新构建前端后浏览器仍会加载旧版本的 index.html；
		// 而 /assets/* 下的带哈希资源则由浏览器正常缓存（内容变则 URL 变）。
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		// 返回 index.html 文件，由前端 React Router 接管后续路由
		c.File(indexPath)
	})
	// 打印静态托管启用日志，便于调试确认
	s.logger.Info("Console UI enabled", "static_dir", distDir)
}

// dirExists 判断指定路径是否存在且为目录。
// 用于静态文件托管前检查 assets 子目录是否可用。
func dirExists(path string) bool {
	// os.Stat 获取文件/目录信息，err != nil 表示不存在
	info, err := os.Stat(path)
	// 存在且为目录时返回 true
	return err == nil && info.IsDir()
}

// corsMiddleware is replaced by shared middleware.CORS() in RegisterRoutes.
// corsMiddleware 已由共享 middleware.CORS() 替代，保留函数签名以免破坏兼容性。

// Health 检查 Go 代理自身与上游 agent 的连通性，返回结构化健康状态。
//
// 响应字段与 Python 后端保持一致：
//   - backend：Go 代理自身状态（始终为 "ok"）
//   - agent：上游 agent 状态（"ok" 或 "unreachable"）
//   - agent_url：上游 agent 的 gRPC 地址
//   - latency_ms：Health RPC 调用耗时（毫秒）
//   - error：连接失败时的错误信息
//   - via：后端标识 "go-grpc"
//   - protocol：协议标识 "gRPC"
//
// 前端通过该接口判断后端连接是否正常，并展示状态灯。
func (s *Server) Health(c *gin.Context) {
	// 记录请求开始时间，用于计算 Health RPC 调用耗时
	start := time.Now()
	// 使用独立 30 秒超时的 Context 访问上游 gRPC：
	// - 避免前端快速频繁点击连发切断 HTTP 请求时，c.Request.Context() 被取消导致 gRPC Health 连带报错；
	// - 30 秒与客户端 waitForReady + 重试窗口匹配，agent 崩溃/重启期间（约 10~30 秒）
	//   健康检查会等待连接恢复而不是立即失败，agent 起来后自动返回 ok。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 通过 gRPC 客户端调用上游 agent 的 Health RPC
	resp, err := s.client.Health(ctx)
	// 计算调用耗时（毫秒）
	latency := time.Since(start).Milliseconds()

	if err != nil {
		// 上游 agent 不可达时，backend 仍为 "ok"（Go 代理自身正常），
		// agent 标记为 "unreachable" 并携带错误信息
		c.JSON(http.StatusOK, models.ConsoleHealth{
			Backend:   "ok",                 // Go 代理自身始终正常
			Agent:     "unreachable",        // 上游 agent 无法连接
			AgentURL:  s.cfg.AgentAddress(), // 上游 agent 地址，便于调试
			LatencyMs: &latency,             // 尝试连接的耗时
			Error:     err.Error(),          // 具体错误信息
			Via:       backendVia,           // "go-grpc"
			Protocol:  agentProtocol,        // "gRPC"
		})
		return
	}

	// 上游 agent 正常时，返回其状态与命名空间信息
	c.JSON(http.StatusOK, models.ConsoleHealth{
		Backend:   "ok",                                                                  // Go 代理自身正常
		Agent:     map[string]string{"status": resp.Status, "namespace": resp.Namespace}, // agent 状态详情
		AgentURL:  s.cfg.AgentAddress(),                                                  // 上游 agent 地址
		LatencyMs: &latency,                                                              // Health RPC 调用耗时
		Via:       backendVia,                                                            // "go-grpc"
		Protocol:  agentProtocol,                                                         // "gRPC"
	})
}

// Samples 返回所有 gRPC 支持端点的示例 payload 列表。
//
// 前端在启动时调用该接口获取所有端点的示例数据，
// 填充到侧边导航与请求编辑器中，供用户快速测试。
func (s *Server) Samples(c *gin.Context) {
	// samples.List() 返回内置的示例数据列表，直接序列化为 JSON 返回
	c.JSON(http.StatusOK, models.SamplesResponse{Samples: samples.List()})
}

// Proxy 将前端的单请求转发到上游 agent 的对应 gRPC 方法。
//
// 请求体格式（前端发送到 POST /api/proxy）：
//
//	{
//	  "method": "POST",
//	  "path": "/v1/privacy/mask",
//	  "body": {"field_name":"email","value":"alice@example.com"}
//	}
//
// 响应体格式：
//
//	{
//	  "status": 200,
//	  "duration_ms": 12,
//	  "data": { ... },
//	  "via": "go-grpc",
//	  "protocol": "gRPC"
//	}
//
// 执行逻辑：
//  1. 解析前端请求体为 ProxyRequest
//  2. 通过 mapper.Dispatch 根据 path 查找对应的 gRPC 方法并调用
//  3. 将 protobuf 响应转换为 JSON 可序列化的 map 结构
//  4. 返回统一的 ProxyResponse 格式
func (s *Server) Proxy(c *gin.Context) {
	// 解析请求体 JSON，绑定到 ProxyRequest 结构体
	var req models.ProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 请求体格式不合法时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request body: %v", err)})
		return
	}

	// 前端始终 POST 到 /api/proxy，但原始 method 携带在请求体中。
	// 这里忽略 req.Method，由 mapper 根据 path 决定 gRPC 调用语义。
	// 记录调用开始时间，用于计算 gRPC 调用耗时
	start := time.Now()
	// 拦截并转发纯 REST 路径（无 gRPC 对应）：
	// - /v1/dynclassification/* 动态分类分级
	// - /v1/ops/* 运维诊断
	// - /health 健康检查
	if restOnlyPath(req.Path) {
		s.proxyRest(c, start, req)
		return
	}

	// 核心调用：mapper 根据 req.Path 查找对应的 handler，
	// handler 负责解析 body、构造 protobuf 请求、调用 gRPC、转换响应。
	// 使用 grpcCallTimeout 超时包裹：waitForReady 开启后，若 agent 重启中请求会
	// 等待连接恢复后自动发送，超时兜底避免无限挂起。
	ctx, cancel := context.WithTimeout(c.Request.Context(), s.grpcCallTimeout())
	defer cancel()
	data, err := s.mapper.Dispatch(s.client.WithAuth(ctx), s.client.Raw(), req.Path, req.Body)
	// 计算 gRPC 调用耗时（毫秒）
	duration := time.Since(start).Milliseconds()

	if err != nil {
		// gRPC 调用失败时尝试回退到 REST 转发
		if strings.Contains(err.Error(), "unsupported gRPC path") {
			s.proxyRest(c, start, req)
			return
		}
		status := http.StatusBadRequest
		if isUnavailable(err) {
			status = http.StatusBadGateway // 上游连接类错误返回 502
		}
		c.JSON(status, gin.H{"detail": err.Error(), "status": status})
		return
	}

	// 调用成功，返回统一的 ProxyResponse 格式
	c.JSON(http.StatusOK, models.ProxyResponse{
		Status:     http.StatusOK, // HTTP 状态码 200
		DurationMs: duration,      // gRPC 调用耗时（毫秒）
		Data:       data,          // gRPC 响应转换后的 JSON 可序列化数据
		Via:        backendVia,    // "go-grpc"，标识响应经由的后端类型
		Protocol:   agentProtocol, // "gRPC"，标识与 agent 通信的协议
	})
}

// restOnlyPath 报告路径是否仅存在于 Agent REST 服务（无 gRPC 对应方法），
// 这些路径直接走 REST 转发。Proxy 与 ConcurrencyTest 共用同一判定。
func restOnlyPath(path string) bool {
	return strings.HasPrefix(path, "/v1/dynclassification/") ||
		strings.HasPrefix(path, "/v1/ops/") ||
		path == "/health"
}

// isAllowedConcurrencyPath 校验压测路径是否在白名单内。
// P37 fix: 防止通过压测端点访问敏感内部接口（如 /v1/ops/* 运维接口）。
// 白名单与 Proxy 路由保持一致，仅允许隐私处理与分类分级相关路径。
func isAllowedConcurrencyPath(rawPath string) bool {
	// path.Clean 先规范化路径，消除 ".." 穿越，防止 /v1/privacy/../ops/health 绕过前缀检查
	cleaned := path.Clean(rawPath)
	return strings.HasPrefix(cleaned, "/v1/privacy/") ||
		strings.HasPrefix(cleaned, "/v1/dynclassification/") ||
		strings.HasPrefix(cleaned, "/v1/medical/") ||
		strings.HasPrefix(cleaned, "/v1/pipeline/") ||
		cleaned == "/health"
}

// agentRestBaseURL 返回 agent REST 服务的基础地址。
// REST 与 gRPC 是 agent 的两个独立服务，主机/端口可能不同，
// 因此默认值使用 agent REST 默认地址（127.0.0.1:8079），
// 而非 gRPC 主机（AgentGRPCHost），避免配置不一致时路由到错误地址。
func agentRestBaseURL() string {
	if u := os.Getenv("PRIVACY_AGENT_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := os.Getenv("PRIVACY_AGENT_REST_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}

	restHost := os.Getenv("PRIVACY_AGENT_REST_HOST")
	if restHost == "" {
		restHost = os.Getenv("PRIVACY_REST_HOST")
	}
	if restHost == "" {
		restHost = os.Getenv("PRIVACY_AGENT_GRPC_HOST")
	}
	if restHost == "" {
		restHost = "127.0.0.1"
	}

	restPort := os.Getenv("PRIVACY_REST_PORT")
	if restPort == "" {
		restPort = "8079"
	}

	return fmt.Sprintf("http://%s:%s", restHost, restPort)
}

// extractRestErrorDetail 从上游错误响应体中提取可读的错误描述。
// 优先取 JSON 体中的 detail 字段（FastAPI 规范，对齐 Python 后端
// console/bff-py/app/client.py 的 _extract_detail 行为）；
// 非 JSON 或无 detail 字段时降级为截断后的原始文本，避免把整段
// HTML/堆栈塞进响应 detail。
func extractRestErrorDetail(body []byte, statusCode int) string {
	var data any
	if err := json.Unmarshal(body, &data); err == nil {
		if m, ok := data.(map[string]any); ok {
			if d, exists := m["detail"]; exists {
				if s, ok := d.(string); ok {
					return s
				}
				// detail 非字符串（如校验错误数组）时序列化为 JSON 返回
				if raw, err := json.Marshal(d); err == nil {
					return string(raw)
				}
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Sprintf("agent REST returned status %d", statusCode)
	}
	const maxDetailLen = 512
	if len(text) > maxDetailLen {
		text = text[:maxDetailLen] + "..."
	}
	return text
}

// callRest 执行底层的 HTTP REST 请求并返回解析后的数据、HTTP 状态码和可能的错误。
func (s *Server) callRest(ctx context.Context, method, path string, body json.RawMessage, rawPayloadB64, contentType string) (any, int, error) {
	method = strings.ToUpper(method)
	if method == "" {
		method = "POST"
	}
	targetURL := agentRestBaseURL() + path

	var reqBodyReader io.Reader
	if rawPayloadB64 != "" {
		rawBytes, err := base64.StdEncoding.DecodeString(rawPayloadB64)
		if err != nil {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid base64 payload: %w", err)
		}
		reqBodyReader = bytes.NewReader(rawBytes)
		// 二进制请求（如 Arrow IPC）：若附带 JSON body，将其作为 URL query params 传递
		if len(body) > 0 {
			var paramsMap map[string]any
			if err := json.Unmarshal(body, &paramsMap); err == nil && len(paramsMap) > 0 {
				q := url.Values{}
				for k, v := range paramsMap {
					q.Set(k, fmt.Sprintf("%v", v))
				}
				if strings.Contains(targetURL, "?") {
					targetURL += "&" + q.Encode()
				} else {
					targetURL += "?" + q.Encode()
				}
			}
		}
	} else if len(body) > 0 {
		reqBodyReader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, targetURL, reqBodyReader)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	} else {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if s.cfg.AgentAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.cfg.AgentAPIKey)
	}

	client := s.httpClient
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("Agent REST HTTP error: %w", err)
	}
	defer resp.Body.Close()

	// P32 fix: limit response body size to prevent OOM from malicious upstream
	const maxRespSize = 64 << 20 // 64 MB
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxRespSize))
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("reading REST response: %w", err)
	}
	if int64(len(respBytes)) >= maxRespSize {
		return nil, http.StatusBadGateway, fmt.Errorf("REST response exceeded %d MB limit", maxRespSize>>20)
	}

	var respData any
	_ = json.Unmarshal(respBytes, &respData)

	if resp.StatusCode >= 400 {
		detail := extractRestErrorDetail(respBytes, resp.StatusCode)
		return respData, resp.StatusCode, errors.New(detail)
	}

	return respData, resp.StatusCode, nil
}

// proxyRest 辅助函数：通过 HTTP 将 REST 请求透明代理到 Agent REST 服务
func (s *Server) proxyRest(c *gin.Context, start time.Time, req models.ProxyRequest) {
	respData, statusCode, err := s.callRest(c.Request.Context(), req.Method, req.Path, req.Body, req.RawPayloadB64, req.ContentType)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		c.JSON(statusCode, gin.H{"detail": err.Error(), "status": statusCode})
		return
	}

	c.JSON(http.StatusOK, models.ProxyResponse{
		Status:     http.StatusOK,
		DurationMs: duration,
		Data:       respData,
		Via:        "go-rest-proxy",
		Protocol:   "REST",
	})
}

// callRestOnce 以 REST 方式向 agent 发送单个请求（不写出 HTTP 响应），
// 供 ConcurrencyTest 压测 REST-only 路径或 gRPC 不支持路径的回退使用。
func (s *Server) callRestOnce(method, path string, body json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, err := s.callRest(ctx, method, path, body, "", "")
	return err
}

// Batch 逐个转发一组请求并汇总成功/失败统计。
//
// 用于前端“一键批量测试”：单个请求失败不会中断整个批次，
// 返回与 Python 后端一致的 {total, passed, failed, results} 结构。
//
// 执行逻辑：
//  1. 解析请求体为 BatchRequest（包含多个待转发请求）
//  2. 逐个转发请求（REST-only 路径走 REST，其它路径走 gRPC 并在 unsupported 时回退 REST）
//  3. 每个请求独立记录成功/失败与耗时
//  4. 汇总统计后返回 BatchResponse
func (s *Server) Batch(c *gin.Context) {
	// 解析请求体 JSON，绑定到 BatchRequest 结构体
	var req models.BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 请求体格式不合法时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request body: %v", err)})
		return
	}

	// P41 fix: 限制批量请求数量上限为 100，防止单次提交数千请求导致长时间占用连接（DoS 防护）
	const maxBatchSize = 100
	if len(req.Requests) > maxBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"detail": fmt.Sprintf("batch too large: %d requests (max %d)", len(req.Requests), maxBatchSize),
		})
		return
	}

	// 预分配结果切片，容量为请求数量以避免多次扩容
	results := make([]models.BatchResultItem, 0, len(req.Requests))
	// 成功计数器
	passed := 0
	// 逐个转发每个请求，单个失败不中断整个批次
	for _, item := range req.Requests {
		// 将 HTTP 方法转为大写（如 "post" → "POST"），用于结果展示
		method := strings.ToUpper(item.Method)
		if method == "" {
			method = "POST"
		}
		// 记录单个请求的开始时间
		start := time.Now()

		var (
			data       any
			statusCode int
			callErr    error
		)

		if restOnlyPath(item.Path) {
			data, statusCode, callErr = s.callRest(c.Request.Context(), method, item.Path, item.Body, item.RawPayloadB64, item.ContentType)
		} else {
			// 通过 mapper 转发到上游 agent 的对应 gRPC 方法。
			// 使用 grpcCallTimeout 超时包裹：agent 重启期间请求等待连接恢复而非立即失败。
			ctx, cancel := context.WithTimeout(c.Request.Context(), s.grpcCallTimeout())
			data, callErr = s.mapper.Dispatch(s.client.WithAuth(ctx), s.client.Raw(), item.Path, item.Body)
			cancel()

			if callErr != nil && strings.Contains(callErr.Error(), "unsupported gRPC path") {
				// gRPC 不支持该路径时回退到 REST 转发
				data, statusCode, callErr = s.callRest(c.Request.Context(), method, item.Path, item.Body, item.RawPayloadB64, item.ContentType)
			} else if callErr != nil {
				statusCode = http.StatusBadRequest
				if isUnavailable(callErr) {
					statusCode = http.StatusBadGateway // 上游不可达返回 502
				}
			} else {
				statusCode = http.StatusOK
			}
		}

		// 计算单个请求耗时（毫秒）
		duration := time.Since(start).Milliseconds()

		if callErr != nil {
			// 记录失败结果，包含错误信息，继续处理下一个请求
			results = append(results, models.BatchResultItem{
				Method:     method,      // HTTP 方法
				Path:       item.Path,   // 请求路径
				Status:     statusCode,  // HTTP 状态码
				DurationMs: duration,    // 耗时（毫秒）
				Error:      callErr.Error(), // 错误信息
			})
			continue // 跳过后续成功逻辑，处理下一个请求
		}

		// 请求成功：累加成功计数并记录结果
		passed++
		results = append(results, models.BatchResultItem{
			Method:     method,        // HTTP 方法
			Path:       item.Path,     // 请求路径
			Status:     http.StatusOK, // 成功状态码 200
			DurationMs: duration,      // 耗时（毫秒）
			Data:       data,          // 响应数据
		})
	}

	// 返回批量测试汇总结果
	c.JSON(http.StatusOK, models.BatchResponse{
		Total:    len(results),          // 总请求数
		Passed:   passed,                // 成功数
		Failed:   len(results) - passed, // 失败数
		Results:  results,               // 逐条结果详情
		Via:      backendVia,            // "go-grpc"
		Protocol: agentProtocol,         // "gRPC"
	})
}

// Upload 接收前端上传的 CSV/JSON 文件并执行隐私处理。
//
// 支持的表单字段：
//   - file：数据文件（.csv 或 .json）
//   - operation：操作类型（mask_dataframe | k_anonymize）
//   - params：JSON 字符串，如 {"columns":[...],"qi_cols":[...],"k":2,"context":""}
//
// 执行逻辑：
//  1. 从 multipart 表单中读取上传文件
//  2. 按文件扩展名解析为 records + schema
//  3. 解析 params JSON 为操作参数
//  4. 根据 operation 调用对应的 gRPC 方法
//  5. 返回统一的 ProxyResponse（data 为 UploadData）
func (s *Server) Upload(c *gin.Context) {
	// 从 multipart 表单中读取名为 "file" 的上传文件
	// file：文件读取句柄；header：文件元信息（文件名、大小等）
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// 缺少文件或读取失败时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("缺少文件: %v", err), "status": http.StatusBadRequest})
		return
	}
	// 注册 defer：函数退出时自动关闭文件句柄，释放资源
	defer file.Close()

	// 上传大小限制：超限返回 413，避免大文件耗尽内存（DoS 防护）。
	if s.cfg.MaxUploadBytes > 0 && header.Size > s.cfg.MaxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"detail": fmt.Sprintf("文件过大（%d 字节），上限 %d 字节", header.Size, s.cfg.MaxUploadBytes),
			"status": http.StatusRequestEntityTooLarge,
		})
		return
	}

	// 读取表单中的 operation 字段，决定执行哪种隐私处理操作
	operation := c.PostForm("operation")
	// 读取表单中的 params 字段，JSON 格式的操作参数
	params := c.PostForm("params")
	// params 为空时默认为空 JSON 对象，避免后续解析失败
	if params == "" {
		params = "{}"
	}

	// P34 fix: 读取文件内容，使用 io.LimitReader 防止 forged Content-Length 攻击
	// 即使 header.Size 检查通过，实际读取时仍加上限保护
	maxReadSize := int64(50 << 20) // 50 MB hard limit
	if s.cfg.MaxUploadBytes > 0 && s.cfg.MaxUploadBytes < maxReadSize {
		maxReadSize = s.cfg.MaxUploadBytes
	}
	content, err := io.ReadAll(io.LimitReader(file, maxReadSize+1))
	if err != nil {
		// 文件读取失败时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("读取文件失败: %v", err), "status": http.StatusBadRequest})
		return
	}
	if int64(len(content)) > maxReadSize {
		// 文件实际大小超过限制
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"detail": fmt.Sprintf("文件实际大小超过上限 %d 字节", maxReadSize),
			"status": http.StatusRequestEntityTooLarge,
		})
		return
	}

	// 按文件扩展名解析为 records（行数据）
	var records []map[string]string // 每行是一个 map[column_name]value
	// 将文件名转为小写，确保扩展名匹配不区分大小写
	filename := strings.ToLower(header.Filename)
	switch {
	case strings.HasSuffix(filename, ".csv"):
		// CSV 文件：解析表头为 schema，每行解析为 map
		records, _, err = fileparse.ParseCSV(content)
	case strings.HasSuffix(filename, ".json"):
		// JSON 文件：解析为对象数组，键名作为 schema
		records, _, err = fileparse.ParseJSON(content)
	default:
		// 不支持的文件格式时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": "仅支持 .csv 与 .json 文件", "status": http.StatusBadRequest})
		return
	}
	if err != nil {
		// 文件解析失败（如格式不合法）时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error(), "status": http.StatusBadRequest})
		return
	}

	// 解析 params 字段为 map，用于提取操作参数（columns、qi_cols、k 等）
	var options map[string]any
	if err := json.Unmarshal([]byte(params), &options); err != nil {
		// params 不是合法 JSON 时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("params 需为合法 JSON: %v", err), "status": http.StatusBadRequest})
		return
	}

	// 将 records 转换为 gRPC 的 RecordEntry 列表（protobuf 格式）
	entries := toRecordEntries(records)
	// 记录输入行数，用于响应中返回
	rowsIn := len(records)
	// 获取底层 gRPC 客户端，用于直接调用 RPC 方法
	client := s.client.Raw()
	// 使用请求的 context，支持客户端取消操作
	ctx := s.client.WithAuth(c.Request.Context())

	// 记录操作开始时间，用于计算总耗时
	start := time.Now()
	// result 保存最终操作结果（不同操作返回不同类型）
	var result any
	// rowsOut 保存输出行数
	var rowsOut int

	// 根据 operation 分发到对应的 gRPC 方法
	switch operation {
	case "mask_dataframe":
		// 脱敏操作：调用 MaskDataFrame gRPC 方法
		resp, e := client.MaskDataFrame(ctx, &pb.MaskDataFrameRequest{
			Data:    entries,                         // 输入数据
			Columns: stringSlice(options, "columns"), // 需脱敏的列名列表
			Context: stringVal(options, "context"),   // 脱敏上下文（影响脱敏策略）
		})
		if e != nil {
			// gRPC 调用失败时转换为 HTTP 错误响应
			s.writeUpstreamError(c, e)
			return
		}
		// 将 protobuf RecordEntry 列表转回 map 数组，便于 JSON 序列化
		result = recordEntriesToMaps(resp.Data)
		rowsOut = len(resp.Data) // 输出行数等于响应数据行数

	case "k_anonymize":
		// K-匿名操作：提取准标识符列名（必填参数）
		qiCols := stringSlice(options, "qi_cols")
		if len(qiCols) == 0 {
			// 缺少 qi_cols 参数时返回 400 错误
			c.JSON(http.StatusBadRequest, gin.H{"detail": "k_anonymize 操作需提供 qi_cols 参数", "status": http.StatusBadRequest})
			return
		}
		// 调用 KAnonymizeDataFrame gRPC 方法
		resp, e := client.KAnonymizeDataFrame(ctx, &pb.KAnonymizeDataFrameRequest{
			Data:     entries,                            // 输入数据
			QiCols:   qiCols,                             // 准标识符列名列表
			K:        int32Val(options, "k", 5),          // K 值，默认 5
			MaxDepth: int32Val(options, "max_depth", 10), // 最大泛化深度，默认 10
		})
		if e != nil {
			// gRPC 调用失败时转换为 HTTP 错误响应
			s.writeUpstreamError(c, e)
			return
		}
		// 将 protobuf RecordEntry 列表转回 map 数组
		result = recordEntriesToMaps(resp.Data)
		rowsOut = len(resp.Data) // 输出行数等于响应数据行数

	default:
		// 不支持的操作类型时返回 400 错误，并列出可选操作
		c.JSON(http.StatusBadRequest, gin.H{
			"detail": fmt.Sprintf("不支持的操作 '%s'，可选: k_anonymize, mask_dataframe", operation),
			"status": http.StatusBadRequest,
		})
		return
	}

	// 计算操作总耗时（毫秒）
	duration := time.Since(start).Milliseconds()
	// 返回统一的 ProxyResponse 格式，data 为 UploadData 结构
	c.JSON(http.StatusOK, models.ProxyResponse{
		Status:     http.StatusOK, // HTTP 状态码 200
		DurationMs: duration,      // 操作总耗时（毫秒）
		Data: models.UploadData{
			Operation: operation, // 操作类型
			RowsIn:    rowsIn,    // 输入行数
			RowsOut:   rowsOut,   // 输出行数
			Result:    result,    // 操作结果
		},
		Via:      backendVia,    // "go-grpc"
		Protocol: agentProtocol, // "gRPC"
	})
}

// LbTest 按策略向多个后端节点分发探测请求并统计结果。
//
// 由控制台后端自行实现策略分发（round_robin / random / least_connections），
// 探测目标为用户填写的各 agent REST 地址，返回各节点命中数与延迟分布。
//
// 执行逻辑：
//  1. 解析请求体为 LbTestRequest（包含节点列表、策略、探测次数等）
//  2. 调用 lbtest.Run 执行策略分发与探测
//  3. 返回各节点的命中数与延迟统计
func (s *Server) LbTest(c *gin.Context) {
	// 解析请求体 JSON，绑定到 LbTestRequest 结构体
	var req models.LbTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 请求体格式不合法时返回 400 错误
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request body: %v", err), "status": http.StatusBadRequest})
		return
	}
	// SSRF 防护：逐个校验探测目标 URL 的 scheme / host 白名单。
	if err := lbtest.ValidateBackends(req.Backends, splitHosts(s.cfg.LBAllowedHosts)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error(), "status": http.StatusBadRequest})
		return
	}
	// 调用 lbtest 模块执行负载均衡测试，第三个参数为可选的自定义 HTTP 客户端（nil 使用默认）
	resp, err := lbtest.Run(c.Request.Context(), req, nil)
	if err != nil {
		// 测试执行失败时返回 400 错误，包含具体错误信息
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error(), "status": http.StatusBadRequest})
		return
	}
	// 返回测试结果 JSON
	c.JSON(http.StatusOK, resp)
}

// grpcCallTimeout 返回单次 gRPC 调用的超时时间。
//
// 默认 60 秒；可用环境变量 PRIVACY_GRPC_CALL_TIMEOUT 覆盖（Go duration 格式，如 "30s"）。
// 作用：waitForReady 开启后，agent 重启期间 RPC 会等待连接恢复，该超时提供兜底，
// 避免连接长期不可用时请求无限挂起。
func (s *Server) grpcCallTimeout() time.Duration {
	if v := os.Getenv("PRIVACY_GRPC_CALL_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 60 * time.Second
}

// writeUpstreamError 将 gRPC 上游错误转换为 HTTP JSON 响应。
//
// 错误分类策略：
//   - 连接类错误（上游不可达/超时/DNS 失败）→ 502 Bad Gateway
//   - 其他错误（参数错误/业务错误）→ 400 Bad Request
//
// 该方法是 Proxy/Upload 等多个 handler 的公共错误处理入口。
func (s *Server) writeUpstreamError(c *gin.Context, err error) {
	// 默认返回 400（客户端错误）
	status := http.StatusBadRequest
	// 如果是上游连接类错误，则返回 502（网关错误）
	if isUnavailable(err) {
		status = http.StatusBadGateway
	}
	// 返回 JSON 格式的错误响应，包含错误详情与状态码
	c.JSON(status, gin.H{"detail": err.Error(), "status": status})
}

// toRecordEntries 将 Go map 数组转换为 gRPC RecordEntry 列表。
//
// 前端上传的文件解析结果为 []map[string]string，
// 而 gRPC 接口要求 []*pb.RecordEntry 格式，
// 本函数负责完成两种表示之间的转换。
func toRecordEntries(records []map[string]string) []*pb.RecordEntry {
	// 预分配切片，容量等于记录数以避免多次扩容
	entries := make([]*pb.RecordEntry, 0, len(records))
	// 遍历每条记录，将 map 转换为 RecordEntry 的 Fields 字段
	for _, r := range records {
		// 创建新 map 副本，避免修改原始数据
		fields := make(map[string]string, len(r))
		for k, v := range r {
			fields[k] = v
		}
		// 将 map 包装为 RecordEntry 并追加到结果列表
		entries = append(entries, &pb.RecordEntry{Fields: fields})
	}
	return entries
}

// recordEntriesToMaps 将 gRPC RecordEntry 列表转换回 Go map 数组。
//
// 与 toRecordEntries 相反，用于将 gRPC 响应转换为 JSON 可序列化格式，
// 便于前端直接展示。
func recordEntriesToMaps(entries []*pb.RecordEntry) []map[string]string {
	// 预分配切片，容量等于条目数
	out := make([]map[string]string, 0, len(entries))
	// 直接取出每个 RecordEntry 的 Fields map 追加到结果列表
	for _, e := range entries {
		out = append(out, e.Fields)
	}
	return out
}

// stringSlice 从 JSON 解析后的 map 中提取字符串数组字段。
//
// JSON 反序列化后数组类型为 []any，元素类型为 any，
// 本函数负责安全地类型断言并转换为 []string。
// 字段不存在或类型不匹配时返回 nil。
func stringSlice(m map[string]any, key string) []string {
	// 查找指定 key 是否存在
	if v, ok := m[key]; ok {
		// 尝试将值断言为 []any（JSON 数组反序列化后的默认类型）
		if arr, ok := v.([]any); ok {
			// 预分配切片，容量为数组长度
			out := make([]string, 0, len(arr))
			// 遍历数组元素，仅保留字符串类型的元素
			for _, item := range arr {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	// 字段不存在或类型不匹配时返回 nil
	return nil
}

// stringVal 从 JSON 解析后的 map 中提取字符串字段。
// 字段不存在或类型不是 string 时返回空字符串。
func stringVal(m map[string]any, key string) string {
	// 查找指定 key 是否存在
	if v, ok := m[key]; ok {
		// 尝试将值断言为 string 类型
		if s, ok := v.(string); ok {
			return s
		}
	}
	// 字段不存在或类型不匹配时返回空字符串
	return ""
}

// int32Val 从 JSON 解析后的 map 中提取整数字段。
//
// JSON 数字在 Go 中反序列化为 float64，
// 本函数支持 float64、int、int64 三种类型的安全转换。
// 字段不存在或类型不匹配时返回默认值 def。
func int32Val(m map[string]any, key string, def int32) int32 {
	// 查找指定 key 是否存在
	if v, ok := m[key]; ok {
		// 使用类型 switch 处理 JSON 数字可能的 Go 类型
		switch n := v.(type) {
		case float64:
			// JSON 数字默认反序列化为 float64，直接截断为 int32
			return int32(n)
		case int:
			// 部分场景下可能为 int 类型
			return int32(n)
		case int64:
			// 部分场景下可能为 int64 类型
			return int32(n)
		}
	}
	// 字段不存在或类型不匹配时返回默认值
	return def
}

// isUnavailable 判断错误是否表示上游 agent 不可达。
//
// 健壮性判断策略：
//  1. 优先通过 gRPC status.FromError 检查标准状态码（Unavailable / DeadlineExceeded / ResourceExhausted）；
//  2. 兜底检查错误文本中的底层网络连接异常关键字。
//
// 返回 true 表示应返回 502 Bad Gateway，false 表示应返回 400 Bad Request。
func isUnavailable(err error) bool {
	// nil 错误表示无异常，不属于不可达
	if err == nil {
		return false
	}
	// 1. gRPC 标准状态码类型检查
	if st, ok := status.FromError(err); ok {
		code := st.Code()
		if code == codes.Unavailable || code == codes.DeadlineExceeded || code == codes.ResourceExhausted {
			return true
		}
	}
	// 2. 文本关键字兜底匹配
	msg := err.Error()
	return containsAny(msg, []string{
		"connection refused", "dns", "timeout", "Unavailable",
		"connection reset", "broken pipe", "context deadline exceeded",
	})
}

// containsAny 检查字符串 s 是否包含 subs 列表中的任意一个子串。
// 用于 isUnavailable 中匹配连接类错误关键词。
func containsAny(s string, subs []string) bool {
	// 遍历子串列表，任一匹配即返回 true
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	// 全部不匹配时返回 false
	return false
}

// securityMiddleware 返回可选的 API Key 鉴权 + 限流中间件（默认关闭 / 宽松）。
//
//   - apiKey 非空时，/api/*（除 /api/health）需携带 Authorization: Bearer <key>；
//   - rateLimit > 0 时，每分钟每客户端 IP 超过该阈值返回 429（进程内滑动窗口）。
//
// CORS 预检（OPTIONS）已由 corsMiddleware 提前返回 204，不会进入本中间件；
// 静态资源等非 /api 路径与 /api/health 均子以豁免。
func securityMiddleware(apiKey string, rateLimit int) (gin.HandlerFunc, func()) {
	// 限流状态：每个客户端 IP 的请求时间戳列表（60 秒滑动窗口）。
	var mu sync.Mutex
	hits := make(map[string][]time.Time)

	// P57 fix: done channel signals the cleanup goroutine to exit on shutdown,
	// preventing goroutine leak when the server is stopped.
	done := make(chan struct{})

	// 后台 goroutine 定期清理过期 IP 条目，防止长期运行时 map 无限增长（内存泄漏）。
	// 每 5 分钟扫描一次，删除 60 秒内无请求的 IP 记录。
	if rateLimit > 0 {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					mu.Lock()
					cutoff := time.Now().Add(-60 * time.Second)
					for ip, window := range hits {
						// 过滤掉 60 秒内的记录；若过滤后为空则删除该 IP 条目
						kept := window[:0]
						for _, t := range window {
							if t.After(cutoff) {
								kept = append(kept, t)
							}
						}
						if len(kept) == 0 {
							delete(hits, ip)
						} else {
							hits[ip] = kept
						}
					}
					mu.Unlock()
				}
			}
		}()
	}

	cleanup := func() {
		select {
		case <-done:
			// already closed
		default:
			close(done)
		}
	}

	handler := func(c *gin.Context) {
		path := c.Request.URL.Path
		// 仅对 /api/* 生效；健康检查豁免。
		if !strings.HasPrefix(path, "/api/") || path == "/api/health" {
			c.Next()
			return
		}
		// API Key 鉴权（配置了才校验）。
		if apiKey != "" {
			token := extractBearer(c.GetHeader("Authorization"))
			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "Unauthorized: invalid console api key"})
				return
			}
		}
		// 限流（rateLimit <= 0 时关闭）。
		if rateLimit > 0 {
			ip := c.ClientIP()
			now := time.Now()
			cutoff := now.Add(-60 * time.Second)
			mu.Lock()
			window := hits[ip]
			// 就地过滤掉 60 秒窗口外的旧记录。
			kept := window[:0]
			for _, t := range window {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) >= rateLimit {
				hits[ip] = kept
				mu.Unlock()
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"detail": "Too many requests"})
				return
			}
			hits[ip] = append(kept, now)
			mu.Unlock()
		}
		c.Next()
	}
	return handler, cleanup
}

// extractBearer 从 Authorization 头提取 Bearer token，格式不符时返回空字符串。
func extractBearer(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}

// ConcurrencyTest 并发压测：以指定并发度向 agent 发送请求并统计延迟分布与吞吐量。
func (s *Server) ConcurrencyTest(c *gin.Context) {
	var req models.ConcurrencyTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": fmt.Sprintf("invalid request body: %v", err), "status": http.StatusBadRequest})
		return
	}
	if req.Path == "" {
		req.Path = "/v1/privacy/mask"
	}
	// P37 fix: validate path against allowlist to prevent SSRF via pressure test endpoint
	// 校验压测路径白名单，防止通过压测端点访问敏感内部接口
	if !isAllowedConcurrencyPath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{
			"detail": fmt.Sprintf("path %q not allowed for concurrency test; allowed prefixes: /v1/privacy/, /v1/dynclassification/, /v1/medical/, /v1/pipeline/, /health", req.Path),
			"status": http.StatusBadRequest,
		})
		return
	}
	if req.Method == "" {
		req.Method = "POST"
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 50
	}
	if req.Concurrency > 500 {
		req.Concurrency = 500
	}
	if req.TotalRequests <= 0 {
		req.TotalRequests = 200
	}
	if req.TotalRequests > 5000 {
		req.TotalRequests = 5000
	}

	total := req.TotalRequests
	concurrency := req.Concurrency
	if concurrency > total {
		concurrency = total
	}

	latencies := make([]float64, 0, total)
	var latenciesMu sync.Mutex
	var successCount, failedCount int

	jobs := make(chan struct{}, total)
	for i := 0; i < total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)

	// REST-only 路径（/health、/v1/dynclassification/* 等）无 gRPC 对应方法，
	// 与 Proxy 保持一致的前缀拦截：整段压测直接走 REST，避免全部请求失败。
	useREST := restOnlyPath(req.Path)

	startTime := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				start := time.Now()
				var err error
				if useREST {
					err = s.callRestOnce(req.Method, req.Path, req.Body)
				} else {
					ctx, cancel := context.WithTimeout(s.client.WithAuth(context.Background()), 30*time.Second)
					_, err = s.mapper.Dispatch(ctx, s.client.Raw(), req.Path, req.Body)
					cancel()
					// gRPC 不支持该路径时回退 REST（与 Proxy 的错误回退策略一致）
					if err != nil && strings.Contains(err.Error(), "unsupported gRPC path") {
						err = s.callRestOnce(req.Method, req.Path, req.Body)
					}
				}
				elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0

				latenciesMu.Lock()
				latencies = append(latencies, elapsedMs)
				if err == nil {
					successCount++
				} else {
					failedCount++
				}
				latenciesMu.Unlock()
			}
		}()
	}

	wg.Wait()
	durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0

	sort.Float64s(latencies)
	n := len(latencies)

	if n == 0 {
		c.JSON(http.StatusOK, models.ConcurrencyTestResponse{
			Total:        total,
			Success:      0,
			Failed:       total,
			DurationMs:   math.Round(durationMs*100) / 100,
			Qps:          0,
			AvgLatencyMs: 0,
			MinLatencyMs: 0,
			MaxLatencyMs: 0,
			P50LatencyMs: 0,
			P95LatencyMs: 0,
			P99LatencyMs: 0,
		})
		return
	}

	var sum float64
	for _, l := range latencies {
		sum += l
	}

	percentile := func(p float64) float64 {
		if n == 1 {
			return latencies[0]
		}
		k := float64(n-1) * (p / 100.0)
		f := int(k)
		cIdx := f + 1
		if cIdx >= n {
			cIdx = n - 1
		}
		if f == cIdx {
			return latencies[f]
		}
		return latencies[f]*(float64(cIdx)-k) + latencies[cIdx]*(k-float64(f))
	}

	qps := 0.0
	if durationMs > 0 {
		qps = float64(total) / (durationMs / 1000.0)
	}

	resp := models.ConcurrencyTestResponse{
		Total:        total,
		Success:      successCount,
		Failed:       failedCount,
		DurationMs:   math.Round(durationMs*100) / 100,
		Qps:          math.Round(qps*100) / 100,
		AvgLatencyMs: math.Round((sum/float64(n))*100) / 100,
		MinLatencyMs: math.Round(latencies[0]*100) / 100,
		MaxLatencyMs: math.Round(latencies[n-1]*100) / 100,
		P50LatencyMs: math.Round(percentile(50)*100) / 100,
		P95LatencyMs: math.Round(percentile(95)*100) / 100,
		P99LatencyMs: math.Round(percentile(99)*100) / 100,
	}

	c.JSON(http.StatusOK, resp)
}

// loadSampleRecords 读取 internal/samples 下的内置 CSV 样本。
// 优先解析相对 StaticDistDir 的部署布局路径，其次回退到 CWD 相对路径；
// 文件缺失或解析为空时返回明确错误，调用方应映射为 404 而非转发空数据。
func (s *Server) loadSampleRecords(name string) ([]map[string]string, error) {
	samplePath := filepath.Join(s.cfg.StaticDistDir, "..", "internal", "samples", name)
	if _, err := os.Stat(samplePath); err != nil {
		samplePath = filepath.Join("internal", "samples", name)
	}
	data, err := os.ReadFile(samplePath)
	if err != nil {
		return nil, fmt.Errorf("示例数据文件缺失: %s", name)
	}
	parsed, _, err := fileparse.ParseCSV(data)
	if err != nil || len(parsed) == 0 {
		return nil, fmt.Errorf("示例数据文件为空或解析失败: %s", name)
	}
	return parsed, nil
}

// MedicalPipeline 医疗敏感数据全流程治理代理端点：分类分级与 L4/L5 数据脱敏。
func (s *Server) MedicalPipeline(c *gin.Context) {
	var body struct {
		Records []map[string]string `json:"records"`
	}
	_ = c.ShouldBindJSON(&body)

	records := body.Records
	if len(records) == 0 {
		loaded, err := s.loadSampleRecords("kangyang.csv")
		if err != nil {
			// 明确报错而非代理空记录集，避免前端把"样本缺失"误显示为"0 条记录"
			c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "status": http.StatusNotFound})
			return
		}
		records = loaded
	}

	start := time.Now()
	proxyReq := models.ProxyRequest{
		Method: "POST",
		Path:   "/v1/medical/process",
	}
	reqBytes, _ := json.Marshal(map[string]any{"records": records})
	proxyReq.Body = reqBytes

	s.proxyRest(c, start, proxyReq)
}

// YibaoPipeline 医保结算数据全流程治理代理端点：读入 yibao.csv 18 字段进行治理。
func (s *Server) YibaoPipeline(c *gin.Context) {
	var body struct {
		Records []map[string]string `json:"records"`
		Dataset string              `json:"dataset"`
	}
	_ = c.ShouldBindJSON(&body)

	records := body.Records
	if len(records) == 0 {
		loaded, err := s.loadSampleRecords("yibao.csv")
		if err != nil {
			// 明确报错而非代理空记录集，避免前端把"样本缺失"误显示为"0 条记录"
			c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "status": http.StatusNotFound})
			return
		}
		records = loaded
	}

	start := time.Now()
	proxyReq := models.ProxyRequest{
		Method: "POST",
		Path:   "/v1/medical/process",
	}
	reqBytes, _ := json.Marshal(map[string]any{"records": records})
	proxyReq.Body = reqBytes

	s.proxyRest(c, start, proxyReq)
}

// PipelineProcess 通用分类分级与脱敏流水线代理端点。
func (s *Server) PipelineProcess(c *gin.Context) {
	var body struct {
		Records  []map[string]string `json:"records"`
		Standard string              `json:"standard"`
		MaskL4   *bool               `json:"mask_l4"`
		MaskL5   *bool               `json:"mask_l5"`
	}
	_ = c.ShouldBindJSON(&body)

	records := body.Records
	if len(records) == 0 {
		loaded, err := s.loadSampleRecords("kangyang.csv")
		if err != nil {
			// 明确报错而非代理空记录集，避免前端把"样本缺失"误显示为"0 条记录"
			c.JSON(http.StatusNotFound, gin.H{"detail": err.Error(), "status": http.StatusNotFound})
			return
		}
		records = loaded
	}

	standard := body.Standard
	if standard == "" {
		standard = "jrt0197"
	}
	maskL4 := true
	if body.MaskL4 != nil {
		maskL4 = *body.MaskL4
	}
	maskL5 := true
	if body.MaskL5 != nil {
		maskL5 = *body.MaskL5
	}

	start := time.Now()
	proxyReq := models.ProxyRequest{
		Method: "POST",
		Path:   "/v1/pipeline/process_records",
	}
	reqBytes, _ := json.Marshal(map[string]any{
		"records":  records,
		"standard": standard,
		"mask_l4":  maskL4,
		"mask_l5":  maskL5,
	})
	proxyReq.Body = reqBytes

	s.proxyRest(c, start, proxyReq)
}

// splitHosts 把逗号分隔的 host 白名单字符串拆分为去除空白后的切片；
// 空字符串返回 nil（表示不限制）。
func splitHosts(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
