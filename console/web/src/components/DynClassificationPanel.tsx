/**
 * 声明式通用动态分类分级面板 / Declarative Universal Dynamic Classification Panel
 *
 * 提供动态分类分级引擎的完整测试界面，包含五个 Tab：
 * Provides a complete test interface for the dynamic classification engine, with five tabs:
 *
 *   1. 字段动态评估 (Eval)：输入字段名/值/领域，获取分类结果（标准由顶部全局切换器控制）；
 *      Field Dynamic Evaluation: input field name/value/domain, get classification result (standard via the global switcher);
 *   2. 记录级分类 (Record)：输入整条 JSON 记录，对每个字段做分类；
 *      Record-level Classification: input full JSON record, classify each field;
 *   3. 标准文档一键生成配置 (Auto Generate)：从规范文档自动提取分类规则；
 *      Standard Doc Auto-generate Config: auto-extract classification rules from spec docs;
 *   4. 标准/领域/算子目录 (Directory)：查询系统已注册的标准、领域、算子；
 *      Standards/Domains/Operators Directory: query registered standards, domains, operators;
 *   5. 规则校验 (Validate)：校验当前 YAML 配置的完整性与一致性。
 *      Rule Validation: validate completeness and consistency of current YAML config.
 *
 * 所有请求均通过 proxyRequest 转发到后端 /v1/dynclassification/* 接口。
 * All requests are forwarded to backend /v1/dynclassification/* endpoints via proxyRequest.
 */

/** 引入 React 状态 Hook / Import React state Hook */
import { useEffect, useState } from 'react';
/** 引入图标组件 / Import icon component */
import { Icon } from '@/components/icons';
/** 引入代理请求 API / Import proxy request API */
import { proxyRequest, fetchStandards } from '@/api/client';
/** 引入标准详情类型 / Import standard detail type */
import type { StandardDetail } from '@/types/api';

/**
 * 动态分类分级主组件 / Dynamic Classification Main Component
 *
 * 通过 tab 状态切换五个功能面板，每个面板独立维护输入/输出/加载状态。
 * Switches between five functional panels via tab state, each panel independently maintains input/output/loading state.
 */
export default function DynClassificationPanel() {
  /** 当前活动 Tab / Currently active tab */
  const [tab, setTab] = useState<'eval' | 'record' | 'generate' | 'info' | 'validate'>('eval');

  /* ====== 全局标准切换器状态 / Global Standard Switcher State ====== */
  /** 后端返回的标准详情列表（含等级体系） / Standard details list from backend (incl. level systems) */
  const [standards, setStandards] = useState<StandardDetail[]>([]);
  /** 标准列表加载中 / Standards list loading */
  const [standardsLoading, setStandardsLoading] = useState(true);
  /** 当前选中的标准 ID（空串 = 默认通用规则引擎） / Currently selected standard ID (empty = default engine) */
  const [currentStandard, setCurrentStandard] = useState('');

  /**
   * 面板挂载时拉取后端可用标准列表 / Fetch available standards from backend on mount
   *
   * 标准是分类分级的核心上下文：切换后所有评估请求（字段级/记录级）
   * 均携带新标准 ID，agent 侧随之加载对应的 taxonomy 与规则包，
   * 实现前端 → 控制台后端 → agent 全链路切换。
   * Standards are the core context of classification: after switching, all eval
   * requests carry the new standard ID and the agent loads the corresponding
   * taxonomy & rule packs, achieving full-chain switching.
   */
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const resp = await fetchStandards();
        if (!cancelled) setStandards(resp.details ?? []);
      } catch {
        /* 拉取失败不阻断面板使用，仅标准切换器为空 / Fetch failure doesn't block the panel */
      } finally {
        if (!cancelled) setStandardsLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  /** 当前选中标准的详情（未选中时为 null） / Details of the selected standard (null when none) */
  const currentDetail: StandardDetail | null =
    standards.find((s) => s.standard_id === currentStandard) ?? null;

  /**
   * 切换标准 / Switch standard
   *
   * 切换后清空已有评估结果，避免展示旧标准下的结论造成误导。
   * Clears existing eval results after switching to avoid showing stale conclusions.
   */
  const handleStandardChange = (id: string) => {
    setCurrentStandard(id);
    setEvalResult(null);
    setEvalError(null);
    setRecordResult(null);
    setRecordError(null);
  };

  /* ====== 字段动态评估 (Eval) 状态 / Field Dynamic Evaluation State ====== */
  const [fieldName, setFieldName] = useState('mobile_phone');   // 字段名 / Field name
  const [fieldValue, setFieldValue] = useState('13800138000');  // 字段值 / Field value
  const [domain, setDomain] = useState('');                     // 领域（可选，标准优先）/ Domain (optional, standard takes precedence)
  const [evalResult, setEvalResult] = useState<any>(null);      // 评估结果 / Evaluation result
  const [evalLoading, setEvalLoading] = useState(false);        // 加载中标记 / Loading flag
  const [evalError, setEvalError] = useState<string | null>(null); // 错误信息 / Error message

  /* ====== 标准文档生成配置 (Generate) 状态 / Standard Doc Generate Config State ====== */
  const [docPath, setDocPath] = useState('docs/standard/四川省健康医疗大数据应用指南.md'); // 文档路径 / Doc path
  const [genResult, setGenResult] = useState<any>(null);      // 生成结果 / Generation result
  const [genLoading, setGenLoading] = useState(false);        // 加载中标记 / Loading flag
  const [genError, setGenError] = useState<string | null>(null); // 错误信息 / Error message

  /* ====== 系统信息查询 (Info) 状态 / System Info Query State ====== */
  const [infoData, setInfoData] = useState<any>(null);    // 查询结果 / Query result
  const [infoLoading, setInfoLoading] = useState(false);  // 加载中标记 / Loading flag

  /* ====== 规则校验 (Validate) 状态 / Rule Validation State ====== */
  const [valResult, setValResult] = useState<any>(null);    // 校验结果 / Validation result
  const [valLoading, setValLoading] = useState(false);      // 加载中标记 / Loading flag

  /* ====== 记录级分类 (Record) 状态 / Record-level Classification State ====== */
  const [recordJson, setRecordJson] = useState('{"name": "张三", "id_card": "110101199001011237", "phone": "13800138000"}'); // JSON 记录 / JSON record
  const [recordDomain, setRecordDomain] = useState('');                    // 领域（可选）/ Domain (optional)
  const [recordResult, setRecordResult] = useState<any>(null);           // 分类结果 / Classification result
  const [recordLoading, setRecordLoading] = useState(false);             // 加载中标记 / Loading flag
  const [recordError, setRecordError] = useState<string | null>(null);   // 错误信息 / Error message

  /**
   * 执行字段动态评估 / Execute Field Dynamic Evaluation
   *
   * 组装 payload（fieldName/value/domain/standard），
   * POST 到 /v1/dynclassification/eval 获取分类结果。
   * Assembles payload (fieldName/value/domain/standard),
   * POSTs to /v1/dynclassification/eval to get classification result.
   */
  const handleEval = async () => {
    setEvalLoading(true);
    setEvalError(null);
    setEvalResult(null);
    try {
      const payload: any = { fieldName };
      if (fieldValue) payload.value = fieldValue;
      if (domain) payload.domain = domain;
      if (currentStandard) payload.standard = currentStandard;

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

  /**
   * 执行记录级分类 / Execute Record-level Classification
   *
   * 解析 JSON 记录，POST 到 /v1/dynclassification/eval_record，
   * 对记录中每个字段做分类分级。
   * Parses JSON record, POSTs to /v1/dynclassification/eval_record,
   * classifies each field in the record.
   */
  const handleRecordEval = async () => {
    setRecordLoading(true);
    setRecordError(null);
    setRecordResult(null);
    try {
      let record: any;
      try {
        record = JSON.parse(recordJson);
      } catch {
        setRecordError('JSON 格式错误，请检查输入');
        setRecordLoading(false);
        return;
      }
      const payload: any = { record };
      if (recordDomain) payload.domain = recordDomain;
      if (currentStandard) payload.standard = currentStandard;

      const res = await proxyRequest({
        method: 'POST',
        path: '/v1/dynclassification/eval_record',
        body: payload,
      });
      setRecordResult(res.data);
    } catch (e: any) {
      setRecordError(e.message || '记录级分类失败');
    } finally {
      setRecordLoading(false);
    }
  };

  /**
   * 执行标准文档自动生成配置 / Execute Standard Doc Auto-generate Config
   *
   * POST 到 /v1/dynclassification/generate_profile，
   * 从规范文档中自动提取分类规则并生成 YAML 配置。
   * POSTs to /v1/dynclassification/generate_profile,
   * auto-extracts classification rules from spec doc and generates YAML config.
   */
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

  /**
   * 查询系统信息（标准/领域/算子）/ Query System Info (Standards/Domains/Operators)
   *
   * GET 到 /v1/dynclassification/{type}，获取已注册的目录列表。
   * GETs /v1/dynclassification/{type}, retrieves registered directory list.
   *
   * @param type - 查询类型 / Query type
   */
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

  /**
   * 执行规则校验 / Execute Rule Validation
   *
   * POST 到 /v1/dynclassification/validate，
   * 校验当前 YAML 配置的完整性与一致性。
   * POSTs to /v1/dynclassification/validate,
   * validates completeness and consistency of current YAML config.
   */
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

        {/* 全局标准切换器：切换后所有评估请求携带新标准，agent 侧加载对应 taxonomy 与规则包 */}
        {/* Global standard switcher: after switching, all eval requests carry the new standard */}
        <div className="mt-4 flex flex-wrap items-center gap-3 rounded-xl border border-purple-100 bg-white/70 px-4 py-3">
          <span className="text-xs font-semibold text-gray-700">当前标准 (Standard)</span>
          <select
            value={currentStandard}
            onChange={(e) => handleStandardChange(e.target.value)}
            disabled={standardsLoading}
            className="rounded-lg border border-purple-200 bg-white px-3 py-1.5 text-sm text-gray-800 focus:border-purple-500 focus:outline-none disabled:opacity-50"
          >
            <option value="">{standardsLoading ? '加载中…' : '默认（通用规则引擎）'}</option>
            {standards.map((s) => (
              <option key={s.standard_id} value={s.standard_id}>
                {s.standard_id} — {s.description}
              </option>
            ))}
          </select>
          {currentDetail ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded-full bg-purple-100 px-2.5 py-0.5 text-xs font-semibold text-purple-700">
                {currentDetail.description}
              </span>
              {currentDetail.levels.map((lv) => (
                <span
                  key={lv.id}
                  title={lv.name}
                  className="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600"
                >
                  {lv.id}
                </span>
              ))}
              <span className="text-xs text-gray-400">默认等级: {currentDetail.default_level}</span>
            </div>
          ) : (
            !standardsLoading && (
              <span className="text-xs text-gray-400">使用通用规则引擎（未选择标准）</span>
            )
          )}
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
            onClick={() => setTab('record')}
            className={`border-b-2 px-4 py-2 text-sm font-medium transition-colors ${
              tab === 'record' ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            记录级分类 (Record)
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
                <div>
                  <label className="block text-xs font-medium text-gray-700">领域包 (domain, 可选)</label>
                  <input
                    type="text"
                    value={domain}
                    onChange={(e) => setDomain(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                    placeholder="general-pii / medical"
                  />
                  <p className="mt-1 text-xs text-gray-400">
                    分类标准由顶部切换器控制：{currentDetail ? `${currentDetail.standard_id}（${currentDetail.description}）` : '默认通用规则引擎'}
                  </p>
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

        {/* TAB 2: 记录级分类 */}
        {tab === 'record' && (
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
              <h2 className="mb-4 text-base font-semibold text-gray-800">记录级分类输入</h2>
              <div className="space-y-4">
                <div>
                  <label className="block text-xs font-medium text-gray-700">记录 JSON（字段名 → 值）</label>
                  <textarea
                    value={recordJson}
                    onChange={(e) => setRecordJson(e.target.value)}
                    rows={5}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs focus:border-purple-500 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-700">领域 (domain, 可选)</label>
                  <input
                    type="text"
                    value={recordDomain}
                    onChange={(e) => setRecordDomain(e.target.value)}
                    className="mt-1 w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-purple-500 focus:outline-none"
                    placeholder="general-pii / medical"
                  />
                  <p className="mt-1 text-xs text-gray-400">
                    分类标准由顶部切换器控制：{currentDetail ? `${currentDetail.standard_id}（${currentDetail.description}）` : '默认通用规则引擎'}
                  </p>
                </div>
                <button
                  onClick={handleRecordEval}
                  disabled={recordLoading}
                  className="w-full rounded-lg bg-purple-600 py-2.5 text-sm font-semibold text-white shadow-md shadow-purple-100 transition-colors hover:bg-purple-700 disabled:opacity-50"
                >
                  {recordLoading ? '分类计算中…' : '执行记录级分类'}
                </button>
              </div>
            </div>

            <div className="rounded-xl border border-gray-200 bg-gray-50 p-5 shadow-sm">
              <h2 className="mb-4 text-base font-semibold text-gray-800">记录级分类结果</h2>
              {recordError && <div className="rounded-lg bg-red-50 p-3 text-xs text-red-600">{recordError}</div>}
              {recordResult ? (
                <div className="space-y-4">
                  <div className="flex items-center justify-between rounded-lg bg-white p-4 shadow-sm">
                    <div>
                      <span className="text-xs text-gray-500">记录级最终等级</span>
                      <div className="mt-1 text-2xl font-black text-purple-700">
                        {recordResult.recordResult?.finalLevel || 'N/A'}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="text-xs text-gray-500">置信度 / 需人工复核</span>
                      <div className="mt-1 text-sm font-semibold text-gray-800">
                        {recordResult.recordResult?.confidence != null
                          ? `${Math.round(recordResult.recordResult.confidence * 100)}%`
                          : 'N/A'}
                        {recordResult.recordResult?.needsHumanReview ? ' ⚠️' : ''}
                      </div>
                    </div>
                  </div>
                  <div className="overflow-hidden rounded-lg border border-gray-200 bg-gray-900 text-xs text-green-400">
                    <pre className="max-h-72 overflow-auto p-4">{JSON.stringify(recordResult, null, 2)}</pre>
                  </div>
                </div>
              ) : (
                <div className="flex h-48 items-center justify-center text-xs text-gray-400">点击左侧“执行记录级分类”获取结果</div>
              )}
            </div>
          </div>
        )}

        {/* TAB 3: 自动生成配置 */}
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
