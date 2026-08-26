/**
 * DatasourceExplorer — 数据源切片浏览器。
 *
 * 功能概述：
 *  1. 选择数据源并拉取数据切片（分页查看数据源中的记录）
 *  2. 触发流水线处理（将数据源记录发送到 Service Hub 执行脱敏等操作）
 *
 * 注意：此组件目前未在 App.tsx 中直接使用（已被 DataApiPanel 替代），
 * 但保留作为数据源级别的浏览能力。
 */
import React, { useState } from 'react';
import { Datasource, DatasourceSliceResponse } from '../types/api';
import { useI18n } from '../i18n';
import {
  IconDatabase,
  IconPlay,
  IconRefresh,
  IconCheckCircle,
} from './icons';

/** DatasourceExplorer 组件的 Props */
interface DatasourceExplorerProps {
  /** 可用数据源列表 */
  datasources: Datasource[];
  /** 拉取数据切片的回调 */
  onFetchSlice: (id: string, limit: number) => Promise<DatasourceSliceResponse>;
  /** 触发流水线处理的回调 */
  onTriggerPipeline: (dsID: string, limit: number) => Promise<any>;
}

export const DatasourceExplorer: React.FC<DatasourceExplorerProps> = ({
  datasources,
  onFetchSlice,
  onTriggerPipeline,
}) => {
  const { t } = useI18n();

  const [selectedDsId, setSelectedDsId] = useState<string>('ds_yibao');
  const [limit, setLimit] = useState(10);
  const [sliceData, setSliceData] = useState<DatasourceSliceResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [lastTriggerTask, setLastTriggerTask] = useState<string | null>(null);

  const handleFetch = async () => {
    setLoading(true);
    try {
      const res = await onFetchSlice(selectedDsId, limit);
      setSliceData(res);
    } catch (err: any) {
      alert(`拉取切片失败: ${err.message}`);
    } finally {
      setLoading(false);
    }
  };

  const handleDispatch = async () => {
    setTriggering(true);
    try {
      const res = await onTriggerPipeline(selectedDsId, limit);
      setLastTriggerTask(res.task_id || `task-${Date.now()}`);
      alert(`已成功联动数据源派发至调度流水线！\n任务 ID: ${res.task_id || 'task-generated'}`);
    } catch (err: any) {
      alert(`派发失败: ${err.message}`);
    } finally {
      setTriggering(false);
    }
  };

  const selectedDs = datasources.find((d) => d.id === selectedDsId) || datasources[0];

  return (
    <div className="space-y-6">
      {/* Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
              <IconDatabase className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('ds.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('ds.desc')}</p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={handleFetch}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 font-medium text-xs border border-slate-700 transition"
          >
            <IconRefresh className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>{t('ds.sampleSlice')}</span>
          </button>

          <button
            onClick={handleDispatch}
            disabled={triggering}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-bold text-xs shadow-lg shadow-indigo-600/30 transition disabled:opacity-50"
          >
            <IconPlay className="w-3.5 h-3.5" />
            <span>{triggering ? '派发中...' : t('ds.dispatchSlice')}</span>
          </button>
        </div>
      </div>

      {/* Datasources Selector Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {datasources.map((ds) => {
          const isSelected = selectedDsId === ds.id;
          return (
            <div
              key={ds.id}
              onClick={() => {
                setSelectedDsId(ds.id);
                setSliceData(null);
              }}
              className={`cursor-pointer p-5 rounded-2xl border transition-all ${
                isSelected
                  ? 'bg-amber-950/20 border-amber-500/60 ring-2 ring-amber-500/20'
                  : 'bg-slate-900 border-slate-800 hover:border-slate-700'
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="font-mono text-xs font-bold px-2 py-0.5 rounded bg-slate-800 text-amber-400 border border-slate-700">
                  {ds.id}
                </span>
                <span className="text-xs text-slate-400">
                  {t('ds.recordCount')}: <strong className="text-slate-200 font-mono">{ds.records_count}</strong>
                </span>
              </div>

              <h3 className="text-base font-bold text-slate-100 mt-2">{ds.name}</h3>
              <p className="text-xs text-slate-400 mt-1">类别: {ds.category}</p>

              {ds.fields && (
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {ds.fields.map((f) => (
                    <span key={f} className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-950 text-slate-400 border border-slate-800">
                      {f}
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Slice Data Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl space-y-4">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center gap-3">
            <h2 className="text-sm font-bold text-slate-100">
              数据源采样切片预览 — <span className="font-mono text-amber-400">{selectedDs?.name}</span>
            </h2>
          </div>

          <div className="flex items-center gap-2 text-xs">
            <span className="text-slate-400">采样行数:</span>
            <select
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value))}
              className="bg-slate-950 border border-slate-800 rounded px-2 py-1 text-slate-200 font-mono focus:outline-none focus:border-indigo-500"
            >
              <option value={5}>5 条</option>
              <option value={10}>10 条</option>
              <option value={20}>20 条</option>
              <option value={50}>50 条</option>
            </select>
          </div>
        </div>

        {sliceData && sliceData.records && sliceData.records.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs font-mono">
              <thead className="bg-slate-950 text-slate-400 uppercase border-b border-slate-800">
                <tr>
                  {Object.keys(sliceData.records[0]).map((k) => (
                    <th key={k} className="py-2.5 px-3">
                      {k}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/80">
                {sliceData.records.map((r, i) => (
                  <tr key={i} className="hover:bg-slate-800/40 transition">
                    {Object.values(r).map((v, j) => (
                      <td key={j} className="py-2.5 px-3 text-slate-300 truncate max-w-[180px]">
                        {typeof v === 'object' ? JSON.stringify(v) : String(v)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="text-center py-12 text-slate-500 text-sm">
            点击上方“抽取数据切片”加载实时数据源样本
          </div>
        )}

        {lastTriggerTask && (
          <div className="p-3 bg-indigo-950/40 border border-indigo-500/30 rounded-xl text-xs text-indigo-300 flex items-center justify-between">
            <span className="flex items-center gap-1.5">
              <IconCheckCircle className="w-4 h-4 text-emerald-400" />
              已联动生成流水线任务: <strong className="font-mono">{lastTriggerTask}</strong>
            </span>
            <span className="text-slate-400 font-mono">前往“流水线大屏”跟踪执行</span>
          </div>
        )}
      </div>
    </div>
  );
};
