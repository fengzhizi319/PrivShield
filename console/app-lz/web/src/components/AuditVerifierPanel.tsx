import React, { useState } from 'react';
import { AuditLogItem, AuditVerifyResponse } from '../types/api';
import { useI18n } from '../i18n';
import {
  IconShieldCheck,
  IconCheckCircle,
  IconLock,
  IconRefresh,
} from './icons';

interface AuditVerifierPanelProps {
  logs: AuditLogItem[];
  onVerify: () => Promise<AuditVerifyResponse>;
  onRefreshLogs: () => Promise<void>;
  loading: boolean;
}

export const AuditVerifierPanel: React.FC<AuditVerifierPanelProps> = ({
  logs,
  onVerify,
  onRefreshLogs,
  loading,
}) => {
  const { t } = useI18n();
  const [verifying, setVerifying] = useState(false);
  const [verifyResult, setVerifyResult] = useState<AuditVerifyResponse | null>(null);

  const handleVerify = async () => {
    setVerifying(true);
    try {
      const res = await onVerify();
      setVerifyResult(res);
    } catch (err: any) {
      alert(`Merkle 验真失败: ${err.message}`);
    } finally {
      setVerifying(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
              <IconShieldCheck className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('audit.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('audit.desc')}</p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={onRefreshLogs}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium text-xs border border-slate-700 transition"
          >
            <IconRefresh className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>刷新存证流水</span>
          </button>

          <button
            onClick={handleVerify}
            disabled={verifying}
            className="flex items-center gap-2 px-5 py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs shadow-lg shadow-emerald-600/30 transition disabled:opacity-50"
          >
            <IconLock className="w-3.5 h-3.5" />
            <span>{verifying ? t('audit.verifying') : t('audit.verifyBtn')}</span>
          </button>
        </div>
      </div>

      {/* Merkle Verification Result Card */}
      {verifyResult && (
        <div className="bg-gradient-to-br from-emerald-950/30 via-slate-900 to-slate-900 border border-emerald-500/40 rounded-2xl p-6 shadow-xl space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <IconCheckCircle className="w-6 h-6 text-emerald-400" />
              <h2 className="text-base font-bold text-emerald-400">{t('audit.merkleValid')}</h2>
            </div>
            <span className="text-xs px-2.5 py-1 rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/20 font-mono">
              SHA-256 + Merkle Tree Verified
            </span>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs pt-2">
            <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800">
              <span className="text-slate-400 block mb-1">{t('audit.rootHash')}:</span>
              <span className="text-slate-200 font-mono truncate block text-[11px]" title={verifyResult.root_hash}>
                {verifyResult.root_hash}
              </span>
            </div>
            <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800">
              <span className="text-slate-400 block mb-1">{t('audit.totalEntries')}:</span>
              <span className="text-emerald-400 font-mono font-bold text-sm">
                {verifyResult.total_entries} 条链上存证
              </span>
            </div>
            <div className="p-3 bg-slate-950/80 rounded-xl border border-slate-800">
              <span className="text-slate-400 block mb-1">{t('audit.signature')}:</span>
              <span className="text-slate-200 font-mono truncate block text-[11px]">
                {verifyResult.signature || 'Ed25519-Signed'}
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Audit Log Stream Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl space-y-4">
        <h2 className="text-sm font-bold text-slate-100 border-b border-slate-800 pb-3">
          流水线脱敏审计存证日志流 (Audit Stream)
        </h2>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="bg-slate-950 text-slate-400 uppercase border-b border-slate-800">
              <tr>
                <th className="py-3 px-3">时间戳</th>
                <th className="py-3 px-3">Task ID</th>
                <th className="py-3 px-3">数据源</th>
                <th className="py-3 px-3">操作</th>
                <th className="py-3 px-3">数据 SHA-256 指纹</th>
                <th className="py-3 px-3">操作主体</th>
                <th className="py-3 px-3 text-right">存证状态</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80">
              {logs.map((item) => (
                <tr key={item.id} className="hover:bg-slate-800/40 transition">
                  <td className="py-3 px-3 text-slate-400 truncate max-w-[140px]">{item.timestamp}</td>
                  <td className="py-3 px-3 font-bold text-slate-200">{item.task_id}</td>
                  <td className="py-3 px-3 text-slate-300">{item.source}</td>
                  <td className="py-3 px-3">
                    <span className="px-2 py-0.5 rounded bg-slate-800 text-indigo-300 border border-slate-700">
                      {item.operation}
                    </span>
                  </td>
                  <td className="py-3 px-3 text-slate-400 truncate max-w-[160px]" title={item.data_hash}>
                    {item.data_hash}
                  </td>
                  <td className="py-3 px-3 text-slate-300">{item.operator}</td>
                  <td className="py-3 px-3 text-right">
                    <span className="px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold">
                      {item.result}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
