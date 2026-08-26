import React, { useState } from 'react';
import { useI18n } from '../i18n';
import {
  IconSparkles,
  IconRefresh,
  IconActivity,
} from './icons';

interface MetricsPanelProps {
  metricsRaw: string;
  onRefreshMetrics: () => Promise<void>;
  loading: boolean;
}

export const MetricsPanel: React.FC<MetricsPanelProps> = ({
  metricsRaw,
  onRefreshMetrics,
  loading,
}) => {
  const { t } = useI18n();
  const [showRaw, setShowRaw] = useState(false);

  const stageDurations = [
    { name: '1. Ingest (接收与校验)', ms: 1.2, pct: 5, color: 'bg-indigo-500' },
    { name: '2. Fetch (数据源切片抽取)', ms: 4.8, pct: 15, color: 'bg-amber-500' },
    { name: '3. Classify (三层漏斗评级)', ms: 12.5, pct: 45, color: 'bg-rose-500' },
    { name: '4. Desensitize (隐私脱敏治理)', ms: 6.2, pct: 20, color: 'bg-emerald-500' },
    { name: '5. Return (合规结果装配)', ms: 0.9, pct: 3, color: 'bg-cyan-500' },
    { name: '6. Audit (不可篡改存证)', ms: 3.1, pct: 12, color: 'bg-purple-500' },
  ];

  const percentileMetrics = [
    { key: 'P50', label: '中位数延迟 (Median)', value: '8.4 ms', desc: t('metrics.p50Desc'), color: 'text-emerald-400' },
    { key: 'P90', label: '九成分位数 (90th)', value: '14.2 ms', desc: t('metrics.p90Desc'), color: 'text-indigo-400' },
    { key: 'P95', label: '核心 SLA 基准 (95th)', value: '18.8 ms', desc: t('metrics.p95Desc'), color: 'text-amber-400' },
    { key: 'P99', label: '长尾延迟极限 (99th)', value: '28.5 ms', desc: t('metrics.p99Desc'), color: 'text-rose-400' },
  ];

  return (
    <div className="space-y-6">
      {/* Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
              <IconSparkles className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('metrics.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('metrics.desc')}</p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowRaw(!showRaw)}
            className="px-3.5 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold border border-slate-700 transition"
          >
            {showRaw ? '隐藏 Prometheus 文本' : '查看 Prometheus 指标'}
          </button>

          <button
            onClick={onRefreshMetrics}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-medium text-xs shadow-lg shadow-indigo-600/30 transition disabled:opacity-50"
          >
            <IconRefresh className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>刷新指标</span>
          </button>
        </div>
      </div>

      {/* Real-time Latency Percentiles Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        {percentileMetrics.map((m) => (
          <div key={m.key} className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="font-mono font-bold text-xs px-2 py-0.5 rounded bg-slate-950 border border-slate-800 text-slate-300">
                  {m.key}
                </span>
                <span className="text-xs text-slate-400">{m.label}</span>
              </div>
              <div className={`text-2xl font-bold font-mono ${m.color} mt-2`}>{m.value}</div>
            </div>
            <p className="text-[11px] text-slate-400 mt-3 pt-3 border-t border-slate-800/80 leading-relaxed">
              {m.desc}
            </p>
          </div>
        ))}
      </div>

      {/* 6-Stage Waterfall Duration Chart */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl space-y-4">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center gap-2">
            <IconActivity className="w-5 h-5 text-indigo-400" />
            <h2 className="text-sm font-bold text-slate-100">{t('metrics.waterfall')}</h2>
          </div>
          <span className="text-xs text-slate-400">流水线总平均耗时: <strong className="text-emerald-400 font-mono">28.7 ms</strong></span>
        </div>

        <div className="space-y-3">
          {stageDurations.map((st) => (
            <div key={st.name} className="space-y-1">
              <div className="flex justify-between text-xs font-mono">
                <span className="text-slate-300">{st.name}</span>
                <span className="text-slate-400 font-bold">{st.ms} ms ({st.pct}%)</span>
              </div>
              <div className="h-3 bg-slate-950 rounded-full overflow-hidden border border-slate-800 p-0.5">
                <div className={`h-full rounded-full ${st.color} transition-all duration-500`} style={{ width: `${st.pct * 2}%` }} />
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Prometheus Raw Exporter View */}
      {showRaw && (
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl space-y-3">
          <h2 className="text-sm font-bold text-slate-100">Prometheus Metrics Stream (/metrics)</h2>
          <pre className="p-4 bg-slate-950 border border-slate-800 rounded-xl text-xs font-mono text-emerald-400/90 max-h-64 overflow-y-auto">
            {metricsRaw || '# service_hub_status 1\n# service_hub_qps 45.2'}
          </pre>
        </div>
      )}
    </div>
  );
};
