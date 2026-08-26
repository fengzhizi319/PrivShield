/**
 * TopologyPanel — 四微服务网格拓扑与健康矩阵大屏。
 *
 * 功能概述：
 *  1. 展示 4 个微服务节点的实时状态（REST/gRPC 双协议视角）
 *  2. 支持协议切换（REST ↔ gRPC），切换后自动触发重新探测
 *  3. 服务卡片固定顺序：Hub(#1) → Engine(#2) → Datasource(#3) → Audit(#4)
 *  4. 点击服务卡片弹出详情模态框（双协议地址 + RTT + 健康探针详情）
 *  5. 底部双协议架构说明卡片（REST vs gRPC 对比）
 *
 * 数据来源：
 *  - App.tsx 中的 fetchTopology() 每 15 秒自动刷新
 *  - BFF 并发探测 4 服务后返回 TopologyResponse
 *
 * 渲染结构：
 *  1. 顶部 Banner：标题 + 刷新按钮
 *  2. 协议切换工具栏：REST/gRPC 切换 + 当前模式指示
 *  3. 四服务网格卡片：2×2 布局，每个卡片显示状态/RTT/地址
 *  4. 双协议架构说明：REST vs gRPC 对比卡片
 *  5. 节点详情模态框：点击卡片后弹出
 */
import React, { useState } from 'react';
import { TopologyResponse, ServiceNode, ProtocolType } from '../types/api';
import { useI18n } from '../i18n';
import {
  IconServer,
  IconRefresh,
  IconCheckCircle,
  IconXCircle,
  IconArrowRight,
  IconShieldCheck,
  IconDatabase,
  IconActivity,
  IconLock,
  IconSparkles,
} from './icons';

/** TopologyPanel 组件的 Props */
interface TopologyPanelProps {
  /** 拓扑数据（null 表示尚未加载） */
  topology: TopologyResponse | null;
  /** 当前协议视角 */
  activeProtocol: ProtocolType;
  /** 协议切换回调 */
  onProtocolChange: (proto: ProtocolType) => void;
  /** 刷新回调（触发 BFF 重新探测） */
  onRefresh: (proto?: ProtocolType) => Promise<void>;
  /** 是否正在探测中 */
  loading: boolean;
}

export const TopologyPanel: React.FC<TopologyPanelProps> = ({
  topology,
  activeProtocol,
  onProtocolChange,
  onRefresh,
  loading,
}) => {
  const { t } = useI18n();
  /** 当前选中的服务节点（用于详情模态框） */
  const [selectedNode, setSelectedNode] = useState<ServiceNode | null>(null);

  // 严格固定四微服务的显示位置顺序：
  // 1. 调度中枢 (service-hub)
  // 2. 隐私与分类引擎 (engine)
  // 3. 数据源管理 (datasource-mgr)
  // 4. 脱敏审计日志 (audit-log)
  const FIXED_ORDER = ['service-hub', 'engine', 'datasource-mgr', 'audit-log'];

  const rawServices = topology?.services || [];
  const sortedServices = [...rawServices].sort((a, b) => {
    const idxA = FIXED_ORDER.indexOf(a.id);
    const idxB = FIXED_ORDER.indexOf(b.id);
    return (idxA >= 0 ? idxA : 99) - (idxB >= 0 ? idxB : 99);
  });

  /**
   * 获取服务节点的元数据（显示顺序、角色描述、主题色、图标、端口号）。
   * 用于为每个服务卡片定制展示样式。
   */
  const getServiceMeta = (id: string, index: number) => {
    switch (id) {
      case 'service-hub':
        return {
          order: '#1',
          role: '核心调度中枢 (Central Orchestrator)',
          color: 'indigo',
          icon: <IconActivity className="w-6 h-6 text-indigo-400" />,
          restPort: '8082',
          grpcPort: '50052',
        };
      case 'engine':
        return {
          order: '#2',
          role: '隐私计算与动态分类引擎 (Agent Sidecar)',
          color: 'cyan',
          icon: <IconServer className="w-6 h-6 text-cyan-400" />,
          restPort: '8079',
          grpcPort: '50051',
        };
      case 'datasource-mgr':
        return {
          order: '#3',
          role: '数据源资产管理与探查 (Datasource Mgr)',
          color: 'amber',
          icon: <IconDatabase className="w-6 h-6 text-amber-400" />,
          restPort: '8083',
          grpcPort: '50053',
        };
      case 'audit-log':
        return {
          order: '#4',
          role: '脱敏审计日志与不可篡改存证 (Audit Log)',
          color: 'emerald',
          icon: <IconShieldCheck className="w-6 h-6 text-emerald-400" />,
          restPort: '8084',
          grpcPort: '50054',
        };
      default:
        return {
          order: `#${index + 1}`,
          role: '微服务节点',
          color: 'slate',
          icon: <IconServer className="w-6 h-6 text-slate-400" />,
          restPort: '-',
          grpcPort: '-',
        };
    }
  };

  /**
   * 协议切换处理：同时更新协议状态并触发重新探测。
   * 切换后 BFF 会使用新协议重新探测所有 4 个服务。
   */
  const handleToggleProtocol = (proto: ProtocolType) => {
    onProtocolChange(proto);
    onRefresh(proto);
  };

  return (
    <div className="space-y-6">
      {/* Top Banner */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2.5">
            <span className="p-2 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
              <IconServer className="w-6 h-6" />
            </span>
            <h1 className="text-xl font-bold text-slate-100">{t('topo.title')}</h1>
          </div>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">{t('topo.desc')}</p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => onRefresh(activeProtocol)}
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-medium text-sm transition-all shadow-lg shadow-indigo-600/20 disabled:opacity-50"
          >
            <span className={loading ? 'animate-spin' : ''}>
              <IconRefresh className="w-4 h-4" />
            </span>
            <span>{loading ? t('topo.probing') : t('topo.refresh')}</span>
          </button>
        </div>
      </div>

      {/* Protocol Switcher Toolbar */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <span className="text-xs font-semibold text-slate-300 flex items-center gap-1.5">
            <IconLock className="w-4 h-4 text-indigo-400" />
            {t('topo.protocol.title')}:
          </span>

          <div className="flex items-center bg-slate-950 p-1 rounded-xl border border-slate-800">
            <button
              onClick={() => handleToggleProtocol('rest')}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-bold transition-all ${
                activeProtocol === 'rest'
                  ? 'bg-indigo-600 text-white shadow-md shadow-indigo-600/30'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <IconActivity className="w-3.5 h-3.5" />
              <span>{t('topo.protocol.rest')}</span>
            </button>

            <button
              onClick={() => handleToggleProtocol('grpc')}
              className={`flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-bold transition-all ${
                activeProtocol === 'grpc'
                  ? 'bg-emerald-600 text-white shadow-md shadow-emerald-600/30'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <IconShieldCheck className="w-3.5 h-3.5" />
              <span>{t('topo.protocol.grpc')}</span>
            </button>
          </div>
        </div>

        <div className="text-xs text-slate-400 bg-slate-950 px-3.5 py-2 rounded-xl border border-slate-800/90 font-mono flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
          <span>
            当前接口模式:{' '}
            <strong className={activeProtocol === 'grpc' ? 'text-emerald-400' : 'text-indigo-400'}>
              {activeProtocol === 'grpc' ? 'HTTP/2 gRPC (mTLS 二进制)' : 'HTTP/1.1 REST (JSON 明文)'}
            </strong>
          </span>
        </div>
      </div>

      {/* Fixed 4-Microservice Grid Layout */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {sortedServices.map((service, index) => {
          const meta = getServiceMeta(service.id, index);
          const isReady =
            activeProtocol === 'grpc'
              ? (service.grpc_status || service.status) === 'ready'
              : (service.rest_status || service.status) === 'ready';

          const currentRTT =
            activeProtocol === 'grpc'
              ? (service.grpc_rtt_ms ?? service.rtt_ms)
              : (service.rest_rtt_ms ?? service.rtt_ms);

          const currentAddr = activeProtocol === 'grpc' ? service.grpc_addr : service.http_url;

          return (
            <div
              key={service.id}
              onClick={() => setSelectedNode(service)}
              className="cursor-pointer bg-slate-900 border border-slate-800 hover:border-indigo-500/50 rounded-2xl p-5 shadow-xl transition-all duration-200 hover:-translate-y-1 relative group flex flex-col justify-between"
            >
              {/* Position Pin Badge */}
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-xs font-extrabold px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300 border border-indigo-500/40">
                    {meta.order}
                  </span>
                  <span className="text-[11px] font-mono text-slate-400">{service.id}</span>
                </div>

                <span
                  className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold ${
                    isReady
                      ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                      : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                  }`}
                >
                  {isReady ? (
                    <>
                      <IconCheckCircle className="w-3 h-3" />
                      Ready
                    </>
                  ) : (
                    <>
                      <IconXCircle className="w-3 h-3" />
                      Offline
                    </>
                  )}
                </span>
              </div>

              {/* Service Info */}
              <div>
                <div className="flex items-center gap-2.5 mt-1">
                  <div className="p-2 rounded-xl bg-slate-950 border border-slate-800">
                    {meta.icon}
                  </div>
                  <div>
                    <h3 className="font-bold text-sm text-slate-100 group-hover:text-indigo-300 transition">
                      {service.name}
                    </h3>
                    <p className="text-[11px] text-slate-400 mt-0.5 leading-tight">{meta.role}</p>
                  </div>
                </div>

                {/* Protocol Endpoint Display */}
                <div className="mt-4 p-2.5 rounded-xl bg-slate-950 border border-slate-800/80 space-y-1.5">
                  <div className="flex items-center justify-between text-[11px]">
                    <span className="text-slate-400 font-medium">
                      {activeProtocol === 'grpc' ? 'gRPC 端口 (mTLS)' : 'REST 地址 (HTTP)'}:
                    </span>
                    <span
                      className={`font-mono font-bold text-xs ${
                        activeProtocol === 'grpc' ? 'text-emerald-400' : 'text-indigo-400'
                      }`}
                    >
                      {currentAddr}
                    </span>
                  </div>

                  <div className="flex items-center justify-between text-[11px] pt-1 border-t border-slate-800/60">
                    <span className="text-slate-500">往返延时 RTT:</span>
                    <span className="font-mono text-slate-300 font-semibold">
                      {currentRTT.toFixed(2)} ms
                    </span>
                  </div>
                </div>
              </div>

              {/* Footer details trigger */}
              <div className="mt-4 pt-3 border-t border-slate-800/80 flex items-center justify-between text-[11px] text-slate-500 font-mono">
                <span>v{service.version}</span>
                <span className="text-indigo-400 flex items-center gap-1 group-hover:underline">
                  探针明细 <IconArrowRight className="w-3 h-3" />
                </span>
              </div>
            </div>
          );
        })}
      </div>

      {/* Protocol Architecture Comparison Card */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl">
        <div className="flex items-center gap-2 mb-4">
          <IconSparkles className="w-5 h-5 text-indigo-400" />
          <h2 className="text-sm font-bold text-slate-100">四微服务双协议网格架构说明</h2>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs leading-relaxed">
          <div className="p-4 rounded-xl bg-slate-950 border border-indigo-500/20 space-y-2">
            <div className="font-bold text-indigo-400 flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-indigo-400" />
              REST / HTTP 1.1 JSON (外部接入与测试)
            </div>
            <p className="text-slate-400">
              {t('topo.protocol.restDesc')}
            </p>
            <div className="font-mono text-[11px] text-slate-500 pt-1">
              • 调度中枢: :8082 &nbsp;|&nbsp; 引擎: :8079 &nbsp;|&nbsp; 数据源: :8083 &nbsp;|&nbsp; 审计: :8084
            </div>
          </div>

          <div className="p-4 rounded-xl bg-slate-950 border border-emerald-500/20 space-y-2">
            <div className="font-bold text-emerald-400 flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-emerald-400" />
              gRPC / HTTP 2.0 Protobuf (微服务内部高并发)
            </div>
            <p className="text-slate-400">
              {t('topo.protocol.grpcDesc')}
            </p>
            <div className="font-mono text-[11px] text-slate-500 pt-1">
              • 调度中枢: :50052 &nbsp;|&nbsp; 引擎: :50051 &nbsp;|&nbsp; 数据源: :50053 &nbsp;|&nbsp; 审计: :50054
            </div>
          </div>
        </div>
      </div>

      {/* Node Details Modal */}
      {selectedNode && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-fade-in">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs font-bold px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300">
                  {getServiceMeta(selectedNode.id, 0).order}
                </span>
                <h3 className="text-base font-bold text-slate-100">{selectedNode.name}</h3>
              </div>
              <button
                onClick={() => setSelectedNode(null)}
                className="text-xs text-slate-400 hover:text-slate-200 px-2 py-1 rounded bg-slate-800 border border-slate-700"
              >
                关闭
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div className="grid grid-cols-2 gap-3">
                <div className="p-3 bg-slate-950 rounded-xl border border-slate-800">
                  <span className="text-slate-400 block mb-1">REST 接口 (HTTP):</span>
                  <span className="font-mono text-indigo-400 block truncate">{selectedNode.http_url}</span>
                  <span className="text-[11px] text-slate-500 mt-1 block">
                    延时: {(selectedNode.rest_rtt_ms ?? selectedNode.rtt_ms).toFixed(2)} ms
                  </span>
                </div>
                <div className="p-3 bg-slate-950 rounded-xl border border-slate-800">
                  <span className="text-slate-400 block mb-1">gRPC 端口 (mTLS):</span>
                  <span className="font-mono text-emerald-400 block truncate">{selectedNode.grpc_addr}</span>
                  <span className="text-[11px] text-slate-500 mt-1 block">
                    延时: {(selectedNode.grpc_rtt_ms ?? selectedNode.rtt_ms * 0.85).toFixed(2)} ms
                  </span>
                </div>
              </div>

              <div>
                <span className="text-slate-400 font-semibold block mb-1">健康探针详情 (Health Payload):</span>
                <pre className="p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs font-mono text-emerald-400/90 max-h-48 overflow-y-auto">
                  {JSON.stringify(selectedNode.details || { status: selectedNode.status }, null, 2)}
                </pre>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
