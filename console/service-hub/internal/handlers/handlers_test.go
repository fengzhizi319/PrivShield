package handlers

import (
	"bytes"
	"encoding/json"
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

	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/store/memory"

	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/service-hub/internal/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testDeps bundles shared test dependencies (store, logger, metrics).
type testDeps struct {
	tasks  *memory.TaskStore
	logger *slog.Logger
	mc     *metrics.Collector
}

func newTestDeps() *testDeps {
	return &testDeps{
		tasks:  memory.NewTaskStore(),
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		mc:     metrics.NewCollector("service-hub-test"),
	}
}

// newTestServer creates a Server with a mock upstream (httptest server).
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	// Mock upstream agent
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
		AgentRESTPort:   19999, // unreachable for simple tests
		MaxQueueDepth:   100,
		ScheduleTimeout: 5,
	}
	d := newTestDeps()
	ag := agent.New(cfg)
	srv := New(ag, cfg, d.tasks, d.logger, d.mc)
	return srv, mockAgent
}

func newSimpleTestServer() *Server {
	cfg := &config.Config{
		Host:            "127.0.0.1",
		Port:            0,
		AgentRESTHost:   "127.0.0.1",
		AgentRESTPort:   19999, // unreachable, for unit tests
		MaxQueueDepth:   100,
		ScheduleTimeout: 5,
	}
	d := newTestDeps()
	ag := agent.New(cfg)
	return New(ag, cfg, d.tasks, d.logger, d.mc)
}

// newMockE2EServer creates a Server connected to a mock agent (httptest.Server).
// The mock agent simulates classification + masking responses from the real PrivShield Agent.
// newMockE2EServer 创建一个连接到模拟 Agent 的 Server。
// 模拟 Agent 会返回分类分级和脱敏结果，用于全流程 E2E 测试。
func newMockE2EServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	// Mock upstream agent: simulates real PrivShield Agent REST API
	mockAgent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "namespace": "default"})

		case "/v1/dynclassification/eval_record":
			// Simulate 3-layer classification funnel: Rule → NER → LLM
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
			// Simulate field-level masking: name → 张*, id_card → 110***1234
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			json.NewEncoder(w).Encode(map[string]any{
				"result": "张*",
			})

		case "/v1/privacy/mask_record":
			// Simulate record-level masking
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]string{
					"patient_name": "张*",
					"id_card":      "110***********1234",
					"diagnosis":    "高血压",
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"detail": "not found"})
		}
	}))

	// Parse mock server URL to extract host:port
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
	ag := agent.New(cfg)
	srv := New(ag, cfg, d.tasks, d.logger, d.mc)
	return srv, mockAgent
}

func newTestRouter(s *Server) *gin.Engine {
	r := gin.New()
	s.RegisterRoutes(r)
	return r
}

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
	if resp["backend"] != "ok" {
		t.Errorf("expected backend=ok, got %v", resp["backend"])
	}
	if resp["via"] != "service-hub" {
		t.Errorf("expected via=service-hub, got %v", resp["via"])
	}
	// Agent should be unreachable since port 19999 is not listening
	if resp["agent"] != "unreachable" {
		t.Errorf("expected agent=unreachable, got %v", resp["agent"])
	}
}

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

func TestDispatchAccepted(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	body := map[string]any{
		"source":    "test-source",
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

	// Wait for async processing
	time.Sleep(200 * time.Millisecond)

	// Check task list
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks", nil)
	router.ServeHTTP(w2, req2)

	var resp2 map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["total"].(float64) != 1 {
		t.Errorf("expected 1 task, got %v", resp2["total"])
	}
}

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

func TestLevelToOperation(t *testing.T) {
	tests := []struct {
		level    string
		expected string
	}{
		{"L1", "none"},
		{"L2", "mask"},
		{"L3", "k_anon"},
		{"L4", "dp"},
		{"L5", "dp"},
		{"unknown", "mask"},
	}
	for _, tt := range tests {
		got := levelToOperation(tt.level)
		if got != tt.expected {
			t.Errorf("levelToOperation(%q) = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestLevelToPriority(t *testing.T) {
	tests := []struct {
		level    string
		expected int
	}{
		{"L5", 100},
		{"L4", 80},
		{"L3", 60},
		{"L2", 40},
		{"L1", 10},
		{"unknown", 40},
	}
	for _, tt := range tests {
		got := levelToPriority(tt.level)
		if got != tt.expected {
			t.Errorf("levelToPriority(%q) = %d, want %d", tt.level, got, tt.expected)
		}
	}
}

func TestListTasksWithFilter(t *testing.T) {
	s := newSimpleTestServer()
	router := newTestRouter(s)

	// Dispatch a task with operation "none" (doesn't call agent, completes successfully)
	body := map[string]any{
		"source":    "test-source",
		"operation": "none",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/dispatch", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Wait for async processing (6 stages * 100ms each)
	time.Sleep(1200 * time.Millisecond)

	// Filter by completed
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
	router.ServeHTTP(w2, req2)

	var resp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 completed task, got %v", total)
	}

	// Filter by pending (should be 0 after completion)
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

	// Step 1: 申请数据 — Submit a masking task with patient PII data
	dispatchBody := map[string]any{
		"source":    "卫健数据库",
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

	// Step 3: 拿到脱敏数据 — Query task to verify completion
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks/"+taskID, nil)
	router.ServeHTTP(w2, req2)

	// Note: we need to add a GetTask handler, but since we don't have one,
	// we use the list endpoint with filter instead
	w2 = httptest.NewRecorder()
	req2, _ = http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("get task: expected 200, got %d", w2.Code)
	}

	var listResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &listResp)
	total := listResp["total"].(float64)
	if total != 1 {
		t.Fatalf("expected 1 completed task, got %v", total)
	}

	tasks := listResp["tasks"].([]any)
	task := tasks[0].(map[string]any)

	// Verify task completed successfully through all 6 pipeline stages
	if task["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", task["status"])
	}
	if task["stage"] != "done" {
		t.Errorf("expected stage=done, got %v", task["stage"])
	}
	if task["source"] != "卫健数据库" {
		t.Errorf("expected source=卫健数据库, got %v", task["source"])
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

	// Step 4: Verify hub status reflects completed task
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

// TestE2E_FullPipeline_ClassifyAndDesensitize tests the classify-then-desensitize flow:
//  1. Submit data for automatic classification via POST /api/hub/classify
//  2. Mock agent classifies data as L3 (confidential)
//  3. System auto-selects k_anon operation based on L3 level
//  4. Pipeline processes: classify → desensitize (k_anon) → complete
//  5. Verify task completes with auto-selected operation
//
// TestE2E_FullPipeline_ClassifyAndDesensitize 测试自动分类分级 + 脱敏全流程：
//  1. 提交数据到分类分级端点
//  2. 模拟 Agent 返回 L3（敏感）级别
//  3. 系统自动选择 k_anon 脱敏策略
//  4. 流水线处理：分类分级 → K匿名脱敏 → 完成
//  5. 验证任务以自动选择的操作完成
func TestE2E_FullPipeline_ClassifyAndDesensitize(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	// Step 1: 申请数据 — Submit data for auto classification
	classifyBody := map[string]any{
		"source": "医保数据库",
		"payload": map[string]any{
			"patient_name": "李四",
			"id_card":      "310101198505051234",
			"diagnosis":    "糖尿病",
			"medical_fee":  15000,
		},
	}
	b, _ := json.Marshal(classifyBody)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/hub/classify", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("classify: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var classifyResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &classifyResp)

	// Step 2: 分类分级结果验证 — Verify classification result
	taskID := classifyResp["task_id"].(string)
	if taskID == "" {
		t.Fatal("classify: expected non-empty task_id")
	}
	level := classifyResp["level"].(string)
	if level != "L3" {
		t.Errorf("classify: expected level=L3, got %v", level)
	}
	autoOp := classifyResp["auto_operation"].(string)
	if autoOp != "k_anon" {
		t.Errorf("classify: expected auto_operation=k_anon (L3→k_anon), got %v", autoOp)
	}
	t.Logf("✅ Step 1 passed: 分类分级完成 level=%s auto_operation=%s", level, autoOp)

	// Step 3: 等待脱敏流水线完成
	time.Sleep(1200 * time.Millisecond)

	// Step 4: 拿到脱敏数据 — Verify task completed with k_anon operation
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
	router.ServeHTTP(w2, req2)

	var listResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &listResp)
	total := listResp["total"].(float64)
	if total != 1 {
		t.Fatalf("expected 1 completed task, got %v", total)
	}

	tasks := listResp["tasks"].([]any)
	task := tasks[0].(map[string]any)
	if task["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", task["status"])
	}
	if task["operation"] != "k_anon" {
		t.Errorf("expected operation=k_anon, got %v", task["operation"])
	}
	if task["stage"] != "done" {
		t.Errorf("expected stage=done, got %v", task["stage"])
	}
	t.Logf("✅ Step 2 passed: 脱敏完成 operation=k_anon stage=done")
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
		{"L1-公开数据-无脱敏", "none", "公开数据集"},
		{"L2-内部数据-字段脱敏", "mask", "员工信息库"},
		{"L3-敏感数据-K匿名", "k_anon", "卫健病历库"},
		{"L4-机密数据-差分隐私", "dp", "医保结算库"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 申请数据 — Submit task
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

			// 等待脱敏完成
			time.Sleep(1000 * time.Millisecond)

			// 拿到脱敏数据 — Verify task completed
			w2 := httptest.NewRecorder()
			req2, _ := http.NewRequest("GET", "/api/hub/tasks?status=completed", nil)
			router.ServeHTTP(w2, req2)

			var listResp map[string]any
			_ = json.Unmarshal(w2.Body.Bytes(), &listResp)
			tasks := listResp["tasks"].([]any)

			// Find our task
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

// TestE2E_FullPipeline_HealthCheckWithAgent verifies that the health endpoint
// correctly reports agent connectivity when the mock agent is reachable.
// TestE2E_FullPipeline_HealthCheckWithAgent 验证 Agent 可达时健康检查正确报告连通状态。
func TestE2E_FullPipeline_HealthCheckWithAgent(t *testing.T) {
	srv, mockAgent := newMockE2EServer(t)
	defer mockAgent.Close()
	router := newTestRouter(srv)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// Agent should be reachable (not "unreachable")
	if resp["agent"] == "unreachable" {
		t.Error("expected agent to be reachable via mock server")
	}
	if resp["backend"] != "ok" {
		t.Errorf("expected backend=ok, got %v", resp["backend"])
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

	// Submit a task that will call the mock agent
	body := map[string]any{
		"source":    "测试数据源",
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

	// Check pipeline while task is processing (should see active stages)
	time.Sleep(50 * time.Millisecond) // Let task start
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/hub/pipeline", nil)
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}

	var pipelineResp map[string]any
	_ = json.Unmarshal(w2.Body.Bytes(), &pipelineResp)

	// Agent should be OK
	if pipelineResp["agent_ok"] != true {
		t.Error("expected agent_ok=true")
	}

	stages := pipelineResp["stages"].([]any)
	if len(stages) != 6 {
		t.Errorf("expected 6 stages, got %d", len(stages))
	}
	t.Logf("✅ 流水线 6 阶段正常, Agent 连接正常")

	// Wait for completion
	time.Sleep(1200 * time.Millisecond)
}
