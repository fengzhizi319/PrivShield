/**
 * 轻量级国际化 (i18n) 上下文 / Lightweight Internationalization (i18n) Context
 *
 * 自建的中英文切换方案，无外部依赖（不用 react-i18next / react-intl），
 * 通过 React Context + useState 实现，支持占位符替换与 localStorage 持久化。
 * Self-built zh/en switching solution with no external dependencies (no react-i18next / react-intl),
 * implemented via React Context + useState, supports placeholder replacement and localStorage persistence.
 *
 * 使用方式 / Usage：
 *   const { t, lang, setLang } = useI18n();
 *   <span>{t('header.health_ok')}</span>
 *   <span>{t('batch.summary', 10, 8, 2)}</span>  // 占位符 {0},{1},{2}
 *
 * 架构设计 / Architecture：
 *   - I18nProvider 包裹应用根部，提供 lang/setLang/t 三个值；
 *   - useI18n() Hook 在任意组件中获取翻译函数；
 *   - 语言偏好保存在 localStorage('console-lang')，刷新后保持。
 *   - I18nProvider wraps app root, provides lang/setLang/t values;
 *   - useI18n() Hook retrieves translation function in any component;
 *   - Language preference persisted in localStorage('console-lang'), survives refresh.
 */

/** 引入 React Context 相关 API / Import React Context related APIs */
import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

/** 支持的语言类型 / Supported language type */
export type Lang = 'zh' | 'en';

/**
 * 中文字典 / Chinese Dictionary
 *
 * 键为翻译 key（按组件分组的 dot notation），值为中文文本。
 * 支持 {0}, {1}, ... 占位符，由 t() 函数在运行时替换。
 * Key is translation key (dot notation grouped by component), value is Chinese text.
 * Supports {0}, {1}, ... placeholders, replaced at runtime by t() function.
 */
const zh: Record<string, string> = {
  // Header
  'header.detecting': '检测中…',
  'header.agent_ok': 'Agent 正常',
  'header.agent_down': 'Agent 不可达',
  'header.back_home': '返回总览',

  // Sidebar
  'sidebar.search_placeholder': '搜索接口…',
  'sidebar.overview': '接口总览',
  'sidebar.batch_test': '批量测试',
  'sidebar.file_test': '文件处理',
  'sidebar.lb_test': '负载均衡',
  'sidebar.dyn_classify': '通用动态分类分级',
  'sidebar.ops': '运维诊断',
  'sidebar.no_match': '未找到匹配的接口',

  // Overview
  'overview.title': '接口总览',
  'overview.subtitle': '共 {0} 个接口 · {1} 个功能模块，点击卡片开始测试',
  'overview.enter_test': '进入测试',
  'overview.more': '+{0} 个更多…',

  // EndpointView
  'endpoint.back': '返回总览',
  'endpoint.request_body': '请求体',
  'endpoint.format': '格式化',
  'endpoint.curl_copied': '已复制',
  'endpoint.history': '历史',
  'endpoint.reload_sample': '重载示例',
  'endpoint.sending': '发送中…',
  'endpoint.send': '发送请求',
  'endpoint.get_no_body': 'GET 请求无需请求体',
  'endpoint.json_parse_error': '请求体 JSON 解析错误：{0}',
  'endpoint.json_format_error': 'JSON 格式错误：{0}',
  'endpoint.content_type_hint': 'Content-Type: {0}（二进制载荷由后端处理）',

  // ResponsePanel
  'response.empty': '发送请求后在此查看响应',
  'response.failed': '请求失败',
  'response.copy': '复制',
  'response.copied': '已复制',
  'response.download': '下载',

  // BatchTest
  'batch.title': '批量测试',
  'batch.subtitle': '一键顺序调用所选分类下的全部接口，快速回归验证。单个失败不会中断整个批次。',
  'batch.scope': '测试范围',
  'batch.all_categories': '全部分类（{0} 个接口）',
  'batch.running': '测试中…',
  'batch.start': '开始测试（{0}）',
  'batch.all_passed': '全部通过',
  'batch.n_failed': '{0} 个失败',
  'batch.summary': '共 {0} · 通过 {1} · 失败 {2}',
  'batch.col_status': '状态',
  'batch.col_endpoint': '接口',
  'batch.col_duration': '耗时',
  'batch.col_info': '信息',
  'batch.empty_hint': '选择范围后点击"开始测试"',

  // HistoryPanel
  'history.title': '请求历史（{0}）',
  'history.clear': '清空',
  'history.close': '关闭',
  'history.empty': '暂无历史记录',
  'history.body_empty': '(空)',

  // LbTest
  'lb.title': '负载均衡测试',
  'lb.subtitle': '配置多个后端地址，按策略分发探测请求并对比各节点表现。',
  'lb.backends': '后端节点',
  'lb.add_node': '添加节点',
  'lb.name_placeholder': '名称',
  'lb.num_requests': '探测请求数',
  'lb.strategy': '分发策略',
  'lb.strategy_round_robin': '轮询 (round_robin)',
  'lb.strategy_random': '随机 (random)',
  'lb.strategy_least_conn': '最少连接 (least_connections)',
  'lb.running': '测试中…',
  'lb.run': '运行测试',
  'lb.empty_hint': '运行测试后在此查看各节点分发结果',
  'lb.total_requests': '总请求',
  'lb.success': '成功',
  'lb.failed': '失败',
  'lb.total_duration': '总耗时',
  'lb.col_node': '节点',
  'lb.col_distribution': '命中分布',
  'lb.col_hits': '命中数',
  'lb.col_success_rate': '成功率',
  'lb.col_avg_latency': '平均延迟',
  'lb.col_min_max_latency': '最小/最大延迟',
  'lb.at_least_one': '请至少填写一个后端地址',

  // OpsPanel（运维诊断）
  'ops.title': '运维诊断',
  'ops.subtitle': '链路排障 · 引擎降级状态 · 依赖与模型检测 · 硬件加速',
  'ops.refresh': '刷新诊断',
  'ops.refreshing': '诊断中…',
  'ops.loading': '正在采集诊断信息…',
  'ops.copy': '复制命令',
  'ops.updated_at': '诊断时间：',
  'ops.chain.title': '链路诊断（问题出在哪一层？）',
  'ops.chain.frontend': '前端',
  'ops.chain.backend': '控制台后端',
  'ops.chain.agent': 'Agent',
  'ops.chain.frontend_hint': '页面正常渲染',
  'ops.chain.unknown': '未知',
  'ops.chain.no_data': '暂无数据',
  'ops.chain.down': '不可达',
  'ops.engines.title': '分类引擎状态（NER / LLM 降级到哪了？）',
  'ops.engines.ner': 'NER 引擎（Layer-2）',
  'ops.engines.llm': 'LLM 引擎（Layer-3）',
  'ops.engines.active': '当前激活',
  'ops.engines.degraded': '已降级跳过',
  'ops.engines.unavailable': '不可用',
  'ops.engines.fallback': '备选（未到达）',
  'ops.engines.runtime': '运行时',
  'ops.engines.not_initialized': '尚未初始化（无分类请求）',
  'ops.engines.auto_probe': '自动探测',
  'ops.engines.probe_detail': '查看动态探测详情（实际尝试初始化各引擎）',
  'ops.engines.llm_ok': '可用',
  'ops.engines.backend': '后端',
  'ops.engines.model': '模型',
  'ops.deps.title': '依赖与驱动（是否安装 / 如何安装）',
  'ops.deps.col_name': '依赖',
  'ops.deps.col_status': '状态',
  'ops.deps.col_version': '版本',
  'ops.deps.col_purpose': '用途',
  'ops.deps.col_install': '安装命令',
  'ops.deps.installed': '已安装',
  'ops.deps.missing': '未安装',
  'ops.models.title': '模型文件',
  'ops.models.col_name': '模型',
  'ops.models.col_path': '路径',
  'ops.models.col_status': '状态',
  'ops.models.col_download': '下载命令',
  'ops.models.exists': '已就位',
  'ops.models.missing': '缺失',
  'ops.hardware.title': '硬件加速',
  'ops.hardware.cuda': 'CUDA',
  'ops.hardware.on': '可用',
  'ops.hardware.off': '不可用',
  'ops.hardware.unknown': '未知（torch 未加载）',
  'ops.hardware.nvidia_smi': 'nvidia-smi',
  'ops.hardware.found': '已找到',
  'ops.hardware.not_found': '未找到',
  'ops.hardware.platform': '运行平台',

  // App
  'app.loading': '加载接口列表…',
  'app.connect_failed': '无法连接后端 {0}',
  'app.retry': '重试',
};

/**
 * 英文字典 / English Dictionary
 *
 * 与中文字典一一对应，键相同，值为英文文本。
 * Corresponds one-to-one with Chinese dictionary, same keys, English values.
 */
const en: Record<string, string> = {
  // Header
  'header.detecting': 'Checking…',
  'header.agent_ok': 'Agent OK',
  'header.agent_down': 'Agent Unreachable',
  'header.back_home': 'Back to Overview',

  // Sidebar
  'sidebar.search_placeholder': 'Search endpoints…',
  'sidebar.overview': 'Overview',
  'sidebar.batch_test': 'Batch Test',
  'sidebar.file_test': 'File Test',
  'sidebar.lb_test': 'Load Balancer',
  'sidebar.dyn_classify': 'Dynamic Classification',
  'sidebar.ops': 'Ops Diagnostics',
  'sidebar.no_match': 'No matching endpoints',

  // Overview
  'overview.title': 'API Overview',
  'overview.subtitle': '{0} endpoints · {1} modules, click a card to start testing',
  'overview.enter_test': 'Enter Test',
  'overview.more': '+{0} more…',

  // EndpointView
  'endpoint.back': 'Back to Overview',
  'endpoint.request_body': 'Request Body',
  'endpoint.format': 'Format',
  'endpoint.curl_copied': 'Copied',
  'endpoint.history': 'History',
  'endpoint.reload_sample': 'Reload Sample',
  'endpoint.sending': 'Sending…',
  'endpoint.send': 'Send Request',
  'endpoint.get_no_body': 'GET requests have no body',
  'endpoint.json_parse_error': 'Request body JSON parse error: {0}',
  'endpoint.json_format_error': 'JSON format error: {0}',
  'endpoint.content_type_hint': 'Content-Type: {0} (binary payload handled by backend)',

  // ResponsePanel
  'response.empty': 'Send a request to view the response here',
  'response.failed': 'Request Failed',
  'response.copy': 'Copy',
  'response.copied': 'Copied',
  'response.download': 'Download',

  // BatchTest
  'batch.title': 'Batch Test',
  'batch.subtitle': 'Sequentially invoke all endpoints in the selected category for quick regression. A single failure won\'t abort the batch.',
  'batch.scope': 'Scope',
  'batch.all_categories': 'All Categories ({0} endpoints)',
  'batch.running': 'Testing…',
  'batch.start': 'Start Test ({0})',
  'batch.all_passed': 'All Passed',
  'batch.n_failed': '{0} Failed',
  'batch.summary': 'Total {0} · Passed {1} · Failed {2}',
  'batch.col_status': 'Status',
  'batch.col_endpoint': 'Endpoint',
  'batch.col_duration': 'Duration',
  'batch.col_info': 'Info',
  'batch.empty_hint': 'Select a scope and click "Start Test"',

  // HistoryPanel
  'history.title': 'Request History ({0})',
  'history.clear': 'Clear',
  'history.close': 'Close',
  'history.empty': 'No history yet',
  'history.body_empty': '(empty)',

  // LbTest
  'lb.title': 'Load Balancer Test',
  'lb.subtitle': 'Configure multiple backend addresses, distribute probe requests by strategy and compare node performance.',
  'lb.backends': 'Backend Nodes',
  'lb.add_node': 'Add Node',
  'lb.name_placeholder': 'Name',
  'lb.num_requests': 'Probe Requests',
  'lb.strategy': 'Strategy',
  'lb.strategy_round_robin': 'Round Robin',
  'lb.strategy_random': 'Random',
  'lb.strategy_least_conn': 'Least Connections',
  'lb.running': 'Testing…',
  'lb.run': 'Run Test',
  'lb.empty_hint': 'Run the test to view distribution results here',
  'lb.total_requests': 'Total',
  'lb.success': 'Success',
  'lb.failed': 'Failed',
  'lb.total_duration': 'Duration',
  'lb.col_node': 'Node',
  'lb.col_distribution': 'Distribution',
  'lb.col_hits': 'Hits',
  'lb.col_success_rate': 'Success Rate',
  'lb.col_avg_latency': 'Avg Latency',
  'lb.col_min_max_latency': 'Min/Max Latency',
  'lb.at_least_one': 'Please provide at least one backend address',

  // OpsPanel (Operations Diagnostics)
  'ops.title': 'Ops Diagnostics',
  'ops.subtitle': 'Chain troubleshooting · Engine degradation · Dependency & model checks · Hardware acceleration',
  'ops.refresh': 'Refresh',
  'ops.refreshing': 'Diagnosing…',
  'ops.loading': 'Collecting diagnostics…',
  'ops.copy': 'Copy command',
  'ops.updated_at': 'Diagnosed at:',
  'ops.chain.title': 'Chain Diagnosis (which layer fails?)',
  'ops.chain.frontend': 'Frontend',
  'ops.chain.backend': 'Console Backend',
  'ops.chain.agent': 'Agent',
  'ops.chain.frontend_hint': 'Page renders normally',
  'ops.chain.unknown': 'Unknown',
  'ops.chain.no_data': 'No data',
  'ops.chain.down': 'Unreachable',
  'ops.engines.title': 'Classification Engines (where did NER / LLM degrade to?)',
  'ops.engines.ner': 'NER Engine (Layer-2)',
  'ops.engines.llm': 'LLM Engine (Layer-3)',
  'ops.engines.active': 'Active',
  'ops.engines.degraded': 'Degraded past',
  'ops.engines.unavailable': 'Unavailable',
  'ops.engines.fallback': 'Fallback (unreached)',
  'ops.engines.runtime': 'Runtime',
  'ops.engines.not_initialized': 'Not initialized (no classify request yet)',
  'ops.engines.auto_probe': 'Auto Probe',
  'ops.engines.probe_detail': 'View dynamic probe details (actual engine initialization attempts)',
  'ops.engines.llm_ok': 'Available',
  'ops.engines.backend': 'Backend',
  'ops.engines.model': 'Model',
  'ops.deps.title': 'Dependencies & Drivers (installed? how to install?)',
  'ops.deps.col_name': 'Dependency',
  'ops.deps.col_status': 'Status',
  'ops.deps.col_version': 'Version',
  'ops.deps.col_purpose': 'Purpose',
  'ops.deps.col_install': 'Install Command',
  'ops.deps.installed': 'Installed',
  'ops.deps.missing': 'Missing',
  'ops.models.title': 'Model Files',
  'ops.models.col_name': 'Model',
  'ops.models.col_path': 'Path',
  'ops.models.col_status': 'Status',
  'ops.models.col_download': 'Download Command',
  'ops.models.exists': 'Present',
  'ops.models.missing': 'Missing',
  'ops.hardware.title': 'Hardware Acceleration',
  'ops.hardware.cuda': 'CUDA',
  'ops.hardware.on': 'Available',
  'ops.hardware.off': 'Unavailable',
  'ops.hardware.unknown': 'Unknown (torch not loaded)',
  'ops.hardware.nvidia_smi': 'nvidia-smi',
  'ops.hardware.found': 'Found',
  'ops.hardware.not_found': 'Not found',
  'ops.hardware.platform': 'Platform',

  // App
  'app.loading': 'Loading endpoints…',
  'app.connect_failed': 'Cannot connect to backend {0}',
  'app.retry': 'Retry',
};

/** 双语字典映射：语言代码 → 字典 / Bilingual dictionary mapping: language code → dictionary */
const dictionaries: Record<Lang, Record<string, string>> = { zh, en };

/**
 * i18n 上下文值接口 / i18n Context Value Interface
 *
 * 通过 React Context 向下传递的语言服务能力。
 * Language service capabilities passed down via React Context.
 */
interface I18nContextValue {
  /** 当前语言 / Current language */
  lang: Lang;
  /** 切换语言（同时持久化到 localStorage）/ Switch language (also persists to localStorage) */
  setLang: (l: Lang) => void;
  /** 翻译函数：根据 key 查找当前语言文本，并替换 {0},{1},... 占位符 / Translation function: looks up current language text by key, replaces {0},{1},... placeholders */
  t: (key: string, ...args: (string | number)[]) => string;
}

/**
 * 创建 i18n Context（默认值：中文 + 空操作 + 原样返回 key）
 * Create i18n Context (default: Chinese + noop + return key as-is)
 */
const I18nContext = createContext<I18nContextValue>({
  lang: 'zh',          // 默认中文 / Default Chinese
  setLang: () => {},   // 空操作（Provider 外调用时无效）/ Noop (ineffective outside Provider)
  t: (key) => key,     // 原样返回 key（Provider 外调用时的回退）/ Return key as-is (fallback outside Provider)
});

/**
 * 获取初始语言偏好 / Get Initial Language Preference
 *
 * 优先从 localStorage 读取，无效时默认中文。
 * Reads from localStorage first, defaults to Chinese when invalid.
 */
function getInitialLang(): Lang {
  try {
    const stored = localStorage.getItem('console-lang'); // 读取存储 / Read stored value
    if (stored === 'zh' || stored === 'en') return stored; // 有效值直接返回 / Return valid value directly
  } catch { /* 忽略 localStorage 不可用（如隐私模式）/ Ignore localStorage unavailable (e.g. private mode) */ }
  return 'zh'; // 默认中文 / Default Chinese
}

/**
 * i18n 提供者组件 / i18n Provider Component
 *
 * 包裹应用根部，向下提供 lang/setLang/t 三个值。
 * Wraps app root, provides lang/setLang/t values downward.
 *
 * @param children - 子组件 / Child components
 */
export function I18nProvider({ children }: { children: ReactNode }) {
  /** 语言状态（初始值从 localStorage 读取）/ Language state (initial value read from localStorage) */
  const [lang, setLangState] = useState<Lang>(getInitialLang);

  /**
   * 切换语言并持久化 / Switch language and persist
   *
   * 使用 useCallback 避免每次渲染创建新函数引用。
   * Uses useCallback to avoid creating new function reference each render.
   */
  const setLang = useCallback((l: Lang) => {
    setLangState(l); // 更新状态 / Update state
    try { localStorage.setItem('console-lang', l); } catch { /* 忽略存储失败 / Ignore storage failure */ }
  }, []);

  /**
   * 翻译函数 / Translation Function
   *
   * 详细逻辑 / Detailed Logic：
   *   1. 从当前语言字典中查找 key 对应的文本，未找到时回退为 key 本身；
   *   2. 遍历 args，将文本中的 {0}, {1}, ... 替换为对应参数值。
   *   1. Looks up key in current language dictionary, falls back to key itself if not found;
   *   2. Iterates args, replaces {0}, {1}, ... in text with corresponding argument values.
   */
  const t = useCallback(
    (key: string, ...args: (string | number)[]) => {
      let text = dictionaries[lang][key] ?? key; // 查找翻译，回退为 key / Look up translation, fallback to key
      args.forEach((arg, i) => {
        text = text.replace(`{${i}}`, String(arg)); // 替换占位符 / Replace placeholder
      });
      return text;
    },
    [lang], // 仅语言变化时重建 / Rebuild only when language changes
  );

  /* 通过 Context.Provider 向下传递语言服务 / Pass language service down via Context.Provider */
  return <I18nContext.Provider value={{ lang, setLang, t }}>{children}</I18nContext.Provider>;
}

/**
 * i18n Hook：在任意组件中获取翻译服务 / i18n Hook: Get translation service in any component
 *
 * @returns { lang, setLang, t } 语言状态、切换函数、翻译函数 / Language state, switch function, translation function
 */
export function useI18n() {
  return useContext(I18nContext); // 读取最近的 I18nProvider / Read nearest I18nProvider
}
