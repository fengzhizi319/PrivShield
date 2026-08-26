/**
 * TestRunnerPanel — E2E 测试套件运行器大屏。
 *
 * 功能概述：
 *  1. 展示可用测试套件（TS-01 审计验真 / TS-02 高并发压测 / TS-03 租约争抢）
 *  2. 支持全选/反选、指定并发数和压测请求量
 *  3. 执行测试套件并展示结果（通过率、断言详情、终端日志流）
 *  4. 支持导出 Markdown 测试报告
 *
 * 数据来源：
 *  - suites: App.tsx 中 fetchSuites() 拉取
 *  - 执行结果通过 onRunSuites 回调获取
 *
 * 状态管理：
 *  - selectedIds: 选中的测试套件 ID 列表
 *  - concurrency: TS-02 并发数（默认 20）
 *  - benchmarkRequests: TS-02 压测请求数（默认 50）
 *  - lastRun: 最近一次执行结果
 *  - activeLogs: 终端日志流
 */
import React, { useState } from 'react';
import { TestSuiteCase, RunTestSuiteRequest, RunTestSuiteResponse } from '../types/api';
import { useI18n } from '../i18n';
import {
  IconPlay,
  IconCheckCircle,
  IconXCircle,
  IconTerminal,
  IconRefresh,
  IconActivity,
} from './icons';

/** TestRunnerPanel 组件的 Props */
interface TestRunnerPanelProps {
  /** 可用测试套件定义列表 */
  suites: TestSuiteCase[];
  /** 执行测试套件回调（由 App.tsx 提供） */
  onRunSuites: (req: RunTestSuiteRequest) => Promise<RunTestSuiteResponse>;
  /** 是否正在执行中 */
  loading: boolean;
}

export const TestRunnerPanel: React.FC<TestRunnerPanelProps> = ({
  suites,
  onRunSuites,
  loading,
}) => {
  const { t } = useI18n();

  const [selectedIds, setSelectedIds] = useState<string[]>(['TS-01', 'TS-02', 'TS-03']);
  const [concurrency, setConcurrency] = useState(20);
  const [benchmarkRequests, setBenchmarkRequests] = useState(50);
  const [lastRun, setLastRun] = useState<RunTestSuiteResponse | null>(null);
  const [activeLogs, setActiveLogs] = useState<string[]>([]);
  const [selectedSuiteId, setSelectedSuiteId] = useState<string | null>(null);

  const handleToggleSelect = (id: string) => {
    if (selectedIds.includes(id)) {
      setSelectedIds(selectedIds.filter((item) => item !== id));
    } else {
      setSelectedIds([...selectedIds, id]);
    }
  };

  const handleSelectAll = () => {
    if (selectedIds.length === suites.length) {
      setSelectedIds([]);
    } else {
      setSelectedIds(suites.map((s) => s.id));
    }
  };

  const handleRun = async (targetIds?: string[]) => {
    const idsToRun = targetIds || selectedIds;
    if (idsToRun.length === 0) {
      alert('请至少选择一个测试用例！');
      return;
    }

    try {
      const resp = await onRunSuites({
        suite_ids: idsToRun,
        concurrency,
        benchmark_requests: benchmarkRequests,
      });
      setLastRun(resp);

      // Collect logs
      const combinedLogs: string[] = [];
      resp.results?.forEach((r) => {
        if (r.logs && r.logs.length > 0) {
          combinedLogs.push(...r.logs);
        }
      });
      setActiveLogs(combinedLogs);
    } catch (err: any) {
      alert(`测试执行异常: ${err.message}`);
    }
  };

  const handleExportMarkdown = () => {
    if (!lastRun) {
      alert('请先运行测试套件！');
      return;
    }

    let md = `# PrivShield Service Hub E2E 测试套件执行报告\n\n`;
    md += `- **运行 ID**: \`${lastRun.run_id}\`\n`;
    md += `- **执行时间**: \`${lastRun.started_at}\`\n`;
    md += `- **用例总数**: ${lastRun.total_cases} (通过: ${lastRun.passed_cases}, 失败: ${lastRun.failed_cases})\n\n`;
    md += `## 测试用例结果\n\n`;

    lastRun.results?.forEach((r) => {
      md += `### [${r.status === 'passed' ? 'PASS' : 'FAIL'}] ${r.id}: ${r.title}\n`;
      md += `- **耗时**: ${r.duration_ms} ms\n`;
      md += `- **分类**: ${r.category}\n`;
      md += `#### 断言结果:\n`;
      r.assertions?.forEach((a) => {
        md += `  - [${a.passed ? 'x' : ' '}] **${a.name}**: 期望=\`${a.expected}\`, 实际=\`${a.actual}\`\n`;
      });
      md += `\n`;
    });

    const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `PrivShield_E2E_TestReport_${Date.now()}.md`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const displaySuites = lastRun?.results && lastRun.results.length > 0 ? lastRun.results : suites;

  return (
    <div className="space-y-6">
      {/* Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
              <IconPlay className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('runner.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('runner.desc')}</p>
        </div>

        <div className="flex items-center gap-3">
          {lastRun && (
            <button
              onClick={handleExportMarkdown}
              className="px-3.5 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold border border-slate-700 transition"
            >
              {t('runner.exportReport')}
            </button>
          )}

          <button
            onClick={() => handleRun()}
            disabled={loading}
            className="flex items-center gap-2 px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-bold text-sm transition-all shadow-lg shadow-indigo-600/30 disabled:opacity-50"
          >
            {loading ? (
              <>
                <IconRefresh className="w-4 h-4 animate-spin" />
                <span>{t('runner.running')}</span>
              </>
            ) : (
              <>
                <IconPlay className="w-4 h-4" />
                <span>{t('runner.runAll')}</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Benchmark Config Strip */}
      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-4 flex flex-wrap items-center justify-between gap-4 text-xs">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <span className="text-slate-400">{t('runner.concurrency')}:</span>
            <input
              type="number"
              value={concurrency}
              onChange={(e) => setConcurrency(Number(e.target.value))}
              min={1}
              max={100}
              className="w-16 bg-slate-950 border border-slate-800 rounded px-2 py-1 text-slate-200 font-mono text-center focus:outline-none focus:border-indigo-500"
            />
          </div>

          <div className="flex items-center gap-2">
            <span className="text-slate-400">{t('runner.benchRequests')}:</span>
            <input
              type="number"
              value={benchmarkRequests}
              onChange={(e) => setBenchmarkRequests(Number(e.target.value))}
              min={10}
              max={1000}
              className="w-20 bg-slate-950 border border-slate-800 rounded px-2 py-1 text-slate-200 font-mono text-center focus:outline-none focus:border-indigo-500"
            />
          </div>
        </div>

        {lastRun && (
          <div className="flex items-center gap-3">
            <span className="text-slate-400">测试通过率:</span>
            <span className="font-bold text-emerald-400 font-mono text-sm">
              {lastRun.passed_cases} / {lastRun.total_cases} (
              {lastRun.total_cases > 0
                ? ((lastRun.passed_cases / lastRun.total_cases) * 100).toFixed(0) + '%'
                : 'N/A'})
            </span>
          </div>
        )}
      </div>

      {/* Test Suites List */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        {/* Left: Suite Cards */}
        <div className="lg:col-span-7 space-y-3">
          <div className="flex items-center justify-between text-xs text-slate-400 px-1">
            <span>用例清单 (TS-01 / TS-02 / TS-03)</span>
            <button onClick={handleSelectAll} className="text-indigo-400 hover:underline">
              {selectedIds.length === suites.length ? '取消全选' : '全选'}
            </button>
          </div>

          {displaySuites.map((suite) => {
            const suiteID = suite.id;
            const isSelected = selectedIds.includes(suiteID);
            const isPassed = suite.status === 'passed';
            const isFailed = suite.status === 'failed';

            return (
              <div
                key={suiteID}
                onClick={() => setSelectedSuiteId(suiteID)}
                className={`cursor-pointer p-4 rounded-xl border transition-all duration-200 ${
                  selectedSuiteId === suiteID
                    ? 'bg-slate-800/80 border-indigo-500 ring-1 ring-indigo-500/40'
                    : 'bg-slate-900 border-slate-800 hover:border-slate-700'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-start gap-3">
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={(e) => {
                        e.stopPropagation();
                        handleToggleSelect(suiteID);
                      }}
                      className="mt-1 w-4 h-4 accent-indigo-600 rounded cursor-pointer"
                    />
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs font-bold px-2 py-0.5 rounded bg-slate-800 text-indigo-400 border border-slate-700">
                          {suiteID}
                        </span>
                        <h3 className="text-sm font-bold text-slate-100">{suite.title}</h3>
                      </div>
                      <p className="text-xs text-slate-400 mt-1">{suite.description}</p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    {isPassed && (
                      <span className="flex items-center gap-1 text-xs font-bold text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 rounded-full">
                        <IconCheckCircle className="w-3.5 h-3.5" />
                        PASS
                      </span>
                    )}
                    {isFailed && (
                      <span className="flex items-center gap-1 text-xs font-bold text-rose-400 bg-rose-500/10 border border-rose-500/20 px-2 py-0.5 rounded-full">
                        <IconXCircle className="w-3.5 h-3.5" />
                        FAIL
                      </span>
                    )}
                    {suite.status === 'running' && (
                      <span className="text-xs text-indigo-400 animate-pulse">Running...</span>
                    )}
                    {suite.status === 'pending' && (
                      <span className="text-xs text-slate-500">Pending</span>
                    )}
                  </div>
                </div>

                {suite.duration_ms > 0 && (
                  <div className="mt-3 pt-2 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-500 font-mono">
                    <span>分类: {suite.category}</span>
                    <span>耗时: {suite.duration_ms.toFixed(2)} ms</span>
                  </div>
                )}
              </div>
            );
          })}
        </div>

        {/* Right: Logs & Assertions Drawer */}
        <div className="lg:col-span-5 bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl flex flex-col justify-between space-y-4">
          <div>
            <div className="flex items-center gap-2 text-sm font-bold text-slate-100 border-b border-slate-800 pb-3 mb-3">
              <IconTerminal className="w-4 h-4 text-emerald-400" />
              <span>{t('runner.terminalLogs')}</span>
            </div>

            <div className="bg-slate-950 p-4 rounded-xl border border-slate-800/90 font-mono text-xs text-emerald-400/90 h-80 overflow-y-auto space-y-1">
              {activeLogs.length > 0 ? (
                activeLogs.map((lg, i) => (
                  <div key={i} className="leading-relaxed">
                    {lg}
                  </div>
                ))
              ) : (
                <div className="text-slate-600 text-center py-20">
                  点击“一键执行全部套件”启动测试并查看实时流式输出
                </div>
              )}
            </div>
          </div>

          {selectedSuiteId && (
            <div className="bg-slate-950 p-3 rounded-xl border border-slate-800 text-xs space-y-2">
              <div className="font-bold text-slate-200">{selectedSuiteId} 断言明细:</div>
              {displaySuites
                .find((s) => s.id === selectedSuiteId)
                ?.assertions?.map((a, idx) => (
                  <div key={idx} className="flex items-start justify-between gap-2 p-1.5 rounded bg-slate-900">
                    <span className="text-slate-300 font-mono">{a.name}</span>
                    <span className={`font-bold font-mono ${a.passed ? 'text-emerald-400' : 'text-rose-400'}`}>
                      {a.passed ? '✓ PASSED' : '✗ FAILED'}
                    </span>
                  </div>
                ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
