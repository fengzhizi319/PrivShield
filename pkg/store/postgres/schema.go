package postgres

import "context"

// initSchema creates the tasks table and indexes if they don't exist.
// initSchema 在不存在时创建 tasks 表及索引。
func (s *Store) initSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id              TEXT PRIMARY KEY,
			status          TEXT NOT NULL DEFAULT 'pending',
			stage           TEXT NOT NULL DEFAULT 'queued',
			source          TEXT,
			api_code        TEXT DEFAULT '',
			datasource_id   TEXT DEFAULT '',
			operation       TEXT,
			priority        INTEGER DEFAULT 0,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at      TIMESTAMPTZ,
			completed_at    TIMESTAMPTZ,
			duration_ms     BIGINT DEFAULT 0,
			error           TEXT,
			payload_json    TEXT,
			retry_count     INTEGER DEFAULT 0,
			retry_after     TIMESTAMPTZ,
			lease_owner     TEXT DEFAULT '',
			lease_token     TEXT DEFAULT '',
			lease_expires_at TIMESTAMPTZ,
			version         INTEGER DEFAULT 0,
			max_retries     INTEGER DEFAULT 3
		);

		-- Migration for existing databases
		ALTER TABLE tasks ADD COLUMN IF NOT EXISTS api_code TEXT DEFAULT '';
		ALTER TABLE tasks ADD COLUMN IF NOT EXISTS datasource_id TEXT DEFAULT '';

		-- Backfill canonical identifiers
		UPDATE tasks SET datasource_id = source
		WHERE (datasource_id IS NULL OR datasource_id = '') AND SUBSTRING(source, 1, 3) = 'ds_';
		UPDATE tasks SET api_code = 'api1_yibao'
		WHERE datasource_id = 'ds_yibao' AND (api_code IS NULL OR api_code = '');
		UPDATE tasks SET api_code = 'api2_kangyang'
		WHERE datasource_id = 'ds_kangyang' AND (api_code IS NULL OR api_code = '');

		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status);
		CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks (created_at);
		CREATE INDEX IF NOT EXISTS idx_tasks_retry_after ON tasks (retry_after);
		CREATE INDEX IF NOT EXISTS idx_tasks_lease_expires ON tasks (lease_expires_at);
		CREATE INDEX IF NOT EXISTS idx_tasks_datasource_id ON tasks (datasource_id);

		-- Partial index for ClaimNext: only pending tasks, ordered by priority DESC, created_at ASC.
		-- 部分索引用于 ClaimNext：仅 pending 任务，按优先级降序、创建时间升序。
		CREATE INDEX IF NOT EXISTS idx_tasks_claim
			ON tasks (priority DESC, created_at ASC)
			WHERE status = 'pending';
	`)
	return err
}
