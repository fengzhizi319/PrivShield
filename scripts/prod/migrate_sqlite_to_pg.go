// Package main implements SQLite to PostgreSQL migration.
//
// scripts/prod/migrate_sqlite_to_pg.go
//
// PrivShield SQLite → PostgreSQL 生产平滑迁移工具
//
// 功能：
//  1. 从 SQLite WAL 数据库（service-hub.db / audit-log.db）流式抽取存量数据
//  2. 逐条核验 9 要素审计哈希链连续性（防篡改链完整性校验）
//  3. 按批次原子写入 PostgreSQL（pgx.Batch，单批 500 条）
//  4. 迁移后立即在 PostgreSQL 端执行全量哈希链验真
//  5. 输出迁移统计报告
//
// 用法：
//
//	go run scripts/prod/migrate_sqlite_to_pg.go \
//	  --service-hub-db /var/lib/privshield/service-hub.db \
//	  --audit-log-db   /var/lib/privshield/audit-log.db \
//	  --hub-pg-dsn     "postgres://hub_user:hub_pass@pg-prod:5432/privshield_hub?sslmode=verify-full" \
//	  --audit-pg-dsn   "postgres://audit_user:audit_pass@pg-prod:5432/privshield_audit?sslmode=verify-full" \
//	  [--batch-size 500] [--dry-run]
//
// 环境变量（替代命令行参数）：
//
//	SERVICE_HUB_DB_PATH  → --service-hub-db
//	AUDIT_LOG_DB_PATH    → --audit-log-db
//	SERVICE_HUB_PG_DSN   → --hub-pg-dsn
//	AUDIT_LOG_PG_DSN     → --audit-pg-dsn
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver / 纯 Go SQLite 驱动
)

// ─────────────────────────────────────────────────────────────
// 配置 / Configuration
// ─────────────────────────────────────────────────────────────

type config struct {
	serviceHubDB string
	auditLogDB   string
	hubPgDSN     string
	auditPgDSN   string
	batchSize    int
	dryRun       bool
}

func loadConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.serviceHubDB, "service-hub-db", envOr("SERVICE_HUB_DB_PATH", ""), "Path to service-hub SQLite database")
	flag.StringVar(&cfg.auditLogDB, "audit-log-db", envOr("AUDIT_LOG_DB_PATH", ""), "Path to audit-log SQLite database")
	flag.StringVar(&cfg.hubPgDSN, "hub-pg-dsn", envOr("SERVICE_HUB_PG_DSN", ""), "PostgreSQL DSN for service-hub")
	flag.StringVar(&cfg.auditPgDSN, "audit-pg-dsn", envOr("AUDIT_LOG_PG_DSN", ""), "PostgreSQL DSN for audit-log")
	flag.IntVar(&cfg.batchSize, "batch-size", 500, "Batch size for PostgreSQL writes")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "Dry-run mode: validate only, do not write to PostgreSQL")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ─────────────────────────────────────────────────────────────
// 数据模型 / Data models (mirror pkg/store/store.go)
// ─────────────────────────────────────────────────────────────

type taskRow struct {
	ID, Status, Stage, Source, APICode, DatasourceID, Operation string
	Priority                                                    int
	CreatedAt                                                   string
	StartedAt, CompletedAt                                      sql.NullString
	DurationMs                                                  int64
	Error, PayloadJSON                                          string
	RetryCount                                                  int
	RetryAfter                                                  sql.NullString
	TraceID                                                     string
	LeaseOwner, LeaseToken                                      string
	LeaseExpiresAt                                              sql.NullString
	Version, MaxRetries                                         int
}

type auditLogRow struct {
	ID, TaskID, APICode, DatasourceID                          string
	Timestamp                                                  string
	Operation, DataSource                                      string
	InputHash, OutputHash, Algorithm                           string
	ParametersJSON                                             string
	InputRows, OutputRows                                      int
	DurationMs                                                 int64
	UserName, Status, ErrorMessage, SecurityLevel              string
	PrevHash, IntegrityHash                                    string
}

type snapshotRow struct {
	ID, AuditLogID                          string
	Timestamp                               string
	InputSample, OutputSample, Algorithm    string
	ParametersJSON, IntegrityHash, PrevHash string
}

// ─────────────────────────────────────────────────────────────
// 主流程 / Main
// ─────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("=== PrivShield SQLite → PostgreSQL Migration Tool ===")
	log.Printf("Batch size: %d | Dry-run: %v", cfg.batchSize, cfg.dryRun)

	if cfg.dryRun {
		log.Println("[DRY-RUN] Validation-only mode; no PostgreSQL writes will occur")
	}

	// ── Step 0: Validate inputs ──
	if cfg.serviceHubDB == "" && cfg.auditLogDB == "" {
		log.Fatal("ERROR: At least one of --service-hub-db or --audit-log-db must be specified")
	}
	if !cfg.dryRun {
		if cfg.serviceHubDB != "" && cfg.hubPgDSN == "" {
			log.Fatal("ERROR: --hub-pg-dsn is required when migrating service-hub data")
		}
		if cfg.auditLogDB != "" && cfg.auditPgDSN == "" {
			log.Fatal("ERROR: --audit-pg-dsn is required when migrating audit-log data")
		}
	}

	// ── Step 1: Migrate service-hub tasks ──
	if cfg.serviceHubDB != "" {
		if err := migrateTasks(cfg); err != nil {
			log.Fatalf("ERROR: Task migration failed: %v", err)
		}
	}

	// ── Step 2: Migrate audit-log entries + snapshots ──
	if cfg.auditLogDB != "" {
		if err := migrateAuditLogs(cfg); err != nil {
			log.Fatalf("ERROR: Audit-log migration failed: %v", err)
		}
	}

	log.Println("=== Migration completed successfully ===")
}

// ─────────────────────────────────────────────────────────────
// Task migration / 任务迁移
// ─────────────────────────────────────────────────────────────

func migrateTasks(cfg config) error {
	log.Printf("[service-hub] Opening SQLite: %s", cfg.serviceHubDB)

	srcDB, err := sql.Open("sqlite", cfg.serviceHubDB)
	if err != nil {
		return fmt.Errorf("open source SQLite: %w", err)
	}
	defer srcDB.Close()

	// Validate SQLite integrity
	var integrityResult string
	if err := srcDB.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if integrityResult != "ok" {
		return fmt.Errorf("SQLite corruption detected: %s", integrityResult)
	}
	log.Println("[service-hub] SQLite integrity check: OK")

	// Extract all tasks ordered by created_at (preserves insertion order)
	rows, err := srcDB.Query(`
		SELECT id, status, stage, source, api_code, datasource_id, operation, priority,
		       created_at, started_at, completed_at, duration_ms, error, payload_json,
		       retry_count, retry_after, trace_id,
		       lease_owner, lease_token, lease_expires_at, version, max_retries
		FROM tasks ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []taskRow
	for rows.Next() {
		var t taskRow
		if err := rows.Scan(
			&t.ID, &t.Status, &t.Stage, &t.Source, &t.APICode, &t.DatasourceID,
			&t.Operation, &t.Priority, &t.CreatedAt, &t.StartedAt, &t.CompletedAt,
			&t.DurationMs, &t.Error, &t.PayloadJSON, &t.RetryCount, &t.RetryAfter,
			&t.TraceID, &t.LeaseOwner, &t.LeaseToken, &t.LeaseExpiresAt,
			&t.Version, &t.MaxRetries,
		); err != nil {
			return fmt.Errorf("scan task row: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task rows: %w", err)
	}
	log.Printf("[service-hub] Extracted %d tasks from SQLite", len(tasks))

	if cfg.dryRun {
		log.Println("[DRY-RUN] Skipping PostgreSQL write for tasks")
		return nil
	}

	// Write to PostgreSQL in batches
	return writeTasksToPG(cfg, tasks)
}

func writeTasksToPG(cfg config, tasks []taskRow) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.hubPgDSN)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	log.Println("[service-hub] PostgreSQL connection established")

	totalInserted := 0
	for i := 0; i < len(tasks); i += cfg.batchSize {
		end := i + cfg.batchSize
		if end > len(tasks) {
			end = len(tasks)
		}
		batch := tasks[i:end]

		b := &pgx.Batch{}
		for _, t := range batch {
			b.Queue(`
				INSERT INTO tasks (id, status, stage, source, api_code, datasource_id, operation, priority,
				                   created_at, started_at, completed_at, duration_ms, error, payload_json,
				                   retry_count, retry_after, trace_id,
				                   lease_owner, lease_token, lease_expires_at, version, max_retries)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
				ON CONFLICT (id) DO UPDATE SET
					status=EXCLUDED.status, stage=EXCLUDED.stage, error=EXCLUDED.error,
					duration_ms=EXCLUDED.duration_ms, retry_count=EXCLUDED.retry_count,
					trace_id=EXCLUDED.trace_id, version=EXCLUDED.version
			`, t.ID, t.Status, t.Stage, t.Source, t.APICode, t.DatasourceID,
				t.Operation, t.Priority, t.CreatedAt, nullStr(t.StartedAt), nullStr(t.CompletedAt),
				t.DurationMs, t.Error, t.PayloadJSON, t.RetryCount, nullStr(t.RetryAfter),
				t.TraceID, t.LeaseOwner, t.LeaseToken, nullStr(t.LeaseExpiresAt),
				t.Version, t.MaxRetries)
		}

		results := pool.SendBatch(ctx, b)
		for range batch {
			_, err := results.Exec()
			if err != nil {
				results.Close()
				return fmt.Errorf("batch insert task: %w", err)
			}
		}
		results.Close()
		totalInserted += len(batch)
		log.Printf("[service-hub] Batch written: %d tasks (%d/%d total)", len(batch), totalInserted, len(tasks))
	}

	log.Printf("[service-hub] Migration complete: %d tasks upserted", totalInserted)
	return nil
}

// ─────────────────────────────────────────────────────────────
// Audit-log migration / 审计日志迁移
// ─────────────────────────────────────────────────────────────

func migrateAuditLogs(cfg config) error {
	log.Printf("[audit-log] Opening SQLite: %s", cfg.auditLogDB)

	srcDB, err := sql.Open("sqlite", cfg.auditLogDB)
	if err != nil {
		return fmt.Errorf("open source SQLite: %w", err)
	}
	defer srcDB.Close()

	// Validate SQLite integrity
	var integrityResult string
	if err := srcDB.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if integrityResult != "ok" {
		return fmt.Errorf("SQLite corruption detected: %s", integrityResult)
	}
	log.Println("[audit-log] SQLite integrity check: OK")

	// ── Step A: Extract audit_logs ordered by rowid (hash chain order) ──
	logRows, err := srcDB.Query(`
		SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource,
		       input_hash, output_hash, algorithm, parameters_json,
		       input_rows, output_rows, duration_ms, user_name, status, error_message,
		       security_level, prev_hash, integrity_hash
		FROM audit_logs ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query audit_logs: %w", err)
	}
	defer logRows.Close()

	var logs []auditLogRow
	for logRows.Next() {
		var a auditLogRow
		if err := logRows.Scan(
			&a.ID, &a.TaskID, &a.APICode, &a.DatasourceID, &a.Timestamp,
			&a.Operation, &a.DataSource, &a.InputHash, &a.OutputHash, &a.Algorithm,
			&a.ParametersJSON, &a.InputRows, &a.OutputRows, &a.DurationMs,
			&a.UserName, &a.Status, &a.ErrorMessage, &a.SecurityLevel,
			&a.PrevHash, &a.IntegrityHash,
		); err != nil {
			return fmt.Errorf("scan audit_log row: %w", err)
		}
		logs = append(logs, a)
	}
	if err := logRows.Err(); err != nil {
		return fmt.Errorf("iterate audit_log rows: %w", err)
	}
	log.Printf("[audit-log] Extracted %d audit log entries from SQLite", len(logs))

	// ── Step B: Validate hash chain BEFORE migration ──
	if err := validateHashChain(logs, "pre-migration"); err != nil {
		return fmt.Errorf("pre-migration hash chain validation FAILED: %w", err)
	}
	log.Println("[audit-log] Pre-migration hash chain validation: PASSED")

	// ── Step C: Extract snapshots ──
	snapRows, err := srcDB.Query(`
		SELECT id, audit_log_id, timestamp, input_sample, output_sample,
		       algorithm, parameters_json, integrity_hash, prev_hash
		FROM snapshots ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query snapshots: %w", err)
	}
	defer snapRows.Close()

	var snaps []snapshotRow
	for snapRows.Next() {
		var s snapshotRow
		if err := snapRows.Scan(
			&s.ID, &s.AuditLogID, &s.Timestamp, &s.InputSample, &s.OutputSample,
			&s.Algorithm, &s.ParametersJSON, &s.IntegrityHash, &s.PrevHash,
		); err != nil {
			return fmt.Errorf("scan snapshot row: %w", err)
		}
		snaps = append(snaps, s)
	}
	if err := snapRows.Err(); err != nil {
		return fmt.Errorf("iterate snapshot rows: %w", err)
	}
	log.Printf("[audit-log] Extracted %d snapshot records from SQLite", len(snaps))

	if cfg.dryRun {
		log.Println("[DRY-RUN] Skipping PostgreSQL write for audit-logs and snapshots")
		return nil
	}

	// ── Step D: Write to PostgreSQL ──
	return writeAuditToPG(cfg, logs, snaps)
}

func writeAuditToPG(cfg config, logs []auditLogRow, snaps []snapshotRow) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.auditPgDSN)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	log.Println("[audit-log] PostgreSQL connection established")

	// Write audit_logs in batches
	totalLogs := 0
	for i := 0; i < len(logs); i += cfg.batchSize {
		end := i + cfg.batchSize
		if end > len(logs) {
			end = len(logs)
		}
		batch := logs[i:end]

		b := &pgx.Batch{}
		for _, a := range batch {
			b.Queue(`
				INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation,
				                        datasource, input_hash, output_hash, algorithm, parameters_json,
				                        input_rows, output_rows, duration_ms, user_name, status,
				                        error_message, security_level, prev_hash, integrity_hash)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
				ON CONFLICT (id) DO NOTHING
			`, a.ID, a.TaskID, a.APICode, a.DatasourceID, a.Timestamp, a.Operation,
				a.DataSource, a.InputHash, a.OutputHash, a.Algorithm, a.ParametersJSON,
				a.InputRows, a.OutputRows, a.DurationMs, a.UserName, a.Status,
				a.ErrorMessage, a.SecurityLevel, a.PrevHash, a.IntegrityHash)
		}

		results := pool.SendBatch(ctx, b)
		for range batch {
			if _, err := results.Exec(); err != nil {
				results.Close()
				return fmt.Errorf("batch insert audit_log: %w", err)
			}
		}
		results.Close()
		totalLogs += len(batch)
		log.Printf("[audit-log] Batch written: %d audit logs (%d/%d total)", len(batch), totalLogs, len(logs))
	}

	// Write snapshots in batches
	totalSnaps := 0
	for i := 0; i < len(snaps); i += cfg.batchSize {
		end := i + cfg.batchSize
		if end > len(snaps) {
			end = len(snaps)
		}
		batch := snaps[i:end]

		b := &pgx.Batch{}
		for _, s := range batch {
			b.Queue(`
				INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample,
				                       algorithm, parameters_json, integrity_hash, prev_hash)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
				ON CONFLICT (id) DO NOTHING
			`, s.ID, s.AuditLogID, s.Timestamp, s.InputSample, s.OutputSample,
				s.Algorithm, s.ParametersJSON, s.IntegrityHash, s.PrevHash)
		}

		results := pool.SendBatch(ctx, b)
		for range batch {
			if _, err := results.Exec(); err != nil {
				results.Close()
				return fmt.Errorf("batch insert snapshot: %w", err)
			}
		}
		results.Close()
		totalSnaps += len(batch)
	}

	log.Printf("[audit-log] Migration complete: %d audit logs, %d snapshots", totalLogs, totalSnaps)

	// ── Step E: Post-migration hash chain verification ──
	log.Println("[audit-log] Running post-migration hash chain verification on PostgreSQL...")
	if err := verifyChainOnPG(ctx, pool); err != nil {
		return fmt.Errorf("post-migration hash chain verification FAILED: %w", err)
	}
	log.Println("[audit-log] Post-migration hash chain verification: PASSED")

	return nil
}

// ─────────────────────────────────────────────────────────────
// Hash chain validation / 哈希链验真
// ─────────────────────────────────────────────────────────────

// validateHashChain verifies that each audit log's prev_hash matches
// the previous entry's integrity_hash, ensuring chain continuity.
func validateHashChain(logs []auditLogRow, label string) error {
	if len(logs) == 0 {
		log.Printf("[chain-verify:%s] No audit logs to verify", label)
		return nil
	}

	// First entry should have empty prev_hash (genesis)
	if logs[0].PrevHash != "" {
		return fmt.Errorf("genesis entry has non-empty prev_hash: %q", logs[0].PrevHash)
	}

	for i := 1; i < len(logs); i++ {
		prev := logs[i-1]
		curr := logs[i]

		if curr.PrevHash != prev.IntegrityHash {
			return fmt.Errorf(
				"chain broken at index %d (id=%s): expected prev_hash=%s, got %s",
				i, curr.ID, prev.IntegrityHash, curr.PrevHash,
			)
		}
	}

	log.Printf("[chain-verify:%s] Verified %d entries, chain is continuous", label, len(logs))
	return nil
}

// verifyChainOnPG reads audit_logs from PostgreSQL in rowid/created order
// and re-validates the hash chain.
func verifyChainOnPG(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT id, prev_hash, integrity_hash
		FROM audit_logs
		ORDER BY timestamp ASC, id ASC
	`)
	if err != nil {
		return fmt.Errorf("query audit_logs from PG: %w", err)
	}
	defer rows.Close()

	var prevIntegrityHash string
	count := 0
	for rows.Next() {
		var id, prevHash, integrityHash string
		if err := rows.Scan(&id, &prevHash, &integrityHash); err != nil {
			return fmt.Errorf("scan PG audit_log: %w", err)
		}

		if count == 0 {
			// Genesis entry
			if prevHash != "" {
				return fmt.Errorf("PG genesis entry (id=%s) has non-empty prev_hash: %q", id, prevHash)
			}
		} else {
			if prevHash != prevIntegrityHash {
				return fmt.Errorf(
					"PG chain broken at id=%s: expected prev_hash=%s, got %s",
					id, prevIntegrityHash, prevHash,
				)
			}
		}
		prevIntegrityHash = integrityHash
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PG audit_logs: %w", err)
	}

	log.Printf("[chain-verify:post-migration-pg] Verified %d entries on PostgreSQL, chain is continuous", count)
	return nil
}

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

// nullStr converts a sql.NullString to a nullable any value for PostgreSQL.
func nullStr(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

