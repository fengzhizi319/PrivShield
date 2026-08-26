export type ProtocolType = 'rest' | 'grpc';

export interface ServiceNode {
  id: string;
  name: string;
  http_url: string;
  grpc_addr: string;
  status: 'ready' | 'unhealthy' | 'unreachable';
  rtt_ms: number;
  rest_status?: 'ready' | 'unhealthy' | 'unreachable';
  rest_rtt_ms?: number;
  grpc_status?: 'ready' | 'unhealthy' | 'unreachable';
  grpc_rtt_ms?: number;
  protocol?: ProtocolType;
  version: string;
  details?: Record<string, any>;
  error?: string;
}

export interface TopologyResponse {
  status: string;
  active_protocol?: ProtocolType;
  timestamp: string;
  services: ServiceNode[];
}

export interface PipelineStage {
  name: 'ingest' | 'fetch' | 'classify' | 'desensitize' | 'return' | 'audit';
  title: string;
  status: 'idle' | 'processing' | 'error';
  active_count: number;
  avg_duration_ms: number;
}

export interface PipelineStatusResponse {
  stages: PipelineStage[];
  agent_connected: boolean;
  datasource_connected: boolean;
  audit_connected: boolean;
  qps: number;
  recent_tasks_count: number;
}

export interface DispatchRequest {
  source: string;
  operation: string;
  payload: Record<string, any>;
  priority: number;
}

export interface DispatchResponse {
  task_id: string;
  status: string;
  via?: string;
  error?: string;
}

export interface ClassifyDispatchRequest {
  source: string;
  payload: Record<string, any>;
  priority: number;
}

export interface ClassifyDispatchResponse {
  task_id: string;
  level: string;
  auto_operation: string;
  classify_result: Record<string, any>;
  via?: string;
  error?: string;
}

export interface TriggerDatasourceRequest {
  datasource_id: string;
  limit: number;
  operation: string;
}

export interface TriggerDatasourceResponse {
  task_id: string;
  datasource_id: string;
  records_count: number;
  operation: string;
  status: string;
  via?: string;
  error?: string;
}

export interface Task {
  id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  stage: string;
  source: string;
  operation: string;
  priority: number;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  duration_ms: number;
  error?: string;
  payload_json?: string;
  result_json?: string;
  retry_count: number;
  lease_owner?: string;
  lease_expires_at?: string;
  via?: string;
}

export interface TasksResponse {
  total: number;
  tasks: Task[];
  via?: string;
}

export interface LeasedTaskSummary {
  task_id: string;
  stage: string;
  priority: number;
  lease_expires_in_seconds: number;
}

export interface WorkerLeaseInfo {
  worker_id: string;
  claimed_tasks_count: number;
  tasks: LeasedTaskSummary[];
}

export interface LeasedTasksResponse {
  store_backend: string;
  total_leased_tasks: number;
  workers: WorkerLeaseInfo[];
  orphan_recovery: Record<string, any>;
}

export interface TestSuiteAssertion {
  name: string;
  expected: string;
  actual: string;
  passed: boolean;
}

export interface TestSuiteCase {
  id: string;
  title: string;
  description: string;
  category: string;
  status: 'pending' | 'running' | 'passed' | 'failed' | 'skipped';
  duration_ms: number;
  error?: string;
  assertions: TestSuiteAssertion[];
  logs: string[];
}

export interface RunTestSuiteRequest {
  suite_ids?: string[];
  concurrency?: number;
  benchmark_requests?: number;
}

export interface RunTestSuiteResponse {
  run_id: string;
  status: string;
  total_cases: number;
  passed_cases: number;
  failed_cases: number;
  started_at: string;
  completed_at?: string;
  results: TestSuiteCase[];
  summary?: Record<string, any>;
}

export interface Datasource {
  id: string;
  name: string;
  category: string;
  records_count: number;
  fields?: string[];
}

export interface DatasourceSliceResponse {
  datasource_id: string;
  count: number;
  total: number;
  records: Record<string, any>[];
}

export interface AuditLogItem {
  id: string;
  timestamp: string;
  task_id: string;
  source: string;
  operation: string;
  data_hash: string;
  operator: string;
  encryption: string;
  result: string;
}

export interface AuditVerifyResponse {
  merkle_valid: boolean;
  root_hash: string;
  total_entries: number;
  timestamp: string;
  signature?: string;
  error?: string;
}

// ---------------------------------------------------------------------------
// Preset Data API Session Types (4 预设数据 API)
// ---------------------------------------------------------------------------

export interface DataApiDef {
  id: number;
  name: string;
  datasource_id: string;
  category: string;
  description: string;
  fields: string[];
  status: 'active' | 'reserved';
}

export interface DataApiSessionStage {
  name: string;
  title: string;
  status: 'success' | 'error' | 'skipped';
  duration_ms: number;
  detail?: string;
}

export interface DataApiSessionResponse {
  session_id: string;
  api_id: number;
  api_name: string;
  status: 'completed' | 'partial' | 'failed' | 'skipped';
  raw_records: Record<string, any>[];
  sanitized_data: Record<string, any>[];
  stages: DataApiSessionStage[];
  audit_entry_id?: string;
  total_duration_ms: number;
  error?: string;
}
