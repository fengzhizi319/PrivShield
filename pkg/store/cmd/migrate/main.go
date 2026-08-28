// Command migrate copies PrivShield Phase A (SQLite) data into Phase B (PostgreSQL).
//
// Usage:
//
//	go run ./pkg/store/cmd/migrate \
//	  -hub-db ./data/service-hub.db \
//	  -audit-db ./data/audit-log.db \
//	  -pg-dsn "postgres://user:pass@localhost:5432/privshield?sslmode=disable" \
//	  -batch 500 \
//	  -dry-run
//
// The tool is idempotent: rows are inserted with ON CONFLICT DO NOTHING, so
// re-running against a partially migrated database is safe.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"

	pkcrypto "github.com/fengzhizi319/PrivShield/pkg/crypto"
	"github.com/fengzhizi319/PrivShield/pkg/store/postgres"
)

var (
	hubDBPath          = flag.String("hub-db", "./data/service-hub.db", "Path to service-hub SQLite database")
	auditDBPath        = flag.String("audit-db", "./data/audit-log.db", "Path to audit-log SQLite database")
	pgDSN              = flag.String("pg-dsn", os.Getenv("PRIVSHIELD_MIGRATE_PG_DSN"), "Target PostgreSQL DSN (also env PRIVSHIELD_MIGRATE_PG_DSN)")
	batchSize          = flag.Int("batch", 500, "Batch insert size")
	dryRun             = flag.Bool("dry-run", false, "Print counts without writing to PostgreSQL")
	verify             = flag.Bool("verify", false, "Run hash-chain verification after migration")
	snapshotVerifyMode = flag.String("snapshot-verify-mode", func() string {
		if v := os.Getenv("PRIVSHIELD_MIGRATE_SNAPSHOT_VERIFY"); v != "" {
			return v
		}
		return "skip"
	}(), "Snapshot ciphertext verification mode: skip, after-migrate, only (also env PRIVSHIELD_MIGRATE_SNAPSHOT_VERIFY)")
	auditEncryptionKey = flag.String("audit-encryption-key", os.Getenv("AUDIT_LOG_ENCRYPTION_KEY"), "Audit log snapshot encryption key (fallback env PRIVACY_AUDIT_KEY)")
)

func main() {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *pgDSN == "" {
		fmt.Fprintln(os.Stderr, "Error: -pg-dsn is required (or set PRIVSHIELD_MIGRATE_PG_DSN)")
		flag.Usage()
		os.Exit(1)
	}

	auditKey := *auditEncryptionKey
	if auditKey == "" {
		auditKey = os.Getenv("PRIVACY_AUDIT_KEY")
	}

	if err := run(context.Background(), logger, runConfig{
		hubDBPath:          *hubDBPath,
		auditDBPath:        *auditDBPath,
		pgDSN:              *pgDSN,
		batchSize:          *batchSize,
		dryRun:             *dryRun,
		verify:             *verify,
		snapshotVerifyMode: *snapshotVerifyMode,
		auditEncryptionKey: auditKey,
	}); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

type runConfig struct {
	hubDBPath          string
	auditDBPath        string
	pgDSN              string
	batchSize          int
	dryRun             bool
	verify             bool
	snapshotVerifyMode string
	auditEncryptionKey string
}

func run(ctx context.Context, logger *slog.Logger, cfg runConfig) error {
	var pgPool *pgxpool.Pool
	if !cfg.dryRun {
		pool, err := pgxpool.New(ctx, cfg.pgDSN)
		if err != nil {
			return fmt.Errorf("open postgres pool: %w", err)
		}
		defer pool.Close()
		pgPool = pool
	} else {
		logger.Info("dry-run mode: no writes will be performed")
	}

	switch cfg.snapshotVerifyMode {
	case "only":
		if cfg.dryRun {
			logger.Info("snapshot-verify-only mode with dry-run: nothing to do")
			return nil
		}
		if err := verifySnapshots(ctx, logger, pgPool, cfg.auditEncryptionKey); err != nil {
			return fmt.Errorf("verify snapshots: %w", err)
		}
		logger.Info("snapshot verification completed")
		return nil
	case "skip", "after-migrate":
		// proceed with migration
	default:
		return fmt.Errorf("invalid snapshot-verify-mode %q (expected skip, after-migrate, or only)", cfg.snapshotVerifyMode)
	}

	if err := migrateTasks(ctx, logger, cfg.hubDBPath, pgPool, cfg.batchSize, cfg.dryRun); err != nil {
		return fmt.Errorf("migrate tasks: %w", err)
	}

	if err := migrateAudit(ctx, logger, cfg.auditDBPath, pgPool, cfg.dryRun); err != nil {
		return fmt.Errorf("migrate audit: %w", err)
	}

	if cfg.verify && !cfg.dryRun {
		if err := verifyChain(ctx, logger, cfg.pgDSN); err != nil {
			return fmt.Errorf("verify chain: %w", err)
		}
	}

	if cfg.snapshotVerifyMode == "after-migrate" && !cfg.dryRun {
		if err := verifySnapshots(ctx, logger, pgPool, cfg.auditEncryptionKey); err != nil {
			return fmt.Errorf("verify snapshots: %w", err)
		}
	}

	logger.Info("migration completed")
	return nil
}

func migrateTasks(ctx context.Context, logger *slog.Logger, sqlitePath string, pgPool *pgxpool.Pool, batchSize int, dryRun bool) error {
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT id, status, stage, source, api_code, datasource_id, operation, priority, created_at,
		       started_at, completed_at, duration_ms, error, payload_json, retry_count, retry_after,
		       lease_owner, lease_token, lease_expires_at, version, max_retries
		FROM tasks
		ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var total, migrated int
	batch := make([][]any, 0, batchSize)

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if dryRun {
			migrated += len(batch)
			batch = batch[:0]
			return nil
		}

		pgBatch := &pgx.Batch{}
		for _, row := range batch {
			pgBatch.Queue(`
				INSERT INTO tasks (id, status, stage, source, api_code, datasource_id, operation, priority, created_at,
					started_at, completed_at, duration_ms, "error", payload_json, retry_count, retry_after,
					lease_owner, lease_token, lease_expires_at, version, max_retries)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
				ON CONFLICT (id) DO NOTHING
			`, row...)
		}

		br := pgPool.SendBatch(ctx, pgBatch)
		for i := 0; i < len(batch); i++ {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return fmt.Errorf("batch exec: %w", err)
			}
		}
		if err := br.Close(); err != nil {
			return fmt.Errorf("batch close: %w", err)
		}
		migrated += len(batch)
		batch = batch[:0]
		return nil
	}

	for rows.Next() {
		total++
		var id, status, stage, source, apiCode, datasourceID, operation string
		var priority int
		var createdAt time.Time
		var startedAt, completedAt, retryAfter, leaseExpiresAt sql.NullTime
		var durationMs int64
		var errStr, payloadJSON, leaseOwner, leaseToken string
		var retryCount, version, maxRetries int

		if err := rows.Scan(&id, &status, &stage, &source, &apiCode, &datasourceID, &operation, &priority, &createdAt,
			&startedAt, &completedAt, &durationMs, &errStr, &payloadJSON, &retryCount, &retryAfter,
			&leaseOwner, &leaseToken, &leaseExpiresAt, &version, &maxRetries); err != nil {
			return fmt.Errorf("scan task: %w", err)
		}

		row := []any{
			id, status, stage, source, apiCode, datasourceID, operation, priority, createdAt,
			nullTimePtr(startedAt), nullTimePtr(completedAt), durationMs, errStr, payloadJSON, retryCount, nullTimePtr(retryAfter),
			leaseOwner, leaseToken, nullTimePtr(leaseExpiresAt), version, maxRetries,
		}
		batch = append(batch, row)

		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate tasks: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}

	logger.Info("tasks migrated", "source", sqlitePath, "total", total, "migrated", migrated)
	return nil
}

func migrateAudit(ctx context.Context, logger *slog.Logger, sqlitePath string, pgPool *pgxpool.Pool, dryRun bool) error {
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite %s: %w", sqlitePath, err)
	}
	defer db.Close()

	logRows, err := db.QueryContext(ctx, `
		SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash,
		       algorithm, parameters_json, input_rows, output_rows, duration_ms, user_name, status,
		       error_message, security_level, prev_hash, integrity_hash
		FROM audit_logs
		ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query audit_logs: %w", err)
	}
	defer logRows.Close()

	type auditKey struct {
		id string
		ts time.Time
	}
	// Preserve original insertion order to keep the hash-chain sequence intact.
	logOrder := []auditKey{}
	logByID := make(map[string][]any)

	for logRows.Next() {
		var id, taskID, apiCode, datasourceID, operation, datasource, inputHash, outputHash, algorithm string
		var parametersJSON, userName, statusStr, errorMessage, securityLevel, prevHash, integrityHash string
		var timestamp time.Time
		var inputRows, outputRows int
		var durationMs int64

		if err := logRows.Scan(&id, &taskID, &apiCode, &datasourceID, &timestamp, &operation, &datasource,
			&inputHash, &outputHash, &algorithm, &parametersJSON, &inputRows, &outputRows, &durationMs,
			&userName, &statusStr, &errorMessage, &securityLevel, &prevHash, &integrityHash); err != nil {
			return fmt.Errorf("scan audit_log: %w", err)
		}

		logByID[id] = []any{
			id, taskID, apiCode, datasourceID, timestamp, operation, datasource,
			inputHash, outputHash, algorithm, parametersJSON, inputRows, outputRows, durationMs,
			userName, statusStr, errorMessage, securityLevel, prevHash, integrityHash,
		}
		logOrder = append(logOrder, auditKey{id: id, ts: timestamp})
	}
	if err := logRows.Err(); err != nil {
		return fmt.Errorf("iterate audit_logs: %w", err)
	}

	// Load snapshots keyed by audit_log_id.
	snapRows, err := db.QueryContext(ctx, `
		SELECT id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash
		FROM snapshots
		ORDER BY rowid ASC
	`)
	if err != nil {
		return fmt.Errorf("query snapshots: %w", err)
	}
	defer snapRows.Close()

	snapsByLog := make(map[string][][]any)
	for snapRows.Next() {
		var id, auditLogID, inputSample, outputSample, algorithm, parametersJSON, integrityHash, prevHash string
		var timestamp time.Time
		if err := snapRows.Scan(&id, &auditLogID, &timestamp, &inputSample, &outputSample, &algorithm,
			&parametersJSON, &integrityHash, &prevHash); err != nil {
			return fmt.Errorf("scan snapshot: %w", err)
		}
		snapsByLog[auditLogID] = append(snapsByLog[auditLogID], []any{
			id, auditLogID, timestamp, inputSample, outputSample, algorithm, parametersJSON, integrityHash, prevHash,
		})
	}
	if err := snapRows.Err(); err != nil {
		return fmt.Errorf("iterate snapshots: %w", err)
	}

	if dryRun {
		logger.Info("audit dry-run", "logs", len(logOrder), "snapshots", countSnapshots(snapsByLog))
		return nil
	}

	// Insert logs in original rowid order, followed by their snapshots, in a single transaction.
	tx, err := pgPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer tx.Rollback(ctx)

	logStmt := `
		INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource,
			input_hash, output_hash, algorithm, parameters_json, input_rows, output_rows, duration_ms,
			user_name, status, error_message, security_level, prev_hash, integrity_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (id) DO NOTHING
	`
	snapStmt := `
		INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm,
			parameters_json, integrity_hash, prev_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`

	for _, key := range logOrder {
		logArgs := logByID[key.id]
		if _, err := tx.Exec(ctx, logStmt, logArgs...); err != nil {
			return fmt.Errorf("insert audit_log %s: %w", key.id, err)
		}
		for _, snapArgs := range snapsByLog[key.id] {
			if _, err := tx.Exec(ctx, snapStmt, snapArgs...); err != nil {
				return fmt.Errorf("insert snapshot for %s: %w", key.id, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit tx: %w", err)
	}

	logger.Info("audit migrated", "source", sqlitePath, "logs", len(logOrder), "snapshots", countSnapshots(snapsByLog))
	return nil
}

func verifyChain(ctx context.Context, logger *slog.Logger, pgDSN string) error {
	cfg := postgres.Config{DSN: pgDSN}
	store, err := postgres.NewAuditStore(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("create audit store: %w", err)
	}
	defer store.Close()

	res, err := store.VerifyChain(0)
	if err != nil {
		return fmt.Errorf("verify chain: %w", err)
	}
	if !res.Valid {
		return fmt.Errorf("hash chain invalid: %s (broken_at_id=%s)", res.Message, res.BrokenAtID)
	}
	logger.Info("hash chain verified", "total_verified", res.TotalVerified, "message", res.Message)
	return nil
}

func verifySnapshots(ctx context.Context, logger *slog.Logger, pgPool *pgxpool.Pool, key string) error {
	if key == "" {
		return fmt.Errorf("audit encryption key is required for snapshot verification (set AUDIT_LOG_ENCRYPTION_KEY or PRIVACY_AUDIT_KEY)")
	}

	rows, err := pgPool.Query(ctx, `SELECT id, input_sample, output_sample FROM snapshots ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var total, encrypted, plaintext, failed int
	var failedIDs []string
	var firstErr error

	for rows.Next() {
		var id, inputSample, outputSample string
		if err := rows.Scan(&id, &inputSample, &outputSample); err != nil {
			return fmt.Errorf("scan snapshot: %w", err)
		}
		total++

		check := func(value string) {
			if value == "" {
				return
			}
			if strings.HasPrefix(value, pkcrypto.EncryptedPrefix) {
				encrypted++
				if _, err := pkcrypto.DecryptString(value, key); err != nil {
					failed++
					if len(failedIDs) < 5 {
						failedIDs = append(failedIDs, id)
					}
					if firstErr == nil {
						firstErr = err
					}
				}
			} else {
				plaintext++
			}
		}

		check(inputSample)
		check(outputSample)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate snapshots: %w", err)
	}

	logger.Info("snapshot verification",
		"total_snapshots", total,
		"encrypted_samples", encrypted,
		"plaintext_samples", plaintext,
		"failed_samples", failed,
	)

	if failed > 0 {
		return fmt.Errorf("snapshot ciphertext verification failed for %d encrypted sample(s) (first ids: %v): %w", failed, failedIDs, firstErr)
	}
	return nil
}

func nullTimePtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		t := nt.Time
		return &t
	}
	return nil
}

func countSnapshots(m map[string][][]any) int {
	total := 0
	for _, v := range m {
		total += len(v)
	}
	return total
}
