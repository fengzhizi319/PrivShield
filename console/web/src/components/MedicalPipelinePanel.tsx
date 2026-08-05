/**
 * 医疗敏感数据治理视图 / Medical Privacy Pipeline View
 *
 * 展示从 data1.csv 到分类分级 (3-Layer L1~L5) 与脱敏清洗 (PII + L4/L5 强抹平)
 * 的全流程治理，提供双结构数据输出（分级报告与合规清洗数据）。
 */

import { useEffect, useState } from 'react';
import type { MedicalPipelineResponse, MedicalRecordReport } from '@/types/api';
import { runMedicalPipeline } from '@/api/client';
import { Icon } from '@/components/icons';
import { getErrorMessage } from '@/utils/error';

interface MedicalPipelinePanelProps {
  agentUrl?: string;
}

export default function MedicalPipelinePanel({ agentUrl }: MedicalPipelinePanelProps) {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<MedicalPipelineResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'report' | 'sanitized'>('report');
  const [expandedRecord, setExpandedRecord] = useState<number | null>(1);

  const handleExecute = async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await runMedicalPipeline();
      setResult(resp);
    } catch (e) {
      setError(getErrorMessage(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    handleExecute();
  }, []);

  const levelBadgeCls = (level: string) => {
    switch (level) {
      case 'L5':
        return 'bg-red-100 text-red-700 border-red-200';
      case 'L4':
        return 'bg-orange-100 text-orange-700 border-orange-200';
      case 'L3':
        return 'bg-amber-100 text-amber-700 border-amber-200';
      case 'L2':
        return 'bg-blue-100 text-blue-700 border-blue-200';
      default:
        return 'bg-emerald-100 text-emerald-700 border-emerald-200';
    }
  };

  return (
    <div className="flex h-full flex-col bg-gray-50 p-6 overflow-y-auto">
      {/* 头部标题与控制工具栏 */}
      <div className="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-4 rounded-xl shadow-sm">
        <div>
          <h2 className="flex items-center gap-2.5 text-lg font-bold text-gray-900">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-teal-50 text-teal-600">
              <Icon name="shield" className="h-5 w-5" />
            </span>
            医疗敏感数据分类分级与脱敏全流程治理 (Medical Pipeline)
          </h2>
          <p className="mt-1 text-xs text-gray-500">
            加载仿真医疗数据集 <span className="font-mono font-medium text-indigo-600">data1.csv</span>，执行 3-Layer 分类分级（识别 L4/L5 特高风险病史）与 PII/L4/L5 强抹平脱敏。
          </p>
        </div>
        <div className="flex items-center gap-3">
          {agentUrl && (
            <span className="hidden items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-600 sm:inline-flex">
              <span className="h-2 w-2 rounded-full bg-emerald-500" />
              Agent: {agentUrl}
            </span>
          )}
          <button
            onClick={handleExecute}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-lg bg-teal-600 px-4 py-2 text-sm font-semibold text-white shadow transition-all hover:bg-teal-700 disabled:opacity-50"
          >
            {loading ? (
              <span className="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent" />
            ) : (
              <Icon name="refresh" className="h-4 w-4" />
            )}
            重新加载并治理 data1.csv
          </button>
        </div>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="mt-4 rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">
          <strong>治理任务执行失败:</strong> {error}
        </div>
      )}

      {/* 统计指标面板 */}
      {result && (
        <div className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
          <div className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
            <p className="text-xs font-medium text-gray-500">总处理记录数</p>
            <p className="mt-1 text-2xl font-extrabold text-gray-900">{result.summary.total_records}</p>
          </div>
          <div className="rounded-xl border border-red-200 bg-red-50/50 p-4 shadow-sm">
            <p className="text-xs font-medium text-red-600">L5 级极高风险记录</p>
            <p className="mt-1 text-2xl font-extrabold text-red-700">{result.summary.l5_records_count}</p>
          </div>
          <div className="rounded-xl border border-orange-200 bg-orange-50/50 p-4 shadow-sm">
            <p className="text-xs font-medium text-orange-600">L4 级高风险记录</p>
            <p className="mt-1 text-2xl font-extrabold text-orange-700">{result.summary.l4_records_count}</p>
          </div>
          <div className="rounded-xl border border-blue-200 bg-blue-50/50 p-4 shadow-sm">
            <p className="text-xs font-medium text-blue-600">单条脱敏 PII 字段</p>
            <p className="mt-1 text-2xl font-extrabold text-blue-700">{result.summary.sanitized_pii_fields_per_record} 列</p>
          </div>
          <div className="rounded-xl border border-emerald-200 bg-emerald-50/50 p-4 shadow-sm col-span-2">
            <p className="text-xs font-medium text-emerald-700">合规保障承诺</p>
            <p className="mt-1 flex items-center gap-1.5 text-sm font-bold text-emerald-800">
              <Icon name="check" className="h-4 w-4 text-emerald-600" />
              100% 抹平 L4/L5 原始高危病史词汇 (耗时 {result.summary.duration_ms} ms)
            </p>
          </div>
        </div>
      )}

      {/* 视图 Tab 切换与数据内容区 */}
      {result && (
        <div className="mt-6 flex flex-1 flex-col rounded-xl border border-gray-200 bg-white shadow-sm overflow-hidden">
          <div className="flex border-b border-gray-200 bg-gray-50/80 px-4 pt-3">
            <button
              onClick={() => setActiveTab('report')}
              className={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-semibold transition-colors ${
                activeTab === 'report'
                  ? 'border-teal-600 text-teal-700 bg-white rounded-t-lg shadow-sm'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              <Icon name="activity" className="h-4 w-4" />
              1. 数据分类分级报告 ({result.classification_report.length} 条)
            </button>
            <button
              onClick={() => setActiveTab('sanitized')}
              className={`flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-semibold transition-colors ${
                activeTab === 'sanitized'
                  ? 'border-teal-600 text-teal-700 bg-white rounded-t-lg shadow-sm'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              <Icon name="shield" className="h-4 w-4" />
              2. 脱敏清洗合规数据 (无 L4/L5 泄露)
            </button>
          </div>

          {/* Tab 1: 分类分级报告 */}
          {activeTab === 'report' && (
            <div className="flex-1 overflow-y-auto p-4">
              <div className="flex flex-col gap-3">
                {result.classification_report.map((rep: MedicalRecordReport) => {
                  const isExpanded = expandedRecord === rep.record_index;
                  return (
                    <div
                      key={rep.record_index}
                      className="rounded-xl border border-gray-200 bg-white transition-all hover:border-gray-300"
                    >
                      <div
                        onClick={() => setExpandedRecord(isExpanded ? null : rep.record_index)}
                        className="flex cursor-pointer items-center justify-between p-4"
                      >
                        <div className="flex items-center gap-3">
                          <span className="text-sm font-bold text-gray-700">记录 #{rep.record_index}</span>
                          <span className={`inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-bold ${levelBadgeCls(rep.max_level)}`}>
                            {rep.max_level} 级风险
                          </span>
                          <div className="flex flex-wrap gap-1.5">
                            {rep.high_sensitivity_detected.map((tag, idx) => (
                              <span key={idx} className="rounded bg-red-50 px-2 py-0.5 text-xs font-medium text-red-600 border border-red-100">
                                {tag}
                              </span>
                            ))}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-gray-400">
                            {rep.field_details.length} 个字段详情
                          </span>
                          <span className="text-gray-400">{isExpanded ? '▲' : '▼'}</span>
                        </div>
                      </div>

                      {isExpanded && (
                        <div className="border-t border-gray-100 bg-gray-50/50 p-4">
                          <div className="overflow-x-auto">
                            <table className="min-w-full divide-y divide-gray-200 text-left text-xs">
                              <thead className="bg-gray-100/70 text-gray-600">
                                <tr>
                                  <th className="px-3 py-2">字段名</th>
                                  <th className="px-3 py-2">等级</th>
                                  <th className="px-3 py-2">安全标签</th>
                                  <th className="px-3 py-2">命中的治理规则</th>
                                  <th className="px-3 py-2">字段描述</th>
                                </tr>
                              </thead>
                              <tbody className="divide-y divide-gray-100 bg-white">
                                {rep.field_details.map((fd, idx) => (
                                  <tr key={idx} className="hover:bg-gray-50">
                                    <td className="px-3 py-2 font-mono font-bold text-gray-800">{fd.field_name}</td>
                                    <td className="px-3 py-2">
                                      <span className={`inline-block rounded px-2 py-0.5 text-[11px] font-bold border ${levelBadgeCls(fd.level)}`}>
                                        {fd.level}
                                      </span>
                                    </td>
                                    <td className="px-3 py-2 font-mono text-gray-600">{fd.security_tag}</td>
                                    <td className="px-3 py-2 text-indigo-600 font-medium">{fd.rule_matched}</td>
                                    <td className="px-3 py-2 text-gray-500">{fd.description}</td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* Tab 2: 脱敏清洗合规数据 */}
          {activeTab === 'sanitized' && (
            <div className="flex-1 overflow-auto p-4">
              <div className="overflow-x-auto rounded-lg border border-gray-200 shadow-inner">
                <table className="min-w-full divide-y divide-gray-200 text-left text-xs">
                  <thead className="bg-gray-100 text-gray-700 sticky top-0">
                    <tr>
                      <th className="px-3 py-2.5 font-bold">#</th>
                      {Object.keys(result.sanitized_data[0] || {}).map((col) => (
                        <th key={col} className="px-3 py-2.5 font-mono font-semibold whitespace-nowrap">
                          {col}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 bg-white">
                    {result.sanitized_data.map((row, idx) => (
                      <tr key={idx} className="hover:bg-gray-50">
                        <td className="px-3 py-2 font-bold text-gray-400">{idx + 1}</td>
                        {Object.entries(row).map(([col, val]) => {
                          const isPii = ['name', 'id_card_no', 'registered_address', 'disability_cert_no', 'medical_insurance_no'].includes(col);
                          const isMaskedMed = val.includes('[L5-') || val.includes('[L4-');
                          return (
                            <td key={col} className="px-3 py-2 whitespace-nowrap">
                              {isPii ? (
                                <span className="rounded bg-emerald-50 px-2 py-0.5 font-mono text-emerald-700 border border-emerald-200">
                                  {val}
                                </span>
                              ) : isMaskedMed ? (
                                <span className="rounded bg-purple-50 px-2 py-0.5 text-purple-700 border border-purple-200 font-medium">
                                  {val}
                                </span>
                              ) : (
                                <span className="text-gray-700">{val}</span>
                              )}
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
