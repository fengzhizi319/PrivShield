/**
 * 前后端数据契约（TypeScript 类型定义）。
 *
 * 本文件与后端 Pydantic 模型一一对应，是前后端的“单一事实来源”：
 *   - 字段命名保持一致（示例用 camelCase，代理转发用 snake_case）；
 *   - 修改任何接口时，需同步更新本文件与后端模型。
 */

/** 单个端点示例（来自后端 /api/samples）。 */
export interface EndpointSample {
  method: string;
  path: string;
  /** UI 展示的简短名称 */
  label: string;
  /** 功能分类（侧边栏分组依据，如 Masking / DP） */
  category: string;
  /** 中文功能描述 */
  description: string;
  /** 默认 JSON 请求体 */
  body?: Record<string, any> | null;
  /** 二进制载荷的 Content-Type（如 Arrow IPC） */
  contentType?: string | null;
  /** 二进制载荷的 base64 编码 */
  rawPayloadB64?: string | null;
  /** 可用性标识：rest 仅 Python 后端，both 两后端都支持 */
  backend?: "rest" | "grpc" | "both";
}

/** 通用代理请求体（发往 /api/proxy）。 */
export interface ProxyRequest {
  method: string;
  path: string;
  body?: Record<string, any> | null;
  raw_payload_b64?: string | null;
  content_type?: string | null;
}

/** 通用代理统一响应包装。 */
export interface ProxyResponse {
  status: number;
  /** 转发耗时（毫秒） */
  duration_ms: number;
  data: any;
  /** 处理本请求的控制台后端标识（python-rest / go-grpc） */
  via?: string;
  /** 该后端与 agent 的通信协议（REST / gRPC） */
  protocol?: string;
}

/** 后端健康检查响应（/api/health）。 */
export interface ConsoleHealth {
  backend: string;
  /** agent 健康信息；不可达时为字符串 "unreachable" */
  agent: string | Record<string, any>;
  agent_url: string;
  latency_ms?: number;
  error?: string;
  /** 处理本请求的控制台后端标识（python-rest / go-grpc） */
  via?: string;
  /** 该后端与 agent 的通信协议（REST / gRPC） */
  protocol?: string;
}

/** 批量测试：单个请求项。 */
export interface BatchRequestItem {
  method: string;
  path: string;
  body?: Record<string, any> | null;
}

/** 批量测试：单个结果项。 */
export interface BatchResultItem {
  method: string;
  path: string;
  status: number;
  duration_ms: number;
  data?: any;
  error?: string | null;
}

/** 批量测试：汇总响应。 */
export interface BatchResponse {
  total: number;
  passed: number;
  failed: number;
  results: BatchResultItem[];
  /** 处理本请求的控制台后端标识（python-rest / go-grpc） */
  via?: string;
  /** 该后端与 agent 的通信协议（REST / gRPC） */
  protocol?: string;
}

/** 请求历史记录（存于 localStorage）。 */
export interface HistoryEntry {
  id: string;
  method: string;
  path: string;
  /** 请求体 JSON 文本 */
  body: string;
  /** 响应状态码（0 表示网络错误） */
  status: number;
  timestamp: number;
}

/** 数据文件隐私处理支持的操作类型。 */
export type FileOperation = 'mask_dataframe' | 'k_anonymize';

/** /api/upload 响应中 data 的处理结果。 */
export interface UploadData {
  operation: FileOperation;
  /** 输入记录数 */
  rows_in: number;
  /** 输出记录数 */
  rows_out: number;
  /** 处理结果：脱敏/K-匿名为记录数组，分类为结果对象 */
  result: any;
}

/** /api/upload 的统一响应包装（复用 ProxyResponse 结构）。 */
export interface UploadResponse {
  status: number;
  duration_ms: number;
  data: UploadData;
  /** 处理本请求的控制台后端标识（python-rest / go-grpc） */
  via?: string;
  /** 该后端与 agent 的通信协议（REST / gRPC） */
  protocol?: string;
}

/** 负载均衡测试：单个目标后端节点。 */
export interface LbBackend {
  name: string;
  url: string;
}

/** 负载均衡测试支持的策略。 */
export type LbStrategy = 'round_robin' | 'random' | 'least_connections';

/** 负载均衡测试请求体（发往 /api/lb_test）。 */
export interface LbTestRequest {
  backends: LbBackend[];
  num_requests: number;
  strategy: LbStrategy;
  /** 探测路径，默认 /health */
  probe_path?: string;
  /** 提供时以 POST 发送该 JSON 体，否则用 GET */
  probe_body?: Record<string, any> | null;
}

/** 负载均衡测试：单个节点的统计结果。 */
export interface LbDistItem {
  name: string;
  url: string;
  count: number;
  success: number;
  failed: number;
  avg_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
}

/** 负载均衡测试的汇总响应。 */
export interface LbTestResponse {
  strategy: LbStrategy;
  total: number;
  success: number;
  failed: number;
  duration_ms: number;
  distribution: LbDistItem[];
}

/* ==================== 运维诊断（/v1/ops/diagnostics） ==================== */

/** 单个依赖的安装检测结果。 */
export interface OpsDependency {
  name: string;
  installed: boolean;
  version: string | null;
  /** 该依赖的用途说明 */
  purpose: string;
  /** 安装命令提示 */
  install: string;
}

/** 单个模型文件的存在性检测结果。 */
export interface OpsModel {
  name: string;
  /** 相对项目根的路径 */
  path: string;
  exists: boolean;
  /** 下载命令提示 */
  download: string;
  /** 附加诊断备注（如孤儿 .onnx.data 说明）；无则为 null */
  note: string | null;
}

/** NER 降级链中的单个引擎。 */
export interface OpsNerEngine {
  engine: string;
  available: boolean;
  /** 不可用原因（可用时为 null） */
  reason: string | null;
  deps: string[];
  model: string;
  note: string;
  /** 动态探测时的实际错误信息（可选） */
  probe_error?: string;
}

/** 动态探测单个引擎结果。 */
export interface OpsProbeDetail {
  engine: string;
  ok: boolean;
  error: string | null;
}

/** NER 动态探测结果（与 tests/dynclassification 相同的判定方式）。 */
export interface OpsNerProbe {
  active_engine: string;
  available: boolean;
  details: OpsProbeDetail[];
}

/** NER 引擎降级链路状态。 */
export interface OpsNerStatus {
  /** 当前激活的引擎（自动判断结果），全不可用时为 "none" */
  active_engine: string;
  available: boolean;
  /** 判定来源：runtime（运行时已初始化）/ probe（动态探测） */
  determined_by: 'runtime' | 'probe';
  /** 动态探测详情（实际尝试初始化各引擎的结果） */
  probe: OpsNerProbe;
  /** 静态推断：当前依赖/模型条件下会激活的引擎 */
  predicted_engine: string;
  /** 运行时真实激活引擎（尚无分类请求时为 null） */
  runtime_engine: string | null;
  degradation_chain: OpsNerEngine[];
}

/** LLM 动态探测结果。 */
export interface OpsLlmProbe {
  available: boolean;
  error: string | null;
}

/** LLM 引擎状态。 */
export interface OpsLlmStatus {
  backend: string;
  available: boolean;
  /** 判定来源：runtime / probe */
  determined_by: 'runtime' | 'probe';
  /** 动态探测结果（实际尝试实例化 LlmAdapter） */
  probe: OpsLlmProbe;
  /** 运行时真实可用状态（尚未初始化时为 null） */
  runtime_available: boolean | null;
  deps: string[];
  deps_met: boolean;
  model: string;
  model_exists: boolean;
  reason: string | null;
  note: string;
}

/** 硬件加速（CUDA/GPU）状态。 */
export interface OpsHardware {
  platform: string;
  machine: string;
  nvidia_smi_found: boolean;
  /** CUDA 是否可用；torch 未加载时为 null */
  cuda_available: boolean | null;
  cuda_detail: string;
}

/** 运维诊断完整响应（Agent GET /v1/ops/diagnostics）。 */
export interface OpsDiagnostics {
  status: string;
  timestamp: string;
  service: {
    name: string;
    namespace: string;
    python_version: string;
    project_root: string;
    rest_port: number;
    grpc_port: number;
  };
  engines: {
    ner: OpsNerStatus;
    llm: OpsLlmStatus;
  };
  dependencies: OpsDependency[];
  models: OpsModel[];
  hardware: OpsHardware;
}

/* ==================== 动态分类分级标准（/v1/dynclassification/standards） ==================== */

/** 标准等级体系中的单个等级定义。 */
export interface StandardLevel {
  /** 等级 ID（如 L3 / C4 / G2） */
  id: string;
  /** 等级名称（如「敏感数据/第3级」） */
  name: string;
  /** 排序权重（越大越敏感） */
  rank: number;
}

/** 单个标准的详细信息（含等级体系，供标准切换器渲染）。 */
export interface StandardDetail {
  /** 标准标识（如 sc_health_db51 / jrt0197 / gd_health） */
  standard_id: string;
  /** 标准名称描述 */
  description: string;
  /** 引用的 taxonomy 名称 */
  taxonomy: string;
  /** 组合的领域包列表 */
  domains: string[];
  /** 默认等级（未命中任何规则时的兄底等级） */
  default_level: string;
  /** 等级体系（按 rank 升序排列） */
  levels: StandardLevel[];
}

/** 标准列表响应（GET /v1/dynclassification/standards）。 */
export interface StandardsResponse {
  /** 标准 ID 列表（向后兼容字段） */
  standards: string[];
  /** 标准详情列表（含等级体系） */
  details: StandardDetail[];
}
