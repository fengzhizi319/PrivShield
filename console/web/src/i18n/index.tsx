/**
 * Lightweight i18n context: provides zh/en switching without external dependencies.
 *
 * Usage:
 *   const { t, lang, setLang } = useI18n();
 *   <span>{t('header.health_ok')}</span>
 */
import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

export type Lang = 'zh' | 'en';

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

  // App
  'app.loading': '加载接口列表…',
  'app.connect_failed': '无法连接后端 {0}',
  'app.retry': '重试',
};

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

  // App
  'app.loading': 'Loading endpoints…',
  'app.connect_failed': 'Cannot connect to backend {0}',
  'app.retry': 'Retry',
};

const dictionaries: Record<Lang, Record<string, string>> = { zh, en };

interface I18nContextValue {
  lang: Lang;
  setLang: (l: Lang) => void;
  /** Translate a key with optional positional placeholders {0}, {1}, ... */
  t: (key: string, ...args: (string | number)[]) => string;
}

const I18nContext = createContext<I18nContextValue>({
  lang: 'zh',
  setLang: () => {},
  t: (key) => key,
});

function getInitialLang(): Lang {
  try {
    const stored = localStorage.getItem('console-lang');
    if (stored === 'zh' || stored === 'en') return stored;
  } catch { /* ignore */ }
  return 'zh';
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(getInitialLang);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try { localStorage.setItem('console-lang', l); } catch { /* ignore */ }
  }, []);

  const t = useCallback(
    (key: string, ...args: (string | number)[]) => {
      let text = dictionaries[lang][key] ?? key;
      args.forEach((arg, i) => {
        text = text.replace(`{${i}}`, String(arg));
      });
      return text;
    },
    [lang],
  );

  return <I18nContext.Provider value={{ lang, setLang, t }}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  return useContext(I18nContext);
}
