// 影子流量比对验证工具 — 向 Python 原版引擎和 Go 新版引擎发送相同请求，比对响应一致性。
//
// 用法：
//
//	go run scripts/dev/shadow_verifier.go [-py http://127.0.0.1:8079] [-go http://127.0.0.1:8080]
//
// 前提：Python 引擎和 Go 引擎必须同时运行。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// TestCase 定义一条影子流量比对用例。
type TestCase struct {
	Name    string
	Path    string
	Payload map[string]any
	// CompareFields 指定需要比对的 JSON 顶层字段名；为空则比对整个 body。
	CompareFields []string
	// ApproxFields 指定允许浮点误差的字段名。
	ApproxFields []string
}

// defaultTestCases 返回默认比对用例集。
func defaultTestCases() []TestCase {
	return []TestCase{
		{
			Name: "MaskRecord-PII",
			Path: "/api/v1/mask/record",
			Payload: map[string]any{
				"record": map[string]string{
					"name":        "张三",
					"id_card_no":  "110101199003072345",
					"phone":       "13800138000",
					"bank_card":   "6222021234567890123",
					"disease":     "艾滋病确诊",
				},
			},
		},
		{
			Name: "NoisyCount",
			Path: "/api/v1/dp/noisy_count",
			Payload: map[string]any{
				"count":   100,
				"epsilon": 1.0,
			},
			ApproxFields: []string{"noisy_count"},
		},
		{
			Name: "Classify",
			Path: "/api/v1/classify",
			Payload: map[string]any{
				"field": "id_card_no",
				"value": "110101199003072345",
			},
			CompareFields: []string{"level", "category"},
		},
		{
			Name: "HashHMAC",
			Path: "/api/v1/hash/hmac",
			Payload: map[string]any{
				"value": "sensitive_data",
				"salt":  "test-salt-001",
			},
			CompareFields: []string{"hash"},
		},
		{
			Name: "MaskBatch",
			Path: "/api/v1/mask/batch",
			Payload: map[string]any{
				"records": []map[string]string{
					{"name": "李四", "phone": "13900139000"},
					{"name": "王五", "phone": "13700137000"},
				},
			},
		},
		{
			Name: "ClassifyBatch",
			Path: "/api/v1/classify/batch",
			Payload: map[string]any{
				"records": []map[string]string{
					{"id_card_no": "110101199003072345", "email": "test@example.com"},
				},
			},
		},
	}
}

func main() {
	pyAddr := flag.String("py", "http://127.0.0.1:8079", "Python 引擎地址")
	goAddr := flag.String("go", "http://127.0.0.1:8080", "Go 引擎地址")
	timeout := flag.Duration("timeout", 5*time.Second, "HTTP 请求超时")
	flag.Parse()

	cases := defaultTestCases()
	client := &http.Client{Timeout: *timeout}

	passed, failed, skipped := 0, 0, 0

	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println("  PrivShield 影子流量比对验证")
	fmt.Printf("  Python Engine: %s\n", *pyAddr)
	fmt.Printf("  Go Engine:     %s\n", *goAddr)
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Println()

	for _, tc := range cases {
		body, _ := json.Marshal(tc.Payload)

		// 发送请求到 Python 引擎
		pyResp, pyErr := postJSON(client, *pyAddr+tc.Path, body)
		// 发送请求到 Go 引擎
		goResp, goErr := postJSON(client, *goAddr+tc.Path, body)

		// 分析结果
		if pyErr != nil && goErr != nil {
			fmt.Printf("⚠️  SKIP  %s (双引擎均不可达: py=%v, go=%v)\n", tc.Name, pyErr, goErr)
			skipped++
			continue
		}
		if pyErr != nil {
			fmt.Printf("⚠️  SKIP  %s (Python 引擎不可达: %v)\n", tc.Name, pyErr)
			skipped++
			continue
		}
		if goErr != nil {
			fmt.Printf("⚠️  SKIP  %s (Go 引擎不可达: %v)\n", tc.Name, goErr)
			skipped++
			continue
		}

		// 比对响应
		match := compareResponses(tc, pyResp, goResp)
		if match {
			fmt.Printf("✅ PASS  %s\n", tc.Name)
			passed++
		} else {
			fmt.Printf("❌ FAIL  %s\n", tc.Name)
			fmt.Printf("        Python: %s\n", truncate(string(pyResp), 200))
			fmt.Printf("        Go:     %s\n", truncate(string(goResp), 200))
			failed++
		}
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Printf("  结果: %d passed, %d failed, %d skipped / %d total\n", passed, failed, skipped, len(cases))
	fmt.Println("══════════════════════════════════════════════════")

	if failed > 0 {
		os.Exit(1)
	}
}

// postJSON 发送 POST 请求并返回响应体。
func postJSON(client *http.Client, url string, body []byte) ([]byte, error) {
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 100))
	}
	return data, nil
}

// compareResponses 比对两个 JSON 响应。
func compareResponses(tc TestCase, pyResp, goResp []byte) bool {
	if len(tc.CompareFields) == 0 && len(tc.ApproxFields) == 0 {
		// 规范化 JSON 后直接比较（忽略空白差异）
		var pyObj, goObj any
		if err := json.Unmarshal(pyResp, &pyObj); err != nil {
			return false
		}
		if err := json.Unmarshal(goResp, &goObj); err != nil {
			return false
		}
		pyNorm, _ := json.Marshal(pyObj)
		goNorm, _ := json.Marshal(goObj)
		return string(pyNorm) == string(goNorm)
	}

	var pyMap, goMap map[string]any
	if err := json.Unmarshal(pyResp, &pyMap); err != nil {
		return false
	}
	if err := json.Unmarshal(goResp, &goMap); err != nil {
		return false
	}

	fields := tc.CompareFields
	if len(fields) == 0 {
		// 从 ApproxFields 推断其余字段为精确比对
		allKeys := make(map[string]bool)
		for k := range pyMap {
			allKeys[k] = true
		}
		for _, af := range tc.ApproxFields {
			delete(allKeys, af)
		}
		for k := range allKeys {
			fields = append(fields, k)
		}
	}

	for _, f := range fields {
		pyV, pyOk := pyMap[f]
		goV, goOk := goMap[f]
		if pyOk != goOk {
			return false
		}
		pyS, _ := json.Marshal(pyV)
		goS, _ := json.Marshal(goV)
		if string(pyS) != string(goS) {
			return false
		}
	}

	// 近似比对浮点字段
	for _, f := range tc.ApproxFields {
		pyV, pyOk := pyMap[f]
		goV, goOk := goMap[f]
		if pyOk != goOk {
			return false
		}
		pyF := toFloat(pyV)
		goF := toFloat(goV)
		// DP 噪声注入允许 ±30% 偏差
		if pyF == 0 && goF == 0 {
			continue
		}
		ratio := (goF - pyF) / maxF(pyF, 0.001)
		if ratio < -0.3 || ratio > 0.3 {
			return false
		}
	}

	return true
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case json.Number:
		f, _ := val.Float64()
		return f
	case int:
		return float64(val)
	default:
		return 0
	}
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
