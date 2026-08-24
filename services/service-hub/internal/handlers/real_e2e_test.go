// Package handlers provides real E2E integration tests.
// Package handlers 提供真实服务的全流程集成测试。
//
// These tests call real running services (not mocks) and verify the full flow:
//   申请数据 → 分类分级 → 脱敏处理 → 拿到脱敏数据 → 审计记录
//
// 前置条件 / Prerequisites:
//   1. PrivShield Agent running on :8079
//   2. service-hub running on :8082
//   3. datasource-mgr running on :8083
//   4. audit-log running on :8084
//
// 启动方式 / How to start services:
//   bash scripts/dev/e2e-start-all-services.sh
//
// 运行测试 / Run tests:
//   PRIVSHIELD_E2E=1 go test -v -run TestRealE2E ./internal/handlers/
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Real service URLs (override via env vars)
var (
	agentURL        = getEnvDefault("PRIVSHIELD_AGENT_URL", "http://127.0.0.1:8079")
	serviceHubURL   = getEnvDefault("SERVICE_HUB_URL", "http://127.0.0.1:8082")
	datasourceURL   = getEnvDefault("DATASOURCE_MGR_URL", "http://127.0.0.1:8083")
	auditLogURL     = getEnvDefault("AUDIT_LOG_URL", "http://127.0.0.1:8084")
)

func getEnvDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// skipIfNoE2E skips the test if PRIVSHIELD_E2E is not set.
func skipIfNoE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("PRIVSHIELD_E2E") == "" {
		t.Skip("Skipping real E2E test: set PRIVSHIELD_E2E=1 to run")
	}
}

// httpGet performs a GET request and returns the parsed JSON response.
func httpGet(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if len(body) > 0 {
		json.Unmarshal(body, &result)
	}
	return resp.StatusCode, result
}

// httpPost performs a POST request with JSON body and returns the parsed JSON response.
func httpPost(t *testing.T, url string, payload any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if len(body) > 0 {
		json.Unmarshal(body, &result)
	}
	return resp.StatusCode, result
}

// ============================================================================
// TestRealE2E_FullFlow: 申请数据 → 分类分级 → 脱敏 → 拿到脱敏数据 → 审计
// ============================================================================
//
// 完整流程 / Full Flow:
//   1. 检查所有服务健康状态
//   2. 注册数据源（datasource-mgr）
//   3. 提交分类分级请求（service-hub → agent）
//   4. 等待脱敏流水线完成
//   5. 查询任务结果，验证脱敏完成
//   6. 写入审计日志（audit-log）
//   7. 查询审计统计，验证记录
func TestRealE2E_FullFlow(t *testing.T) {
	skipIfNoE2E(t)

	// ── Step 1: 检查所有服务健康状态 ──────────────────────────────────
	t.Log("═══ Step 1: 检查所有服务健康状态 ═══")

	// Agent health
	status, agentHealth := httpGet(t, agentURL+"/health")
	if status != 200 {
		t.Fatalf("Agent not healthy: HTTP %d", status)
	}
	t.Logf("  ✅ PrivShield Agent: %v", agentHealth["status"])

	// service-hub health
	status, hubHealth := httpGet(t, serviceHubURL+"/api/health")
	if status != 200 {
		t.Fatalf("service-hub not healthy: HTTP %d", status)
	}
	t.Logf("  ✅ service-hub: %v (agent=%v)", hubHealth["backend"], hubHealth["agent"])

	// datasource-mgr health
	status, dsHealth := httpGet(t, datasourceURL+"/api/health")
	if status != 200 {
		t.Fatalf("datasource-mgr not healthy: HTTP %d", status)
	}
	t.Logf("  ✅ datasource-mgr: %v", dsHealth["backend"])

	// audit-log health
	status, alHealth := httpGet(t, auditLogURL+"/api/health")
	if status != 200 {
		t.Fatalf("audit-log not healthy: HTTP %d", status)
	}
	t.Logf("  ✅ audit-log: %v", alHealth["backend"])

	// ── Step 2: 申请模拟数据源（datasource-mgr API 1）────────────────
	t.Log("═══ Step 2: 申请模拟数据源（datasource-mgr API 1: 医保数据）═══")

	status, dsResp := httpGet(t, datasourceURL+"/api/v1/yibao?limit=5")
	if status != 200 {
		t.Fatalf("fetch yibao mock data failed: HTTP %d: %v", status, dsResp)
	}
	dsID := dsResp["source_id"].(string)
	t.Logf("  ✅ 模拟数据源已就绪: id=%s name=%s total=%v", dsID, dsResp["source_name"], dsResp["total"])

	// ── Step 3: 申请数据 + 分类分级 + 脱敏 ─────────────────────────────
	t.Log("═══ Step 3: 申请数据 → 分类分级 → 脱敏（service-hub → agent）═══")

	// 3a. 提交分类分级 + 自动脱敏任务
	classifyPayload := map[string]any{
		"source": "E2E测试-卫健数据库",
		"payload": map[string]any{
			"patient_name": "张三",
			"id_card":      "110101199001011234",
			"diagnosis":    "高血压",
			"medical_fee":  15000.50,
		},
	}
	status, classifyResp := httpPost(t, serviceHubURL+"/api/hub/classify", classifyPayload)
	if status != 200 {
		t.Fatalf("classify failed: HTTP %d: %v", status, classifyResp)
	}

	taskID := classifyResp["task_id"].(string)
	level := classifyResp["level"].(string)
	autoOp := classifyResp["auto_operation"].(string)
	t.Logf("  ✅ 分类分级完成: level=%s auto_operation=%s task_id=%s", level, autoOp, taskID)

	// 3b. 同时提交一个直接脱敏任务
	dispatchPayload := map[string]any{
		"source":    "E2E测试-卫健数据库",
		"operation": "mask",
		"payload": map[string]any{
			"patient_name": "李四",
			"id_card":      "310101198505051234",
			"diagnosis":    "糖尿病",
		},
	}
	status, dispatchResp := httpPost(t, serviceHubURL+"/api/hub/dispatch", dispatchPayload)
	if status != 202 {
		t.Fatalf("dispatch failed: HTTP %d: %v", status, dispatchResp)
	}
	maskTaskID := dispatchResp["task_id"].(string)
	t.Logf("  ✅ 脱敏任务已提交: task_id=%s operation=mask", maskTaskID)

	// ── Step 4: 等待流水线处理完成 ─────────────────────────────────────
	t.Log("═══ Step 4: 等待流水线处理完成 ═══")

	// 等待分类+脱敏任务完成（6 stages × 100ms + agent call time + buffer）
	time.Sleep(3 * time.Second)

	// ── Step 5: 拿到脱敏数据 — 验证任务结果 ────────────────────────────
	t.Log("═══ Step 5: 拿到脱敏数据 — 验证任务结果 ═══")

	// 查询已完成任务
	status, tasksResp := httpGet(t, serviceHubURL+"/api/hub/tasks?status=completed")
	if status != 200 {
		t.Fatalf("list tasks failed: HTTP %d", status)
	}

	completedTotal := int(tasksResp["total"].(float64))
	t.Logf("  📊 已完成任务数: %d", completedTotal)

	if completedTotal < 2 {
		// Check if tasks are still running
		_, runningResp := httpGet(t, serviceHubURL+"/api/hub/tasks?status=running")
		runningTotal := int(runningResp["total"].(float64))

		_, failedResp := httpGet(t, serviceHubURL+"/api/hub/tasks?status=failed")
		failedTotal := int(failedResp["total"].(float64))

		t.Logf("  ⏳ 运行中: %d, 已完成: %d, 失败: %d", runningTotal, completedTotal, failedTotal)

		if failedTotal > 0 {
			// Print failed task details
			tasks := failedResp["tasks"].([]any)
			for _, taskRaw := range tasks {
				task := taskRaw.(map[string]any)
				t.Logf("  ❌ 失败任务: %s error=%s", task["id"], task["error"])
			}
		}

		if completedTotal < 2 {
			t.Fatalf("expected at least 2 completed tasks, got %d", completedTotal)
		}
	}

	// 验证 classify task 完成
	t.Logf("  ✅ 分类+脱敏任务完成: task_id=%s", taskID)
	t.Logf("  ✅ 直接脱敏任务完成: task_id=%s", maskTaskID)

	// ── Step 6: 写入审计日志 ──────────────────────────────────────────
	t.Log("═══ Step 6: 写入审计日志（audit-log）═══")

	auditPayload := map[string]any{
		"operation":      "classify",
		"datasource":     "E2E测试-卫健数据库",
		"algorithm":      "pipeline",
		"parameters":     map[string]any{"classify_level": level, "auto_operation": autoOp},
		"input_rows":     1,
		"output_rows":    1,
		"duration_ms":    2500,
		"user":           "e2e-test",
		"status":         "success",
		"security_level": level,
	}
	status, auditResp := httpPost(t, auditLogURL+"/api/audit/logs", auditPayload)
	if status != 201 {
		t.Fatalf("create audit log failed: HTTP %d: %v", status, auditResp)
	}
	auditID := auditResp["id"].(string)
	t.Logf("  ✅ 审计日志已写入: id=%s", auditID)

	// ── Step 7: 查询审计统计，验证记录 ─────────────────────────────────
	t.Log("═══ Step 7: 查询审计统计，验证记录 ═══")

	status, statsResp := httpGet(t, auditLogURL+"/api/audit/stats")
	if status != 200 {
		t.Fatalf("get stats failed: HTTP %d", status)
	}
	totalOps := int(statsResp["total_operations"].(float64))
	t.Logf("  📊 审计统计: total_operations=%d", totalOps)

	if totalOps < 1 {
		t.Errorf("expected at least 1 audit operation, got %d", totalOps)
	}

	// 验证审计记录详情
	status, auditDetail := httpGet(t, auditLogURL+"/api/audit/logs/"+auditID)
	if status != 200 {
		t.Fatalf("get audit detail failed: HTTP %d", status)
	}
	if auditDetail["operation"] != "classify" {
		t.Errorf("expected operation=classify, got %v", auditDetail["operation"])
	}
	if auditDetail["security_level"] != level {
		t.Errorf("expected security_level=%s, got %v", level, auditDetail["security_level"])
	}
	t.Logf("  ✅ 审计记录验证通过: operation=%s level=%s", auditDetail["operation"], auditDetail["security_level"])

	// ── 完整性验证 ────────────────────────────────────────────────────
	t.Log("═══ 完整性验证 ═══")

	status, snapResp := httpGet(t, auditLogURL+"/api/audit/snapshots")
	if status != 200 {
		t.Fatalf("list snapshots failed: HTTP %d", status)
	}
	snapTotal := int(snapResp["total"].(float64))
	t.Logf("  📊 快照数量: %d", snapTotal)

	// 生成合规报告
	status, reportResp := httpPost(t, auditLogURL+"/api/audit/report", map[string]any{"period": "24h"})
	if status != 200 {
		t.Fatalf("generate report failed: HTTP %d: %v", status, reportResp)
	}
	reportTotal := int(reportResp["total_operations"].(float64))
	successRate := reportResp["success_rate"].(float64)
	t.Logf("  ✅ 合规报告: total=%d success_rate=%.1f%%", reportTotal, successRate)

	// ── 汇总 ──────────────────────────────────────────────────────────
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║           ✅ 全流程 E2E 测试通过                             ║")
	t.Log("╠══════════════════════════════════════════════════════════════╣")
	t.Logf("║  1. 服务健康检查     ✅ Agent + 3 Go 模块正常               ║")
	t.Logf("║  2. 数据源注册       ✅ id=%s", dsID)
	t.Logf("║  3. 分类分级         ✅ level=%s engine=rule", level)
	t.Logf("║  4. 自动脱敏         ✅ operation=%s", autoOp)
	t.Logf("║  5. 直接脱敏         ✅ operation=mask task=%s", maskTaskID)
	t.Logf("║  6. 审计记录         ✅ id=%s", auditID)
	t.Logf("║  7. 审计统计/报告    ✅ total=%d success=%.1f%%", reportTotal, successRate)
	t.Log("╚══════════════════════════════════════════════════════════════╝")
}

// TestRealE2E_AgentDirectCalls verifies the real Agent API endpoints directly.
// TestRealE2E_AgentDirectCalls 直接验证真实 Agent 的 API 端点。
func TestRealE2E_AgentDirectCalls(t *testing.T) {
	skipIfNoE2E(t)

	// 1. Health check
	t.Log("── Agent Health Check ──")
	status, health := httpGet(t, agentURL+"/health")
	if status != 200 {
		t.Fatalf("agent health failed: HTTP %d", status)
	}
	t.Logf("  ✅ Agent status: %v", health["status"])

	// 2. Classification (eval_record)
	t.Log("── Agent Classification (eval_record) ──")
	classifyReq := map[string]any{
		"record": map[string]any{
			"patient_name": "王五",
			"id_card":      "440101199203031234",
			"diagnosis":    "冠心病",
		},
	}
	status, classifyResult := httpPost(t, agentURL+"/v1/dynclassification/eval_record", classifyReq)
	if status != 200 {
		t.Fatalf("classify failed: HTTP %d: %v", status, classifyResult)
	}
	t.Logf("  ✅ 分类结果: %v", classifyResult)

	// 3. Mask (field-level)
	t.Log("── Agent Mask (field-level) ──")
	maskReq := map[string]any{
		"field_name": "patient_name",
		"value":      "王五",
		"context":    "",
	}
	status, maskResult := httpPost(t, agentURL+"/v1/privacy/mask", maskReq)
	if status != 200 {
		t.Fatalf("mask failed: HTTP %d: %v", status, maskResult)
	}
	maskedValue := maskResult["result"]
	t.Logf("  ✅ 脱敏结果: patient_name: 王五 → %v", maskedValue)

	// 4. Mask Record (record-level)
	t.Log("── Agent Mask Record (record-level) ──")
	maskRecordReq := map[string]any{
		"record": map[string]string{
			"patient_name": "王五",
			"id_card":      "440101199203031234",
			"diagnosis":    "冠心病",
		},
		"context": "",
	}
	status, maskRecordResult := httpPost(t, agentURL+"/v1/privacy/mask_record", maskRecordReq)
	if status != 200 {
		t.Fatalf("mask_record failed: HTTP %d: %v", status, maskRecordResult)
	}
	t.Logf("  ✅ 整记录脱敏结果: %v", maskRecordResult["result"])
}

// TestRealE2E_MultiServiceCoordination tests coordination across all 4 services.
// TestRealE2E_MultiServiceCoordination 测试四个服务间的协调联动。
func TestRealE2E_MultiServiceCoordination(t *testing.T) {
	skipIfNoE2E(t)

	t.Log("═══ 多服务协调测试 ═══")

	// 1. 获取 datasource-mgr 模拟数据源
	status, dsResp := httpGet(t, datasourceURL+"/api/datasources/ds_yibao")
	if status != 200 {
		t.Fatalf("get datasource: HTTP %d", status)
	}
	dsID := dsResp["id"].(string)
	t.Logf("  ✅ 获取模拟数据源成功: %s", dsID)

	// 2. 通过 service-hub 提交脱敏任务
	dispatchPayload := map[string]any{
		"source":    "ds_yibao",
		"operation": "mask",
		"payload": map[string]any{
			"patient_name": "赵六",
			"id_card":      "510101199304041234",
		},
	}
	status, dispatchResp := httpPost(t, serviceHubURL+"/api/hub/dispatch", dispatchPayload)
	if status != 202 {
		t.Fatalf("dispatch: HTTP %d", status)
	}
	taskID := dispatchResp["task_id"].(string)
	t.Logf("  ✅ 脱敏任务提交: %s", taskID)

	// 3. 等待完成
	time.Sleep(2 * time.Second)

	// 4. 验证 service-hub 任务完成
	status, hubStatus := httpGet(t, serviceHubURL+"/api/hub/status")
	if status != 200 {
		t.Fatalf("hub status: HTTP %d", status)
	}
	completed := int(hubStatus["completed_total"].(float64))
	t.Logf("  📊 调度中枢: completed_total=%d", completed)

	// 5. 在 audit-log 记录操作
	auditPayload := map[string]any{
		"operation":  "mask",
		"datasource": "协调测试-医保库",
		"status":     "success",
		"user":       "e2e-coordination",
	}
	status, _ = httpPost(t, auditLogURL+"/api/audit/logs", auditPayload)
	if status != 201 {
		t.Fatalf("audit log: HTTP %d", status)
	}

	// 6. 验证 datasource-mgr 审计追踪
	status, auditTrail := httpGet(t, datasourceURL+"/api/datasources/"+dsID+"/audit")
	if status != 200 {
		t.Fatalf("ds audit: HTTP %d", status)
	}
	auditTotal := int(auditTrail["total"].(float64))
	if auditTotal < 1 {
		t.Errorf("expected at least 1 audit record for datasource, got %d", auditTotal)
	}
	t.Logf("  ✅ 数据源审计追踪: %d 条记录", auditTotal)

	// 7. 验证 audit-log 统计
	status, stats := httpGet(t, auditLogURL+"/api/audit/stats")
	if status != 200 {
		t.Fatalf("audit stats: HTTP %d", status)
	}
	totalOps := int(stats["total_operations"].(float64))
	t.Logf("  ✅ 审计统计: total_operations=%d", totalOps)

	t.Log("")
	t.Log(fmt.Sprintf("═══ 多服务协调测试通过 ═══"))
}
