package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// TaskStore implements store.TaskStore backed by SQLite.
// TaskStore 实现基于 SQLite 的 store.TaskStore。
type TaskStore struct {
	db *sql.DB
}

// NewTaskStore creates a new SQLite-backed task store.
func NewTaskStore(db *sql.DB) (*TaskStore, error) {
	if err := InitTaskTables(db); err != nil {
		return nil, fmt.Errorf("init task tables: %w", err)
	}
	return &TaskStore{db: db}, nil
}

func (s *TaskStore) Save(task *store.Task) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tasks (id, status, stage, source, operation, priority, created_at, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Status, task.Stage, task.Source, task.Operation, task.Priority,
		task.CreatedAt.Format(time.RFC3339Nano), task.PayloadJSON)
	return err
}

func (s *TaskStore) Get(id string) (*store.Task, error) {
	row := s.db.QueryRow(`
		SELECT id, status, stage, source, operation, priority, created_at, started_at, completed_at, duration_ms, error
		FROM tasks WHERE id = ?
	`, id)
	return scanTask(row)
}

func (s *TaskStore) List(filter store.TaskFilter) ([]store.Task, int, error) {
	// Count total
	countQuery := "SELECT COUNT(*) FROM tasks"
	var total int
	var args []any
	if filter.Status != "" {
		countQuery += " WHERE status = ?"
		args = append(args, filter.Status)
	}
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch rows
	query := "SELECT id, status, stage, source, operation, priority, created_at, started_at, completed_at, duration_ms, error FROM tasks"
	if filter.Status != "" {
		query += " WHERE status = ?"
	}
	query += " ORDER BY created_at DESC"
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

	var listArgs []any
	if filter.Status != "" {
		listArgs = append(listArgs, filter.Status)
	}

	rows, err := s.db.Query(query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]store.Task, 0)
	for rows.Next() {
		t, err := scanTaskRow(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, *t)
	}
	return tasks, total, rows.Err()
}

func (s *TaskStore) Update(task *store.Task) error {
	_, err := s.db.Exec(`
		UPDATE tasks SET status=?, stage=?, started_at=?, completed_at=?, duration_ms=?, error=?
		WHERE id=?
	`, task.Status, task.Stage, nullTime(task.StartedAt), nullTime(task.CompletedAt),
		task.DurationMs, task.Error, task.ID)
	return err
}

func (s *TaskStore) Counts() (store.TaskCounts, error) {
	var c store.TaskCounts
	rows, err := s.db.Query("SELECT status, COUNT(*) FROM tasks GROUP BY status")
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return c, err
		}
		switch status {
		case "pending":
			c.Pending = count
		case "running":
			c.Running = count
		case "completed":
			c.Completed = count
		case "failed":
			c.Failed = count
		}
	}
	return c, rows.Err()
}

// scanTaskFields scans common task fields from any scanner interface.
// P45 fix: extract common scanning logic to eliminate duplication between
// scanTask (*sql.Row) and scanTaskRow (*sql.Rows).
func scanTaskFields(scan func(dest ...any) error) (*store.Task, error) {
	var t store.Task
	var createdAt string
	var startedAt, completedAt, errMsg sql.NullString

	err := scan(&t.ID, &t.Status, &t.Stage, &t.Source, &t.Operation, &t.Priority,
		&createdAt, &startedAt, &completedAt, &t.DurationMs, &errMsg)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if startedAt.Valid {
		if ts, err := time.Parse(time.RFC3339Nano, startedAt.String); err == nil {
			t.StartedAt = &ts
		}
	}
	if completedAt.Valid {
		if ts, err := time.Parse(time.RFC3339Nano, completedAt.String); err == nil {
			t.CompletedAt = &ts
		}
	}
	t.Error = errMsg.String
	return &t, nil
}

// scanTask scans a single task from a QueryRow.
func scanTask(row *sql.Row) (*store.Task, error) {
	return scanTaskFields(row.Scan)
}

// scanTaskRow scans a single task from a Rows iterator.
func scanTaskRow(rows *sql.Rows) (*store.Task, error) {
	return scanTaskFields(rows.Scan)
}

// nullTime converts a *time.Time to sql.NullString for storage.
func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: t.Format(time.RFC3339Nano), Valid: true}
}
