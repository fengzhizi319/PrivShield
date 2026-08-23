package memory

import (
	"testing"
	"time"

	"github.com/fengzhizi319/PrivShield/console/pkg/store"
)

// ─────────────────────────────────────────────────────────────
// TaskStore tests
// ─────────────────────────────────────────────────────────────

func TestTaskStore_SaveAndGet(t *testing.T) {
	s := NewTaskStore()
	task := &store.Task{
		ID:        "t1",
		Status:    "pending",
		Stage:     "queued",
		Source:    "test",
		Operation: "mask",
		CreatedAt: time.Now(),
	}

	if err := s.Save(task); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.Get("t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "t1" || got.Status != "pending" {
		t.Errorf("unexpected task: %+v", got)
	}
}

func TestTaskStore_GetNotFound(t *testing.T) {
	s := NewTaskStore()
	if _, err := s.Get("nonexistent"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestTaskStore_List(t *testing.T) {
	s := NewTaskStore()
	for i, status := range []string{"pending", "running", "completed"} {
		s.Save(&store.Task{
			ID:        "t" + string(rune('0'+i+1)),
			Status:    status,
			CreatedAt: time.Now(),
		})
	}

	// List all
	tasks, total, err := s.List(store.TaskFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3, got %d", total)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// Filter by status
	filtered, count, _ := s.List(store.TaskFilter{Status: "running"})
	if count != 1 {
		t.Errorf("expected 1 running, got %d", count)
	}
	if len(filtered) != 1 || filtered[0].Status != "running" {
		t.Errorf("filter mismatch: %+v", filtered)
	}
}

func TestTaskStore_Update(t *testing.T) {
	s := NewTaskStore()
	s.Save(&store.Task{ID: "t1", Status: "pending", CreatedAt: time.Now()})

	got, _ := s.Get("t1")
	got.Status = "completed"
	if err := s.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}

	updated, _ := s.Get("t1")
	if updated.Status != "completed" {
		t.Errorf("expected completed, got %s", updated.Status)
	}
}

func TestTaskStore_Counts(t *testing.T) {
	s := NewTaskStore()
	for _, status := range []string{"pending", "running", "completed", "completed", "failed"} {
		s.Save(&store.Task{ID: "t-" + status + "-" + time.Now().String(), Status: status, CreatedAt: time.Now()})
	}

	counts, err := s.Counts()
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Pending != 1 {
		t.Errorf("pending: expected 1, got %d", counts.Pending)
	}
	if counts.Running != 1 {
		t.Errorf("running: expected 1, got %d", counts.Running)
	}
	if counts.Completed != 2 {
		t.Errorf("completed: expected 2, got %d", counts.Completed)
	}
	if counts.Failed != 1 {
		t.Errorf("failed: expected 1, got %d", counts.Failed)
	}
}

// ─────────────────────────────────────────────────────────────
// DataSourceStore tests
// ─────────────────────────────────────────────────────────────

func TestDataSourceStore_CRUD(t *testing.T) {
	s := NewDataSourceStore()

	ds := &store.DataSource{
		ID:   "ds1",
		Name: "test-db",
		Type: "database",
		Host: "localhost",
		Port: 5432,
	}

	// Save
	if err := s.SaveDS(ds); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Get
	got, err := s.GetDS("ds1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test-db" {
		t.Errorf("expected test-db, got %s", got.Name)
	}

	// List
	list, _, err := s.ListDS(store.DataSourceFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1, got %d", len(list))
	}

	// Update
	got.Name = "updated-db"
	if err := s.UpdateDS(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, _ := s.GetDS("ds1")
	if updated.Name != "updated-db" {
		t.Errorf("expected updated-db, got %s", updated.Name)
	}

	// Delete
	if err := s.DeleteDS("ds1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetDS("ds1"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestDataSourceStore_Audit(t *testing.T) {
	s := NewDataSourceStore()

	rec := store.AccessAuditRecord{
		ID:           "a1",
		DataSourceID: "ds1",
		Operation:    "read",
		User:         "admin",
		Timestamp:    time.Now(),
	}
	if err := s.SaveAudit(&rec); err != nil {
		t.Fatalf("save audit: %v", err)
	}

	records, _, err := s.ListAudit("ds1", 0, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 audit record, got %d", len(records))
	}
}

// ─────────────────────────────────────────────────────────────
// AuditStore tests
// ─────────────────────────────────────────────────────────────

func TestAuditStore_LogCRUD(t *testing.T) {
	s := NewAuditStore()

	log := &store.AuditLog{
		ID:        "log1",
		Timestamp: time.Now(),
		Operation: "mask",
		Status:    "success",
	}

	if err := s.SaveLog(log); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetLog("log1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Operation != "mask" {
		t.Errorf("expected mask, got %s", got.Operation)
	}

	logs, total, err := s.ListLogs(store.AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Errorf("expected 1, got total=%d len=%d", total, len(logs))
	}
}

func TestAuditStore_LogFilter(t *testing.T) {
	s := NewAuditStore()

	for _, op := range []string{"mask", "mask", "k_anon"} {
		s.SaveLog(&store.AuditLog{
			ID:        "log-" + op + "-" + time.Now().String(),
			Timestamp: time.Now(),
			Operation: op,
			Status:    "success",
		})
	}

	filtered, count, _ := s.ListLogs(store.AuditFilter{Operation: "mask"})
	if count != 2 {
		t.Errorf("expected 2 mask logs, got %d", count)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered logs, got %d", len(filtered))
	}
}

func TestAuditStore_Snapshots(t *testing.T) {
	s := NewAuditStore()

	snap := &store.SnapshotRecord{
		ID:            "snap1",
		AuditLogID:    "log1",
		Timestamp:     time.Now(),
		Algorithm:     "field_mask",
		IntegrityHash: "abc123",
	}

	if err := s.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	snaps, total, err := s.ListSnapshots(10, 0)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}

	got, err := s.GetSnapshot("snap1")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if got.IntegrityHash != "abc123" {
		t.Errorf("expected abc123, got %s", got.IntegrityHash)
	}
}
