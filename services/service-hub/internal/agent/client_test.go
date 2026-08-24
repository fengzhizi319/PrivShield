package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
)

func TestAgentClient(t *testing.T) {
	// Setup mock upstream agent server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":    "ok",
				"namespace": "default",
			})
		case "/v1/dynclassification/eval_record":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"level": "L3",
				"tags":  []string{"PII", "Healthcare"},
				"fields": map[string]any{
					"name": map[string]any{"level": "L3", "category": "PII"},
				},
			})
		case "/v1/privacy/mask":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": "张**",
				"field":  body["field_name"],
			})
		case "/v1/privacy/mask_record":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"name":  "张**",
					"phone": "138****0000",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	t.Setenv("PRIVACY_AGENT_URLS", mockServer.URL)
	cfg := config.Load()

	client := New(cfg)
	if client == nil {
		t.Fatal("New(cfg) returned nil")
	}

	ctx := context.Background()

	t.Run("Health", func(t *testing.T) {
		res, err := client.Health(ctx)
		if err != nil {
			t.Fatalf("Health() failed: %v", err)
		}
		if res["status"] != "ok" {
			t.Errorf("Health() got status %v, want ok", res["status"])
		}
	})

	t.Run("Classify", func(t *testing.T) {
		payload := map[string]any{
			"name":  "张三",
			"phone": "13800138000",
		}
		res, err := client.Classify(ctx, payload)
		if err != nil {
			t.Fatalf("Classify() failed: %v", err)
		}
		if res["level"] != "L3" {
			t.Errorf("Classify() got level %v, want L3", res["level"])
		}
	})

	t.Run("Mask", func(t *testing.T) {
		payload := map[string]any{
			"field_name": "name",
			"value":      "张三",
		}
		res, err := client.Mask(ctx, payload)
		if err != nil {
			t.Fatalf("Mask() failed: %v", err)
		}
		if res["result"] != "张**" {
			t.Errorf("Mask() got result %v, want 张**", res["result"])
		}
	})

	t.Run("MaskRecord", func(t *testing.T) {
		record := map[string]string{
			"name":  "张三",
			"phone": "13800138000",
		}
		res, err := client.MaskRecord(ctx, record)
		if err != nil {
			t.Fatalf("MaskRecord() failed: %v", err)
		}
		resultMap, ok := res["result"].(map[string]any)
		if !ok || resultMap["name"] != "张**" {
			t.Errorf("MaskRecord() unexpected result: %+v", res)
		}
	})
}
