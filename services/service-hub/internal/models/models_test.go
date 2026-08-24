package models

import (
	"encoding/json"
	"testing"
	"time"
)

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
		{"L6", "mask"}, // unknown level defaults to mask
		{"unknown", "mask"},
		{"", "mask"},
	}

	for _, tt := range tests {
		t.Run("Level_"+tt.level, func(t *testing.T) {
			got := LevelToOperation(tt.level)
			if got != tt.expected {
				t.Errorf("LevelToOperation(%q) = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestModelSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)

	t.Run("HubStatus", func(t *testing.T) {
		status := HubStatus{
			Status:         "running",
			Uptime:         "1h20m",
			ActiveTasks:    5,
			QueuedTasks:    2,
			CompletedTotal: 100,
			FailedTotal:    3,
			AgentURL:       "http://127.0.0.1:8079",
		}
		data, err := json.Marshal(status)
		if err != nil {
			t.Fatalf("Marshal HubStatus failed: %v", err)
		}
		var parsed HubStatus
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Unmarshal HubStatus failed: %v", err)
		}
		if parsed.Status != status.Status || parsed.ActiveTasks != status.ActiveTasks {
			t.Errorf("HubStatus mismatch: got %+v, want %+v", parsed, status)
		}
	})

	t.Run("Task", func(t *testing.T) {
		task := Task{
			ID:          "task-12345",
			Status:      "completed",
			Stage:       "done",
			Source:      "yibao.csv",
			Operation:   "mask",
			CreatedAt:   now,
			StartedAt:   &now,
			CompletedAt: &now,
			DurationMs:  450,
			Error:       "",
		}
		data, err := json.Marshal(task)
		if err != nil {
			t.Fatalf("Marshal Task failed: %v", err)
		}
		var parsed Task
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Unmarshal Task failed: %v", err)
		}
		if parsed.ID != task.ID || parsed.Operation != task.Operation {
			t.Errorf("Task mismatch: got %+v, want %+v", parsed, task)
		}
	})

	t.Run("PipelineStatus", func(t *testing.T) {
		pStatus := PipelineStatus{
			Stages: []PipelineStage{
				{
					Name:         "ingest",
					Status:       "idle",
					ActiveCount:  0,
					AvgLatencyMs: 25,
					Throughput:   120,
				},
				{
					Name:         "mask",
					Status:       "processing",
					ActiveCount:  3,
					AvgLatencyMs: 150,
					Throughput:   300,
				},
			},
			TotalRPS: 12.5,
			AgentOK:  true,
		}
		data, err := json.Marshal(pStatus)
		if err != nil {
			t.Fatalf("Marshal PipelineStatus failed: %v", err)
		}
		var parsed PipelineStatus
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Unmarshal PipelineStatus failed: %v", err)
		}
		if len(parsed.Stages) != 2 || !parsed.AgentOK {
			t.Errorf("PipelineStatus mismatch: got %+v, want %+v", parsed, pStatus)
		}
	})

	t.Run("DispatchAndProxyResponse", func(t *testing.T) {
		req := DispatchRequest{
			Source:    "kangyang.csv",
			Operation: "k_anon",
			Payload:   map[string]any{"age": 65, "diagnosis": "hypertension"},
			Priority:  1,
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal DispatchRequest failed: %v", err)
		}
		var parsedReq DispatchRequest
		if err := json.Unmarshal(data, &parsedReq); err != nil {
			t.Fatalf("Unmarshal DispatchRequest failed: %v", err)
		}
		if parsedReq.Source != req.Source || parsedReq.Operation != req.Operation {
			t.Errorf("DispatchRequest mismatch: got %+v, want %+v", parsedReq, req)
		}

		resp := DispatchResponse{
			TaskID: "task-999",
			Status: "accepted",
			Via:    "service-hub",
		}
		respData, _ := json.Marshal(resp)
		var parsedResp DispatchResponse
		_ = json.Unmarshal(respData, &parsedResp)
		if parsedResp.TaskID != resp.TaskID {
			t.Errorf("DispatchResponse mismatch: got %+v, want %+v", parsedResp, resp)
		}

		proxy := ProxyResponse{
			Status:     200,
			DurationMs: 35,
			Data:       resp,
			Via:        "service-hub",
		}
		proxyData, _ := json.Marshal(proxy)
		var parsedProxy ProxyResponse
		_ = json.Unmarshal(proxyData, &parsedProxy)
		if parsedProxy.Status != 200 || parsedProxy.Via != "service-hub" {
			t.Errorf("ProxyResponse mismatch: got %+v, want %+v", parsedProxy, proxy)
		}
	})
}
