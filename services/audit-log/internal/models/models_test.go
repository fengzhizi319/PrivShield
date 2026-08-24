package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAuditLogSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	log := AuditLog{
		ID:            "audit_1001",
		Timestamp:     now,
		Operation:     "mask",
		DataSource:    "yibao.csv",
		InputHash:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		OutputHash:    "ca978112ca1bbdcafac231b39a23dc4da786081998d6365faf57629009733549",
		Algorithm:     "field_mask",
		Parameters:    map[string]any{"pattern": "id_card"},
		InputRows:     50,
		OutputRows:    50,
		DurationMs:    12,
		User:          "sec_officer",
		Status:        "success",
		SecurityLevel: "L4",
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal AuditLog: %v", err)
	}

	var parsed AuditLog
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal AuditLog: %v", err)
	}

	if parsed.ID != log.ID || parsed.Operation != log.Operation || parsed.InputRows != 50 {
		t.Errorf("unmarshaled AuditLog mismatch: %+v vs %+v", parsed, log)
	}
}

func TestSnapshotRecordSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	snap := SnapshotRecord{
		ID:            "snap_1",
		AuditLogID:    "audit_1001",
		Timestamp:     now,
		InputSample:   `{"id_card": "510101199001011234"}`,
		OutputSample:  `{"id_card": "510101********1234"}`,
		Algorithm:     "field_mask",
		IntegrityHash: "ca978112ca1bbdcafac231b39a23dc4da786081998d6365faf57629009733549",
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("failed to marshal SnapshotRecord: %v", err)
	}

	var parsed SnapshotRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal SnapshotRecord: %v", err)
	}

	if parsed.ID != snap.ID || parsed.IntegrityHash != snap.IntegrityHash {
		t.Errorf("unmarshaled SnapshotRecord mismatch: %+v", parsed)
	}
}

func TestComplianceReportSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	rep := ComplianceReport{
		ID:              "rep_1",
		GeneratedAt:     now,
		Period:          "30d",
		TotalOps:        1500,
		SuccessRate:     99.8,
		ByLevel:         map[string]int{"L1": 200, "L3": 800, "L4": 500},
		TopOperations:   []string{"mask", "classify", "k_anon"},
		Recommendations: []string{"强化 L4/L5 敏感数据审计"},
	}

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("failed to marshal ComplianceReport: %v", err)
	}

	var parsed ComplianceReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal ComplianceReport: %v", err)
	}

	if parsed.TotalOps != 1500 || parsed.SuccessRate != 99.8 {
		t.Errorf("unmarshaled ComplianceReport mismatch: %+v", parsed)
	}
}
