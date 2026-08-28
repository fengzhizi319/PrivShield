package grpcserver

import (
	"encoding/json"
	"testing"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
)

// newTestServer 创建测试用 gRPC 服务端
func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := service.DefaultConfig()
	svc, err := service.NewPrivacyService(cfg)
	if err != nil {
		t.Fatalf("NewPrivacyService: %v", err)
	}
	return NewServer(svc)
}

func TestHandleHealth(t *testing.T) {
	srv := newTestServer(t)
	respBytes, err := srv.handleHealth(nil)
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
	if resp["engine"] != "go" {
		t.Errorf("engine = %q, want %q", resp["engine"], "go")
	}
}

func TestHandleMask(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name      string
		fieldName string
		value     string
		wantEmpty bool
	}{
		{"id_card", "id_card_no", "110101199003072345", false},
		{"phone", "phone", "13812345678", false},
		{"name", "patient_name", "张三", false},
		{"email", "email", "test@example.com", false},
		{"unknown_field", "foo", "bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := map[string]string{
				"field_name": tt.fieldName,
				"value":      tt.value,
			}
			reqBytes, _ := json.Marshal(req)

			respBytes, err := srv.handleMask(reqBytes)
			if err != nil {
				t.Fatalf("handleMask: %v", err)
			}

			var resp map[string]string
			if err := json.Unmarshal(respBytes, &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			result := resp["result"]
			if tt.wantEmpty && result != "" {
				t.Errorf("result = %q, want empty", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Errorf("result is empty, want non-empty")
			}
			// 脱敏结果不应等于原值（对于已知 PII 类型）
			if tt.name != "unknown_field" && result == tt.value {
				t.Errorf("result = %q, should be masked (original: %q)", result, tt.value)
			}
		})
	}
}

func TestHandleMaskRecord(t *testing.T) {
	srv := newTestServer(t)

	req := map[string]interface{}{
		"record": map[string]string{
			"id_card_no":   "110101199003072345",
			"phone":        "13812345678",
			"patient_name": "张三",
			"diagnosis":    "感冒",
		},
	}
	reqBytes, _ := json.Marshal(req)

	respBytes, err := srv.handleMaskRecord(reqBytes)
	if err != nil {
		t.Fatalf("handleMaskRecord: %v", err)
	}

	var resp map[string]map[string]string
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	result := resp["result"]
	if result == nil {
		t.Fatal("result is nil")
	}

	// 身份证号应被脱敏
	if result["id_card_no"] == "110101199003072345" {
		t.Error("id_card_no should be masked")
	}
	// 手机号应被脱敏
	if result["phone"] == "13812345678" {
		t.Error("phone should be masked")
	}
	// 姓名应被脱敏
	if result["patient_name"] == "张三" {
		t.Error("patient_name should be masked")
	}
	// 非 PII 字段保持不变
	if result["diagnosis"] != "感冒" {
		t.Errorf("diagnosis = %q, want %q", result["diagnosis"], "感冒")
	}
}

func TestHandleHash(t *testing.T) {
	srv := newTestServer(t)

	req := map[string]string{"value": "hello", "salt": "world"}
	reqBytes, _ := json.Marshal(req)

	respBytes, err := srv.handleHash(reqBytes)
	if err != nil {
		t.Fatalf("handleHash: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	hash := resp["result"]
	if hash == "" {
		t.Error("hash is empty")
	}
	if hash == "hello" {
		t.Error("hash should not equal original value")
	}

	// 同一输入应产生相同输出（确定性）
	respBytes2, _ := srv.handleHash(reqBytes)
	var resp2 map[string]string
	json.Unmarshal(respBytes2, &resp2)
	if resp2["result"] != hash {
		t.Error("hash should be deterministic")
	}
}

func TestHandleDPNoisyCount(t *testing.T) {
	srv := newTestServer(t)

	req := map[string]interface{}{
		"true_count": 100.0,
		"epsilon":    1.0,
	}
	reqBytes, _ := json.Marshal(req)

	respBytes, err := srv.handleDPNoisyCount(reqBytes)
	if err != nil {
		t.Fatalf("handleDPNoisyCount: %v", err)
	}

	var resp map[string]float64
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	result := resp["result"]
	// 噪声计数应在合理范围内（100 ± 大量噪声）
	if result < -100 || result > 300 {
		t.Errorf("noisy count = %f, out of reasonable range", result)
	}
}

func TestHandleDynClassify(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		field    string
		value    string
		wantLevel string
	}{
		{"id_card_no", "110101199003072345", "secret"},
		{"phone", "13812345678", "confidential"},
		{"email", "test@example.com", "confidential"},
		{"random_field", "hello", "public"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			req := map[string]string{
				"field_name":  tt.field,
				"field_value": tt.value,
				"domain":      "medical",
			}
			reqBytes, _ := json.Marshal(req)

			respBytes, err := srv.handleDynClassify(reqBytes)
			if err != nil {
				t.Fatalf("handleDynClassify: %v", err)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(respBytes, &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			maxLevel := resp["max_level"].(string)
			if maxLevel != tt.wantLevel {
				t.Errorf("max_level = %q, want %q", maxLevel, tt.wantLevel)
			}
		})
	}
}

func TestHandleGeneric(t *testing.T) {
	srv := newTestServer(t)

	respBytes, err := srv.handleGeneric("SomeFutureMethod", nil)
	if err != nil {
		t.Fatalf("handleGeneric: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["status"] != "unimplemented" {
		t.Errorf("status = %q, want %q", resp["status"], "unimplemented")
	}
}

func TestInferMaskType(t *testing.T) {
	tests := []struct {
		field string
		want  string
	}{
		{"id_card_no", "id_card"},
		{"idcard", "id_card"},
		{"cert_no", "id_card"},
		{"phone", "phone"},
		{"mobile", "phone"},
		{"tel", "phone"},
		{"bank_card", "bank_card"},
		{"credit_card", "bank_card"},
		{"email", "email"},
		{"mail", "email"},
		{"address", "address"},
		{"home_address", "address"},
		{"name", "name"},
		{"patient_name", "name"},
		{"officer_id", "officer_id"},
		{"unknown", "default"},
		{"diagnosis", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := inferMaskType(tt.field)
			if got != tt.want {
				t.Errorf("inferMaskType(%q) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestRoute(t *testing.T) {
	srv := newTestServer(t)

	// 测试 Health 路由
	respBytes, err := srv.route("Health", nil)
	if err != nil {
		t.Fatalf("route Health: %v", err)
	}
	var healthResp map[string]string
	json.Unmarshal(respBytes, &healthResp)
	if healthResp["status"] != "ok" {
		t.Errorf("Health status = %q, want %q", healthResp["status"], "ok")
	}

	// 测试未知方法路由
	respBytes, err = srv.route("UnknownMethod", []byte("{}"))
	if err != nil {
		t.Fatalf("route UnknownMethod: %v", err)
	}
	var genericResp map[string]string
	json.Unmarshal(respBytes, &genericResp)
	if genericResp["status"] != "unimplemented" {
		t.Errorf("UnknownMethod status = %q, want %q", genericResp["status"], "unimplemented")
	}
}

func TestRawCodec(t *testing.T) {
	codec := rawCodec{}

	// Test Name
	if codec.Name() != "proto" {
		t.Errorf("Name() = %q, want %q", codec.Name(), "proto")
	}

	// Test Marshal
	data := []byte("hello world")
	b, err := codec.Marshal(&data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != "hello world" {
		t.Errorf("Marshal = %q, want %q", string(b), "hello world")
	}

	// Test Unmarshal
	var out []byte
	err = codec.Unmarshal([]byte("test data"), &out)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(out) != "test data" {
		t.Errorf("Unmarshal = %q, want %q", string(out), "test data")
	}

	// Test Marshal with wrong type
	_, err = codec.Marshal("wrong type")
	if err == nil {
		t.Error("Marshal should fail with wrong type")
	}

	// Test Unmarshal with wrong type
	var wrong string
	err = codec.Unmarshal([]byte("data"), &wrong)
	if err == nil {
		t.Error("Unmarshal should fail with wrong type")
	}
}
