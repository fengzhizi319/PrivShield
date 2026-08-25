package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// ── Basic TaskStore implementation / 基础 TaskStore 实现 ──

// Save inserts or replaces a task.
func (s *Store) Save(task *store.Task) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tasks (id, status, stage, source, operation, priority, created_at, payload_json, retry_count, retry_after, max_retries)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			status=EXCLUDED.status, stage=EXCLUDED.stage, source=EXCLUDED.source,
			operation=EXCLUDED.operation, priority=EXCLUDED.priority,
			payload_json=EXCLUDED.payload_json, retry_count=EXCLUDED.retry_count,
			retry_after=EXCLUDED.retry_after, max_retries=EXCLUDED.max_retries
	`, task.ID, task.Status, task.Stage, task.Source, task.Operation, task.Priority,
		task.CreatedAt, task.PayloadJSON, task.RetryCount, task.RetryAfter, task.MaxRetries)
	return err
}

// Get retrieves a task by ID.
func (s *Store) Get(id string) (*store.Task, error) {
	ctx := context.Background()
	row := s.pool.QueryRow(ctx, `
		SELECT id, status, stage, source, operation, priority, created_at, started_at,
			completed_at, duration_ms, error, retry_count, retry_after,
			lease_owner, lease_token, lease_expires_at, version, max_retries
		FROM tasks WHERE id = $1
	`, id)
	return scanTask(row)
}

// List returns tasks matching the filter with total count.
func (s *Store) List(filter store.TaskFilter) ([]store.Task, int, error) {
	ctx := context.Background()

	// Count total / 统计总数
	countQuery := "SELECT COUNT(*) FROM tasks"
	var total int
	var args []any
	if filter.Status != "" {
		countQuery += " WHERE status = $1"
		args = append(args, filter.Status)
	}
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("postgres: count tasks: %w", err)
	}

	// Fetch rows / 查询行
	query := `SELECT id, status, stage, source, operation, priority, created_at, started_at,
		completed_at, duration_ms, error, retry_count, retry_after,
		lease_owner, lease_token, lease_expires_at, version, max_retries
		FROM tasks`
	var listArgs []any
	if filter.Status != "" {
		query += " WHERE status = $1"
		listArgs = append(listArgs, filter.Status)
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

	rows, err := s.pool.Query(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list tasks: %w", err)
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

// Update modifies an existing task's mutable fields.
func (s *Store) Update(task *store.Task) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		UPDATE tasks SET
			status=$1, stage=$2, started_at=$3, completed_at=$4,
			duration_ms=$5, error=$6, retry_count=$7, retry_after=$8,
			lease_owner=$9, lease_token=$10, lease_expires_at=$11, version=$12
		WHERE id=$13
	`, task.Status, task.Stage, task.StartedAt, task.CompletedAt,
		task.DurationMs, task.Error, task.RetryCount, task.RetryAfter,
		task.LeaseOwner, task.LeaseToken, task.LeaseExpiresAt, task.Version,
		task.ID)
	return err
}

// Counts returns aggregated task counts by status.
func (s *Store) Counts() (store.TaskCounts, error) {
	ctx := context.Background()
	var c store.TaskCounts
	rows, err := s.pool.Query(ctx, "SELECT status, COUNT(*) FROM tasks GROUP BY status")
	if err != nil {
		return c, fmt.Errorf("postgres: count by status: %w", err)
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

// CleanupOld deletes terminal tasks older than the cutoff time.
func (s *Store) CleanupOld(before time.Time) (int64, error) {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM tasks WHERE status IN ('completed', 'failed') AND created_at < $1", before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Scanning helpers / 行扫描辅助函数 ──

// rowScanner is satisfied by both pgxpool.Row and pgxpool.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (*store.Task, error) {
	var t store.Task
	if err := row.Scan(
		&t.ID, &t.Status, &t.Stage, &t.Source, &t.Operation, &t.Priority,
		&t.CreatedAt, &t.StartedAt, &t.CompletedAt, &t.DurationMs, &t.Error,
		&t.RetryCount, &t.RetryAfter,
		&t.LeaseOwner, &t.LeaseToken, &t.LeaseExpiresAt, &t.Version, &t.MaxRetries,
	); err != nil {
		return nil, fmt.Errorf("postgres: scan task: %w", err)
	}
	return &t, nil
}

func scanTaskRow(rows interface {
	Scan(dest ...any) error
}) (*store.Task, error) {
	return scanTask(rows)
}

// Compile-time interface assertion / 编译时接口断言
var _ store.LeasedTaskStore = (*Store)(nil)
