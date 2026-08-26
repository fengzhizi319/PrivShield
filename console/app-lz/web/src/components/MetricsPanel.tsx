import React, { useState } from 'react';
import { useI18n } from '../i18n';
import {
  IconSparkles,
  IconRefresh,
  IconActivity,
} from './icons';

interface ParsedMetrics {
  stage_durations: Record<string, number>;
  qps: number;
  percentiles: Record<string, number>;
  total_requests: number;
  source: string;
}

interface MetricsPanelProps {
  metricsRaw: string;
  parsedMetrics: ParsedMetrics | null;
  onRefreshMetrics: () => Promise<void>;
  loading: boolean;
}

export const MetricsPanel: React.FC<MetricsPanelProps> = ({
  metricsRaw,
  parsedMetrics,
  onRefreshMetrics,
  loading,
}) => {
  const { t } = useI18n();
  const [showRaw, setShowRaw] = useState(false);

  // G-2: Use real parsed metrics from Prometheus, fallback to defaults
  const stageDurations = [
    { key: 'ingest', name: '1. Ingest (接收与校验)', color: 'bg-indigo-500' },
    { key: 'fetch', name: '2. Fetch (数据源切片抽取)', color: 'bg-amber-500' },
    { key: 'classify', name: '3. Classify (三层漏斗评级)', color: 'bg-rose-500' },
    { key: 'desensitize', name: '4. Desensitize (隐私脱敏治理)', color: 'bg-emerald-500' },
    { key: 'return', name: '5. Return (合规结果装配)', color: 'bg-cyan-500' },
    { key: 'audit', name: '6. Audit (不可篡改存证)', color: 'bg-purple-500' },
  ].map((s) => ({
    ...s,
    ms: parsedMetrics?.stage_durations?.[s.key] ?? 0,
  }));

  const totalDuration = stageDurations.reduce((sum, s) => sum + s.ms, 0);
  const stageWithPct = stageDurations.map((s) => ({
    ...s,
    pct: totalDuration > 0 ? Math.round((s.ms / totalDuration) * 100) : 0,
  }));

  // G-2: Real percentiles from parsed metrics
  const percentileMetrics = [
    { key: 'P50', label: '中位数延迟 (Median)', value: `${(parsedMetrics?.percentiles?.p50 ?? 0).toFixed(1)} ms`, desc: t('metrics.p50Desc'), color: 'text-emerald-400' },
    { key: 'P90', label: '九成分位数 (90th)', value: `${(parsedMetrics?.percentiles?.p90 ?? 0).toFixed(1)} ms`, desc: t('metrics.p90Desc'), color: 'text-indigo-400' },
    { key: 'P95', label: '核心 SLA 基准 (95th)', value: `${(parsedMetrics?.percentiles?.p95 ?? 0).toFixed(1)} ms`, desc: t('metrics.p95Desc'), color: 'text-amber-400' },
    { key: 'P99', label: '长尾延迟极限 (99th)', value: `${(parsedMetrics?.percentiles?.p99 ?? 0).toFixed(1)} ms`, desc: t('metrics.p99Desc'), color: 'text-rose-400' },
  ];

  // G-3: Real QPS from parsed metrics
  const currentQPS = parsedMetrics?.qps ?? 0;
  const totalRequests = parsedMetrics?.total_requests ?? 0;
  const metricsSource = parsedMetrics?.source ?? 'fallback';

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
          <span className={`text-[10px] font-mono px-2 py-1 rounded-full border ${
            metricsSource === 'prometheus'
              ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
              : 'bg-amber-500/10 text-amber-400 border-amber-500/30'
          }`}>
            {metricsSource === 'prometheus' ? '● LIVE Prometheus' : '○ Fallback Defaults'}
          </span>

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

      {/* Real-time QPS & Total Requests */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">{t('metrics.qps')}</div>
          <div className="text-3xl font-bold font-mono text-cyan-400">{currentQPS.toFixed(1)}</div>
          <div className="text-[11px] text-slate-500 mt-1">总请求数: {totalRequests.toFixed(0)}</div>
        </div>
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl">
          <div className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">流水线总平均耗时</div>
          <div className="text-3xl font-bold font-mono text-emerald-400">{totalDuration.toFixed(1)} <span className="text-sm text-slate-500">ms</span></div>
          <div className="text-[11px] text-slate-500 mt-1">6 阶段累计</div>
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
          <span className="text-xs text-slate-400">流水线总平均耗时: <strong className="text-emerald-400 font-mono">{totalDuration.toFixed(1)} ms</strong></span>
        </div>

        <div className="space-y-3">
          {stageWithPct.map((st) => (
            <div key={st.key} className="space-y-1">
              <div className="flex justify-between text-xs font-mono">
                <span className="text-slate-300">{st.name}</span>
                <span className="text-slate-400 font-bold">{st.ms} ms ({st.pct}%)</span>
              </div>
              <div className="h-3 bg-slate-950 rounded-full overflow-hidden border border-slate-800 p-0.5">
                <div className={`h-full rounded-full ${st.color} transition-all duration-500`} style={{ width: `${Math.max(st.pct * 2, 1)}%` }} />
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
            {metricsRaw || '# service_hub_status 1\n# service_hub_qps 0'}
          </pre>
        </div>
      )}
    </div>
  );
};
