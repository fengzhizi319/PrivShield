import {
  TopologyResponse,
  DispatchRequest,
  DispatchResponse,
  TasksResponse,
  Task,
  LeasedTasksResponse,
  TestSuiteCase,
  RunTestSuiteRequest,
  RunTestSuiteResponse,
  AuditLogItem,
  AuditVerifyResponse,
  DataApiDef,
  DataApiSessionResponse,
} from '../types/api';

const BASE_URL = '/api/lz';

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });
  if (!res.ok) {
    let errMsg = `HTTP Error ${res.status}`;
    try {
      const errBody = await res.json();
      if (errBody.error || errBody.detail) {
        errMsg = errBody.error || errBody.detail;
      }
    } catch {
      // ignore
    }
    throw new Error(errMsg);
  }
  return res.json() as Promise<T>;
}

export const api = {
  // 1. Topology & Mesh Health
  async getTopology(protocol: 'rest' | 'grpc' = 'rest'): Promise<TopologyResponse> {
    return fetchJSON<TopologyResponse>(`${BASE_URL}/topology?protocol=${protocol}`);
  },

  async probeAll(protocol: 'rest' | 'grpc' = 'rest'): Promise<TopologyResponse> {
    return fetchJSON<TopologyResponse>(`${BASE_URL}/probe/all?protocol=${protocol}`, { method: 'POST' });
  },

  // 2. Tasks & Leases
  async listTasks(status = '', limit = 50, offset = 0): Promise<TasksResponse> {
    const params = new URLSearchParams();
    if (status) params.set('status', status);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return fetchJSON<TasksResponse>(`${BASE_URL}/tasks?${params.toString()}`);
  },

  async getTask(id: string): Promise<Task> {
    return fetchJSON<Task>(`${BASE_URL}/tasks/${id}`);
  },

  async getLeases(): Promise<LeasedTasksResponse> {
    return fetchJSON<LeasedTasksResponse>(`${BASE_URL}/tasks/leases`);
  },

  async dispatchTask(req: DispatchRequest): Promise<DispatchResponse> {
    return fetchJSON<DispatchResponse>(`${BASE_URL}/tasks/dispatch`, {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  // 4. Test Suites Runner
  async getSuites(): Promise<{ suites: TestSuiteCase[] }> {
    return fetchJSON<{ suites: TestSuiteCase[] }>(`${BASE_URL}/suites`);
  },

  async runSuites(req: RunTestSuiteRequest): Promise<RunTestSuiteResponse> {
    return fetchJSON<RunTestSuiteResponse>(`${BASE_URL}/suites/run`, {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  // 5. Datasources (removed — use Data API instead)

  // 6. Audit & Merkle
  async getAuditLogs(limit = 50, offset = 0): Promise<{ logs: AuditLogItem[] }> {
    return fetchJSON<{ logs: AuditLogItem[] }>(`${BASE_URL}/audit/logs?limit=${limit}&offset=${offset}`);
  },

  async verifyAudit(): Promise<AuditVerifyResponse> {
    return fetchJSON<AuditVerifyResponse>(`${BASE_URL}/audit/verify`, { method: 'POST' });
  },

  // 7. Metrics
  async getMetrics(): Promise<string> {
    const res = await fetch(`${BASE_URL}/metrics`);
    return res.text();
  },

  async getParsedMetrics(): Promise<{
    stage_durations: Record<string, number>;
    qps: number;
    percentiles: Record<string, number>;
    total_requests: number;
    source: string;
  }> {
    return fetchJSON(`${BASE_URL}/metrics/parsed`);
  },

  // 8. Preset Data APIs (4 预设数据 API)
  async getDataApiDefinitions(): Promise<{ apis: DataApiDef[] }> {
    return fetchJSON<{ apis: DataApiDef[] }>(`${BASE_URL}/data-api/definitions`);
  },

  async invokeDataApi(apiId: number, limit = 5): Promise<DataApiSessionResponse> {
    return fetchJSON<DataApiSessionResponse>(`${BASE_URL}/data-api/invoke`, {
      method: 'POST',
      body: JSON.stringify({ api_id: apiId, limit }),
    });
  },
};
