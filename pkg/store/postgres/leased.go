// Phase B: LeasedTaskStore — Atomic task ownership via PostgreSQL.
// Phase B：基于 PostgreSQL 的原子任务领取与租约管理。
//
// ClaimNext uses FOR UPDATE SKIP LOCKED to enable lock-free competitive claiming
// among multiple Hub replicas. Each lease carries a unique token so that stale
// owners cannot overwrite results after a lease has been reassigned.
//
// ClaimNext 使用 FOR UPDATE SKIP LOCKED 实现多 Hub 副本间的无阻塞竞争领取。
// 每个租约携带唯一令牌，防止过期所有者在租约已被重新分配后覆盖结果。
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// ClaimNext atomically claims the next pending task for the given owner.
// 使用 FOR UPDATE SKIP LOCKED 实现无阻塞竞争领取；无可用任务时返回 (nil, nil)。
//
// The operation is a single short transaction:
//  1. SELECT one pending row FOR UPDATE SKIP LOCKED (skip rows locked by other replicas)
//  2. UPDATE it to running + write lease metadata
//  3. RETURNING the full row
//
// 整个操作为单个短事务：
//  1. SELECT 一行 pending 任务 FOR UPDATE SKIP LOCKED（跳过被其他副本锁定的行）
//  2. UPDATE 为 running + 写入租约元数据
//  3. RETURNING 返回完整行
func (s *Store) ClaimNext(owner string, leaseTTL time.Duration) (*store.TaskLease, error) {
	ctx := context.Background()
	token := generateToken()

	row := s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM tasks
			WHERE status = 'pending'
			  AND (retry_after IS NULL OR retry_after <= NOW())
			  AND retry_count < max_retries
			ORDER BY priority DESC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE tasks
		SET status = 'running',
		    stage = 'running',
		    started_at = COALESCE(started_at, NOW()),
		    lease_owner = $1,
		    lease_token = $2,
		    lease_expires_at = NOW() + ($3::TEXT || ' seconds')::INTERVAL,
		    version = version + 1
		WHERE id IN (SELECT id FROM candidate)
		RETURNING id, status, stage, source, api_code, datasource_id, operation, priority, created_at, started_at,
			completed_at, duration_ms, error, retry_count, retry_after, trace_id,
			lease_owner, lease_token, lease_expires_at, version, max_retries
	`, owner, token, fmt.Sprintf("%.0f", leaseTTL.Seconds()))

	task, err := scanTask(row)
	if err != nil {
		// No rows returned = no pending tasks available / 无可用任务
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("postgres: claim next: %w", err)
	}

	return &store.TaskLease{
		Task:      task,
		Owner:     task.LeaseOwner,
		Token:     task.LeaseToken,
		ExpiresAt: *task.LeaseExpiresAt,
	}, nil
}

// RenewLease extends the lease for a task, conditional on ownership and non-expiry.
// 续租操作，条件为当前所有者持有有效且未过期的租约。
// 返回 false 表示租约已过期或所有权已丢失。
func (s *Store) RenewLease(id, owner, token string, leaseTTL time.Duration) (bool, error) {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks
		SET lease_expires_at = NOW() + ($4::TEXT || ' seconds')::INTERVAL,
		    version = version + 1
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2
		  AND lease_token = $3
		  AND lease_expires_at > NOW()
	`, id, owner, token, fmt.Sprintf("%.0f", leaseTTL.Seconds()))
	if err != nil {
		return false, fmt.Errorf("postgres: renew lease: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CompleteLease marks a task as completed, conditional on ownership.
// 条件完成操作：仅当当前副本仍持有有效租约时才标记完成。
// 返回 false 表示当前副本已失去所有权（其他副本可能已接管）。
func (s *Store) CompleteLease(id, owner, token string, result store.TaskResult) (bool, error) {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks
		SET status = 'completed',
		    stage = CASE WHEN $4 != '' THEN $4 ELSE stage END,
		    completed_at = NOW(),
		    duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000,
		    lease_expires_at = NULL,
		    version = version + 1
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2
		  AND lease_token = $3
		  AND lease_expires_at > NOW()
	`, id, owner, token, result.Stage)
	if err != nil {
		return false, fmt.Errorf("postgres: complete lease: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// FailLease marks a task as failed, conditional on ownership.
// 条件失败操作：仅当当前副本仍持有有效租约时才标记失败。
// 若 failure.Retryable 为 true 且重试次数未耗尽，任务回退为 pending 等待重试。
// 返回 false 表示当前副本已失去所有权。
func (s *Store) FailLease(id, owner, token string, failure store.TaskFailure) (bool, error) {
	ctx := context.Background()

	if failure.Retryable {
		// Retryable failure: reset to pending with backoff / 可重试失败：回退为 pending 并设置退避
		tag, err := s.pool.Exec(ctx, `
			UPDATE tasks
			SET status = 'pending',
			    stage = 'queued',
			    error = $4,
			    retry_count = retry_count + 1,
			    retry_after = NOW() + (LEAST(5 * POWER(2, retry_count), 60)::TEXT || ' seconds')::INTERVAL,
			    lease_owner = '',
			    lease_token = '',
			    lease_expires_at = NULL,
			    version = version + 1
			WHERE id = $1
			  AND status = 'running'
			  AND lease_owner = $2
			  AND lease_token = $3
			  AND lease_expires_at > NOW()
			  AND retry_count < max_retries
		`, id, owner, token, failure.Error)
		if err != nil {
			return false, fmt.Errorf("postgres: fail lease (retryable): %w", err)
		}
		if tag.RowsAffected() > 0 {
			return true, nil
		}

		// A valid lease at its retry limit must become terminal. Leaving it
		// running would let lease recovery requeue a task that ClaimNext can no
		// longer claim because retry_count has reached max_retries.
		tag, err = s.pool.Exec(ctx, `
			UPDATE tasks
			SET status = 'failed',
			    error = $4,
			    completed_at = NOW(),
			    duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000,
			    lease_expires_at = NULL,
			    version = version + 1
			WHERE id = $1
			  AND status = 'running'
			  AND lease_owner = $2
			  AND lease_token = $3
			  AND lease_expires_at > NOW()
			  AND retry_count >= max_retries
		`, id, owner, token, failure.Error)
		if err != nil {
			return false, fmt.Errorf("postgres: fail lease after retry exhaustion: %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}

	// Non-retryable failure: mark as terminal failed / 不可重试失败：标记为终态 failed
	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks
		SET status = 'failed',
		    error = $4,
		    completed_at = NOW(),
		    duration_ms = EXTRACT(EPOCH FROM (NOW() - started_at)) * 1000,
		    lease_expires_at = NULL,
		    version = version + 1
		WHERE id = $1
		  AND status = 'running'
		  AND lease_owner = $2
		  AND lease_token = $3
		  AND lease_expires_at > NOW()
	`, id, owner, token, failure.Error)
	if err != nil {
		return false, fmt.Errorf("postgres: fail lease (terminal): %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// RequeueExpiredLeases reclaims tasks whose lease has expired.
// 批量回收过期租约：将 running 但租约已过期的任务回退为 pending。
// 使用条件 UPDATE 确保不会覆盖仍健康副本的租约。
func (s *Store) RequeueExpiredLeases(limit int) (int, error) {
	ctx := context.Background()
	if limit <= 0 {
		limit = 100
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE tasks
		SET status = 'pending',
		    stage = 'queued',
		    lease_owner = '',
		    lease_token = '',
		    lease_expires_at = NULL,
		    version = version + 1
		FROM (
			SELECT id FROM tasks
			WHERE status = 'running'
			  AND lease_expires_at IS NOT NULL
			  AND lease_expires_at <= NOW()
			ORDER BY lease_expires_at ASC
			LIMIT $1
		) AS expired
		WHERE tasks.id = expired.id
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("postgres: requeue expired leases: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// generateToken creates a random 16-byte hex token for lease identification.
// generateToken 生成 16 字节随机十六进制令牌，用于租约唯一标识。
func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
