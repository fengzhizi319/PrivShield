import { useState } from 'react';
import { Icon } from '@/components/icons';
import { proxyRequest } from '@/api/client';

export default function DynClassificationPanel() {
  const [tab, setTab] = useState<'eval' | 'generate' | 'info' | 'validate'>('eval');

  // Eval 状态
  const [fieldName, setFieldName] = useState('mobile_phone');
  const [fieldValue, setFieldValue] = useState('13800138000');
  const [domain, setDomain] = useState('general-pii');
  const [standard, setStandard] = useState('');
  const [evalResult, setEvalResult] = useState<any>(null);
  const [evalLoading, setEvalLoading] = useState(false);
  const [evalError, setEvalError] = useState<string | null>(null);

  // Generate 状态
  const [docPath, setDocPath] = useState('docs/standard/四川省健康医疗大数据应用指南.md');
  const [genResult, setGenResult] = useState<any>(null);
  const [genLoading, setGenLoading] = useState(false);
  const [genError, setGenError] = useState<string | null>(null);

  // Info 状态 (Standards / Domains / Operators)
  const [infoData, setInfoData] = useState<any>(null);
  const [infoLoading, setInfoLoading] = useState(false);

  // Validate 状态
  const [valResult, setValResult] = useState<any>(null);
  const [valLoading, setValLoading] = useState(false);

  // 执行评估
  const handleEval = async () => {
    setEvalLoading(true);
    setEvalError(null);
    setEvalResult(null);
    try {
      const payload: any = { fieldName };
      if (fieldValue) payload.value = fieldValue;
      if (domain) payload.domain = domain;
      if (standard) payload.standard = standard;

      const res = await proxyRequest({
        method: 'POST',
        path: '/v1/dynclassification/eval',
        body: payload,
      });
      setEvalResult(res.data);
    } catch (e: any) {
      setEvalError(e.message || '评估失败');
    } finally {
      setEvalLoading(false);
    }
  };

  // 执行文档生成
  const handleGenerate = async () => {
    setGenLoading(true);
    setGenError(null);
    setGenResult(null);
    try {
      const res = await proxyRequest({
        method: 'POST',
        path: '/v1/dynclassification/generate_profile',
        body: { docPath },
      });
      setGenResult(res.data);
    } catch (e: any) {
      setGenError(e.message || '生成失败');
    } finally {
      setGenLoading(false);
    }
  };

  // 查询系统信息
  const handleFetchInfo = async (type: 'standards' | 'domains' | 'operators') => {
    setInfoLoading(true);
    try {
      const res = await proxyRequest({
        method: 'GET',
        path: `/v1/dynclassification/${type}`,
      });
      setInfoData(res.data);
    } catch (e: any) {
      setInfoData({ error: e.message });
    } finally {
      setInfoLoading(false);
    }
  };

  // 执行校验
  const handleValidate = async () => {
    setValLoading(true);
    try {
      const res = await proxyRequest({
        method: 'POST',
        path: '/v1/dynclassification/validate',
      });
      setValResult(res.data);
    } catch (e: any) {
      setValResult({ error: e.message });
    } finally {
      setValLoading(false);
    }
  };

  return (
    <div className="flex h-full flex-col bg-white">
      {/* 头部面板标题 */}
      <div className="border-b border-gray-100 bg-gradient-to-r from-purple-50 to-indigo-50 p-6">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-purple-600 text-white shadow-md shadow-purple-200">
            <Icon name="sparkles" className="h-6 w-6" />
          </div>
          <div>
            <h1 className="text-xl font-bold text-gray-900">声明式通用动态分类分级 (Dynamic Classification)</h1>
            <p className="text-xs text-gray-500">
              支持多领域、多行业标准（Sichuan/GD/Financial等）YAML 配置、开箱即用匹配算子与规范文档自动提取配置生成。
            </p>
          </div>
        </div>

        {/* Tab 导航切换 */}
        <div className="mt-6 flex border-b border-gray-200">
          <button
            onClick={() => setTab('eval')}
            className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === 'eval' ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            字段动态评估 (Eval)
          </button>
          <button
            onClick={() => setTab('generate')}
            className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === 'generate' ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            标准文档一键生成配置 (Auto Generate)
          </button>
          <button
            onClick={() => {
              setTab('info');
              handleFetchInfo('standards');
            }}
            className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === 'info' ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            标准/领域/算子目录 (Directory)
          </button>
          <button
            onClick={() => {
              setTab('validate');
              handleValidate();
            }}
            className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === 'validate' ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            规则在线校验 (Validate)
          </button>
        </div>
      </div>

      {/* 主体卡片区域 */}
      <div className="flex-1 overflow-y-auto p-6">
        {/* TAB 1: 字段评估 */}
        {tab === 'eval' && (
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
              <h2 className="mb-4 text-base font-semibold text-gray-800">评估参数输入</h2>
              <div className="space-y-4">
                <div>
                  <label className="block text-xs font-medium text-gray-700">字段名称 (fieldName)</label>
                  <input
                    type="text"
                    value={fieldName}
                    onChange={(e) => setFieldName(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                    placeholder="e.g. mobile_phone, patient_brca1_gene"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700">字段数值 (value, 可选)</label>
                  <input
                    type="text"
                    value={fieldValue}
                    onChange={(e) => setFieldValue(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                    placeholder="e.g. 13800138000, 110101199003072375"
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-gray-700">领域包 (domain)</label>
                    <input
                      type="text"
                      value={domain}
                      onChange={(e) => setDomain(e.target.value)}
                      className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                      placeholder="general-pii / medical"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-gray-700">特定标准 (standard, 优先)</label>
                    <input
                      type="text"
                      value={standard}
                      onChange={(e) => setStandard(e.target.value)}
                      className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                      placeholder="sc_health_db51 / jrt0197"
                    />
                  </div>
                </div>
                <button
                  onClick={handleEval}
                  disabled={evalLoading}
                  className="w-full rounded-lg bg-purple-600 py-2.5 text-sm font-semibold text-white shadow-md shadow-purple-100 transition-colors hover:bg-purple-700 disabled:opacity-50"
                >
                  {evalLoading ? '评估计算中…' : '执行动态分类评估'}
                </button>
              </div>
            </div>

            {/* 评估结果显示 */}
            <div className="rounded-xl border border-gray-200 bg-gray-50 p-5 shadow-sm">
              <h2 className="mb-4 text-base font-semibold text-gray-800">评估结果</h2>
              {evalError && <div className="rounded-lg bg-red-50 p-3 text-xs text-red-600">{evalError}</div>}
              {evalResult ? (
                <div className="space-y-4">
                  <div className="flex items-center justify-between rounded-lg bg-white p-4 shadow-sm">
                    <div>
                      <span className="text-xs text-gray-500">最终判定敏感等级</span>
                      <div className="mt-1 text-2xl font-black text-purple-700">
                        {evalResult.fieldResult?.finalLevel || 'L1'}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="text-xs text-gray-500">置信度 / 需人工复核</span>
                      <div className="mt-1 text-sm font-semibold text-gray-800">
                        {evalResult.fieldResult?.confidence != null
                          ? `${Math.round(evalResult.fieldResult.confidence * 100)}%`
                          : 'N/A'}
                      </div>
                    </div>
                  </div>

                  {/* 详细 JSON 结构 */}
                  <div className="overflow-hidden rounded-lg border border-gray-200 bg-gray-900 text-xs text-green-400">
                    <pre className="max-h-72 overflow-auto p-4">{JSON.stringify(evalResult, null, 2)}</pre>
                  </div>
                </div>
              ) : (
                <div className="flex h-48 items-center justify-center text-xs text-gray-400">点击左侧“执行动态分类评估”获取求值结果</div>
              )}
            </div>
          </div>
        )}

        {/* TAB 2: 自动生成配置 */}
        {tab === 'generate' && (
          <div className="max-w-3xl space-y-6">
            <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
              <h2 className="text-base font-semibold text-gray-800">标准 Markdown 规范文档一键提取 YAML 配置</h2>
              <p className="mt-1 text-xs text-gray-500">
                支持输入符合地方或行业标准的规范文档（如《四川省健康医疗大数据应用指南.md》），自动识别分级矩阵并提取 YAML 配置文件。
              </p>
              <div className="mt-4 space-y-4">
                <div>
                  <label className="block text-xs font-medium text-gray-700">文档文件路径 (docPath)</label>
                  <input
                    type="text"
                    value={docPath}
                    onChange={(e) => setDocPath(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                  />
                </div>
                <button
                  onClick={handleGenerate}
                  disabled={genLoading}
                  className="rounded-lg bg-purple-600 px-5 py-2 text-sm font-semibold text-white shadow-md transition-colors hover:bg-purple-700 disabled:opacity-50"
                >
                  {genLoading ? '解析抽取中…' : '一键自动生成全套 YAML 配置'}
                </button>
              </div>
            </div>

            {genError && <div className="rounded-lg bg-red-50 p-4 text-xs text-red-600">{genError}</div>}
            {genResult && (
              <div className="rounded-xl border border-green-200 bg-green-50 p-5">
                <h3 className="text-sm font-bold text-green-800">生成成功！</h3>
                <p className="mt-1 text-xs text-green-700">{genResult.message}</p>
                <div className="mt-3 overflow-hidden rounded-lg bg-gray-900 p-3 text-xs text-green-400">
                  <pre>{JSON.stringify(genResult.generated_files, null, 2)}</pre>
                </div>
              </div>
            )}
          </div>
        )}

        {/* TAB 3: 目录查询 */}
        {tab === 'info' && (
          <div className="space-y-4">
            <div className="flex gap-2">
              <button
                onClick={() => handleFetchInfo('standards')}
                className="rounded-lg bg-purple-50 px-3 py-1.5 text-xs font-semibold text-purple-700 hover:bg-purple-100"
              >
                可用标准 (Standards)
              </button>
              <button
                onClick={() => handleFetchInfo('domains')}
                className="rounded-lg bg-purple-50 px-3 py-1.5 text-xs font-semibold text-purple-700 hover:bg-purple-100"
              >
                领域匹配包 (Domains)
              </button>
              <button
                onClick={() => handleFetchInfo('operators')}
                className="rounded-lg bg-purple-50 px-3 py-1.5 text-xs font-semibold text-purple-700 hover:bg-purple-100"
              >
                注册算子库 (Operators)
              </button>
            </div>
            <div className="overflow-hidden rounded-xl border border-gray-200 bg-gray-900 p-4 text-xs text-green-400">
              {infoLoading ? <p>加载中…</p> : <pre>{JSON.stringify(infoData, null, 2)}</pre>}
            </div>
          </div>
        )}

        {/* TAB 4: 规则校验 */}
        {tab === 'validate' && (
          <div className="space-y-4">
            <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
              <div className="flex items-center justify-between">
                <div>
                  <h2 className="text-base font-semibold text-gray-800">规则 YAML 文件合法性在线校验</h2>
                  <p className="text-xs text-gray-500">检测算子未找到错误、语法错误与拼写模糊纠错提示。</p>
                </div>
                <button
                  onClick={handleValidate}
                  disabled={valLoading}
                  className="rounded-lg bg-purple-600 px-4 py-2 text-xs font-semibold text-white hover:bg-purple-700"
                >
                  重新校验
                </button>
              </div>
            </div>
            {valResult && (
              <div className="overflow-hidden rounded-xl border border-gray-200 bg-gray-900 p-4 text-xs text-green-400">
                <pre>{JSON.stringify(valResult, null, 2)}</pre>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
