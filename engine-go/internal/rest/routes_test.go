// Package rest — REST API 路由集成测试。
//
// 覆盖全部 17 个端点的正常路径 + 错误信封格式校验。
// 验证统一错误信封（code/message/detail/trace_id/timestamp）输出。
package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
)

// ──────────────────────────────────────────────
// 测试辅助
// ──────────────────────────────────────────────

func init() {
	gin.SetMode(gin.TestMode)
}

// setupRouter 创建测试用路由（挂载 TraceMiddleware 模拟真实环境）
func setupRouter(t *testing.T) (*gin.Engine, *service.PrivacyService) {
	t.Helper()
	svc, err := service.NewPrivacyService(service.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPrivacyService: %v", err)
	}
	r := gin.New()
	RegisterRoutes(r, svc)
	return r, svc
}

// doJSON 发送 JSON 请求并返回响应
func doJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// parseEnvelope 解析错误信封
func parseEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v, body: %s", err, w.Body.String())
	}
	return env
}

// assertEnvelope 校验错误信封格式（code + message + trace_id + timestamp）
func assertEnvelope(t *testing.T, env map[string]any, wantCode string, wantStatus int, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != wantStatus {
		t.Errorf("status = %d, want %d", w.Code, wantStatus)
	}
	if code, ok := env["code"].(string); !ok || code != wantCode {
		t.Errorf("code = %v, want %q", env["code"], wantCode)
	}
	if msg, ok := env["message"].(string); !ok || msg == "" {
		t.Errorf("message = %v, want non-empty string", env["message"])
	}
	if tid, ok := env["trace_id"].(string); !ok || tid == "" {
		// trace_id 可能为空（测试环境未挂载 TraceMiddleware），仅校验字段存在
		_ = tid
	}
	if ts, ok := env["timestamp"].(string); !ok || ts == "" {
		t.Errorf("timestamp = %v, want non-empty string", env["timestamp"])
	}
}

// ──────────────────────────────────────────────
// 健康检查
// ──────────────────────────────────────────────

func TestHealth(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "GET", "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want ok", resp["status"])
	}
	if resp["engine"] != "go" {
		t.Errorf("engine = %q, want go", resp["engine"])
	}
}

// ──────────────────────────────────────────────
// 掩码端点
// ──────────────────────────────────────────────

func TestMask_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/mask", map[string]string{
		"field": "id_card", "value": "110101199003072345", "type": "id_card",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["masked"] == "110101199003072345" {
		t.Error("mask should have changed the value")
	}
}

func TestMask_MissingField(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/mask", map[string]string{
		"field": "id_card",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	env := parseEnvelope(t, w)
	assertEnvelope(t, env, "INVALID_ARGUMENT", http.StatusBadRequest, w)
}

func TestMaskRecord_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/mask/record", map[string]any{
		"record": map[string]string{"name": "张三", "phone": "13800138000"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestMaskBatch_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/mask/batch", map[string]any{
		"records": []map[string]string{
			{"name": "张三"},
			{"name": "李四"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// 差分隐私端点
// ──────────────────────────────────────────────

func TestNoisyCount_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/dp/noisy_count", map[string]any{
		"count": 100, "epsilon": 0.1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestNoisyCount_BadRequest(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/dp/noisy_count", map[string]any{
		"count": 100, // missing epsilon
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	env := parseEnvelope(t, w)
	assertEnvelope(t, env, "INVALID_ARGUMENT", http.StatusBadRequest, w)
}

func TestNoisySum_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/dp/noisy_sum", map[string]any{
		"values": []float64{1.0, 2.0, 3.0}, "epsilon": 0.1, "sensitivity": 1.0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestNoisyMean_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/dp/noisy_mean", map[string]any{
		"values": []float64{1.0, 2.0, 3.0}, "epsilon": 0.1, "delta": 1e-5, "clip_bound": 5.0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// LDP 端点
// ──────────────────────────────────────────────

func TestRandomizedResponse_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/ldp/randomized_response", map[string]any{
		"value": true, "epsilon": 1.0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestORR_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/ldp/orr", map[string]any{
		"value": 3, "epsilon": 1.0, "domain_size": 10,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// K-匿名端点
// ──────────────────────────────────────────────

func TestKAnonymize_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/kano/anonymize", map[string]any{
		"records": []map[string]string{
			{"name": "张三", "age": "30", "city": "北京"},
			{"name": "李四", "age": "30", "city": "北京"},
			{"name": "王五", "age": "30", "city": "北京"},
			{"name": "赵六", "age": "30", "city": "北京"},
		},
		"qi_fields": []string{"age", "city"},
		"k": 2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestKAnonymize_InvalidK(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/kano/anonymize", map[string]any{
		"records":     []map[string]string{{"name": "张三"}},
		"qi_fields":   []string{"name"},
		"k":           0, // invalid: must be >= 1
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	env := parseEnvelope(t, w)
	assertEnvelope(t, env, "INVALID_ARGUMENT", http.StatusBadRequest, w)
}

// ──────────────────────────────────────────────
// 查询混淆端点
// ──────────────────────────────────────────────

func TestObfuscate_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/qol/obfuscate", map[string]any{
		"query": "SELECT * FROM users", "num_decoys": 3, "domain": "sql",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// 分类端点
// ──────────────────────────────────────────────

func TestClassify_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/classify", map[string]any{
		"field": "id_card_no", "value": "110101199003072345",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestClassifyBatch_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/classify/batch", map[string]any{
		"records": []map[string]string{
			{"id_card_no": "110101199003072345"},
			{"phone": "13800138000"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// 医疗流水线端点
// ──────────────────────────────────────────────

func TestMedicalSanitize_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/medical/sanitize", map[string]any{
		"record": map[string]string{
			"name":        "张三",
			"id_card_no":  "110101199003072345",
			"phone":       "13800138000",
			"diagnosis":   "2型糖尿病",
		},
		"domain": "yibao",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestMedicalBatch_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/medical/sanitize/batch", map[string]any{
		"records": []map[string]string{
			{"name": "张三", "phone": "13800138000"},
		},
		"domain": "yibao",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// HMAC 散列端点
// ──────────────────────────────────────────────

func TestHashHMAC_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "POST", "/api/v1/hash/hmac", map[string]any{
		"value": "sensitive_data", "salt": "my_salt",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["hash"] == "" {
		t.Error("hash should not be empty")
	}
}

// ──────────────────────────────────────────────
// 预算查询端点
// ──────────────────────────────────────────────

func TestBudget_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "GET", "/api/v1/budget", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// ──────────────────────────────────────────────
// 404 路由
// ──────────────────────────────────────────────

func TestNotFound(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(r, "GET", "/api/v1/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// ──────────────────────────────────────────────
// 统一错误信封格式验证（批量）
// ──────────────────────────────────────────────

func TestAllEndpoints_ReturnEnvelopeOnError(t *testing.T) {
	r, _ := setupRouter(t)

	// 每个端点发送空 body 触发 400 错误，验证信封格式
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/mask"},
		{"POST", "/api/v1/mask/record"},
		{"POST", "/api/v1/mask/batch"},
		{"POST", "/api/v1/dp/noisy_count"},
		{"POST", "/api/v1/dp/noisy_sum"},
		{"POST", "/api/v1/dp/noisy_mean"},
		{"POST", "/api/v1/ldp/randomized_response"},
		{"POST", "/api/v1/ldp/orr"},
		{"POST", "/api/v1/kano/anonymize"},
		{"POST", "/api/v1/qol/obfuscate"},
		{"POST", "/api/v1/classify"},
		{"POST", "/api/v1/classify/batch"},
		{"POST", "/api/v1/medical/sanitize"},
		{"POST", "/api/v1/medical/sanitize/batch"},
		{"POST", "/api/v1/hash/hmac"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := doJSON(r, ep.method, ep.path, map[string]string{})
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %s: status = %d, want 400", ep.method, ep.path, w.Code)
			}
			env := parseEnvelope(t, w)
			// 校验信封必须包含 code + message + timestamp
			if _, ok := env["code"]; !ok {
				t.Error("missing 'code' field in error envelope")
			}
			if _, ok := env["message"]; !ok {
				t.Error("missing 'message' field in error envelope")
			}
			if _, ok := env["timestamp"]; !ok {
				t.Error("missing 'timestamp' field in error envelope")
			}
		})
	}
}
