// Package sqlite provides SQLite-backed implementations of the store interfaces.
// Package sqlite 提供基于 SQLite 的存储接口实现。
//
// 使用 modernc.org/sqlite（纯 Go SQLite 实现），无 CGO 依赖，
// 容器构建友好（无需 gcc / libsqlite3-dev）。
package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite" // SQLite driver / SQLite 驱动
)

// Open opens a SQLite database at the given path and initializes tables.
// Open 打开指定路径的 SQLite 数据库并初始化表结构。
//
// path 为空字符串时返回 nil（调用方应回退到内存实现）。
func Open(path string, logger *slog.Logger) (*sql.DB, error) {
	if path == "" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// WAL mode for better concurrent read performance
	// WAL 模式提升并发读性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Reasonable busy timeout to handle brief lock contention
	// 合理的忙等待超时，处理短暂锁竞争
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	// P24 fix: configure connection pool for SQLite
	// SQLite only supports one writer at a time; limit connections to prevent excessive lock contention
	// SQLite 同一时间仅支持一个写入者；限制连接数防止过度锁竞争
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * 60 * time.Second)

	// P26 fix: set synchronous=NORMAL for better write performance with WAL (still crash-safe)
	// 设置 synchronous=NORMAL 提升 WAL 模式下的写入性能（仍然崩溃安全）
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set synchronous: %w", err)
	}

	// Enable foreign key enforcement
	// 启用外键约束强制执行
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}

	logger.Info("sqlite database opened", "path", path)
	return db, nil
}

// ValidateIntegrity performs a SQLite integrity check on the database file at dbPath.
// ValidateIntegrity 对指定路径的 SQLite 数据库文件执行完整性校验。
//
// 返回 nil 表示校验通过或 dbPath 为空（内存模式无需校验）。
// 返回 error 表示数据库损坏或校验失败。
//
// 突然断电可能导致 SQLite 数据库文件损坏，此函数在服务启动早期检测损坏，防止带病运行。
func ValidateIntegrity(dbPath string) error {
	if dbPath == "" {
		return nil // 内存模式无需校验
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open database for integrity check: %w", err)
	}
	defer db.Close()

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity check query failed: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("database corruption detected: %s", result)
	}

	return nil
}

// InitTaskTables creates the tasks table if it doesn't exist.
// InitTaskTables 在不存在时创建 tasks 表。
func InitTaskTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
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
			payload_json TEXT,
			retry_count INTEGER DEFAULT 0,
			retry_after DATETIME,
			lease_owner TEXT DEFAULT '',
			lease_token TEXT DEFAULT '',
			lease_expires_at DATETIME,
			version INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 3
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);
	`)
	if err != nil {
		return err
	}

	// Backward compatibility: add columns if they don't exist (for existing databases)
	// 向后兼容：如果列不存在则添加（针对已有数据库）
	cursor, err := db.Query("PRAGMA table_info(tasks)")
	if err != nil {
		return err
	}
	defer cursor.Close()

	columns := make(map[string]bool)
	for cursor.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := cursor.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		columns[name] = true
	}

	if !columns["retry_count"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN retry_count INTEGER DEFAULT 0"); err != nil {
			return err
		}
	}
	if !columns["retry_after"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN retry_after DATETIME"); err != nil {
			return err
		}
	}

	// ── Phase B: Lease columns for multi-replica Hub / 多副本 Hub 租约列 ──
	if !columns["lease_owner"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN lease_owner TEXT DEFAULT ''"); err != nil {
			return err
		}
	}
	if !columns["lease_token"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN lease_token TEXT DEFAULT ''"); err != nil {
			return err
		}
	}
	if !columns["lease_expires_at"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN lease_expires_at DATETIME"); err != nil {
			return err
		}
	}
	if !columns["version"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN version INTEGER DEFAULT 0"); err != nil {
			return err
		}
	}
	if !columns["max_retries"] {
		if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN max_retries INTEGER DEFAULT 3"); err != nil {
			return err
		}
	}

	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_retry_after ON tasks(retry_after)"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_lease_expires ON tasks(lease_expires_at)"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_tasks_claim ON tasks(status, priority DESC, created_at) WHERE status='pending'"); err != nil {
		return err
	}

	return nil
}

// InitDataSourceTables creates the datasources and access_audit tables.
// InitDataSourceTables 在不存在时创建 datasources 和 access_audit 表。
func InitDataSourceTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS datasources (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT,
			host TEXT,
			port INTEGER,
			database_name TEXT,
			security_level TEXT,
			status TEXT NOT NULL DEFAULT 'disconnected',
			created_at DATETIME NOT NULL,
			last_check_at DATETIME,
			tags_json TEXT
		);
		CREATE TABLE IF NOT EXISTS access_audit (
			id TEXT PRIMARY KEY,
			datasource_id TEXT,
			datasource_name TEXT,
			operation TEXT,
			user_name TEXT,
			timestamp DATETIME NOT NULL,
			records_count INTEGER DEFAULT 0,
			status TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_access_audit_ds ON access_audit(datasource_id);
	`)
	return err
}

// InitAuditTables creates the audit_logs and snapshots tables.
// InitAuditTables 在不存在时创建 audit_logs 和 snapshots 表。
func InitAuditTables(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
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
		CREATE TABLE IF NOT EXISTS snapshots (
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
		CREATE INDEX IF NOT EXISTS idx_audit_logs_ts ON audit_logs(timestamp);
		CREATE INDEX IF NOT EXISTS idx_audit_logs_op ON audit_logs(operation);
		CREATE INDEX IF NOT EXISTS idx_snapshots_audit ON snapshots(audit_log_id);
	`)
	return err
}
