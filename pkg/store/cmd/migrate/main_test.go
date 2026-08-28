package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store/postgres"
	"github.com/fengzhizi319/PrivShield/pkg/store/sqlite"
)

func getTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PRIVSHIELD_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PRIVSHIELD_PG_TEST_DSN not set, skipping migration integration test")
	}
	return dsn
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func computeIntegrityHash(logID, prevHash string, timestamp time.Time, algorithm, inputHash, outputHash, user, securityLevel, paramsJSON string) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%v",
		prevHash, logID, timestamp.Format(time.RFC3339Nano), algorithm,
		inputHash, outputHash, user, securityLevel, paramsJSON)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func TestMigrateSQLiteToPostgres(t *testing.T) {
	ctx := context.Background()
	dsn := getTestDSN(t)
	logger := testLogger()

	// Prepare target PostgreSQL tables and clean any leftover test data.
	taskStore, err := postgres.New(ctx, postgres.Config{DSN: dsn, MaxConn: 3, MinConn: 1}, logger)
	if err != nil {
		t.Fatalf("create postgres task store: %v", err)
	}
	defer taskStore.Close()

	auditStore, err := postgres.NewAuditStore(ctx, postgres.Config{DSN: dsn}, logger)
	if err != nil {
		t.Fatalf("create postgres audit store: %v", err)
	}
	defer auditStore.Close()

	pool := taskStore.Pool()
	if _, err := pool.Exec(ctx, "DELETE FROM snapshots"); err != nil {
		t.Fatalf("clean snapshots: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM audit_logs"); err != nil {
		t.Fatalf("clean audit_logs: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM tasks"); err != nil {
		t.Fatalf("clean tasks: %v", err)
	}

	// Prepare source SQLite files in a temporary directory.
	tmpDir := t.TempDir()
	hubDBPath := filepath.Join(tmpDir, "service-hub.db")
	auditDBPath := filepath.Join(tmpDir, "audit-log.db")

	hubDB, err := sql.Open("sqlite", hubDBPath)
	if err != nil {
		t.Fatalf("open hub sqlite: %v", err)
	}
	if err := sqlite.InitTaskTables(hubDB); err != nil {
		t.Fatalf("init hub task tables: %v", err)
	}
	if _, err := hubDB.Exec(`
		INSERT INTO tasks (id, status, stage, source, api_code, datasource_id, operation, priority, created_at, duration_ms)
		VALUES ('task-1', 'completed', 'done', 'ds_yibao', 'api1_yibao', 'ds_yibao', 'mask', 50, ?, 120)
	`, time.Now()); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	_ = hubDB.Close()

	auditDB, err := sql.Open("sqlite", auditDBPath)
	if err != nil {
		t.Fatalf("open audit sqlite: %v", err)
	}
	if err := sqlite.InitAuditTables(auditDB); err != nil {
		t.Fatalf("init audit tables: %v", err)
	}

	ts := time.Now().UTC().Truncate(time.Microsecond)
	prev1 := ""
	hash1 := computeIntegrityHash("log-1", prev1, ts, "mask", "input1", "output1", "admin", "L3", "{}")
	if _, err := auditDB.Exec(`
		INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource,
			input_hash, output_hash, algorithm, parameters_json, input_rows, output_rows, duration_ms,
			user_name, status, error_message, security_level, prev_hash, integrity_hash)
		VALUES ('log-1', 'task-1', 'api1_yibao', 'ds_yibao', ?, 'mask', 'ds_yibao',
			'input1', 'output1', 'mask', '{}', 1, 1, 100,
			'admin', 'success', '', 'L3', ?, ?)
	`, ts, prev1, hash1); err != nil {
		t.Fatalf("insert audit log 1: %v", err)
	}

	ts2 := ts.Add(time.Second)
	hash2 := computeIntegrityHash("log-2", hash1, ts2, "mask", "input2", "output2", "admin", "L3", "{}")
	if _, err := auditDB.Exec(`
		INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource,
			input_hash, output_hash, algorithm, parameters_json, input_rows, output_rows, duration_ms,
			user_name, status, error_message, security_level, prev_hash, integrity_hash)
		VALUES ('log-2', 'task-1', 'api1_yibao', 'ds_yibao', ?, 'mask', 'ds_yibao',
			'input2', 'output2', 'mask', '{}', 1, 1, 100,
			'admin', 'success', '', 'L3', ?, ?)
	`, ts2, hash1, hash2); err != nil {
		t.Fatalf("insert audit log 2: %v", err)
	}

	if _, err := auditDB.Exec(`
		INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash)
		VALUES ('snap-1', 'log-1', ?, 'in', 'out', 'mask', '{}', ?, ?)
	`, ts, hash1, prev1); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	_ = auditDB.Close()

	// Run migration.
	cfg := runConfig{
		hubDBPath:   hubDBPath,
		auditDBPath: auditDBPath,
		pgDSN:       dsn,
		batchSize:   100,
		dryRun:      false,
		verify:      false,
	}
	if err := run(ctx, logger, cfg); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	// Verify migrated counts.
	var taskCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM tasks").Scan(&taskCount); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 1 {
		t.Errorf("expected 1 task, got %d", taskCount)
	}

	var logCount, snapCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs").Scan(&logCount); err != nil {
		t.Fatalf("count audit_logs: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM snapshots").Scan(&snapCount); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if logCount != 2 {
		t.Errorf("expected 2 audit logs, got %d", logCount)
	}
	if snapCount != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapCount)
	}

	// Verify hash chain integrity.
	res, err := auditStore.VerifyChain(0)
	if err != nil {
		t.Fatalf("verify chain: %v", err)
	}
	if !res.Valid {
		t.Fatalf("hash chain invalid: %s (broken_at_id=%s)", res.Message, res.BrokenAtID)
	}
	if res.TotalVerified != 2 {
		t.Errorf("expected 2 verified logs, got %d", res.TotalVerified)
	}
}
