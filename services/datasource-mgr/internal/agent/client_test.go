package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fengzhizi319/PrivShield/services/datasource-mgr/internal/config"
)

func TestClientHealthAndClassify(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "version": "0.1.0"})
	})
	mux.HandleFunc("/v1/dynclassification/classify", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"max_level": "L4",
			"tags": []map[string]any{
				{"category": "PII_IDCard", "level": "L4"},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("PRIVACY_AGENT_URLS", srv.URL)
	cfg := config.Load()
	c := New(cfg)

	ctx := context.Background()

	// 1. Health
	health, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("expected health status ok, got %v", health["status"])
	}

	// 2. Classify
	res, err := c.Classify(ctx, map[string]any{
		"field_name":  "id_card",
		"field_value": "510101199001011234",
	})
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}
	if res["max_level"] != "L4" {
		t.Errorf("expected max_level L4, got %v", res["max_level"])
	}
}
