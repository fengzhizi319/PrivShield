/**
 * 负载均衡测试视图。
 *
 * 用户配置多个 agent 后端地址（name + url，默认用 health.agent_url 预填一行），
 * 设置请求数与分发策略，运行后由后端按策略分发探测请求，
 * 以表格 + 简易条形可视化展示各节点的命中数、成功率与平均延迟。
 */
import { useEffect, useState } from 'react';
import type { LbBackend, LbStrategy, LbTestResponse } from '@/types/api';
import { lbTest } from '@/api/client';
import { Icon } from '@/components/icons';
import { useI18n } from '@/i18n';

/** Strategy option labels (resolved at render time via i18n). */
const STRATEGY_KEYS: { value: LbStrategy; i18nKey: string }[] = [
  { value: 'round_robin', i18nKey: 'lb.strategy_round_robin' },
  { value: 'random', i18nKey: 'lb.strategy_random' },
  { value: 'least_connections', i18nKey: 'lb.strategy_least_conn' },
];

interface LbTestProps {
  /** agent REST 地址，用于预填第一个后端节点 */
  agentUrl?: string;
}

export default function LbTest({ agentUrl }: LbTestProps) {
  const { t } = useI18n();
  const [backends, setBackends] = useState<LbBackend[]>([
    { name: 'agent-1', url: agentUrl || 'http://127.0.0.1:8079' },
  ]);
  const [numRequests, setNumRequests] = useState(20);
  const [strategy, setStrategy] = useState<LbStrategy>('round_robin');

  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<LbTestResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // agentUrl 变化（切换后端）时，若第一行仍为默认值则同步更新。
  useEffect(() => {
    if (agentUrl) {
      setBackends((prev) => {
        if (prev.length === 1 && (prev[0].url === 'http://127.0.0.1:8079' || prev[0].url === '')) {
          return [{ ...prev[0], url: agentUrl }];
        }
        return prev;
      });
    }
  }, [agentUrl]);

  const updateBackend = (idx: number, patch: Partial<LbBackend>) => {
    setBackends((prev) => prev.map((b, i) => (i === idx ? { ...b, ...patch } : b)));
  };
  const addBackend = () => {
    setBackends((prev) => [...prev, { name: `agent-${prev.length + 1}`, url: agentUrl || 'http://127.0.0.1:8079' }]);
  };
  const removeBackend = (idx: number) => {
    setBackends((prev) => (prev.length > 1 ? prev.filter((_, i) => i !== idx) : prev));
  };

  const handleRun = async () => {
    const valid = backends.filter((b) => b.url.trim());
    if (valid.length === 0) {
      setError(t('lb.at_least_one'));
      return;
    }
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const resp = await lbTest({
        backends: valid,
        num_requests: numRequests,
        strategy,
      });
      setResult(resp);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const inputCls =
    'rounded-lg border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-700 placeholder-gray-400 transition-colors focus:border-indigo-400 focus:bg-white focus:outline-none focus:ring-2 focus:ring-indigo-100';

  // 条形可视化的最大命中数基准。
  const maxCount = result ? Math.max(1, ...result.distribution.map((d) => d.count)) : 1;

  return (
    <div className="flex h-full">
      {/* 左侧：配置 */}
      <div className="flex w-[380px] shrink-0 flex-col gap-4 overflow-y-auto border-r border-gray-200 bg-white p-5">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold text-gray-800">
            <span className="flex h-6 w-6 items-center justify-center rounded bg-indigo-50 text-indigo-600">
              <Icon name="scale" className="h-3.5 w-3.5" />
            </span>
            负载均衡测试
          </h2>
          <p className="mt-1 text-xs text-gray-500">{t('lb.subtitle')}</p>
        </div>

        {/* 后端列表 */}
        <div>
          <div className="mb-1 flex items-center justify-between">
            <label className="text-xs font-medium text-gray-600">{t('lb.backends')}</label>
            <button
              onClick={addBackend}
              className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs text-indigo-600 transition-colors hover:bg-indigo-50"
            >
              <Icon name="copy" className="h-3 w-3" />
              {t('lb.add_node')}
            </button>
          </div>
          <div className="space-y-2">
            {backends.map((b, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <input
                  value={b.name}
                  onChange={(e) => updateBackend(idx, { name: e.target.value })}
                  className={`${inputCls} w-24 shrink-0`}
                  placeholder={t('lb.name_placeholder')}
                />
                <input
                  value={b.url}
                  onChange={(e) => updateBackend(idx, { url: e.target.value })}
                  className={`${inputCls} flex-1`}
                  placeholder="http://127.0.0.1:8079"
                />
                <button
                  onClick={() => removeBackend(idx)}
                  disabled={backends.length <= 1}
                  className="shrink-0 rounded-md p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-40"
                  title="删除节点"
                >
                  <Icon name="trash" className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        </div>

        {/* 请求数 */}
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">{t('lb.num_requests')}</label>
          <input
            type="number"
            min={1}
            max={1000}
            value={numRequests}
            onChange={(e) => setNumRequests(Math.min(1000, Math.max(1, Number(e.target.value) || 1)))}
            className={`${inputCls} w-full`}
          />
        </div>

        {/* 策略 */}
        <div>
          <label className="mb-1 block text-xs font-medium text-gray-600">{t('lb.strategy')}</label>
          <select
            value={strategy}
            onChange={(e) => setStrategy(e.target.value as LbStrategy)}
            className={`${inputCls} w-full`}
          >
            {STRATEGY_KEYS.map((s) => (
              <option key={s.value} value={s.value}>
                {t(s.i18nKey)}
              </option>
            ))}
          </select>
        </div>

        <button
          onClick={handleRun}
          disabled={loading}
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {loading ? (
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" />
          ) : (
            <Icon name="play" className="h-4 w-4" />
          )}
          {loading ? '测试中…' : t('lb.run')}
        </button>
      </div>

      {/* 右侧：结果 */}
      <div className="flex-1 overflow-y-auto p-5">
        {error && (
          <div className="mb-4 flex items-center gap-2 rounded-lg border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-600">
            <Icon name="alert" className="h-4 w-4 shrink-0" />
            {error}
          </div>
        )}

        {!result && !error && (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-gray-300">
            <Icon name="scale" className="h-10 w-10" strokeWidth={1.5} />
            <p className="text-sm text-gray-400">{t('lb.empty_hint')}</p>
          </div>
        )}

        {result && (
          <div className="space-y-5">
            {/* 汇总卡片 */}
            <div className="grid grid-cols-4 gap-3">
              <SummaryCard label={t('lb.total_requests')} value={result.total} tone="text-gray-800" />
              <SummaryCard label={t('lb.success')} value={result.success} tone="text-emerald-600" />
              <SummaryCard label={t('lb.failed')} value={result.failed} tone="text-red-500" />
              <SummaryCard label={t('lb.total_duration')} value={`${result.duration_ms.toFixed(1)} ms`} tone="text-indigo-600" />
            </div>

            {/* 分发结果表 */}
            <div className="overflow-hidden rounded-xl border border-gray-200 bg-white">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
                    <th className="px-4 py-2 font-medium">{t('lb.col_node')}</th>
                    <th className="px-4 py-2 font-medium">{t('lb.col_distribution')}</th>
                    <th className="px-4 py-2 text-right font-medium">{t('lb.col_hits')}</th>
                    <th className="px-4 py-2 text-right font-medium">{t('lb.col_success_rate')}</th>
                    <th className="px-4 py-2 text-right font-medium">{t('lb.col_avg_latency')}</th>
                    <th className="px-4 py-2 text-right font-medium">{t('lb.col_min_max_latency')}</th>
                  </tr>
                </thead>
                <tbody>
                  {result.distribution.map((d, i) => {
                    const rate = d.count > 0 ? (d.success / d.count) * 100 : 0;
                    return (
                      <tr key={i} className="border-b border-gray-50 last:border-0 hover:bg-indigo-50/30">
                        <td className="px-4 py-2.5">
                          <div className="font-medium text-gray-800">{d.name}</div>
                          <div className="text-xs text-gray-400">{d.url}</div>
                        </td>
                        <td className="px-4 py-2.5">
                          <div className="h-2.5 w-full overflow-hidden rounded-full bg-gray-100">
                            <div
                              className="h-full rounded-full bg-indigo-500"
                              style={{ width: `${(d.count / maxCount) * 100}%` }}
                            />
                          </div>
                        </td>
                        <td className="px-4 py-2.5 text-right font-medium text-gray-700">{d.count}</td>
                        <td className="px-4 py-2.5 text-right">
                          <span className={rate === 100 ? 'text-emerald-600' : rate > 0 ? 'text-amber-600' : 'text-red-500'}>
                            {rate.toFixed(0)}%
                          </span>
                        </td>
                        <td className="px-4 py-2.5 text-right text-gray-700">{d.avg_latency_ms.toFixed(2)} ms</td>
                        <td className="px-4 py-2.5 text-right text-xs text-gray-500">
                          {d.min_latency_ms.toFixed(2)} / {d.max_latency_ms.toFixed(2)} ms
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/** 汇总小卡片。 */
function SummaryCard({ label, value, tone }: { label: string; value: number | string; tone: string }) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white px-4 py-3">
      <div className="text-xs text-gray-400">{label}</div>
      <div className={`mt-1 text-xl font-semibold ${tone}`}>{value}</div>
    </div>
  );
}
