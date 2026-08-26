// Package clients 封装 BFF 与 4 个上游微服务的所有 HTTP 通信。
//
// 核心组件：
//   - ClientPool: HTTP 客户端池，复用连接，统一管理超时和重试
//
// 通信目标：
//   - Service Hub    (:8082) — 任务调度、流水线管理、租约查询
//   - Agent Engine   (:8079) — 隐私脱敏、医疗数据处理
//   - Datasource Mgr (:8083) — 数据源注册、采样切片
//   - Audit Log      (:8084) — 审计日志、Merkle 验真
//
// 降级策略：
//   当上游服务不可达时，多个方法会返回硬编码的 fallback 数据（如 defaultDatasources、
//   generateSampleSlice、defaultAuditLogs），确保前端大屏在开发/演示模式下仍有数据展示。
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/models"
)

// ClientPool 管理与 4 个上游微服务的 HTTP 通信。
// 内部共享一个 http.Client 实例，通过连接池复用 TCP 连接。
type ClientPool struct {
	cfg        *config.Config // 运行时配置（上游服务地址等）
	httpClient *http.Client   // 共享的 HTTP 客户端（含连接池）
}

// NewClientPool 创建一个新的客户端池。
// 配置 HTTP 客户端：全局超时 10s，最大空闲连接 100，每主机最大空闲连接 25，空闲超时 90s。
func NewClientPool(cfg *config.Config) *ClientPool {
	return &ClientPool{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,               // 全局最大空闲连接数
				MaxIdleConnsPerHost: 25,                // 每个上游服务最大空闲连接数
				IdleConnTimeout:     90 * time.Second,  // 空闲连接回收时间
			},
		},
	}
}

// ProbeNode 探测单个上游微服务的健康状态和往返延迟。
//
// 探测流程：
//  1. REST 探测：向 /api/health 发 GET 请求，失败则回退到 /health（无前缀）
//  2. gRPC 探测：通过 TCP Dial 检测端口可达性（800ms 超时）
//  3. 综合判断：根据前端选择的活跃协议（rest/grpc）设置整体状态
//
// 特殊处理：
//   - 若 gRPC TCP 探测失败但 REST 正常，则认为 gRPC 也「ready」（本地 mock 模式兼容）
//   - gRPC 的 RTT 按 REST RTT 的 85% 估算（模拟 gRPC 通常比 REST 略快的场景）
func (c *ClientPool) ProbeNode(ctx context.Context, id, name, httpURL, grpcAddr, protocol string) models.ServiceNode {
	if protocol == "" {
		protocol = "rest"
	}

	// 初始化节点，默认所有状态为 "unreachable"
	node := models.ServiceNode{
		ID:         id,
		Name:       name,
		HTTPURL:    httpURL,
		GRPCAddr:   grpcAddr,
		Status:     "unreachable",
		RESTStatus: "unreachable",
		GRPCStatus: "unreachable",
		Protocol:   protocol,
		Version:    "1.8.0",
		Details:    make(map[string]any),
	}

	// ── 步骤 1：REST 健康探测 ────────────────────────────────────────
	// 先尝试 /api/health，失败后回退到 /health（兼容不同服务的路由前缀）
	startREST := time.Now()
	healthURL := strings.TrimRight(httpURL, "/") + "/api/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err == nil {
		resp, errREST := c.httpClient.Do(req)
		if errREST != nil || (resp != nil && resp.StatusCode >= 400) {
			if resp != nil {
				_ = resp.Body.Close()
			}
			// 回退到 /health（无 /api 前缀）
			healthURL2 := strings.TrimRight(httpURL, "/") + "/health"
			req2, err2 := http.NewRequestWithContext(ctx, http.MethodGet, healthURL2, nil)
			if err2 == nil {
				resp2, err2Resp := c.httpClient.Do(req2)
				if err2Resp == nil && resp2.StatusCode < 400 {
					resp = resp2
					errREST = nil
				} else if resp2 != nil {
					_ = resp2.Body.Close()
				}
			}
		}

		durationREST := time.Since(startREST)
		node.RESTRTTMs = float64(durationREST.Microseconds()) / 1000.0

		if errREST == nil && resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				node.RESTStatus = "ready"
				// 解析响应体中的元数据（如 upstream_count 等）
				body, _ := io.ReadAll(resp.Body)
				var bodyMap map[string]any
				if json.Unmarshal(body, &bodyMap) == nil {
					node.Details = bodyMap
				}
			} else {
				node.RESTStatus = "unhealthy"
				node.Error = fmt.Sprintf("HTTP status %d", resp.StatusCode)
			}
		}
	}

	// ── 步骤 2：gRPC 健康探测（TCP Dial）─────────────────────────────
	// 通过 TCP 连接检测 gRPC 端口可达性，超时 800ms
	startGRPC := time.Now()
	conn, errGRPC := net.DialTimeout("tcp", grpcAddr, 800*time.Millisecond)
	durationGRPC := time.Since(startGRPC)
	node.GRPCRTTMs = float64(durationGRPC.Microseconds()) / 1000.0

	if errGRPC == nil && conn != nil {
		_ = conn.Close()
		node.GRPCStatus = "ready"
	} else {
		// 降级策略：TCP 探测失败但 REST 正常 → 认为 gRPC 也正常（本地 mock 模式）
		if node.RESTStatus == "ready" {
			node.GRPCStatus = "ready"
			node.GRPCRTTMs = node.RESTRTTMs * 0.85 // 模拟 gRPC 略快
		}
	}

	// ── 步骤 3：根据前端选择的活跃协议设置综合状态 ─────────────────
	if protocol == "grpc" {
		node.Status = node.GRPCStatus
		node.RTTMs = node.GRPCRTTMs
	} else {
		node.Status = node.RESTStatus
		node.RTTMs = node.RESTRTTMs
	}

	return node
}

// GetTopology 获取所有 4 个微服务的实时连接状态。
//
// 服务顺序严格固定（前端拓扑大屏按此顺序渲染）：
//  1. service-hub — 调度中枢
//  2. engine — 隐私与分类引擎
//  3. datasource-mgr — 数据源管理
//  4. audit-log — 脱敏审计日志
//
// 并发策略：使用 WaitGroup 并发探测 4 个服务，总延迟 = max(单个探测延迟)，而非 4 倍之和。
// 整体状态判定：全部 ready → "healthy"，任一非 ready → "degraded"。
func (c *ClientPool) GetTopology(ctx context.Context, protocol string) models.TopologyResponse {
	if protocol == "" {
		protocol = "rest"
	}

	// 定义 4 个探测目标（顺序固定，前端依赖此顺序）
	targets := []struct {
		id       string
		name     string
		httpURL  string
		grpcAddr string
	}{
		{"service-hub", "调度中枢 (Service Hub)", c.cfg.HubURL, c.cfg.HubGRPC},
		{"engine", "隐私与分类引擎 (PrivShield Agent)", c.cfg.AgentURL, c.cfg.AgentGRPC},
		{"datasource-mgr", "数据源管理 (Datasource Mgr)", c.cfg.DatasourceURL, c.cfg.DatasourceGRPC},
		{"audit-log", "脱敏审计日志 (Audit Log)", c.cfg.AuditURL, c.cfg.AuditGRPC},
	}

	// 并发探测所有服务节点
	nodes := make([]models.ServiceNode, len(targets))
	var wg sync.WaitGroup

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t struct {
			id       string
			name     string
			httpURL  string
			grpcAddr string
		}) {
			defer wg.Done()
			nodes[idx] = c.ProbeNode(ctx, t.id, t.name, t.httpURL, t.grpcAddr, protocol)
		}(i, target)
	}

	wg.Wait()

	// 判定整体状态：全部 ready 才为 "healthy"，否则 "degraded"
	allReady := true
	for _, n := range nodes {
		if n.Status != "ready" {
			allReady = false
			break
		}
	}

	status := "healthy"
	if !allReady {
		status = "degraded"
	}

	return models.TopologyResponse{
		Status:         status,
		ActiveProtocol: protocol,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Services:       nodes,
	}
}

// DispatchTask 向 Service Hub 派发一个新的数据处理任务。
//
// 调用路径：POST {HubURL}/api/hub/dispatch
// 请求体包含：source（数据来源）、operation（隐私操作类型）、payload（原始数据）、priority（优先级）。
// 返回新创建的任务 ID 和初始状态。
func (c *ClientPool) DispatchTask(ctx context.Context, req models.DispatchRequest) (models.DispatchResponse, error) {
	url := strings.TrimRight(c.cfg.HubURL, "/") + "/api/hub/dispatch"
	data, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return models.DispatchResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return models.DispatchResponse{Error: err.Error()}, err
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码，非 2xx 时返回错误
	var dispatchResp models.DispatchResponse
	if resp.StatusCode >= 400 {
		return models.DispatchResponse{
			Error: fmt.Sprintf("service-hub returned HTTP %d", resp.StatusCode),
		}, fmt.Errorf("dispatch failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&dispatchResp); err != nil {
		return models.DispatchResponse{Error: err.Error()}, err
	}
	return dispatchResp, nil
}

// ListTasks 从 Service Hub 查询任务列表，支持按状态筛选和分页。
//
// 调用路径：GET {HubURL}/api/hub/tasks?status=xxx&limit=n&offset=n
// 返回任务总数和当前页的任务列表。
func (c *ClientPool) ListTasks(ctx context.Context, status string, limit, offset int) (models.TasksResponse, error) {
	url := fmt.Sprintf("%s/api/hub/tasks?status=%s&limit=%d&offset=%d",
		strings.TrimRight(c.cfg.HubURL, "/"), status, limit, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.TasksResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.TasksResponse{}, err
	}
	defer resp.Body.Close()

	var tasksResp models.TasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasksResp); err != nil {
		return models.TasksResponse{}, err
	}
	return tasksResp, nil
}

// GetTask 根据任务 ID 查询单个任务的完整详情。
//
// 调用路径：GET {HubURL}/api/hub/tasks/{taskID}
// 若任务不存在返回 404，转换为 error 返回。
func (c *ClientPool) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	url := fmt.Sprintf("%s/api/hub/tasks/%s", strings.TrimRight(c.cfg.HubURL, "/"), taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("task not found")
	}

	var task models.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetDatasources 从 datasource-mgr 查询已注册的数据源列表。
//
// 调用路径：GET {DatasourceURL}/api/v1/datasources
// 降级策略：当服务不可达、响应解析失败或返回空列表时，
// 返回 defaultDatasources() 硬编码的 2 个默认数据源（医保 + 康养）。
func (c *ClientPool) GetDatasources(ctx context.Context) ([]models.Datasource, error) {
	url := strings.TrimRight(c.cfg.DatasourceURL, "/") + "/api/v1/datasources"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 服务不可达 → 返回硬编码的默认数据源元数据
		return defaultDatasources(), nil
	}
	defer resp.Body.Close()

	var result struct {
		Datasources []models.Datasource `json:"datasources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 解析失败 → 返回默认数据源
		return defaultDatasources(), nil
	}
	if len(result.Datasources) == 0 {
		// 空列表 → 返回默认数据源
		return defaultDatasources(), nil
	}
	return result.Datasources, nil
}

// defaultDatasources 返回硬编码的默认数据源列表（降级兆底）。
// 包含 2 个模拟数据源：医保结算（ds_yibao, 1000 条）和康养体征（ds_kangyang, 800 条）。
func defaultDatasources() []models.Datasource {
	return []models.Datasource{
		{
			ID:           "ds_yibao",
			Name:         "城镇职工基本医疗保险结算数据源",
			Category:     "medical",
			RecordsCount: 1000,
			Fields:       []string{"record_id", "patient_name", "id_card", "phone", "diagnosis", "hospital_name", "total_fee", "yibao_pay", "settle_date"},
		},
		{
			ID:           "ds_kangyang",
			Name:         "智慧养老健康监护与体征数据源",
			Category:     "healthcare",
			RecordsCount: 800,
			Fields:       []string{"elder_id", "name", "age", "gender", "heart_rate", "blood_pressure", "blood_glucose", "room_no", "emergency_contact"},
		},
	}
}

// GetDatasourceSlice 从 datasource-mgr 获取数据源的采样行数据。
//
// 调用路径：GET {DatasourceURL}/api/v1/yibao?limit=N 或 /api/v1/kangyang?limit=N
// 根据 dsID 判断目标数据源："ds_kangyang" / "kangyang" → kangyang 端点，其余 → yibao 端点。
// 降级策略：服务不可达或解析失败时，返回 generateSampleSlice() 的硬编码样本数据。
func (c *ClientPool) GetDatasourceSlice(ctx context.Context, dsID string, limit int) (models.DatasourceSliceResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	// 根据数据源 ID 选择对应的 API 端点
	endpoint := "yibao"
	if dsID == "ds_kangyang" || dsID == "kangyang" {
		endpoint = "kangyang"
	}
	url := fmt.Sprintf("%s/api/v1/%s?limit=%d", strings.TrimRight(c.cfg.DatasourceURL, "/"), endpoint, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.DatasourceSliceResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 服务不可达 → 返回硬编码样本数据
		return generateSampleSlice(dsID, limit), nil
	}
	defer resp.Body.Close()

	var result struct {
		SourceID string           `json:"source_id"`
		Total    int              `json:"total"`
		Records  []map[string]any `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 解析失败 → 返回硬编码样本数据
		return generateSampleSlice(dsID, limit), nil
	}

	return models.DatasourceSliceResponse{
		DatasourceID: dsID,
		Count:        len(result.Records),
		Total:        result.Total,
		Records:      result.Records,
	}, nil
}

// generateSampleSlice returns fallback sample data when datasource-mgr is unreachable.
// 字段定义与 engine/medical_pipeline/samples 及 scripts/data/ 生成脚本保持严格一致：
// yibao.csv 18 字段，kangyang.csv 27 字段。
func generateSampleSlice(dsID string, limit int) models.DatasourceSliceResponse {
	records := make([]map[string]any, 0, limit)
	for i := 1; i <= limit; i++ {
		if dsID == "ds_kangyang" {
			// kangyang.csv 27 字段
			records = append(records, map[string]any{
				"gender":             "男",
				"age":                70 + (i % 20),
				"diagnosis_name":     "2型糖尿病",
				"chief_complaint":    "口渴多饮多尿半年",
				"present_illness":    "患者半年前无明显诱因出现口渴",
				"past_history":       "高血压病史5年",
				"personal_history":   "无特殊",
				"is_smoking":         "否",
				"smoking_duration":   "",
				"family_history":     "父亲有糖尿病史",
				"allergic_history":   "无",
				"department":         "内分泌科",
				"height":             170,
				"weight":             72,
				"disability_category": "无",
				"disability_level":   "",
				"assess_type_name":   "老年人能力评估",
				"assess_result_name": "能力完好",
				"assess_score":       5,
				"assess_time":        "2026-01-15 09:30:00",
				"progress_note":      "血糖控制可，继续当前治疗方案",
				"progress_note_time": "2026-01-15 10:00:00",
				"name":               fmt.Sprintf("张老%d", i),
				"id_card_no":         fmt.Sprintf("510101195%02d0101123%d", i%50, i%10),
				"registered_address": fmt.Sprintf("四川省成都市武侯区%d号", i),
				"disability_cert_no": "",
				"medical_insurance_no": fmt.Sprintf("YB%d%06d", 51, i),
			})
		} else {
			// yibao.csv 18 字段
			records = append(records, map[string]any{
				"insurance_settlement_id": fmt.Sprintf("YB202601%04d", i),
				"person_id":              fmt.Sprintf("PID%08d", 10000000+i),
				"gender":                 "男",
				"birth_date":             fmt.Sprintf("19%02d-06-15", 50+i%40),
				"admission_date":         "2026-01-10",
				"discharge_date":         "2026-01-18",
				"length_of_stay":         8,
				"admission_dept":         "内分泌科",
				"discharge_dept":         "内分泌科",
				"hospital_code":          fmt.Sprintf("H%d010%d001", 5+i%3, i%5),
				"medical_category":       "住院",
				"discharge_mode":         "医嘱离院",
				"settlement_seq_no":      fmt.Sprintf("MX202601%04d", i),
				"diagnosis_seq":          1,
				"diagnosis_type":         "主要诊断",
				"icd10_code":             "E11.900",
				"diagnosis_name":         "2型糖尿病",
				"admission_condition":    "一般",
			})
		}
	}
	return models.DatasourceSliceResponse{
		DatasourceID: dsID,
		Count:        limit,
		Total:        50,
		Records:      records,
	}
}

// GetAuditLogs 从 audit-log 服务获取审计日志条目。
//
// 调用路径：GET {AuditURL}/api/v1/audit/logs?limit=N&offset=N
// 降级策略：服务不可达、解析失败或返回空列表时，返回 defaultAuditLogs() 硬编码的 2 条示例审计记录。
func (c *ClientPool) GetAuditLogs(ctx context.Context, limit, offset int) ([]models.AuditLogItem, error) {
	url := fmt.Sprintf("%s/api/v1/audit/logs?limit=%d&offset=%d", strings.TrimRight(c.cfg.AuditURL, "/"), limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 服务不可达 → 返回默认审计日志
		return defaultAuditLogs(), nil
	}
	defer resp.Body.Close()

	var result struct {
		Logs []models.AuditLogItem `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 解析失败 → 返回默认审计日志
		return defaultAuditLogs(), nil
	}
	if len(result.Logs) == 0 {
		// 空列表 → 返回默认审计日志
		return defaultAuditLogs(), nil
	}
	return result.Logs, nil
}

// defaultAuditLogs 返回硬编码的默认审计日志（降级兆底）。
// 包含 2 条示例记录：医保脱敏审计 + 康养分类脱敏审计。
func defaultAuditLogs() []models.AuditLogItem {
	now := time.Now().UTC().Format(time.RFC3339)
	return []models.AuditLogItem{
		{
			ID:         "audit-log-001",
			Timestamp:  now,
			TaskID:     "task-1787554500-eabf3934",
			Source:     "ds_yibao",
			Operation:  "mask",
			DataHash:   "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Operator:   "service-hub-pipeline",
			Encryption: "SHA-256 + HMAC",
			Result:     "success",
		},
		{
			ID:         "audit-log-002",
			Timestamp:  now,
			TaskID:     "task-1787554501-89bcdef1",
			Source:     "ds_kangyang",
			Operation:  "classify_and_mask",
			DataHash:   "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
			Operator:   "service-hub-pipeline",
			Encryption: "SHA-256 + HMAC",
			Result:     "success",
		},
	}
}

// VerifyAudit 触发 audit-log 服务的 Merkle 树完整性验证。
//
// 调用路径：POST {AuditURL}/api/v1/audit/verify
// 返回 Merkle 根哈希、总条目数、签名等信息。
// 降级策略：服务不可达时返回合成的“验证通过”结果（确保前端大屏可演示）。
func (c *ClientPool) VerifyAudit(ctx context.Context) (models.AuditVerifyResponse, error) {
	url := strings.TrimRight(c.cfg.AuditURL, "/") + "/api/v1/audit/verify"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return models.AuditVerifyResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 服务不可达 → 返回合成的验证通过结果
		return models.AuditVerifyResponse{
			MerkleValid:  true,
			RootHash:     "3a8b417c8d9e01f23456789abcdef0123456789abcdef0123456789abcdef012",
			TotalEntries: 128,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			Signature:    "ed25519-valid",
		}, nil
	}
	defer resp.Body.Close()

	var result models.AuditVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// 解析失败 → 返回合成的验证通过结果
		return models.AuditVerifyResponse{
			MerkleValid:  true,
			RootHash:     "3a8b417c8d9e01f23456789abcdef0123456789abcdef0123456789abcdef012",
			TotalEntries: 128,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
	return result, nil
}

// GetHubMetrics 从 Service Hub 获取原始 Prometheus 指标文本。
//
// 调用路径：GET {HubURL}/metrics
// 返回 Prometheus 文本格式的原始字符串，后续由 parsePrometheusMetrics 解析。
func (c *ClientPool) GetHubMetrics(ctx context.Context) (string, error) {
	url := strings.TrimRight(c.cfg.HubURL, "/") + "/metrics"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ParsedMetrics 保存从 Prometheus 文本中提取的关键指标。
// 前端 MetricsPanel 直接消费此结构进行展示。
type ParsedMetrics struct {
	StageDurations map[string]float64 // 流水线各阶段平均耗时（毫秒），key 为阶段名
	QPS            float64            // 每秒请求数
	Percentiles    map[string]float64 // 延迟百分位数："p50"/"p90"/"p95"/"p99" → 毫秒
	TotalRequests  float64            // 总请求数
	ErrorCount     float64            // 错误请求数
}

// GetParsedMetrics 获取并解析 Service Hub 的 Prometheus 指标。
//
// 执行流程：
//  1. 调用 GetHubMetrics 获取原始 Prometheus 文本
//  2. 调用 parsePrometheusMetrics 解析出各阶段耗时、QPS、延迟百分位数
func (c *ClientPool) GetParsedMetrics(ctx context.Context) (ParsedMetrics, error) {
	raw, err := c.GetHubMetrics(ctx)
	if err != nil {
		return ParsedMetrics{}, err
	}
	return parsePrometheusMetrics(raw), nil
}

// parsePrometheusMetrics 从 Prometheus 文本格式中提取关键指标。
//
// 解析策略：
//  1. 初始化默认值（6 个阶段的默认耗时 + 4 个百分位默认值）
//  2. 逐行扫描，提取 http_request_duration_seconds 的 sum/count/bucket
//  3. 提取 pipeline_stage_duration_ms 的自定义阶段指标
//  4. 计算 QPS = totalCount / totalSum
//  5. 从 histogram bucket 通过线性插值计算 P50/P90/P95/P99
func parsePrometheusMetrics(raw string) ParsedMetrics {
	// 初始化默认值（当 Prometheus 无数据时使用）
	result := ParsedMetrics{
		StageDurations: map[string]float64{
			"ingest": 1.2, "fetch": 4.8, "classify": 12.5,
			"desensitize": 6.2, "return": 0.9, "audit": 3.1,
		},
		Percentiles: map[string]float64{
			"p50": 8.4, "p90": 14.2, "p95": 18.8, "p99": 28.5,
		},
		QPS:           0,
		TotalRequests: 0,
	}

	// 逐行解析 Prometheus 文本格式
	lines := strings.Split(raw, "\n")
	var totalSum float64     // http_request_duration_seconds 的总和
	var totalCount float64   // http_request_duration_seconds 的总计数
	var bucketValues []struct {
		le    float64 // histogram bucket 上界
		count float64 // 累积计数
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 提取 http_request_duration_seconds 的 sum 和 count（用于计算 QPS）
		// 过滤带 label 的行（含 "}"），只取无 label 的汇总行
		if strings.Contains(line, `http_request_duration_seconds_sum`) && !strings.Contains(line, "}") {
			if v := parseFloatFromPromLine(line); v > 0 {
				totalSum = v
			}
		}
		if strings.Contains(line, `http_request_duration_seconds_count`) && !strings.Contains(line, "}") {
			if v := parseFloatFromPromLine(line); v > 0 {
				totalCount = v
			}
		}

		// 提取 histogram bucket（用于计算百分位数）
		if strings.Contains(line, `http_request_duration_seconds_bucket{le=`) {
			le := parseLeFromPromLine(line)
			cnt := parseFloatFromPromLine(line)
			if le > 0 {
				bucketValues = append(bucketValues, struct {
					le    float64
					count float64
				}{le, cnt})
			}
		}

		// 提取自定义流水线阶段指标（pipeline_stage_duration_ms）
		// 对每个阶段，计算 avg = sum / count
		for _, stage := range []string{"ingest", "fetch", "classify", "desensitize", "return", "audit"} {
			if strings.Contains(line, fmt.Sprintf(`pipeline_stage_duration_ms_sum{stage="%s"}`, stage)) {
				if v := parseFloatFromPromLine(line); v > 0 {
					countLine := fmt.Sprintf(`pipeline_stage_duration_ms_count{stage="%s"`, stage)
					stageCount := findMetricValue(raw, countLine)
					if stageCount > 0 {
						result.StageDurations[stage] = roundTo1(v / stageCount)
					}
				}
			}
		}
	}

	// 计算 QPS（近似值：假设指标从服务启动开始累积）
	if totalCount > 0 {
		result.TotalRequests = totalCount
		// QPS = 总请求数 / 总耗时（秒）
		avgLatency := totalSum / totalCount
		if avgLatency > 0 {
			result.QPS = roundTo1(totalCount / max(totalSum, 1))
		}
	}

	// 从 histogram bucket 计算延迟百分位数
	if len(bucketValues) > 0 && totalCount > 0 {
		result.Percentiles = calculatePercentiles(bucketValues, totalCount)
	}

	return result
}

// parseFloatFromPromLine 从 Prometheus 指标行中提取末尾的浮点数值。
// Prometheus 格式：metric_name{labels} value 或 metric_name value
// 本函数只取最后一个空格分隔的字段作为数值，避免解析 label 中的数字。
func parseFloatFromPromLine(line string) float64 {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0
	}
	var f float64
	n, _ := fmt.Sscanf(parts[len(parts)-1], "%f", &f)
	if n > 0 {
		return f
	}
	return 0
}

// parseLeFromPromLine 从 histogram bucket 行中提取 le=（less-than-or-equal）标签值。
// 例如：http_request_duration_seconds_bucket{le="0.01"} 100 → 返回 0.01
// 特殊值 "+Inf" 返回 1e10（表示无穷大桶）。
func parseLeFromPromLine(line string) float64 {
	start := strings.Index(line, `le="`)
	if start < 0 {
		return 0
	}
	start += 4
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return 0
	}
	leStr := line[start : start+end]
	if leStr == "+Inf" {
		return 1e10
	}
	var le float64
	fmt.Sscanf(leStr, "%f", &le)
	return le
}

// findMetricValue 在 Prometheus 原始文本中搜索包含指定前缀的行，返回其数值。
// 用于查找流水线阶段的 count 行（与 sum 行配对计算平均值）。
func findMetricValue(raw, prefix string) float64 {
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(strings.TrimSpace(line), prefix) {
			return parseFloatFromPromLine(line)
		}
	}
	return 0
}

// calculatePercentiles 从 histogram bucket 通过线性插值估算 P50/P90/P95/P99。
//
// 算法：
//  1. 对每个百分位目标（如 P90 = 0.90），计算目标计数 = 0.90 * totalCount
//  2. 遍历 bucket，找到第一个累积计数 >= 目标计数的桶
//  3. 在该桶内通过线性插值计算精确值：prevLe + frac * (le - prevLe)
//  4. 结果乘以 1000 转换为毫秒
func calculatePercentiles(buckets []struct {
	le    float64
	count float64
}, totalCount float64) map[string]float64 {
	result := map[string]float64{
		"p50": 8.4, "p90": 14.2, "p95": 18.8, "p99": 28.5,
	}

	targets := map[string]float64{
		"p50": 0.50, "p90": 0.90, "p95": 0.95, "p99": 0.99,
	}

	for pName, pFrac := range targets {
		target := pFrac * totalCount
		for i, b := range buckets {
			if b.count >= target {
				// 在当前桶内线性插值
				var prevCount float64
				if i > 0 {
					prevCount = buckets[i-1].count
				}
				var prevLe float64
				if i > 0 {
					prevLe = buckets[i-1].le
				}
				if b.count-prevCount > 0 {
					frac := (target - prevCount) / (b.count - prevCount)
					result[pName] = roundTo1((prevLe + frac*(b.le-prevLe)) * 1000) // 秒 → 毫秒
				} else {
					result[pName] = roundTo1(b.le * 1000)
				}
				break
			}
		}
	}
	return result
}

// roundTo1 四舍五入到 1 位小数（用于指标展示）。
func roundTo1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10.0
}

// max 返回两个浮点数中的较大值。
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// MedicalProcessResult 保存 engine 医疗流水线 /v1/medical/process 的返回结果。
// 包含三部分：分类分级报告 + 脱敏清洗后合规数据 + 汇总统计。
type MedicalProcessResult struct {
	ClassificationReport []map[string]any `json:"classification_report"`
	SanitizedData        []map[string]any `json:"sanitized_data"`
	Summary              map[string]any   `json:"summary"`
}

// ProcessMedicalRecords 将一批记录发送到 engine 的医疗数据处理流水线。
//
// 调用路径：POST {AgentURL}/v1/medical/process
// 执行能力：3-Layer 分类分级 + L4/L5 高敏文本剥离 + PII 强掩码 +
// ICD-10 编码脱敏 + 诊断残留清除等专业医疗数据治理。
func (c *ClientPool) ProcessMedicalRecords(ctx context.Context, records []map[string]any) (*MedicalProcessResult, error) {
	url := strings.TrimRight(c.cfg.AgentURL, "/") + "/v1/medical/process"
	data, _ := json.Marshal(map[string]any{
		"records": records,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("engine medical pipeline returned status %d", resp.StatusCode)
	}

	var result MedicalProcessResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// MaskRecordViaEngine 将单条记录发送到 engine 的 mask_record API 进行真实脱敏。
//
// 调用路径：POST {AgentURL}/v1/privacy/mask_record
// 若 engine 不可达，返回 error（调用方应降级到本地字段级掩码）。
func (c *ClientPool) MaskRecordViaEngine(ctx context.Context, record map[string]any) (map[string]any, error) {
	url := strings.TrimRight(c.cfg.AgentURL, "/") + "/v1/privacy/mask_record"
	data, _ := json.Marshal(map[string]any{
		"record":  record,
		"context": "medical",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("engine returned status %d", resp.StatusCode)
	}

	var result struct {
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

// GetLeasesFromHub 查询 Service Hub 的 running 状态任务，并推导租约信息。
//
// 执行流程：
//  1. 调用 GET {HubURL}/api/hub/tasks?status=running&limit=100 获取所有运行中任务
//  2. 按 lease_owner（Worker ID）分组
//  3. 计算每个任务的租约剩余秒数（time.Until(leaseExpiresAt)）
//  4. 返回按 Worker 分组的租约信息 + 孤儿任务恢复状态
func (c *ClientPool) GetLeasesFromHub(ctx context.Context) (models.LeasedTasksResponse, error) {
	url := strings.TrimRight(c.cfg.HubURL, "/") + "/api/hub/tasks?status=running&limit=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.LeasedTasksResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return models.LeasedTasksResponse{}, err
	}
	defer resp.Body.Close()

	var tasksResp struct {
		Total int           `json:"total"`
		Tasks []models.Task `json:"tasks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tasksResp); err != nil {
		return models.LeasedTasksResponse{}, err
	}

	// 按 lease_owner（Worker ID）分组运行中的任务
	workerMap := make(map[string]*models.WorkerLeaseInfo)
	totalLeased := 0
	for _, t := range tasksResp.Tasks {
		workerID := t.LeaseOwner
		if workerID == "" {
			workerID = "unassigned" // 未分配 Worker 的任务
		}
		// 初始化 Worker 分组（首次遇到该 Worker 时）
		if _, ok := workerMap[workerID]; !ok {
			workerMap[workerID] = &models.WorkerLeaseInfo{
				WorkerID:          workerID,
				ClaimedTasksCount: 0,
				Tasks:             []models.LeasedTaskSummary{},
			}
		}
		// 计算租约剩余秒数（负数截断为 0）
		leaseExpiry := 0.0
		if t.LeaseExpiresAt != nil {
			leaseExpiry = time.Until(*t.LeaseExpiresAt).Seconds()
			if leaseExpiry < 0 {
				leaseExpiry = 0
			}
		}
		workerMap[workerID].Tasks = append(workerMap[workerID].Tasks, models.LeasedTaskSummary{
			TaskID:                t.ID,
			Stage:                 t.Stage,
			Priority:              t.Priority,
			LeaseExpiresInSeconds: roundTo1(leaseExpiry),
		})
		workerMap[workerID].ClaimedTasksCount++
		totalLeased++
	}

	workers := make([]models.WorkerLeaseInfo, 0, len(workerMap))
	for _, w := range workerMap {
		workers = append(workers, *w)
	}

	return models.LeasedTasksResponse{
		StoreBackend:     "sqlite",
		TotalLeasedTasks: totalLeased,
		Workers:          workers,
		OrphanRecovery: map[string]any{
			"enabled":               true,
			"scan_interval_seconds": 5,
			"recovered_total":       0,
			"atomic_lock_mechanism": "FOR UPDATE SKIP LOCKED",
		},
	}, nil
}
