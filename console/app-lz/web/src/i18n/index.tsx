import React, { createContext, useContext, useState, ReactNode } from 'react';

export type Language = 'zh-CN' | 'en-US';

export interface I18nContextType {
  lang: Language;
  setLang: (lang: Language) => void;
  t: (key: string) => string;
}

const translations: Record<Language, Record<string, string>> = {
  'zh-CN': {
    // App Header & Nav
    'app.title': '数盾 · 调度之眼',
    'app.subtitle': '数据服务调度中枢 (Service Hub) 全景测试与治理工作台',
    'nav.topology': '四服务集群拓扑',
    'nav.pipeline': '6阶段流水线大屏',
    'nav.tasks': '任务生命周期与租约',
    'nav.runner': '自动化测试套件',
    'nav.datasources': '数据源资产探查',
    'nav.audit': '不可篡改审计验真',
    'nav.metrics': '实时性能与分位数',

    // Topology
    'topo.title': '四微服务网格拓扑与健康矩阵',
    'topo.desc': '实时探测固定 4 微服务节点（1. 调度中枢 ➔ 2. 隐私与分类引擎 ➔ 3. 数据源管理 ➔ 4. 脱敏审计日志）连通性与延时。',
    'topo.refresh': '并发探测全集群',
    'topo.probing': '探测中...',
    'topo.allHealthy': '集群全部就绪',
    'topo.degraded': '集群部分降级',
    'topo.rtt': '往返延时 RTT',
    'topo.status': '状态',
    'topo.addr': '地址与端口',
    'topo.protocol.title': '通信协议通道选择',
    'topo.protocol.rest': 'REST (HTTP / JSON)',
    'topo.protocol.grpc': 'gRPC (mTLS / Protobuf)',
    'topo.protocol.restDesc': 'HTTP/1.1 REST JSON 接口，明文/标准 JSON 格式，供 Web 控制台、第三方前端与常规系统接入。',
    'topo.protocol.grpcDesc': 'HTTP/2 gRPC 二进制高性能接口，支持 mTLS 双向证书鉴权、SPKI 公钥固定与微服务高吞吐编排。',
    'topo.fixedOrder': '四服务固定全景顺序: 1. 调度中枢 ➔ 2. 隐私与分类引擎 ➔ 3. 数据源管理 ➔ 4. 脱敏审计日志',

    // Pipeline Visualizer
    'pipe.title': '6 阶段流水线动态流转大屏',
    'pipe.desc': '可视化观测数据在 Ingest ➔ Fetch ➔ Classify ➔ Desensitize ➔ Return ➔ Audit 间的实时流转与脱敏前后比对。',
    'pipe.stage.ingest': '1. 任务解析 (Ingest)',
    'pipe.stage.fetch': '2. 切片抽取 (Fetch)',
    'pipe.stage.classify': '3. 智能评级 (Classify)',
    'pipe.stage.desensitize': '4. 隐私治理 (Desensitize)',
    'pipe.stage.return': '5. 结果装配 (Return)',
    'pipe.stage.audit': '6. 存证验真 (Audit)',
    'pipe.dispatch': '提交调度任务',
    'pipe.source': '数据源标识',
    'pipe.operation': '脱敏原语',
    'pipe.payload': '输入测试数据 (JSON)',
    'pipe.submitting': '提交中...',
    'pipe.rawInput': '原始明文输入',
    'pipe.sanitizedOutput': '脱敏合规输出',
    'pipe.diff': '数据穿透前后对比',
    'pipe.loadPreset': '加载预设数据',
    'pipe.preset.yibao': '医保报销结算数据',
    'pipe.preset.kangyang': '智慧康养生命体征',

    // Tasks & Leases
    'tasks.title': '任务全生命周期与 Phase B 租约看板',
    'tasks.desc': '按状态过滤检索历史任务，并对 PostgreSQL 多副本 FOR UPDATE SKIP LOCKED 原子租约进行深度观测。',
    'tasks.filter.all': '全部状态',
    'tasks.filter.pending': '等待中 (Pending)',
    'tasks.filter.running': '运行中 (Running)',
    'tasks.filter.completed': '已完成 (Completed)',
    'tasks.filter.failed': '失败 (Failed)',
    'tasks.id': '任务唯一 ID',
    'tasks.stage': '当前阶段',
    'tasks.duration': '耗时 (ms)',
    'tasks.created': '创建时间',
    'tasks.leaseOwner': '租约持有 Worker',
    'tasks.leaseExpiry': '租约剩余 (s)',
    'tasks.viewDetail': '查看详情',
    'tasks.leaseTitle': 'Phase B PostgreSQL 原子租约争抢视图',
    'tasks.leaseDesc': '展示多 Worker 节点在并发认领任务时的行锁状态与孤儿任务自愈回收。',

    // Test Suite Runner
    'runner.title': '一键全场景自动化测试执行器 (TS-01 ~ TS-07)',
    'runner.desc': '全面执行涵盖基础分发、自适应分类、数据源联动、审计验真、熔断恢复、高并发压测与租约争抢的全量测试用例。',
    'runner.runAll': '一键执行全部套件',
    'runner.runSelected': '执行选应用例',
    'runner.running': '测试执行中...',
    'runner.exportReport': '导出测试报告 (Markdown)',
    'runner.concurrency': '并发协程数',
    'runner.benchRequests': '压测总请求量',
    'runner.passRate': '测试通过率',
    'runner.assertions': '断言详情',
    'runner.terminalLogs': '实时执行日志流',

    // Datasource Explorer
    'ds.title': '模拟数据源资产探查与切片采样',
    'ds.desc': '直连 datasource-mgr 查看医保与康养资产画像，在线抽取切片并一键派发至调度中枢。',
    'ds.sampleSlice': '抽取数据切片',
    'ds.dispatchSlice': '派发至调度流水线',
    'ds.recordCount': '总记录数',
    'ds.fields': '字段列表',

    // Audit Log & Merkle
    'audit.title': '不可篡改脱敏审计存证与 Merkle 验真',
    'audit.desc': '直连 audit-log 校验流水线产生的脱敏存证记录，在线触发 Merkle Tree 链式防篡改验真。',
    'audit.verifyBtn': '触发 Merkle 树完整性验真',
    'audit.verifying': '验真中...',
    'audit.merkleValid': 'Merkle 链完整有效 (未被篡改)',
    'audit.rootHash': 'Merkle Root Hash',
    'audit.totalEntries': '存证总笔数',
    'audit.signature': '防篡改数字签名',

    // Metrics
    'metrics.title': '实时性能指标与分位数监控',
    'metrics.desc': '监控 service-hub 的实时 QPS、6 阶段耗时瀑布图与 P50 / P90 / P95 / P99 延迟分位数。',
    'metrics.qps': '实时调度 QPS',
    'metrics.waterfall': '6 阶段平均耗时瀑布图 (ms)',
    'metrics.p50Desc': '50% 的请求耗时低于该值，代表中位数典型体验',
    'metrics.p90Desc': '90% 的请求耗时低于该值，反映大多数用户实际延迟',
    'metrics.p95Desc': '95% 的请求耗时低于该值，核心 SLA 达标基准线',
    'metrics.p99Desc': '99% 的请求耗时低于该值，排查极端长尾与垃圾回收停顿',
  },
  'en-US': {
    // App Header & Nav
    'app.title': 'PrivShield · Eye of Hub (App-LZ)',
    'app.subtitle': 'Service Hub E2E Testing, Full-Chain Observability & Mesh Governance Console',
    'nav.topology': 'Cluster Topology',
    'nav.pipeline': '6-Stage Pipeline',
    'nav.tasks': 'Task & Lease',
    'nav.runner': 'E2E Test Suites',
    'nav.datasources': 'Datasource Explorer',
    'nav.audit': 'Audit & Merkle',
    'nav.metrics': 'Metrics & Percentiles',

    // Topology
    'topo.title': '4-Microservice Topology & Health Matrix',
    'topo.desc': 'Real-time probe of fixed 4-service mesh (1. Service Hub ➔ 2. PrivShield Agent ➔ 3. Datasource Mgr ➔ 4. Audit Log).',
    'topo.refresh': 'Probe All Services',
    'topo.probing': 'Probing...',
    'topo.allHealthy': 'All Services Ready',
    'topo.degraded': 'Cluster Degraded',
    'topo.rtt': 'Round-Trip RTT',
    'topo.status': 'Status',
    'topo.addr': 'Address & Port',
    'topo.protocol.title': 'Protocol Channel Selection',
    'topo.protocol.rest': 'REST (HTTP / JSON)',
    'topo.protocol.grpc': 'gRPC (mTLS / Protobuf)',
    'topo.protocol.restDesc': 'HTTP/1.1 REST JSON endpoints for Web Consoles, standard client integrations, and ease of inspection.',
    'topo.protocol.grpcDesc': 'HTTP/2 gRPC binary high-throughput endpoints with mTLS mutual certificate authentication and SPKI pinning.',
    'topo.fixedOrder': 'Fixed Service Order: 1. Service Hub ➔ 2. PrivShield Agent ➔ 3. Datasource Mgr ➔ 4. Audit Log',

    // Pipeline Visualizer
    'pipe.title': '6-Stage Pipeline Live Visualizer',
    'pipe.desc': 'Live data flow animation across Ingest ➔ Fetch ➔ Classify ➔ Desensitize ➔ Return ➔ Audit with dual-pane diff.',
    'pipe.stage.ingest': '1. Ingest & Parse',
    'pipe.stage.fetch': '2. Slice Fetch',
    'pipe.stage.classify': '3. Funnel Classify',
    'pipe.stage.desensitize': '4. Desensitize',
    'pipe.stage.return': '5. Assemble & Return',
    'pipe.stage.audit': '6. Audit Log',
    'pipe.dispatch': 'Dispatch Pipeline Task',
    'pipe.source': 'Datasource ID',
    'pipe.operation': 'Privacy Primitive',
    'pipe.payload': 'Input Payload (JSON)',
    'pipe.submitting': 'Dispatching...',
    'pipe.rawInput': 'Raw Input Payload',
    'pipe.sanitizedOutput': 'Sanitized Compliant Output',
    'pipe.diff': 'Data Transformation Diff',
    'pipe.loadPreset': 'Load Sample Data',
    'pipe.preset.yibao': 'Medical Insurance Settlement',
    'pipe.preset.kangyang': 'Elderly Healthcare Vitals',

    // Tasks & Leases
    'tasks.title': 'Task Lifecycle & Phase B Lease Inspector',
    'tasks.desc': 'Filter historical tasks by status and observe PostgreSQL multi-worker FOR UPDATE SKIP LOCKED atomic leases.',
    'tasks.filter.all': 'All Statuses',
    'tasks.filter.pending': 'Pending',
    'tasks.filter.running': 'Running',
    'tasks.filter.completed': 'Completed',
    'tasks.filter.failed': 'Failed',
    'tasks.id': 'Task ID',
    'tasks.stage': 'Stage',
    'tasks.duration': 'Duration (ms)',
    'tasks.created': 'Created At',
    'tasks.leaseOwner': 'Lease Worker',
    'tasks.leaseExpiry': 'Lease TTL (s)',
    'tasks.viewDetail': 'View Details',
    'tasks.leaseTitle': 'Phase B PostgreSQL Atomic Lease Inspector',
    'tasks.leaseDesc': 'Live display of worker task claims, row-level locks, and orphan lease reclamation.',

    // Test Suite Runner
    'runner.title': 'One-Click E2E Test Suite Runner (TS-01 ~ TS-07)',
    'runner.desc': 'Execute all test scenarios covering dispatch, classification, datasource fetch, audit verification, circuit breaking, concurrency, and atomic leases.',
    'runner.runAll': 'Run All Test Suites',
    'runner.runSelected': 'Run Selected Suites',
    'runner.running': 'Executing Suites...',
    'runner.exportReport': 'Export Markdown Report',
    'runner.concurrency': 'Concurrency Workers',
    'runner.benchRequests': 'Total Benchmark Requests',
    'runner.passRate': 'Pass Rate',
    'runner.assertions': 'Assertions',
    'runner.terminalLogs': 'Terminal Execution Logs',

    // Datasource Explorer
    'ds.title': 'Simulated Datasource Explorer & Slice Sampler',
    'ds.desc': 'Direct connection to datasource-mgr for dataset metadata and slice sampling with one-click pipeline dispatch.',
    'ds.sampleSlice': 'Fetch Data Slice',
    'ds.dispatchSlice': 'Dispatch Slice to Pipeline',
    'ds.recordCount': 'Total Records',
    'ds.fields': 'Field Schema',

    // Audit Log & Merkle
    'audit.title': 'Immutable Audit Log & Merkle Verification',
    'audit.desc': 'Inspect SHA-256 audit entries and trigger on-demand Merkle Tree verification for tamper-proof compliance.',
    'audit.verifyBtn': 'Verify Merkle Integrity',
    'audit.verifying': 'Verifying...',
    'audit.merkleValid': 'Merkle Tree Valid (Tamper-Free)',
    'audit.rootHash': 'Merkle Root Hash',
    'audit.totalEntries': 'Total Entries',
    'audit.signature': 'Digital Signature',

    // Metrics
    'metrics.title': 'Live Performance Metrics & Latency Percentiles',
    'metrics.desc': 'Monitor real-time QPS, 6-stage duration breakdown waterfall, and P50 / P90 / P95 / P99 latency percentiles.',
    'metrics.qps': 'Real-time QPS',
    'metrics.waterfall': '6-Stage Duration Waterfall (ms)',
    'metrics.p50Desc': '50% of requests complete within this time (median experience)',
    'metrics.p90Desc': '90% of requests complete within this time (majority experience)',
    'metrics.p95Desc': '95% of requests complete within this time (core SLA baseline)',
    'metrics.p99Desc': '99% of requests complete within this time (tail latency & GC pauses)',
  },
};

const I18nContext = createContext<I18nContextType | null>(null);

export const I18nProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [lang, setLang] = useState<Language>('zh-CN');

  const t = (key: string): string => {
    return translations[lang]?.[key] || translations['zh-CN']?.[key] || key;
  };

  return (
    <I18nContext.Provider value={{ lang, setLang, t }}>
      {children}
    </I18nContext.Provider>
  );
};

export const useI18n = (): I18nContextType => {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n must be used within an I18nProvider');
  }
  return ctx;
};
