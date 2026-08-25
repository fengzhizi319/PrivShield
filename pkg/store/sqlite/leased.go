// Phase B: LeasedTaskStore stub for SQLite backend.
// Phase B：SQLite 后端的 LeasedTaskStore 桩实现。
//
// SQLite does not support cross-replica atomic leases (no FOR UPDATE SKIP LOCKED,
// no multi-writer concurrency). All lease methods return ErrLeaseNotSupported
// to prevent silent misconfiguration when operators accidentally enable multi-replica
// Hub on a SQLite-backed deployment.
//
// SQLite 不支持跨副本原子租约（无 FOR UPDATE SKIP LOCKED，无多写入者并发）。
// 所有租约方法返回 ErrLeaseNotSupported，防止运维人员意外在 SQLite 部署上
// 启用多副本 Hub 时悄然出错。
package sqlite

import (
	"time"

	"github.com/fengzhizi319/PrivShield/pkg/store"
)

// ClaimNext is not supported on SQLite; returns ErrLeaseNotSupported.
func (s *TaskStore) ClaimNext(owner string, leaseTTL time.Duration) (*store.TaskLease, error) {
	return nil, store.ErrLeaseNotSupported
}

// RenewLease is not supported on SQLite; returns ErrLeaseNotSupported.
func (s *TaskStore) RenewLease(id, owner, token string, leaseTTL time.Duration) (bool, error) {
	return false, store.ErrLeaseNotSupported
}

// CompleteLease is not supported on SQLite; returns ErrLeaseNotSupported.
func (s *TaskStore) CompleteLease(id, owner, token string, result store.TaskResult) (bool, error) {
	return false, store.ErrLeaseNotSupported
}

// FailLease is not supported on SQLite; returns ErrLeaseNotSupported.
func (s *TaskStore) FailLease(id, owner, token string, failure store.TaskFailure) (bool, error) {
	return false, store.ErrLeaseNotSupported
}

// RequeueExpiredLeases is not supported on SQLite; returns ErrLeaseNotSupported.
func (s *TaskStore) RequeueExpiredLeases(limit int) (int, error) {
	return 0, store.ErrLeaseNotSupported
}

// Compile-time interface assertion: SQLite TaskStore satisfies LeasedTaskStore
// (methods exist but return ErrLeaseNotSupported at runtime).
var _ store.LeasedTaskStore = (*TaskStore)(nil)
