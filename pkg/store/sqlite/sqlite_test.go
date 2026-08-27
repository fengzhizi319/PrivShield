// Package sqlite_test provides tests for the SQLite-backed store implementations.
package sqlite_test

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
	"github.com/fengzhizi319/PrivShield/pkg/store/sqlite"
)

// openTestDB creates a temporary SQLite database for testing.
func openTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

// testLogger returns a silent logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ─────────────────────────────────────────────────────────────
// Open
// ─────────────────────────────────────────────────────────────

func TestOpen_EmptyPath(t *testing.T) {
	db, err := sqlite.Open("", testLogger())
	if err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
	if db != nil {
		t.Fatal("expected nil db for empty path")
	}
}

func TestOpen_ValidPath(t *testing.T) {
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()
	if db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestOpen_NilLogger(t *testing.T) {
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, nil) // nil logger should not panic
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()
}

// ─────────────────────────────────────────────────────────────
// TaskStore
// ─────────────────────────────────────────────────────────────

func setupTaskStore(t *testing.T) *sqlite.TaskStore {
	t.Helper()
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ts, err := sqlite.NewTaskStore(db)
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	return ts
}

func TestTaskStore_SaveAndGet(t *testing.T) {
	ts := setupTaskStore(t)
	now := time.Now().Truncate(time.Millisecond)
	task := &store.Task{
		ID:        "task-1",
		Status:    "pending",
		Stage:     "queued",
		Source:    "卫健数据",
		Operation: "mask",
		Priority:  5,
		CreatedAt: now,
	}
	if err := ts.Save(task); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := ts.Get("task-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "task-1" || got.Status != "pending" || got.Operation != "mask" {
		t.Fatalf("unexpected task: %+v", got)
	}
}

func TestTaskStore_GetNotFound(t *testing.T) {
	ts := setupTaskStore(t)
	_, err := ts.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestTaskStore_ListAndFilter(t *testing.T) {
	ts := setupTaskStore(t)
	now := time.Now()
	for i, status := range []string{"pending", "pending", "completed"} {
		ts.Save(&store.Task{
			ID:        fmt_id("task-%d", i),
			Status:    status,
			Stage:     "queued",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	// List all
	all, total, err := ts.List(store.TaskFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d (total=%d)", len(all), total)
	}
	// Filter by status
	pending, pTotal, err := ts.List(store.TaskFilter{Status: "pending"})
	if err != nil {
		t.Fatalf("list filter: %v", err)
	}
	if pTotal != 2 || len(pending) != 2 {
		t.Fatalf("expected 2 pending tasks, got %d (total=%d)", len(pending), pTotal)
	}
}

func TestTaskStore_ListWithLimit(t *testing.T) {
	ts := setupTaskStore(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		ts.Save(&store.Task{
			ID:        fmt_id("task-%d", i),
			Status:    "pending",
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	tasks, total, err := ts.List(store.TaskFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks with limit, got %d", len(tasks))
	}
}

func TestTaskStore_Update(t *testing.T) {
	ts := setupTaskStore(t)
	now := time.Now()
	ts.Save(&store.Task{ID: "task-u", Status: "pending", Stage: "queued", CreatedAt: now})
	started := now.Add(time.Second)
	task := &store.Task{
		ID:         "task-u",
		Status:     "running",
		Stage:      "processing",
		StartedAt:  &started,
		DurationMs: 0,
	}
	if err := ts.Update(task); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := ts.Get("task-u")
	if got.Status != "running" || got.Stage != "processing" || got.StartedAt == nil {
		t.Fatalf("update not applied: %+v", got)
	}
}

func TestTaskStore_Counts(t *testing.T) {
	ts := setupTaskStore(t)
	now := time.Now()
	statuses := []string{"pending", "pending", "running", "completed", "failed"}
	for i, s := range statuses {
		ts.Save(&store.Task{ID: fmt_id("c-%d-%s", i, s), Status: s, CreatedAt: now})
	}
	counts, err := ts.Counts()
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Pending != 2 || counts.Running != 1 || counts.Completed != 1 || counts.Failed != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

// ─────────────────────────────────────────────────────────────
// DataSourceStore
// ─────────────────────────────────────────────────────────────

func setupDSStore(t *testing.T) *sqlite.DataSourceStore {
	t.Helper()
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ds, err := sqlite.NewDataSourceStore(db)
	if err != nil {
		t.Fatalf("new datasource store: %v", err)
	}
	return ds
}

func TestDataSourceStore_SaveAndGet(t *testing.T) {
	ds := setupDSStore(t)
	now := time.Now()
	src := &store.DataSource{
		ID:            "ds-1",
		Name:          "卫健数据库",
		Type:          "database",
		Host:          "10.0.0.1",
		Port:          3306,
		Database:      "health",
		SecurityLevel: "high",
		Status:        "connected",
		CreatedAt:     now,
		Tags:          []string{"卫健", "高密"},
	}
	if err := ds.SaveDS(src); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := ds.GetDS("ds-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "卫健数据库" || got.SecurityLevel != "high" {
		t.Fatalf("unexpected ds: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "卫健" {
		t.Fatalf("unexpected tags: %v", got.Tags)
	}
}

func TestDataSourceStore_ListAndDelete(t *testing.T) {
	ds := setupDSStore(t)
	now := time.Now()
	ds.SaveDS(&store.DataSource{ID: "ds-a", Name: "A", CreatedAt: now})
	ds.SaveDS(&store.DataSource{ID: "ds-b", Name: "B", CreatedAt: now})
	list, _, err := ds.ListDS(store.DataSourceFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if err := ds.DeleteDS("ds-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _, _ = ds.ListDS(store.DataSourceFilter{})
	if len(list) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(list))
	}
}

func TestDataSourceStore_DeleteNotFound(t *testing.T) {
	ds := setupDSStore(t)
	err := ds.DeleteDS("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent datasource")
	}
}

func TestDataSourceStore_Update(t *testing.T) {
	ds := setupDSStore(t)
	now := time.Now()
	ds.SaveDS(&store.DataSource{ID: "ds-u", Name: "Old", Status: "disconnected", CreatedAt: now})
	checkAt := now.Add(time.Minute)
	updated := &store.DataSource{
		ID:          "ds-u",
		Name:        "New",
		Status:      "connected",
		LastCheckAt: &checkAt,
		Tags:        []string{"updated"},
	}
	if err := ds.UpdateDS(updated); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := ds.GetDS("ds-u")
	if got.Name != "New" || got.Status != "connected" || got.LastCheckAt == nil {
		t.Fatalf("update not applied: %+v", got)
	}
}

func TestDataSourceStore_Audit(t *testing.T) {
	ds := setupDSStore(t)
	now := time.Now()
	rec := &store.AccessAuditRecord{
		ID:             "audit-1",
		DataSourceID:   "ds-1",
		DataSourceName: "卫健",
		Operation:      "query",
		User:           "admin",
		Timestamp:      now,
		RecordsCount:   100,
		Status:         "success",
	}
	if err := ds.SaveAudit(rec); err != nil {
		t.Fatalf("save audit: %v", err)
	}
	records, _, err := ds.ListAudit("ds-1", 0, 0)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(records) != 1 || records[0].RecordsCount != 100 {
		t.Fatalf("unexpected audit records: %+v", records)
	}
	// Filter by different dsID should return empty
	empty, _, err := ds.ListAudit("ds-other", 0, 0)
	if err != nil {
		t.Fatalf("list audit filter: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 records, got %d", len(empty))
	}
}

// ─────────────────────────────────────────────────────────────
// AuditStore
// ─────────────────────────────────────────────────────────────

func setupAuditStore(t *testing.T) *sqlite.AuditStore {
	t.Helper()
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	as, err := sqlite.NewAuditStore(db)
	if err != nil {
		t.Fatalf("new audit store: %v", err)
	}
	return as
}

func TestAuditStore_SaveAndGetLog(t *testing.T) {
	as := setupAuditStore(t)
	now := time.Now()
	log := &store.AuditLog{
		ID:            "log-1",
		Timestamp:     now,
		Operation:     "mask",
		DataSource:    "卫健",
		InputHash:     "abc123",
		OutputHash:    "def456",
		Algorithm:     "field_mask",
		InputRows:     10,
		OutputRows:    10,
		DurationMs:    50,
		User:          "admin",
		Status:        "success",
		SecurityLevel: "L3",
	}
	if err := as.SaveLog(log); err != nil {
		t.Fatalf("save log: %v", err)
	}
	got, err := as.GetLog("log-1")
	if err != nil {
		t.Fatalf("get log: %v", err)
	}
	if got.Operation != "mask" || got.Algorithm != "field_mask" || got.SecurityLevel != "L3" {
		t.Fatalf("unexpected log: %+v", got)
	}
}

func TestAuditStore_ListLogsFilter(t *testing.T) {
	as := setupAuditStore(t)
	now := time.Now()
	for i, op := range []string{"mask", "mask", "dp"} {
		as.SaveLog(&store.AuditLog{
			ID:        fmt_id("log-%d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Operation: op,
			User:      "admin",
			Status:    "success",
		})
	}
	// Filter by operation
	logs, total, err := as.ListLogs(store.AuditFilter{Operation: "mask"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("expected 2 mask logs, got %d (total=%d)", len(logs), total)
	}
	// Filter by user
	logs2, total2, err := as.ListLogs(store.AuditFilter{User: "admin"})
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if total2 != 3 || len(logs2) != 3 {
		t.Fatalf("expected 3 admin logs, got %d (total=%d)", len(logs2), total2)
	}
	// No filter
	all, allTotal, _ := as.ListLogs(store.AuditFilter{})
	if allTotal != 3 || len(all) != 3 {
		t.Fatalf("expected 3 total, got %d", len(all))
	}
}

func TestAuditStore_ListLogsWithLimit(t *testing.T) {
	as := setupAuditStore(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		as.SaveLog(&store.AuditLog{
			ID:        fmt_id("log-l-%d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Operation: "mask",
		})
	}
	logs, total, err := as.ListLogs(store.AuditFilter{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected total=5, got %d", total)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
}

func TestAuditStore_Snapshots(t *testing.T) {
	as := setupAuditStore(t)
	now := time.Now()

	// P26 fix: create the parent audit log first to satisfy foreign key constraint
	// 先创建父级审计日志记录以满足外键约束
	parentLog := &store.AuditLog{
		ID:            "log-1",
		Timestamp:     now,
		Operation:     "mask",
		DataSource:    "test",
		InputHash:     "input-hash",
		OutputHash:    "output-hash",
		Algorithm:     "field_mask",
		InputRows:     100,
		OutputRows:    100,
		DurationMs:    50,
		User:          "tester",
		Status:        "success",
		SecurityLevel: "L3",
	}
	if err := as.SaveLog(parentLog); err != nil {
		t.Fatalf("save parent log: %v", err)
	}

	snap := &store.SnapshotRecord{
		ID:            "snap-1",
		AuditLogID:    "log-1",
		Timestamp:     now,
		InputSample:   `{"name":"张三"}`,
		OutputSample:  `{"name":"张*"}`,
		Algorithm:     "field_mask",
		IntegrityHash: "sha256:abc",
	}
	if err := as.SaveSnapshot(snap); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	// Get by ID
	got, err := as.GetSnapshot("snap-1")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if got.Algorithm != "field_mask" || got.IntegrityHash != "sha256:abc" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	// List
	snaps, total, err := as.ListSnapshots(10, 0)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
}

func TestAuditStore_GetLogNotFound(t *testing.T) {
	as := setupAuditStore(t)
	_, err := as.GetLog("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent log")
	}
}

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

// fmt_id is a helper to format an ID string.
func fmt_id(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// ─────────────────────────────────────────────────────────────
// ValidateIntegrity
// ─────────────────────────────────────────────────────────────

func TestValidateIntegrity_EmptyPath(t *testing.T) {
	// Empty path should return nil (memory mode, no check needed)
	err := sqlite.ValidateIntegrity("")
	if err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
}

func TestValidateIntegrity_ValidDatabase(t *testing.T) {
	// Create a valid database and check integrity
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Create tables to make it a real database
	if err := sqlite.InitTaskTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}
	db.Close()

	// Now validate integrity
	err = sqlite.ValidateIntegrity(dbPath)
	if err != nil {
		t.Fatalf("expected nil error for valid database, got %v", err)
	}
}

func TestValidateIntegrity_NonexistentPath(t *testing.T) {
	// Non-existent path should return error
	err := sqlite.ValidateIntegrity("/nonexistent/path/to/database.db")
	if err == nil {
		t.Fatal("expected error for nonexistent database path")
	}
}

func TestValidateIntegrity_CorruptedDatabase(t *testing.T) {
	// Create a database, then corrupt it by writing garbage
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := sqlite.InitTaskTables(db); err != nil {
		t.Fatalf("init tables: %v", err)
	}
	db.Close()

	// Corrupt the database by overwriting part of the file
	f, err := os.OpenFile(dbPath, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for corruption: %v", err)
	}
	// Write garbage at the beginning of the file (after the first 100 bytes to partially corrupt)
	_, _ = f.WriteAt([]byte("CORRUPTED_DATA_GARBHERE"), 100)
	f.Close()

	// ValidateIntegrity should detect corruption
	err = sqlite.ValidateIntegrity(dbPath)
	if err == nil {
		t.Fatal("expected error for corrupted database")
	}
}

// ─────────────────────────────────────────────────────────────
// Phase B: LeasedTaskStore rejection tests
// SQLite 租约拒绝测试：验证所有租约方法返回 ErrLeaseNotSupported
// ─────────────────────────────────────────────────────────────

func TestLeasedTaskStore_ClaimNext_ReturnsNotSupported(t *testing.T) {
	ts := setupTaskStore(t)
	lease, err := ts.ClaimNext("hub-1", 60*time.Second)
	if err != store.ErrLeaseNotSupported {
		t.Fatalf("expected ErrLeaseNotSupported, got lease=%v err=%v", lease, err)
	}
}

func TestLeasedTaskStore_RenewLease_ReturnsNotSupported(t *testing.T) {
	ts := setupTaskStore(t)
	ok, err := ts.RenewLease("task-1", "hub-1", "token", 60*time.Second)
	if err != store.ErrLeaseNotSupported {
		t.Fatalf("expected ErrLeaseNotSupported, got ok=%v err=%v", ok, err)
	}
}

func TestLeasedTaskStore_CompleteLease_ReturnsNotSupported(t *testing.T) {
	ts := setupTaskStore(t)
	ok, err := ts.CompleteLease("task-1", "hub-1", "token", store.TaskResult{})
	if err != store.ErrLeaseNotSupported {
		t.Fatalf("expected ErrLeaseNotSupported, got ok=%v err=%v", ok, err)
	}
}

func TestLeasedTaskStore_FailLease_ReturnsNotSupported(t *testing.T) {
	ts := setupTaskStore(t)
	ok, err := ts.FailLease("task-1", "hub-1", "token", store.TaskFailure{Error: "test"})
	if err != store.ErrLeaseNotSupported {
		t.Fatalf("expected ErrLeaseNotSupported, got ok=%v err=%v", ok, err)
	}
}

func TestLeasedTaskStore_RequeueExpiredLeases_ReturnsNotSupported(t *testing.T) {
	ts := setupTaskStore(t)
	count, err := ts.RequeueExpiredLeases(10)
	if err != store.ErrLeaseNotSupported {
		t.Fatalf("expected ErrLeaseNotSupported, got count=%v err=%v", count, err)
	}
}

// TestLeasedTaskStore_InterfaceCompliance verifies compile-time interface assertion.
func TestLeasedTaskStore_InterfaceCompliance(t *testing.T) {
	ts := setupTaskStore(t)
	// This will fail at compile time if sqlite.TaskStore doesn't implement LeasedTaskStore.
	var _ store.LeasedTaskStore = ts
}

// ─────────────────────────────────────────────────────────────
// Legacy Schema Migration Regression Tests (P0-1 & P0-3)
// ─────────────────────────────────────────────────────────────

func TestInitAuditTables_LegacyMigration(t *testing.T) {
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 1. Manually create the 15-column legacy audit_logs schema (without task_id, api_code, datasource_id)
	_, err = db.Exec(`
		CREATE TABLE audit_logs (
			id TEXT PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			operation TEXT,
			datasource TEXT,
			input_hash TEXT,
			output_hash TEXT,
			algorithm TEXT,
			parameters_json TEXT,
			input_rows INTEGER DEFAULT 0,
			output_rows INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			user_name TEXT,
			status TEXT,
			error_message TEXT,
			security_level TEXT
		);
		CREATE TABLE snapshots (
			id TEXT PRIMARY KEY,
			audit_log_id TEXT,
			timestamp DATETIME NOT NULL,
			input_sample TEXT,
			output_sample TEXT,
			algorithm TEXT,
			parameters_json TEXT,
			integrity_hash TEXT,
			FOREIGN KEY(audit_log_id) REFERENCES audit_logs(id)
		);
	`)
	if err != nil {
		t.Fatalf("create legacy audit tables: %v", err)
	}

	// Insert a legacy row
	_, err = db.Exec(`
		INSERT INTO audit_logs (id, timestamp, operation, datasource, user_name, status)
		VALUES ('legacy-1', '2026-08-20T10:00:00Z', 'mask', 'ds_yibao', 'tester', 'success')
	`)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	// 2. Call production InitAuditTables — must migrate cleanly without crash or SQL error
	if err := sqlite.InitAuditTables(db); err != nil {
		t.Fatalf("InitAuditTables failed on legacy db: %v", err)
	}

	// 3. Verify new columns exist and can be queried
	as, err := sqlite.NewAuditStore(db)
	if err != nil {
		t.Fatalf("new audit store: %v", err)
	}

	// Save a new log with canonical fields
	now := time.Now()
	newLog := &store.AuditLog{
		ID:           "migrated-2",
		TaskID:       "task-123",
		APICode:      "api1_yibao",
		DatasourceID: "ds_yibao",
		Timestamp:    now,
		Operation:    "mask",
		DataSource:   "ds_yibao",
		Status:       "success",
	}
	if err := as.SaveLog(newLog); err != nil {
		t.Fatalf("save new log on migrated db: %v", err)
	}

	got, err := as.GetLog("migrated-2")
	if err != nil {
		t.Fatalf("get migrated log: %v", err)
	}
	if got.TaskID != "task-123" || got.APICode != "api1_yibao" || got.DatasourceID != "ds_yibao" {
		t.Fatalf("canonical fields not stored: %+v", got)
	}

	// Filter by task_id and datasource_id
	filtered, total, err := as.ListLogs(store.AuditFilter{TaskID: "task-123", DatasourceID: "ds_yibao"})
	if err != nil {
		t.Fatalf("filter logs: %v", err)
	}
	if total != 1 || len(filtered) != 1 {
		t.Fatalf("expected 1 filtered log, got %d (total=%d)", len(filtered), total)
	}
}

func TestInitTaskTables_LegacyMigration(t *testing.T) {
	dbPath := openTestDB(t)
	db, err := sqlite.Open(dbPath, testLogger())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 1. Manually create legacy tasks schema (without api_code, datasource_id)
	_, err = db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'pending',
			stage TEXT NOT NULL DEFAULT 'queued',
			source TEXT,
			operation TEXT,
			priority INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			duration_ms INTEGER DEFAULT 0,
			error TEXT,
			payload_json TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create legacy tasks table: %v", err)
	}

	// Insert legacy row with source = ds_yibao
	_, err = db.Exec(`
		INSERT INTO tasks (id, status, stage, source, operation, created_at)
		VALUES ('legacy-task-1', 'pending', 'queued', 'ds_yibao', 'mask', '2026-08-20T10:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert legacy task: %v", err)
	}

	// 2. Call production InitTaskTables — must migrate and backfill
	if err := sqlite.InitTaskTables(db); err != nil {
		t.Fatalf("InitTaskTables failed on legacy db: %v", err)
	}

	ts, err := sqlite.NewTaskStore(db)
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}

	// Verify backfill: legacy-task-1 should now have datasource_id="ds_yibao" and api_code="api1_yibao"
	task, err := ts.Get("legacy-task-1")
	if err != nil {
		t.Fatalf("get legacy task: %v", err)
	}
	if task.DatasourceID != "ds_yibao" {
		t.Fatalf("expected backfilled DatasourceID 'ds_yibao', got %q", task.DatasourceID)
	}
	if task.APICode != "api1_yibao" {
		t.Fatalf("expected backfilled APICode 'api1_yibao', got %q", task.APICode)
	}

	// Save new task with canonical fields and get it
	now := time.Now()
	newTask := &store.Task{
		ID:           "new-task-2",
		Status:       "running",
		Stage:        "processing",
		Source:       "ds_kangyang",
		APICode:      "api2_kangyang",
		DatasourceID: "ds_kangyang",
		Operation:    "k_anon",
		CreatedAt:    now,
	}
	if err := ts.Save(newTask); err != nil {
		t.Fatalf("save new task: %v", err)
	}
	gotNew, err := ts.Get("new-task-2")
	if err != nil {
		t.Fatalf("get new task: %v", err)
	}
	if gotNew.DatasourceID != "ds_kangyang" || gotNew.APICode != "api2_kangyang" {
		t.Fatalf("canonical fields not preserved on get: %+v", gotNew)
	}
}
