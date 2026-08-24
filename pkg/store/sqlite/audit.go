package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// AuditStore implements store.AuditStore backed by SQLite.
type AuditStore struct {
	db *sql.DB
}

// NewAuditStore creates a new SQLite-backed audit store.
func NewAuditStore(db *sql.DB) (*AuditStore, error) {
	if err := InitAuditTables(db); err != nil {
		return nil, fmt.Errorf("init audit tables: %w", err)
	}
	return &AuditStore{db: db}, nil
}

func (s *AuditStore) SaveLog(log *store.AuditLog) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (id, timestamp, operation, datasource, input_hash, output_hash,
			algorithm, parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.Timestamp.Format(time.RFC3339Nano), log.Operation, log.DataSource,
		log.InputHash, log.OutputHash, log.Algorithm, log.ParametersJSON,
		log.InputRows, log.OutputRows, log.DurationMs, log.User, log.Status, log.ErrorMessage, log.SecurityLevel)
	return err
}

func (s *AuditStore) GetLog(id string) (*store.AuditLog, error) {
	row := s.db.QueryRow(`
		SELECT id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
			parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level
		FROM audit_logs WHERE id = ?
	`, id)
	return scanAuditLog(row)
}

func (s *AuditStore) ListLogs(filter store.AuditFilter) ([]store.AuditLog, int, error) {
	where, args := buildAuditWhere(filter)

	// Count total
	countQuery := "SELECT COUNT(*) FROM audit_logs" + where
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch rows
	query := `SELECT id, timestamp, operation, datasource, input_hash, output_hash, algorithm,
		parameters_json, input_rows, output_rows, duration_ms, user_name, status, error_message, security_level
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
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]store.AuditLog, 0)
	for rows.Next() {
		l, err := scanAuditLogRow(rows)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, *l)
	}
	return logs, total, rows.Err()
}

func (s *AuditStore) GetStats() (*store.AuditStats, error) {
	stats := &store.AuditStats{
		ByOperation:     make(map[string]int),
		ByStatus:        make(map[string]int),
		BySecurityLevel: make(map[string]int),
	}

	// Total count and average duration
	if err := s.db.QueryRow("SELECT COUNT(*), COALESCE(AVG(duration_ms), 0) FROM audit_logs").Scan(&stats.TotalOperations, &stats.AvgDurationMs); err != nil {
		return nil, err
	}

	// Group by operation
	rows, err := s.db.Query("SELECT operation, COUNT(*) FROM audit_logs GROUP BY operation")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var op string
		var count int
		if err := rows.Scan(&op, &count); err != nil {
			return nil, err
		}
		stats.ByOperation[op] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Group by status
	rows2, err := s.db.Query("SELECT status, COUNT(*) FROM audit_logs GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var status string
		var count int
		if err := rows2.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats.ByStatus[status] = count
	}
	if err := rows2.Err(); err != nil {
		return nil, err
	}

	// Group by security_level
	rows3, err := s.db.Query("SELECT security_level, COUNT(*) FROM audit_logs WHERE security_level != '' GROUP BY security_level")
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	for rows3.Next() {
		var level string
		var count int
		if err := rows3.Scan(&level, &count); err != nil {
			return nil, err
		}
		stats.BySecurityLevel[level] = count
	}
	return stats, rows3.Err()
}

// GenerateReport generates a compliance audit report with SQL-level filtering and aggregation.
// P33 fix: use SQL WHERE for period filtering and SQL aggregation instead of loading 10k records.
func (s *AuditStore) GenerateReport(period string) (*store.AuditReport, error) {
	// Parse period to duration
	var periodDuration string
	switch period {
	case "1h":
		periodDuration = "1 hour"
	case "7d":
		periodDuration = "7 days"
	case "30d":
		periodDuration = "30 days"
	default:
		periodDuration = "24 hours"
	}

	report := &store.AuditReport{
		BySecurityLevel: make(map[string]int),
	}

	// Build WHERE clause for period filtering
	// P56 fix: use parameterized query instead of fmt.Sprintf for period value,
	// defense-in-hardening even though periodDuration comes from a switch whitelist.
	// SQLite datetime function: timestamp > datetime('now', '-24 hours')
	whereClause := "WHERE timestamp > datetime('now', ?)"
	periodParam := "-" + periodDuration

	// 1. Total count and success count in one query
	var totalCount, successCount int
	query := fmt.Sprintf("SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) FROM audit_logs %s", whereClause)
	if err := s.db.QueryRow(query, periodParam).Scan(&totalCount, &successCount); err != nil {
		return nil, err
	}
	report.TotalOperations = totalCount
	if totalCount > 0 {
		report.SuccessRate = float64(successCount) / float64(totalCount) * 100
	}

	// 2. Group by security_level
	query2 := fmt.Sprintf("SELECT security_level, COUNT(*) FROM audit_logs %s AND security_level != '' GROUP BY security_level", whereClause)
	rows, err := s.db.Query(query2, periodParam)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		report.BySecurityLevel[level] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. Get top operations (ORDER BY count DESC LIMIT 5)
	query3 := fmt.Sprintf("SELECT operation, COUNT(*) as cnt FROM audit_logs %s GROUP BY operation ORDER BY cnt DESC LIMIT 5", whereClause)
	rows3, err := s.db.Query(query3, periodParam)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()
	topOps := make([]string, 0, 5)
	for rows3.Next() {
		var op string
		var count int
		if err := rows3.Scan(&op, &count); err != nil {
			return nil, err
		}
		topOps = append(topOps, fmt.Sprintf("%s (%d)", op, count))
	}
	report.TopOperations = topOps
	if err := rows3.Err(); err != nil {
		return nil, err
	}

	// 4. Generate recommendations based on statistics
	report.Recommendations = generateRecommendations(report.BySecurityLevel, report.SuccessRate)

	return report, nil
}

// generateRecommendations generates audit recommendations based on statistics.
func generateRecommendations(byLevel map[string]int, successRate float64) []string {
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
	_, err := s.db.Exec(`
		INSERT INTO snapshots (id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, snap.ID, snap.AuditLogID, snap.Timestamp.Format(time.RFC3339Nano),
		snap.InputSample, snap.OutputSample, snap.Algorithm, snap.ParametersJSON, snap.IntegrityHash)
	return err
}

// ListSnapshots returns paginated snapshots with total count.
// P35 fix: return total count for proper pagination instead of len(snaps).
func (s *AuditStore) ListSnapshots(limit, offset int) ([]store.SnapshotRecord, int, error) {
	// Count total
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM snapshots").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash FROM snapshots ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	snaps := make([]store.SnapshotRecord, 0)
	for rows.Next() {
		snap, err := scanSnapshotRow(rows)
		if err != nil {
			return nil, 0, err
		}
		snaps = append(snaps, *snap)
	}
	return snaps, total, rows.Err()
}

func (s *AuditStore) GetSnapshot(id string) (*store.SnapshotRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, audit_log_id, timestamp, input_sample, output_sample, algorithm, parameters_json, integrity_hash
		FROM snapshots WHERE id = ?
	`, id)

	var snap store.SnapshotRecord
	var ts string
	var paramsJSON sql.NullString

	err := row.Scan(&snap.ID, &snap.AuditLogID, &ts, &snap.InputSample, &snap.OutputSample,
		&snap.Algorithm, &paramsJSON, &snap.IntegrityHash)
	if err != nil {
		return nil, err
	}

	snap.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
	if paramsJSON.Valid {
		_ = json.Unmarshal([]byte(paramsJSON.String), &snap.Parameters)
	}
	return &snap, nil
}

// buildAuditWhere builds the WHERE clause for audit log queries.
func buildAuditWhere(filter store.AuditFilter) (string, []any) {
	conditions := make([]string, 0)
	args := make([]any, 0)

	if filter.Operation != "" {
		conditions = append(conditions, "operation = ?")
		args = append(args, filter.Operation)
	}
	if filter.DataSource != "" {
		conditions = append(conditions, "datasource = ?")
		args = append(args, filter.DataSource)
	}
	if filter.User != "" {
		conditions = append(conditions, "user_name = ?")
		args = append(args, filter.User)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.SecurityLevel != "" {
		conditions = append(conditions, "security_level = ?")
		args = append(args, filter.SecurityLevel)
	}

	if len(conditions) == 0 {
		return "", nil
	}

	where := " WHERE "
	for i, c := range conditions {
		if i > 0 {
			where += " AND "
		}
		where += c
	}
	return where, args
}

// CleanupOld deletes audit logs and their associated snapshots older than the cutoff time.
// CleanupOld 删除早于截止时间的审计日志及其关联快照，防止 SQLite 无限膨胀。
func (s *AuditStore) CleanupOld(before time.Time) (int64, error) {
	cutoff := before.Format(time.RFC3339Nano)
	// Delete associated snapshots first (foreign key dependency)
	_, _ = s.db.Exec(`DELETE FROM snapshots WHERE audit_log_id IN (SELECT id FROM audit_logs WHERE timestamp < ?)`, cutoff)
	result, err := s.db.Exec(`DELETE FROM audit_logs WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// scanAuditFields scans common audit log fields from any scanner interface.
// P55 fix: extract common scanning logic to eliminate duplication between
// scanAuditLog (*sql.Row) and scanAuditLogRow (*sql.Rows).
func scanAuditFields(scan func(dest ...any) error) (*store.AuditLog, error) {
	var l store.AuditLog
	var ts string
	var paramsJSON sql.NullString

	err := scan(&l.ID, &ts, &l.Operation, &l.DataSource, &l.InputHash, &l.OutputHash,
		&l.Algorithm, &paramsJSON, &l.InputRows, &l.OutputRows, &l.DurationMs,
		&l.User, &l.Status, &l.ErrorMessage, &l.SecurityLevel)
	if err != nil {
		return nil, err
	}

	l.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
	l.ParametersJSON = paramsJSON.String
	if paramsJSON.Valid {
		_ = json.Unmarshal([]byte(paramsJSON.String), &l.Parameters)
	}
	return &l, nil
}

// scanAuditLog scans a single AuditLog from a QueryRow.
func scanAuditLog(row *sql.Row) (*store.AuditLog, error) {
	return scanAuditFields(row.Scan)
}

// scanAuditLogRow scans a single AuditLog from a Rows iterator.
func scanAuditLogRow(rows *sql.Rows) (*store.AuditLog, error) {
	return scanAuditFields(rows.Scan)
}

// scanSnapshotRow scans a SnapshotRecord from a Rows iterator.
func scanSnapshotRow(rows *sql.Rows) (*store.SnapshotRecord, error) {
	var snap store.SnapshotRecord
	var ts string
	var paramsJSON sql.NullString

	err := rows.Scan(&snap.ID, &snap.AuditLogID, &ts, &snap.InputSample, &snap.OutputSample,
		&snap.Algorithm, &paramsJSON, &snap.IntegrityHash)
	if err != nil {
		return nil, err
	}

	snap.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
	if paramsJSON.Valid {
		_ = json.Unmarshal([]byte(paramsJSON.String), &snap.Parameters)
	}
	return &snap, nil
}
