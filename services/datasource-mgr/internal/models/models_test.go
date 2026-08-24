package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDataSourceSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	ds := DataSource{
		ID:            "ds_test_1",
		Name:          "Test Database",
		Type:          "database",
		Host:          "127.0.0.1",
		Port:          3306,
		Database:      "medical_db",
		SecurityLevel: "high",
		Status:        "connected",
		CreatedAt:     now,
		LastCheckAt:   &now,
		Tags:          []string{"test", "db"},
	}

	data, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("failed to marshal DataSource: %v", err)
	}

	var parsed DataSource
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal DataSource: %v", err)
	}

	if parsed.ID != ds.ID || parsed.Name != ds.Name || parsed.Port != ds.Port {
		t.Errorf("unmarshaled DataSource mismatch: %+v vs %+v", parsed, ds)
	}
}

func TestMetadataModelsSerialization(t *testing.T) {
	meta := MetadataResponse{
		DataSourceID: "ds_1",
		Via:          "datasource-mgr",
		Tables: []TableMetadata{
			{
				Name:     "patient_records",
				RowCount: 150,
				Fields: []MetadataField{
					{Name: "id", Type: "integer", SecurityLevel: "L1", Classification: "general", Sensitive: false},
					{Name: "name", Type: "string", SecurityLevel: "L3", Classification: "PII_Name", Sensitive: true},
				},
			},
		},
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal MetadataResponse: %v", err)
	}

	var parsed MetadataResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal MetadataResponse: %v", err)
	}

	if len(parsed.Tables) != 1 || len(parsed.Tables[0].Fields) != 2 {
		t.Errorf("unmarshaled MetadataResponse mismatch: %+v", parsed)
	}
}

func TestAccessAuditSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	audit := AccessAuditResponse{
		Total: 1,
		Via:   "datasource-mgr",
		Records: []AccessAuditRecord{
			{
				ID:           "audit_1",
				DataSourceID: "ds_1",
				Operation:    "query",
				User:         "admin",
				Timestamp:    now,
				RecordsCount: 10,
				Status:       "success",
			},
		},
	}

	data, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("failed to marshal AccessAuditResponse: %v", err)
	}

	var parsed AccessAuditResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AccessAuditResponse: %v", err)
	}

	if parsed.Total != 1 || parsed.Records[0].User != "admin" {
		t.Errorf("unmarshaled AccessAuditResponse mismatch: %+v", parsed)
	}
}

func TestConnectionTestResultSerialization(t *testing.T) {
	res := ConnectionTestResult{
		DataSourceID: "ds_1",
		Success:      true,
		LatencyMs:    15,
		Via:          "datasource-mgr",
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal ConnectionTestResult: %v", err)
	}

	var parsed ConnectionTestResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ConnectionTestResult: %v", err)
	}

	if parsed.DataSourceID != "ds_1" || !parsed.Success || parsed.LatencyMs != 15 {
		t.Errorf("unmarshaled ConnectionTestResult mismatch: %+v", parsed)
	}
}
