import React, { useState } from 'react';
import { Task, LeasedTasksResponse } from '../types/api';
import { useI18n } from '../i18n';
import {
  IconLayers,
  IconCheckCircle,
  IconXCircle,
  IconLock,
  IconRefresh,
  IconShieldCheck,
} from './icons';

interface TaskLifecyclePanelProps {
  tasks: Task[];
  leases: LeasedTasksResponse | null;
  onRefresh: () => Promise<void>;
  loading: boolean;
}

export const TaskLifecyclePanel: React.FC<TaskLifecyclePanelProps> = ({
  tasks,
  leases,
  onRefresh,
  loading,
}) => {
  const { t } = useI18n();
  const [filterStatus, setFilterStatus] = useState('all');
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);

  const filteredTasks = tasks.filter((t) => {
    if (filterStatus === 'all') return true;
    return t.status === filterStatus;
  });

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'completed':
        return (
          <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 flex items-center gap-1">
            <IconCheckCircle className="w-3 h-3" />
            Completed
          </span>
        );
      case 'running':
        return (
          <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 flex items-center gap-1">
            <span className="w-1.5 h-1.5 rounded-full bg-indigo-400 animate-ping" />
            Running
          </span>
        );
      case 'failed':
        return (
          <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-rose-500/10 text-rose-400 border border-rose-500/20 flex items-center gap-1">
            <IconXCircle className="w-3 h-3" />
            Failed
          </span>
        );
      default:
        return (
          <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-slate-800 text-slate-400 border border-slate-700">
            Pending
          </span>
        );
    }
  };

  return (
    <div className="space-y-6">
      {/* Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
              <IconLayers className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('tasks.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('tasks.desc')}</p>
        </div>

        <button
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-medium text-sm transition-all shadow-lg shadow-indigo-600/20 disabled:opacity-50"
        >
          <span className={loading ? 'animate-spin' : ''}>
            <IconRefresh className="w-4 h-4" />
          </span>
          <span>刷新任务与租约</span>
        </button>
      </div>

      {/* Phase B PostgreSQL Atomic Lease Inspector Box */}
      {leases && (
        <div className="bg-gradient-to-br from-indigo-950/40 via-slate-900 to-slate-900 border border-indigo-500/30 rounded-2xl p-6 shadow-xl">
          <div className="flex items-center justify-between border-b border-indigo-500/20 pb-3 mb-4">
            <div className="flex items-center gap-2">
              <IconLock className="w-5 h-5 text-indigo-400" />
              <h2 className="text-sm font-bold text-slate-100">{t('tasks.leaseTitle')}</h2>
              <span className="px-2 py-0.5 text-[10px] rounded font-mono bg-indigo-500/20 text-indigo-300 border border-indigo-500/30">
                Backend: {leases.storeBackend} (FOR UPDATE SKIP LOCKED)
              </span>
            </div>
            <div className="text-xs text-slate-400">
              活跃租约数: <span className="text-indigo-400 font-bold font-mono">{leases.totalLeasedTasks}</span>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {leases.workers?.map((w) => (
              <div key={w.worker_id} className="bg-slate-950/80 border border-slate-800 rounded-xl p-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-mono font-bold text-slate-200">{w.worker_id}</span>
                  <span className="text-xs px-2 py-0.5 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                    持有任务: {w.claimed_tasks_count}
                  </span>
                </div>
                <div className="space-y-1.5">
                  {w.tasks?.map((tk) => (
                    <div
                      key={tk.task_id}
                      className="flex items-center justify-between text-xs p-2 rounded bg-slate-900 border border-slate-800"
                    >
                      <span className="font-mono text-slate-300">{tk.task_id}</span>
                      <span className="text-slate-400 font-mono">阶段: {tk.stage}</span>
                      <span className="text-amber-400 font-mono">TTL: {tk.lease_expires_in_seconds.toFixed(1)}s</span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Task Filters & Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-4 border-b border-slate-800 pb-4">
          <div className="flex items-center gap-2">
            {['all', 'running', 'completed', 'failed'].map((st) => (
              <button
                key={st}
                onClick={() => setFilterStatus(st)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                  filterStatus === st
                    ? 'bg-indigo-600 text-white shadow'
                    : 'bg-slate-800 text-slate-400 hover:text-slate-200'
                }`}
              >
                {st === 'all'
                  ? t('tasks.filter.all')
                  : st === 'running'
                  ? t('tasks.filter.running')
                  : st === 'completed'
                  ? t('tasks.filter.completed')
                  : t('tasks.filter.failed')}
              </button>
            ))}
          </div>

          <span className="text-xs text-slate-400">
            共查询到 <span className="text-indigo-400 font-bold font-mono">{filteredTasks.length}</span> 条任务记录
          </span>
        </div>

        {/* Table */}
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-950 text-slate-400 uppercase font-mono border-b border-slate-800">
              <tr>
                <th className="py-3 px-4">Task ID</th>
                <th className="py-3 px-4">状态</th>
                <th className="py-3 px-4">阶段</th>
                <th className="py-3 px-4">数据源</th>
                <th className="py-3 px-4">操作</th>
                <th className="py-3 px-4">耗时</th>
                <th className="py-3 px-4">租约 Worker</th>
                <th className="py-3 px-4 text-right">详情</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80 font-mono">
              {filteredTasks.length > 0 ? (
                filteredTasks.map((task) => (
                  <tr key={task.id} className="hover:bg-slate-800/40 transition">
                    <td className="py-3 px-4 font-bold text-slate-200">{task.id}</td>
                    <td className="py-3 px-4">{getStatusBadge(task.status)}</td>
                    <td className="py-3 px-4 text-slate-300">{task.stage}</td>
                    <td className="py-3 px-4 text-slate-400">{task.source}</td>
                    <td className="py-3 px-4">
                      <span className="px-2 py-0.5 rounded bg-slate-800 text-indigo-300 border border-slate-700">
                        {task.operation}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-slate-300">{task.duration_ms} ms</td>
                    <td className="py-3 px-4 text-slate-400">{task.lease_owner || '-'}</td>
                    <td className="py-3 px-4 text-right">
                      <button
                        onClick={() => setSelectedTask(task)}
                        className="px-2.5 py-1 rounded bg-slate-800 hover:bg-slate-700 text-indigo-400 border border-slate-700 transition"
                      >
                        {t('tasks.viewDetail')}
                      </button>
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={8} className="py-8 text-center text-slate-500 font-sans">
                    暂无符合条件的任务记录
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Task Detail Modal */}
      {selectedTask && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fade-in">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-2xl w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <IconShieldCheck className="w-5 h-5 text-indigo-400" />
                <h3 className="text-base font-bold text-slate-100">任务详情 — {selectedTask.id}</h3>
              </div>
              <button
                onClick={() => setSelectedTask(null)}
                className="text-xs text-slate-400 hover:text-slate-200 px-2 py-1 rounded bg-slate-800 border border-slate-700"
              >
                关闭
              </button>
            </div>

            <div className="grid grid-cols-2 gap-3 text-xs">
              <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800">
                <span className="text-slate-400 block">状态 / 阶段:</span>
                <span className="text-slate-200 font-bold font-mono">{selectedTask.status} / {selectedTask.stage}</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800">
                <span className="text-slate-400 block">数据源 / 操作:</span>
                <span className="text-slate-200 font-bold font-mono">{selectedTask.source} / {selectedTask.operation}</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800">
                <span className="text-slate-400 block">耗时 / 重试:</span>
                <span className="text-slate-200 font-bold font-mono">{selectedTask.duration_ms} ms / {selectedTask.retry_count} 次</span>
              </div>
              <div className="p-2.5 rounded-lg bg-slate-950 border border-slate-800">
                <span className="text-slate-400 block">租约持有节点:</span>
                <span className="text-indigo-400 font-bold font-mono">{selectedTask.lease_owner || 'None'}</span>
              </div>
            </div>

            <div>
              <span className="text-xs font-semibold text-slate-400 mb-1 block">任务输入与处理结果:</span>
              <pre className="p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs font-mono text-slate-300 max-h-48 overflow-y-auto">
                {JSON.stringify(selectedTask, null, 2)}
              </pre>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
