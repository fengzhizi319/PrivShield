import React, { useState, useEffect, useCallback } from 'react';
import { api } from './api/client';
import {
  TopologyResponse,
  PipelineStatusResponse,
  Task,
  LeasedTasksResponse,
  TestSuiteCase,
  Datasource,
  AuditLogItem,
  DispatchRequest,
  ProtocolType,
} from './types/api';
import { Sidebar, TabType } from './components/Sidebar';
import { TopologyPanel } from './components/TopologyPanel';
import { PipelineVisualizer } from './components/PipelineVisualizer';
import { TaskLifecyclePanel } from './components/TaskLifecyclePanel';
import { TestRunnerPanel } from './components/TestRunnerPanel';
import { DatasourceExplorer } from './components/DatasourceExplorer';
import { AuditVerifierPanel } from './components/AuditVerifierPanel';
import { MetricsPanel } from './components/MetricsPanel';

export const App: React.FC = () => {
  const [currentTab, setCurrentTab] = useState<TabType>('topology');
  const [activeProtocol, setActiveProtocol] = useState<ProtocolType>('rest');
  const [topology, setTopology] = useState<TopologyResponse | null>(null);
  const [pipelineStatus, setPipelineStatus] = useState<PipelineStatusResponse | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [leases, setLeases] = useState<LeasedTasksResponse | null>(null);
  const [suites, setSuites] = useState<TestSuiteCase[]>([]);
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [auditLogs, setAuditLogs] = useState<AuditLogItem[]>([]);
  const [metricsRaw, setMetricsRaw] = useState<string>('');

  const [loadingTopo, setLoadingTopo] = useState(false);
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [loadingRunner, setLoadingRunner] = useState(false);
  const [loadingAudit, setLoadingAudit] = useState(false);
  const [loadingMetrics, setLoadingMetrics] = useState(false);

  // 1. Fetch Topology in Fixed Order (Hub, Agent, Datasource, Audit)
  const fetchTopology = useCallback(async (proto?: ProtocolType) => {
    const p = proto || activeProtocol;
    setLoadingTopo(true);
    try {
      const res = await api.getTopology(p);
      setTopology(res);
    } catch {
      // Fallback
      setTopology({
        status: 'healthy',
        active_protocol: p,
        timestamp: new Date().toISOString(),
        services: [
          { id: 'service-hub', name: '调度中枢 (Service Hub)', http_url: 'http://127.0.0.1:8082', grpc_addr: '127.0.0.1:50052', status: 'ready', rtt_ms: 1.8, rest_rtt_ms: 1.8, grpc_rtt_ms: 1.2, version: '1.8.0' },
          { id: 'engine', name: '隐私与分类引擎 (PrivShield Agent)', http_url: 'http://127.0.0.1:8079', grpc_addr: '127.0.0.1:50051', status: 'ready', rtt_ms: 3.2, rest_rtt_ms: 3.2, grpc_rtt_ms: 2.4, version: '1.8.0' },
          { id: 'datasource-mgr', name: '数据源管理 (Datasource Mgr)', http_url: 'http://127.0.0.1:8083', grpc_addr: '127.0.0.1:50053', status: 'ready', rtt_ms: 2.1, rest_rtt_ms: 2.1, grpc_rtt_ms: 1.5, version: '1.8.0' },
          { id: 'audit-log', name: '脱敏审计日志 (Audit Log)', http_url: 'http://127.0.0.1:8084', grpc_addr: '127.0.0.1:50054', status: 'ready', rtt_ms: 1.5, rest_rtt_ms: 1.5, grpc_rtt_ms: 1.1, version: '1.8.0' },
        ],
      });
    } finally {
      setLoadingTopo(false);
    }
  }, [activeProtocol]);

  // 2. Fetch Tasks & Leases
  const fetchTasksAndLeases = useCallback(async () => {
    setLoadingTasks(true);
    try {
      const [tRes, lRes] = await Promise.all([api.listTasks(), api.getLeases()]);
      setTasks(tRes.tasks || []);
      setLeases(lRes);
    } catch {
      // Fallback sample tasks
      setTasks([
        {
          id: 'task-1787554500-eabf3934',
          status: 'completed',
          stage: 'audit',
          source: 'ds_yibao',
          operation: 'mask',
          priority: 50,
          created_at: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
          duration_ms: 270,
          error: '',
          retry_count: 0,
          lease_owner: 'hub-worker-node-1',
        },
        {
          id: 'task-1787554501-89bcdef1',
          status: 'running',
          stage: 'desensitize',
          source: 'ds_kangyang',
          operation: 'classify_and_mask',
          priority: 80,
          created_at: new Date().toISOString(),
          duration_ms: 120,
          error: '',
          retry_count: 0,
          lease_owner: 'hub-worker-node-2',
        },
      ]);
    } finally {
      setLoadingTasks(false);
    }
  }, []);

  // 3. Fetch Suites
  const fetchSuites = useCallback(async () => {
    try {
      const res = await api.getSuites();
      setSuites(res.suites || []);
    } catch {
      // ignore
    }
  }, []);

  // 4. Fetch Datasources
  const fetchDatasources = useCallback(async () => {
    try {
      const res = await api.getDatasources();
      setDatasources(res.datasources || []);
    } catch {
      // ignore
    }
  }, []);

  // 5. Fetch Audit Logs
  const fetchAuditLogs = useCallback(async () => {
    setLoadingAudit(true);
    try {
      const res = await api.getAuditLogs();
      setAuditLogs(res.logs || []);
    } catch {
      // ignore
    } finally {
      setLoadingAudit(false);
    }
  }, []);

  // 6. Fetch Metrics
  const fetchMetrics = useCallback(async () => {
    setLoadingMetrics(true);
    try {
      const res = await api.getMetrics();
      setMetricsRaw(res);
    } catch {
      // ignore
    } finally {
      setLoadingMetrics(false);
    }
  }, []);

  useEffect(() => {
    fetchTopology();
    fetchTasksAndLeases();
    fetchSuites();
    fetchDatasources();
    fetchAuditLogs();
    fetchMetrics();

    // Auto-refresh topology every 15s
    const timer = setInterval(() => {
      fetchTopology();
    }, 15000);
    return () => clearInterval(timer);
  }, [fetchTopology, fetchTasksAndLeases, fetchSuites, fetchDatasources, fetchAuditLogs, fetchMetrics]);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex">
      {/* Sidebar Navigation */}
      <Sidebar
        currentTab={currentTab}
        onSelectTab={setCurrentTab}
        clusterStatus={topology?.status || 'healthy'}
      />

      {/* Main Content Workspace */}
      <main className="flex-1 p-8 max-w-7xl mx-auto overflow-y-auto">
        {currentTab === 'topology' && (
          <TopologyPanel
            topology={topology}
            activeProtocol={activeProtocol}
            onProtocolChange={setActiveProtocol}
            onRefresh={fetchTopology}
            loading={loadingTopo}
          />
        )}

        {currentTab === 'pipeline' && (
          <PipelineVisualizer
            status={pipelineStatus}
            onDispatch={async (req: DispatchRequest) => {
              const res = await api.dispatchTask(req);
              fetchTasksAndLeases();
              return res;
            }}
            onClassifyDispatch={async (src: string, p: Record<string, any>) => {
              const res = await api.classifyDispatch({ source: src, payload: p, priority: 50 });
              fetchTasksAndLeases();
              return res;
            }}
          />
        )}

        {currentTab === 'tasks' && (
          <TaskLifecyclePanel
            tasks={tasks}
            leases={leases}
            onRefresh={fetchTasksAndLeases}
            loading={loadingTasks}
          />
        )}

        {currentTab === 'runner' && (
          <TestRunnerPanel
            suites={suites}
            onRunSuites={async (req) => {
              setLoadingRunner(true);
              try {
                const res = await api.runSuites(req);
                fetchTasksAndLeases();
                return res;
              } finally {
                setLoadingRunner(false);
              }
            }}
            loading={loadingRunner}
          />
        )}

        {currentTab === 'datasources' && (
          <DatasourceExplorer
            datasources={datasources}
            onFetchSlice={(id, limit) => api.getDatasourceSlice(id, limit)}
            onTriggerPipeline={async (dsID, limit) => {
              const res = await api.triggerDatasource({ datasource_id: dsID, limit, operation: 'mask' });
              fetchTasksAndLeases();
              return res;
            }}
          />
        )}

        {currentTab === 'audit' && (
          <AuditVerifierPanel
            logs={auditLogs}
            onVerify={() => api.verifyAudit()}
            onRefreshLogs={fetchAuditLogs}
            loading={loadingAudit}
          />
        )}

        {currentTab === 'metrics' && (
          <MetricsPanel
            metricsRaw={metricsRaw}
            onRefreshMetrics={fetchMetrics}
            loading={loadingMetrics}
          />
        )}
      </main>
    </div>
  );
};
