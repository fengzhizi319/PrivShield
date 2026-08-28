package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/store/memory"

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/datasource"
)

func init() {
	// 设置 Gin 为测试模式，避免打印冗余调试路由日志
	gin.SetMode(gin.TestMode)
}

// testDeps bundles shared test dependencies (store, logger, metrics).
// testDeps 聚合测试所需的通用基础依赖：内存任务仓库、日志记录器与指标收集器。
type testDeps struct {
	tasks  *memory.TaskStore
	logger *slog.Logger
	mc     *metrics.Collector
}

// newTestDeps creates a new instance of testDeps.
func newTestDeps() *testDeps {
	return &testDeps{
		tasks:  memory.NewTaskStore(),
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		mc:     metrics.NewCollector("service-hub-test"),
	}
}

type failingUpdateTaskStore struct {
	*memory.TaskStore
	updateCalls int
}

func (s *failingUpdateTaskStore) Update(task *store.Task) error {
	s.updateCalls++
	return errors.New("simulated task state persistence failure")
}

// newTestServer creates a Server with a mock upstream (httptest server).
// newTestServer 启动一个 Mock Upstream Agent HTTP 服务器，并返回初始化的 Server 实例与 Mock 服务。
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	// 构造 Mock Upstream Agent 路由处理器
	mockAgent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "namespace": "default"})
		case "/v1/dynclassification/classify":
			json.NewEncoder(w).Encode(map[string]any{
				"level":    "L3",
				"fields":   []string{"name", "id_card"},
				"category": "PII",
			})
		case "/v1/privacy/mask":
			json.NewEncoder(w).Encode(map[string]any{
				"masked_value": "张*",
				"field_name":   "name",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"detail": "not found"})
		}
	}))

	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		AgentRESTHost:   "127.0.0.1",
		AgentRESTPort:   19999, // 设置为不可达端口，用于单元测试快速验证错误分支
		MaxQueueDepth:   100,
		ScheduleTimeout: 5,
	}
	d := newTestDeps()
	ag := agent.New(cfg, d.mc)
	ds := datasource.New(cfg)
	srv := New(ag, ds, cfg, d.tasks, d.logger, d.mc)
	return srv, mockAgent
}

// newSimpleTestServer creates a standalone test Server with in-memory store and mock config.
// newSimpleTestServer 快速创建无外部依赖的单测用 Server 实例。
func newSimpleTestServer() *Server {
	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		AgentRESTHost:   "127.0.0.1",
		AgentRESTPort:   19999, // 不可达端口，用于孤立单元测试
		MaxQueueDepth:   100,
		ScheduleTimeout: 5,
	}
	d := newTestDeps()
	ag := agent.New(cfg, d.mc)
	ds := datasource.New(cfg)
	return New(ag, ds, cfg, d.tasks, d.logger, d.mc)
}

// newMockE2EServer creates a Server connected to a mock agent (httptest.Server).
// The mock agent simulates classification + masking responses from the real PrivShield Agent.
// newMockE2EServer 创建一个连接到模拟 Agent 的 Server。
// 模拟 Agent 会返回分类分级和脱敏结果，用于全流程 E2E 测试。
func newMockE2EServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	// 构造 Mock Upstream Agent：模拟动态分类三层漏斗与隐私脱敏 API
	mockAgent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "namespace": "default"})

		case "/v1/dynclassification/eval_record":
			// 模拟 Agent 动态分类三层漏斗（Rule -> NER -> LLM）评估
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			json.NewEncoder(w).Encode(map[string]any{
				"level":      "L3",
				"confidence": 0.92,
				"fields":     []string{"patient_name", "id_card", "diagnosis"},
				"categories": map[string]string{
					"patient_name": "PII",
					"id_card":      "PII",
					"diagnosis":    "PHI",
				},
				"engine": "rule",
			})

		case "/v1/privacy/mask":
			// 模拟字段级掩码脱敏
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			json.NewEncoder(w).Encode(map[string]any{
				"result": "张*",
			})

		case "/v1/privacy/mask_record":
			// 模拟整行记录脱敏
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]string{
					"patient_name": "张*",
					"id_card":      "110***********1234",
					"diagnosis":    "高血压",
				},
			})

		case "/v1/medical/process":
			// 模拟医疗流水线：分类+脱敏一体化（3-Layer 分类 + PII 掩码 + ICD-10 脱敏）
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			records, _ := payload["records"].([]any)
			sanitized := make([]map[string]any, 0, len(records))
			for _, rec := range records {
				if m, ok := rec.(map[string]any); ok {
					s := make(map[string]any, len(m))
					for k, v := range m {
						s[k] = v
					}
					if name, ok := s["patient_name"].(string); ok && len(name) > 1 {
						s["patient_name"] = string(name[0]) + "*"
					}
					if id, ok := s["id_card"].(string); ok && len(id) > 8 {
						s["id_card"] = id[:4] + "***********" + id[len(id)-4:]
					}
					sanitized = append(sanitized, s)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"classification_report": []map[string]any{
					{"level": "L3", "confidence": 0.92, "engine": "rule"},
				},
				"sanitized_data": sanitized,
				"summary":        map[string]any{"total_records": len(records), "pipeline": "medical"},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"detail": "not found"})
		}
	}))

	// 解析 mockServer 的主机与动态端口
	mockURL, _ := url.Parse(mockAgent.URL)
	mockHost, mockPortStr, _ := net.SplitHostPort(mockURL.Host)
	mockPort, _ := strconv.Atoi(mockPortStr)

	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		AgentRESTHost:   mockHost,
		AgentRESTPort:   mockPort,
		MaxQueueDepth:   100,
		ScheduleTimeout: 10,
	}
	d := newTestDeps()
	ag := agent.New(cfg, d.mc)
	ds := datasource.New(cfg)
	srv := New(ag, ds, cfg, d.tasks, d.logger, d.mc)
	return srv, mockAgent
}

// newTestRouter constructs a test Gin engine with all routes registered.
func newTestRouter(s *Server) *gin.Engine {
	r := gin.New()
	s.RegisterRoutes(r)
	return r
}

// TestHealth tests the /api/health liveness probe endpoint.
// TestHealth 验证存活探针端点：进程存活即返回 200。
func TestHealth(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if resp["via"] != "service-hub" {
		t.Errorf("expected via=service-hub, got %v", resp["via"])
	}
}

// TestReadyzAgentUnreachable tests the /readyz readiness probe when the upstream agent is unreachable.
// TestReadyzAgentUnreachable 验证就绪探针在 Agent 不可达时返回 503。
func TestReadyzAgentUnreachable(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp["status"] != "not_ready" {
		t.Errorf("expected status=not_ready, got %v", resp["status"])
	}
	if resp["agent"] != "unreachable" {
		t.Errorf("expected agent=unreachable, got %v", resp["agent"])
	}
}

// TestHubStatus tests the /api/hub/status telemetry overview endpoint.
// TestHubStatus 测试调度中枢状态概览端点返回指标。
func TestHubStatus(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/hub/status", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp["status"] != "running" {
		t.Errorf("expected status=running, got %v", resp["status"])
	}
	if resp["active_tasks"].(float64) != 0 {
		t.Errorf("expected 0 active tasks, got %v", resp["active_tasks"])
	}
}

// TestListTasksEmpty tests querying the task list when the repository is empty.
// TestListTasksEmpty 测试空仓库时的任务列表查询。
func TestListTasksEmpty(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/hub/tasks", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected 0 tasks, got %v", resp["total"])
	}
}

// TestDispatchInvalidBody tests input validation failure on malformed dispatch payloads.
// TestDispatchInvalidBody 测试提交空体或缺失必填字段时的 400 Bad Request 校验阻断。
func TestDispatchInvalidBody(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDispatchAccepted tests normal dispatch flow returning 202 Accepted.
// TestDispatchAccepted 测试任务合法提交后正确受理并返回 202 Accepted 与 TaskID。
func TestDispatchAccepted(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	body := map[string]any{
		"source":    "ds_yibao",
		"operation": "mask",
		"payload":   map[string]any{"field_name": "name", "value": "test"},
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "accepted" {
		t.Errorf("expected status=accepted, got %v", resp["status"])
	}
	if resp["task_id"] == nil || resp["task_id"] == "" {
		t.Error("expected non-empty task_id")
	}

	// 等待后台异步流水线处理
	time.Sleep(200 * time.Millisecond)

	// 校验任务列表已包含该任务
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks", nil)
	router.ServeHTTP(w2, req2)

	var resp2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["total"].(float64) != 1 {
		t.Errorf("expected 1 task, got %v", resp2["total"])
	}
}

func TestProcessTask_StopsWhenStatePersistenceFails(t *testing.T) {
	s := newSimpleTestServer()
	failingStore := &failingUpdateTaskStore{TaskStore: memory.NewTaskStore()}
	s.tasks = failingStore

	task := &store.Task{
		ID:        "task-persist-failure",
		Status:    "pending",
		Stage:     "queued",
		Source:    "ds_yibao",
		Operation: "none",
		CreatedAt: time.Now(),
	}
	if err := failingStore.Save(task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	s.processTask(task, dispatchRequest{DatasourceID: task.Source, Source: task.Source, Operation: task.Operation}, "test-req")

	if failingStore.updateCalls != 1 {
		t.Fatalf("expected one failed stage-state write before stopping, got %d", failingStore.updateCalls)
	}
}

// TestPipeline tests the 6-stage pipeline telemetry status endpoint.
// TestPipeline 测试 /api/hub/pipeline 端点能够准确返回 6 个流水线阶段的实时状态。
func TestPipeline(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/hub/pipeline", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	stages := resp["stages"].([]any)
	if len(stages) != 6 {
		t.Errorf("expected 6 stages, got %d", len(stages))
	}
}

// TestListTasksWithFilter tests task list querying with status filtering (completed vs pending).
// TestListTasksWithFilter 测试基于 status 查询参数的任务列表过滤能力。
func TestListTasksWithFilter(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	// 分发一个 operation=none 任务（无需上游 agent，可快速跑通全流水线）
	body := map[string]any{
		"source":    "ds_yibao",
		"operation": "none",
		"payload":   []map[string]any{{"data": "sample"}},
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// 等待 6 阶段异步流水线执行完成 (6 * 100ms + buffer)
	time.Sleep(1200 * time.Millisecond)

	// 过滤已完成任务
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
	router.ServeHTTP(w2, req2)

	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 completed task, got %v", total)
	}

	// 过滤排队中任务（完成后应为 0）
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/hub/tasks?status=pending", nil)
	router.ServeHTTP(w3, req3)

	var resp3 map[string]any
	_ = json.Unmarshal(w3.Body.Bytes(), &resp3)
	if resp3["total"].(float64) != 0 {
		t.Errorf("expected 0 pending tasks, got %v", resp3["total"])
	}
}

// ============================================================================
// E2E Tests: Full pipeline flow (申请数据 → 分类分级 → 脱敏 → 拿到脱敏数据)
// ============================================================================

// TestE2E_FullPipeline_DispatchMasking tests the complete data desensitization flow:
//  1. Submit a masking task via POST /api/hub/dispatch (operation=mask)
//  2. Pipeline processes 6 stages: ingest → fetch → classify → desensitize → return → audit
//  3. Task completes successfully with masked result from mock agent
//  4. Verify task status = completed, stage = done, duration > 0
//
// TestE2E_FullPipeline_DispatchMasking 测试完整的脱敏数据全流程：
//  1. 提交脱敏任务（operation=mask）
//  2. 流水线跑完 6 阶段：请求接入 → 申请原数 → 分类分级 → 下发脱敏 → 返回结果 → 存证写日志
//  3. 任务成功完成，模拟 Agent 返回脱敏结果
//  4. 验证任务状态=completed，阶段=done，耗时>0
func TestE2E_FullPipeline_DispatchMasking(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	// Step 1: 申请数据 — 提交包含医疗 PII 的脱敏请求
	dispatchBody := map[string]any{
		"source":    "ds_yibao",
		"operation": "mask",
		"payload": map[string]any{
			"patient_name": "张三",
			"id_card":      "110101199001011234",
			"diagnosis":    "高血压",
		},
		"priority": 40,
	}
	b, _ := json.Marshal(dispatchBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("dispatch: expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var dispatchResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &dispatchResp)
	taskID := dispatchResp["task_id"].(string)
	if taskID == "" {
		t.Fatal("dispatch: expected non-empty task_id")
	}
	if dispatchResp["status"] != "accepted" {
		t.Errorf("dispatch: expected status=accepted, got %v", dispatchResp["status"])
	}
	t.Logf("✅ Step 1 passed: 任务已提交 task_id=%s", taskID)

	// Step 2: 等待流水线处理完成 (6 stages × 100ms each + buffer)
	time.Sleep(1200 * time.Millisecond)

	// Step 3: 拿到脱敏数据 — 根据 TaskID 直接查询任务详情
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks/"+taskID, nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get task by id: expected 200, got %d", w2.Code)
	}

	var getResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &getResp)
	task := getResp["task"].(map[string]any)

	// 校验任务已跨越全部 6 个流水线阶段成功完成
	if task["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", task["status"])
	}
	if task["stage"] != "done" {
		t.Errorf("expected stage=done, got %v", task["stage"])
	}
	if task["source"] != "ds_yibao" {
		t.Errorf("expected source=ds_yibao, got %v", task["source"])
	}
	if task["operation"] != "mask" {
		t.Errorf("expected operation=mask, got %v", task["operation"])
	}
	durationMs := task["duration_ms"].(float64)
	if durationMs <= 0 {
		t.Errorf("expected duration_ms > 0, got %v", durationMs)
	}
	if errMsg, ok := task["error"].(string); ok && errMsg != "" {
		t.Errorf("unexpected error: %s", errMsg)
	}
	t.Logf("✅ Step 2 passed: 流水线完成 status=completed stage=done duration=%.0fms", durationMs)

	// Step 4: 校验调度中枢状态中的完成任务计数已增加
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/hub/status", nil)
	router.ServeHTTP(w3, req3)

	var hubStatus map[string]any
	_ = json.Unmarshal(w3.Body.Bytes(), &hubStatus)
	if hubStatus["completed_total"].(float64) != 1 {
		t.Errorf("expected completed_total=1, got %v", hubStatus["completed_total"])
	}
	t.Logf("✅ Step 3 passed: 调度中枢状态已更新 completed_total=1")
}


// TestE2E_FullPipeline_MultiLevelDesensitize tests multiple sensitivity levels
// and their corresponding desensitization operations:
//   - L1 → none (no masking)
//   - L2 → mask (field masking)
//   - L3 → k_anon (K-anonymity)
//   - L4 → dp (differential privacy)
//
// TestE2E_FullPipeline_MultiLevelDesensitize 测试多级别脱敏全流程：
//   - L1 → 无脱敏
//   - L2 → 字段脱敏
//   - L3 → K匿名
//   - L4 → 差分隐私
func TestE2E_FullPipeline_MultiLevelDesensitize(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	testCases := []struct {
		name      string
		operation string
		source    string
	}{
		{"L1-公开数据-无脱敏", "none", "ds_yibao"},
		{"L2-内部数据-字段脱敏", "mask", "ds_yibao"},
		{"L3-敏感数据-K匿名", "k_anon", "ds_kangyang"},
		{"L4-机密数据-差分隐私", "dp", "ds_yibao"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. 提交任务
			body := map[string]any{
				"source":    tc.source,
				"operation": tc.operation,
				"payload": map[string]any{
					"name":    "测试用户",
					"id_card": "110101199001011234",
				},
			}
			b, _ := json.Marshal(body)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)

			if w.Code != http.StatusAccepted {
				t.Fatalf("dispatch: expected 202, got %d", w.Code)
			}

			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			taskID := resp["task_id"].(string)
			t.Logf("  📝 任务已提交: %s (operation=%s)", taskID, tc.operation)

			// 2. 等待脱敏完成
			time.Sleep(1000 * time.Millisecond)

			// 3. 拿到脱敏数据 — 验证已完成列表中的任务匹配
			w2 := httptest.NewRecorder()
			req2, _ := http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
			router.ServeHTTP(w2, req2)

			var listResp map[string]any
			_ = json.Unmarshal(w2.Body.Bytes(), &listResp)
			tasks := listResp["tasks"].([]any)

			found := false
			for _, taskRaw := range tasks {
				task := taskRaw.(map[string]any)
				if task["id"] == taskID {
					found = true
					if task["status"] != "completed" {
						t.Errorf("expected completed, got %v (error: %v)", task["status"], task["error"])
					}
					if task["operation"] != tc.operation {
						t.Errorf("expected operation=%s, got %v", tc.operation, task["operation"])
					}
					t.Logf("  ✅ 脱敏完成: %s → %s", tc.source, tc.operation)
					break
				}
			}
			if !found {
				t.Errorf("task %s not found in completed tasks", taskID)
			}
		})
	}
}

// TestE2E_FullPipeline_HealthCheckWithAgent verifies that the /readyz endpoint
// correctly reports agent connectivity when the mock agent is reachable.
// TestE2E_FullPipeline_HealthCheckWithAgent 验证 Agent 可达时就绪探针正确报告连通状态。
func TestE2E_FullPipeline_HealthCheckWithAgent(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/readyz", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["status"] != "ready" {
		t.Errorf("expected status=ready, got %v", resp["status"])
	}
	if resp["agent"] == "unreachable" {
		t.Error("expected agent to be reachable via mock server")
	}
	t.Logf("✅ Agent 可达: agent=%v", resp["agent"])
}

// TestE2E_FullPipeline_PipelineStagesWithAgent verifies pipeline stage status
// when tasks are actively processing through the mock agent.
// TestE2E_FullPipeline_PipelineStagesWithAgent 验证任务处理期间流水线各阶段状态。
func TestE2E_FullPipeline_PipelineStagesWithAgent(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	// 提交一个会调用 mock agent 的任务
	body := map[string]any{
		"source":    "ds_yibao",
		"operation": "mask",
		"payload":   map[string]any{"name": "测试"},
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}

	// 检查处理中的流水线阶段遥测
	time.Sleep(50 * time.Millisecond)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/pipeline", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var pipelineResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &pipelineResp)

	if pipelineResp["agent_ok"] != true {
		t.Error("expected agent_ok=true")
	}

	stages := pipelineResp["stages"].([]any)
	if len(stages) != 6 {
		t.Errorf("expected 6 stages, got %d", len(stages))
	}
	t.Logf("✅ 流水线 6 阶段正常, Agent 连接正常")

	// 等待流水线全部收敛完成
	time.Sleep(1200 * time.Millisecond)
}

// TestGetTask_SuccessAndNotFound tests single task lookup with existing ID and non-existing ID.
// TestGetTask_SuccessAndNotFound 测试单任务详情查询（命中返回 200 与未命中返回 404）。
func TestGetTask_SuccessAndNotFound(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	now := time.Now()
	_ = s.tasks.Save(&store.Task{ID: "task-abc-123", Status: "running", Source: "source1", CreatedAt: now})

	t.Run("Found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/hub/tasks/task-abc-123", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		taskMap, ok := resp["task"].(map[string]any)
		if !ok || taskMap["id"] != "task-abc-123" || taskMap["status"] != "running" {
			t.Errorf("unexpected task payload: %+v", resp)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/hub/tasks/nonexistent", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})
}

// TestDispatch_OversizedSource tests rejection of oversized source strings.
// TestDispatch_OversizedSource 测试超长源名称（>1024 字节）被安全拦截。
func TestDispatch_OversizedSource(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	oversized := map[string]any{
		"source":    string(make([]byte, 1025)),
		"operation": "mask",
	}
	body, _ := json.Marshal(oversized)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized source, got %d", w.Code)
	}
}

// TestListTasks_InvalidStatusFilter tests rejection of illegal status filters.
// TestListTasks_InvalidStatusFilter 测试非法状态过滤参数被正确拦截。
func TestListTasks_InvalidStatusFilter(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/hub/tasks?status=illegal_status", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status filter, got %d", w.Code)
	}
}

// TestAuthMiddleware_Protection tests API Key authentication middleware protection.
// TestAuthMiddleware_Protection 测试 API Key 鉴权中间件的防护拦截（无认证头 401、携带有效 Bearer 头 200、Health 接口免密放行）。
func TestAuthMiddleware_Protection(t *testing.T) {
	cfg := &config.Config{
		Host:          "127.0.0.1",
		Port:          0,
		AgentRESTHost: "127.0.0.1",
		AgentRESTPort: 19999,
		APIKey:        "secret-token-123",
	}
	d := newTestDeps()
	ag := agent.New(cfg, d.mc)
	ds := datasource.New(cfg)
	s := New(ag, ds, cfg, d.tasks, d.logger, d.mc)

	r := gin.New()
	s.RegisterRoutes(r)

	t.Run("Unauthorized_NoHeader", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/hub/status", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("Authorized_Bearer", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/hub/status", nil)
		req.Header.Set("Authorization", "Bearer secret-token-123")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})

	t.Run("Health_ExemptFromAuth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for health exempt from auth, got %d", w.Code)
		}
	})
}

// TestServer_ShutdownGraceful tests graceful shutdown execution without panic.
// TestServer_ShutdownGraceful 测试优雅停机方法能平滑执行完毕。
func TestServer_ShutdownGraceful(t *testing.T) {
	s := newSimpleTestServer()
	s.Shutdown()
}

// TestServer_LocalPendingWorker tests that StartLocalWorker picks up and processes pending tasks in SQLite/memory mode.
func TestServer_LocalPendingWorker(t *testing.T) {
	s := newSimpleTestServer()
	defer s.Shutdown()

	task := &store.Task{
		ID:          "recovered-pending-task",
		Status:      "pending",
		Stage:       "queued",
		Source:      "ds_yibao",
		Operation:   "none",
		PayloadJSON: `[{"name":"test"}]`,
		CreatedAt:   time.Now(),
	}
	if err := s.tasks.Save(task); err != nil {
		t.Fatalf("save pending task: %v", err)
	}

	if err := s.StartLocalWorker(); err != nil {
		t.Fatalf("start local worker: %v", err)
	}

	// Wait for worker loop to pick up and complete the task (500ms poll + 6*100ms pipeline)
	deadline := time.Now().Add(3 * time.Second)
	completed := false
	for time.Now().Before(deadline) {
		tCheck, err := s.tasks.Get("recovered-pending-task")
		if err == nil && tCheck.Status == "completed" {
			completed = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !completed {
		tCheck, _ := s.tasks.Get("recovered-pending-task")
		t.Fatalf("expected task to be completed by local worker, got state: %+v", tCheck)
	}
}


