package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// AuditStore implements store.AuditStore backed by PostgreSQL.
type AuditStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewAuditStore creates a new PostgreSQL-backed audit store with connection pooling.
func NewAuditStore(ctx context.Context, cfg Config, logger *slog.Logger) (*AuditStore, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("postgres: DSN must not be empty")
	}
	if logger == nil {
		logger = slog.Default()
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres parse DSN: %w", err)
	}

	if cfg.MaxConn > 0 {
		poolCfg.MaxConns = cfg.MaxConn
	} else {
		poolCfg.MaxConns = 10
	}
	if cfg.MinConn > 0 {
		poolCfg.MinConns = cfg.MinConn
	} else {
		poolCfg.MinConns = 2
	}

	poolCfg.HealthCheckPeriod = 30 * time.Second
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	s := &AuditStore{pool: pool, logger: logger}
	if err := s.initAuditSchema(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres init audit schema: %w", err)
	}

	logger.Info("postgresql audit store initialized", "max_conns", poolCfg.MaxConns, "min_conns", poolCfg.MinConns)
	return s, nil
}

// NewAuditStoreWithPool creates an AuditStore using an existing pool.
func NewAuditStoreWithPool(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (*AuditStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres pool must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &AuditStore{pool: pool, logger: logger}
	if err := s.initAuditSchema(ctx); err != nil {
		return nil, fmt.Errorf("postgres init audit schema: %w", err)
	}
	return s, nil
}

func (s *AuditStore) initAuditSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS audit_logs (
			id              TEXT PRIMARY KEY,
			task_id         TEXT DEFAULT '',
			api_code        TEXT DEFAULT '',
			datasource_id   TEXT DEFAULT '',
			timestamp       TIMESTAMPTZ NOT NULL,
			operation       TEXT,
			datasource      TEXT,
			input_hash      TEXT,
			output_hash     TEXT,
			algorithm       TEXT,
			parameters_json TEXT,
			input_rows      INTEGER DEFAULT 0,
			output_rows     INTEGER DEFAULT 0,
			duration_ms     BIGINT DEFAULT 0,
			user_name       TEXT,
			status          TEXT,
			error_message   TEXT,
			security_level  TEXT,
			prev_hash       TEXT DEFAULT '',
			integrity_hash  TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS snapshots (
			id              TEXT PRIMARY KEY,
			audit_log_id    TEXT REFERENCES audit_logs(id) ON DELETE CASCADE,
			timestamp       TIMESTAMPTZ NOT NULL,
			input_sample    TEXT,
			output_sample   TEXT,
			algorithm       TEXT,
			parameters_json TEXT,
			integrity_hash  TEXT,
			prev_hash       TEXT DEFAULT ''
		);

		CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs (timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_operation ON audit_logs (operation);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_datasource_id ON audit_logs (datasource_id);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_task_id ON audit_logs (task_id);
		CREATE INDEX IF NOT EXISTS idx_snapshots_audit_log_id ON snapshots (audit_log_id);

		ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS task_id TEXT DEFAULT '';
		ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS api_code TEXT DEFAULT '';
		ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS datasource_id TEXT DEFAULT '';
		ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
		ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS integrity_hash TEXT DEFAULT '';

		ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS prev_hash TEXT DEFAULT '';
		ALTER TABLE snapshots ADD COLUMN IF NOT EXISTS integrity_hash TEXT DEFAULT '';
	`)
	return err
}

func (s *AuditStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *AuditStore) SaveLog(log *store.AuditLog) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if log.IntegrityHash == "" {
		log.IntegrityHash = computePGAuditIntegrityHash(log.ID, log.PrevHash, log.Timestamp, log.Algorithm, log.InputHash, log.OutputHash, log.User, log.SecurityLevel, log.ParametersJSON)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash,
			algorithm, parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`, log.ID, log.TaskID, log.APICode, log.DatasourceID, log.Timestamp, log.Operation, log.DataSource,
		log.InputHash, log.OutputHash, log.Algorithm, log.ParametersJSON,
		log.InputRows, log.OutputRows, log.DurationMs, log.User, log.Status, log.ErrorMessage, log.SecurityLevel,
		log.PrevHash, log.IntegrityHash)
	return err
}

func (s *AuditStore) SaveLogWithSnapshot(log *store.AuditLog, snapshot *store.SnapshotRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if log.IntegrityHash == "" {
		log.IntegrityHash = computePGAuditIntegrityHash(log.ID, log.PrevHash, log.Timestamp, log.Algorithm, log.InputHash, log.OutputHash, log.User, log.SecurityLevel, log.ParametersJSON)
	}
	if snapshot.IntegrityHash == "" {
		snapshot.IntegrityHash = log.IntegrityHash
	}
	if snapshot.PrevHash == "" {
		snapshot.PrevHash = log.PrevHash
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash,
			algorithm, parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`, log.ID, log.TaskID, log.APICode, log.DatasourceID, log.Timestamp, log.Operation, log.DataSource,
		log.InputHash, log.OutputHash, log.Algorithm, log.ParametersJSON,
		log.InputRows, log.OutputRows, log.DurationMs, log.User, log.Status, log.ErrorMessage, log.SecurityLevel,
		log.PrevHash, log.IntegrityHash); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, snapshot.ID, snapshot.AuditLogID, snapshot.Timestamp,
		snapshot.InputSample, snapshot.OutputSample, snapshot.Algorithm, snapshot.ParametersJSON, snapshot.IntegrityHash, snapshot.PrevHash); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *AuditStore) SaveLogsBatch(logs []store.AuditLog, snapshots []store.SnapshotRecord) error {
	if len(logs) == 0 && len(snapshots) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, log := range logs {
		if log.IntegrityHash == "" {
			log.IntegrityHash = computePGAuditIntegrityHash(log.ID, log.PrevHash, log.Timestamp, log.Algorithm, log.InputHash, log.OutputHash, log.User, log.SecurityLevel, log.ParametersJSON)
		}
		batch.Queue(`
			INSERT INTO audit_logs (id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash,
				algorithm, parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		`, log.ID, log.TaskID, log.APICode, log.DatasourceID, log.Timestamp, log.Operation, log.DataSource,
			log.InputHash, log.OutputHash, log.Algorithm, log.ParametersJSON,
			log.InputRows, log.OutputRows, log.DurationMs, log.User, log.Status, log.ErrorMessage, log.SecurityLevel,
			log.PrevHash, log.IntegrityHash)
	}

	for _, snap := range snapshots {
		batch.Queue(`
			INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, snap.ID, snap.AuditLogID, snap.Timestamp,
			snap.InputSample, snap.OutputSample, snap.Algorithm, snap.ParametersJSON, snap.IntegrityHash, snap.PrevHash)
	}

	br := tx.SendBatch(ctx, batch)
	totalQueued := len(logs) + len(snapshots)
	for i := 0; i < totalQueued; i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return err
		}
	}
	if err := br.Close(); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *AuditStore) GetLog(id string) (*store.AuditLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := s.pool.QueryRow(ctx, `
		SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
			parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash
		FROM audit_logs WHERE id = $1
	`, id)
	return scanPGAuditRow(row)
}

func (s *AuditStore) GetLatestLog() (*store.AuditLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := s.pool.QueryRow(ctx, `
		SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
			parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash
		FROM audit_logs ORDER BY timestamp DESC LIMIT 1
	`)
	log, err := scanPGAuditRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return log, nil
}

func (s *AuditStore) ListLogs(filter store.AuditFilter) ([]store.AuditLog, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	where, args := buildPGAuditWhere(filter)

	countQuery := "SELECT COUNT(*) FROM audit_logs" + where
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
		parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash
		FROM audit_logs` + where + " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		limit := filter.Limit
		if limit > 10000 {
			limit = 10000
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}
		args = append(args, limit, offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]store.AuditLog, 0)
	for rows.Next() {
		l, err := scanPGAuditRow(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, *l)
	}
	return logs, total, rows.Err()
}

func (s *AuditStore) GetStats() (*store.AuditStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats := &store.AuditStats{
		ByOperation:     make(map[string]int),
		ByStatus:        make(map[string]int),
		BySecurityLevel: make(map[string]int),
	}

	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*), COALESCE(AVG(duration_ms), 0) FROM audit_logs").Scan(&stats.TotalOperations, &stats.AvgDurationMs); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, "SELECT operation, COUNT(*) FROM audit_logs GROUP BY operation")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var op string
		var count int
		if err := rows.Scan(&op, &count); err == nil {
			stats.ByOperation[op] = count
		}
	}
	rows.Close()

	rows2, err := s.pool.Query(ctx, "SELECT status, COUNT(*) FROM audit_logs GROUP BY status")
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var status string
		var count int
		if err := rows2.Scan(&status, &count); err == nil {
			stats.ByStatus[status] = count
		}
	}
	rows2.Close()

	rows3, err := s.pool.Query(ctx, "SELECT security_level, COUNT(*) FROM audit_logs WHERE security_level != '' GROUP BY security_level")
	if err != nil {
		return nil, err
	}
	for rows3.Next() {
		var level string
		var count int
		if err := rows3.Scan(&level, &count); err == nil {
			stats.BySecurityLevel[level] = count
		}
	}
	rows3.Close()

	return stats, nil
}

func (s *AuditStore) GenerateReport(period string) (*store.AuditReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var periodInterval string
	switch period {
	case "1h":
		periodInterval = "1 hour"
	case "7d":
		periodInterval = "7 days"
	case "30d":
		periodInterval = "30 days"
	default:
		periodInterval = "24 hours"
	}

	report := &store.AuditReport{
		BySecurityLevel: make(map[string]int),
	}

	query := fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0)
		FROM audit_logs WHERE timestamp > NOW() - INTERVAL '%s'`, periodInterval)
	var totalCount, successCount int
	if err := s.pool.QueryRow(ctx, query).Scan(&totalCount, &successCount); err != nil {
		return nil, err
	}
	report.TotalOperations = totalCount
	if totalCount > 0 {
		report.SuccessRate = float64(successCount) / float64(totalCount) * 100
	}

	query2 := fmt.Sprintf(`SELECT security_level, COUNT(*) FROM audit_logs WHERE timestamp > NOW() - INTERVAL '%s' AND security_level != '' GROUP BY security_level`, periodInterval)
	rows, err := s.pool.Query(ctx, query2)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err == nil {
			report.BySecurityLevel[level] = count
		}
	}
	rows.Close()

	query3 := fmt.Sprintf(`SELECT operation, COUNT(*) as cnt FROM audit_logs WHERE timestamp > NOW() - INTERVAL '%s' GROUP BY operation ORDER BY cnt DESC LIMIT 5`, periodInterval)
	rows3, err := s.pool.Query(ctx, query3)
	if err != nil {
		return nil, err
	}
	topOps := make([]string, 0, 5)
	for rows3.Next() {
		var op string
		var count int
		if err := rows3.Scan(&op, &count); err == nil {
			topOps = append(topOps, fmt.Sprintf("%s (%d)", op, count))
		}
	}
	rows3.Close()
	report.TopOperations = topOps

	report.Recommendations = generatePGRecommendations(report.BySecurityLevel, report.SuccessRate)
	return report, nil
}

func generatePGRecommendations(byLevel map[string]int, successRate float64) []string {
	recs := make([]string, 0)
	if l4 := byLevel["L4"]; l4 > 100 {
		recs = append(recs, "L4 级别操作频繁，建议审查差分隐私预算消耗")
	}
	if l5 := byLevel["L5"]; l5 > 50 {
		recs = append(recs, "L5 绝密数据操作较多，建议加强访问控制审计")
	}
	if successRate < 95 {
		recs = append(recs, fmt.Sprintf("成功率 %.1f%% 低于 95%%，建议排查失败原因", successRate))
	}
	if len(recs) == 0 {
		recs = append(recs, "审计指标正常，无需特别关注")
	}
	return recs
}

func (s *AuditStore) SaveSnapshot(snap *store.SnapshotRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, snap.ID, snap.AuditLogID, snap.Timestamp,
		snap.InputSample, snap.OutputSample, snap.Algorithm, snap.ParametersJSON, snap.IntegrityHash, snap.PrevHash)
	return err
}

func (s *AuditStore) ListSnapshots(limit, offset int) ([]store.SnapshotRecord, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM snapshots").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash FROM snapshots ORDER BY timestamp DESC"
	args := []any{}
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
		if offset > 0 {
			args = append(args, offset)
			query += fmt.Sprintf(" OFFSET $%d", len(args))
		}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	snaps := make([]store.SnapshotRecord, 0)
	for rows.Next() {
		snap, err := scanPGSnapshotRow(rows)
		if err != nil {
			return nil, 0, err
		}
		snaps = append(snaps, *snap)
	}
	return snaps, total, rows.Err()
}

func (s *AuditStore) GetSnapshot(id string) (*store.SnapshotRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := s.pool.QueryRow(ctx, `
		SELECT id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash, prev_hash
		FROM snapshots WHERE id = $1
	`, id)
	return scanPGSnapshotRow(row)
}

func (s *AuditStore) VerifyChain(limit int) (*store.ChainVerificationResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 || limit > 10000 {
		limit = 1000
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, task_id, api_code, datasource_id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
			parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level, prev_hash, integrity_hash
		FROM audit_logs ORDER BY timestamp ASC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var previousHash string
	count := 0

	for rows.Next() {
		log, err := scanPGAuditRow(rows)
		if err != nil {
			return nil, err
		}

		expectedHash := computePGAuditIntegrityHash(log.ID, log.PrevHash, log.Timestamp, log.Algorithm, log.InputHash, log.OutputHash, log.User, log.SecurityLevel, log.ParametersJSON)

		if log.IntegrityHash != "" && log.IntegrityHash != expectedHash {
			return &store.ChainVerificationResult{
				TotalVerified: count,
				Valid:         false,
				BrokenAtID:    log.ID,
				ExpectedHash:  expectedHash,
				ActualHash:    log.IntegrityHash,
				Message:       fmt.Sprintf("integrity hash mismatch at log %s: content modified", log.ID),
			}, nil
		}

		if count > 0 && log.PrevHash != previousHash {
			return &store.ChainVerificationResult{
				TotalVerified: count,
				Valid:         false,
				BrokenAtID:    log.ID,
				ExpectedHash:  previousHash,
				ActualHash:    log.PrevHash,
				Message:       fmt.Sprintf("hash chain broken at log %s: expected prev_hash %s, got %s", log.ID, previousHash, log.PrevHash),
			}, nil
		}

		previousHash = log.IntegrityHash
		if previousHash == "" {
			previousHash = expectedHash
		}
		count++
	}

	return &store.ChainVerificationResult{
		TotalVerified: count,
		Valid:         true,
		Message:       fmt.Sprintf("hash chain verified successfully (%d records checked)", count),
	}, rows.Err()
}

func (s *AuditStore) CleanupOld(before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tag, err := s.pool.Exec(ctx, `DELETE FROM audit_logs WHERE timestamp < $1`, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func buildPGAuditWhere(filter store.AuditFilter) (string, []any) {
	conditions := make([]string, 0)
	args := make([]any, 0)

	if filter.TaskID != "" {
		args = append(args, filter.TaskID)
		conditions = append(conditions, fmt.Sprintf("task_id = $%d", len(args)))
	}
	if filter.APICode != "" {
		args = append(args, filter.APICode)
		conditions = append(conditions, fmt.Sprintf("api_code = $%d", len(args)))
	}
	if filter.DatasourceID != "" {
		args = append(args, filter.DatasourceID)
		conditions = append(conditions, fmt.Sprintf("(datasource_id = $%d OR datasource = $%d)", len(args), len(args)))
	} else if filter.DataSource != "" {
		args = append(args, filter.DataSource)
		conditions = append(conditions, fmt.Sprintf("(datasource = $%d OR datasource_id = $%d)", len(args), len(args)))
	}
	if filter.Operation != "" {
		args = append(args, filter.Operation)
		conditions = append(conditions, fmt.Sprintf("operation = $%d", len(args)))
	}
	if filter.User != "" {
		args = append(args, filter.User)
		conditions = append(conditions, fmt.Sprintf("user_name = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.SecurityLevel != "" {
		args = append(args, filter.SecurityLevel)
		conditions = append(conditions, fmt.Sprintf("security_level = $%d", len(args)))
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

type pgRowScanner interface {
	Scan(dest ...any) error
}

func scanPGAuditRow(row pgRowScanner) (*store.AuditLog, error) {
	var l store.AuditLog
	var paramsJSON *string
	var taskID, apiCode, datasourceID, prevHash, integrityHash *string

	err := row.Scan(&l.ID, &taskID, &apiCode, &datasourceID, &l.Timestamp, &l.Operation, &l.DataSource, &l.InputHash, &l.OutputHash,
		&l.Algorithm, &paramsJSON, &l.InputRows, &l.OutputRows, &l.DurationMs,
		&l.User, &l.Status, &l.ErrorMessage, &l.SecurityLevel, &prevHash, &integrityHash)
	if err != nil {
		return nil, err
	}

	if taskID != nil {
		l.TaskID = *taskID
	}
	if apiCode != nil {
		l.APICode = *apiCode
	}
	if datasourceID != nil {
		l.DatasourceID = *datasourceID
	}
	if prevHash != nil {
		l.PrevHash = *prevHash
	}
	if integrityHash != nil {
		l.IntegrityHash = *integrityHash
	}
	if l.DatasourceID == "" && l.DataSource != "" {
		l.DatasourceID = l.DataSource
	}
	if l.DataSource == "" && l.DatasourceID != "" {
		l.DataSource = l.DatasourceID
	}

	if paramsJSON != nil {
		l.ParametersJSON = *paramsJSON
		_ = json.Unmarshal([]byte(*paramsJSON), &l.Parameters)
	}
	return &l, nil
}

func scanPGSnapshotRow(row pgRowScanner) (*store.SnapshotRecord, error) {
	var snap store.SnapshotRecord
	var paramsJSON, prevHash *string

	err := row.Scan(&snap.ID, &snap.AuditLogID, &snap.Timestamp, &snap.InputSample, &snap.OutputSample,
		&snap.Algorithm, &paramsJSON, &snap.IntegrityHash, &prevHash)
	if err != nil {
		return nil, err
	}

	if prevHash != nil {
		snap.PrevHash = *prevHash
	}
	if paramsJSON != nil {
		snap.ParametersJSON = *paramsJSON
		_ = json.Unmarshal([]byte(*paramsJSON), &snap.Parameters)
	}
	return &snap, nil
}

func computePGAuditIntegrityHash(logID, prevHash string, timestamp time.Time, algorithm, inputHash, outputHash, user, securityLevel, paramsJSON string) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%v",
		prevHash, logID, timestamp.Format(time.RFC3339Nano), algorithm,
		inputHash, outputHash, user, securityLevel, paramsJSON)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

var _ store.AuditStore = (*AuditStore)(nil)
