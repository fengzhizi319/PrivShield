package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// DataSourceStore implements store.DataSourceStore backed by SQLite.
type DataSourceStore struct {
	db *sql.DB
}

// NewDataSourceStore creates a new SQLite-backed data source store.
func NewDataSourceStore(db *sql.DB) (*DataSourceStore, error) {
	if err := InitDataSourceTables(db); err != nil {
		return nil, fmt.Errorf("init datasource tables: %w", err)
	}
	return &DataSourceStore{db: db}, nil
}

func (s *DataSourceStore) SaveDS(ds *store.DataSource) error {
	tagsJSON, _ := json.Marshal(ds.Tags)
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO datasources (id, name, type, host, port, database_name, security_level, status, created_at, tags_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ds.ID, ds.Name, ds.Type, ds.Host, ds.Port, ds.Database, ds.SecurityLevel,
		ds.Status, ds.CreatedAt.Format(time.RFC3339Nano), string(tagsJSON))
	return err
}

func (s *DataSourceStore) GetDS(id string) (*store.DataSource, error) {
	row := s.db.QueryRow(`
		SELECT id, name, type, host, port, database_name, security_level, status, created_at, last_check_at, tags_json
		FROM datasources WHERE id = ?
	`, id)
	return scanDataSource(row)
}

func (s *DataSourceStore) ListDS(filter store.DataSourceFilter) ([]store.DataSource, int, error) {
	// Count total
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM datasources").Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch with SQL-level pagination / P28 fix: 推分页到 SQL 层
	query := "SELECT id, name, type, host, port, database_name, security_level, status, created_at, last_check_at, tags_json FROM datasources ORDER BY created_at DESC"
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

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]store.DataSource, 0)
	for rows.Next() {
		ds, err := scanDataSourceRow(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *ds)
	}
	return result, total, rows.Err()
}

func (s *DataSourceStore) DeleteDS(id string) error {
	res, err := s.db.Exec("DELETE FROM datasources WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("data source %s not found", id)
	}
	return nil
}

func (s *DataSourceStore) UpdateDS(ds *store.DataSource) error {
	tagsJSON, _ := json.Marshal(ds.Tags)
	_, err := s.db.Exec(`
		UPDATE datasources SET name=?, type=?, host=?, port=?, database_name=?, security_level=?,
		status=?, last_check_at=?, tags_json=? WHERE id=?
	`, ds.Name, ds.Type, ds.Host, ds.Port, ds.Database, ds.SecurityLevel,
		ds.Status, nullTime(ds.LastCheckAt), string(tagsJSON), ds.ID)
	return err
}

func (s *DataSourceStore) SaveAudit(rec *store.AccessAuditRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO access_audit (id, datasource_id, datasource_name, operation, user_name, timestamp, records_count, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.DataSourceID, rec.DataSourceName, rec.Operation, rec.User,
		rec.Timestamp.Format(time.RFC3339Nano), rec.RecordsCount, rec.Status)
	return err
}

func (s *DataSourceStore) ListAudit(dsID string, limit, offset int) ([]store.AccessAuditRecord, int, error) {
	where := ""
	var args []any
	if dsID != "" {
		where = " WHERE datasource_id = ?"
		args = append(args, dsID)
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM access_audit" + where
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch with SQL-level pagination / P28 fix: 推分页到 SQL 层
	query := "SELECT id, datasource_id, datasource_name, operation, user_name, timestamp, records_count, status FROM access_audit" + where + " ORDER BY timestamp DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	records := make([]store.AccessAuditRecord, 0)
	for rows.Next() {
		var r store.AccessAuditRecord
		var ts string
		if err := rows.Scan(&r.ID, &r.DataSourceID, &r.DataSourceName, &r.Operation, &r.User,
			&ts, &r.RecordsCount, &r.Status); err != nil {
			return nil, 0, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		records = append(records, r)
	}
	return records, total, rows.Err()
}

// scanDataSourceFields scans common DataSource fields from any scanner interface.
// P58 fix: extract common scanning logic to eliminate duplication between
// scanDataSource (*sql.Row) and scanDataSourceRow (*sql.Rows).
func scanDataSourceFields(scan func(dest ...any) error) (*store.DataSource, error) {
	var ds store.DataSource
	var createdAt string
	var lastCheckAt sql.NullString
	var tagsJSON sql.NullString

	err := scan(&ds.ID, &ds.Name, &ds.Type, &ds.Host, &ds.Port, &ds.Database,
		&ds.SecurityLevel, &ds.Status, &createdAt, &lastCheckAt, &tagsJSON)
	if err != nil {
		return nil, err
	}

	ds.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if lastCheckAt.Valid {
		if ts, err := time.Parse(time.RFC3339Nano, lastCheckAt.String); err == nil {
			ds.LastCheckAt = &ts
		}
	}
	if tagsJSON.Valid {
		_ = json.Unmarshal([]byte(tagsJSON.String), &ds.Tags)
	}
	return &ds, nil
}

// scanDataSource scans a DataSource from a QueryRow.
func scanDataSource(row *sql.Row) (*store.DataSource, error) {
	return scanDataSourceFields(row.Scan)
}

// scanDataSourceRow scans a DataSource from a Rows iterator.
func scanDataSourceRow(rows *sql.Rows) (*store.DataSource, error) {
	return scanDataSourceFields(rows.Scan)
}
