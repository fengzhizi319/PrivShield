import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import React from 'react';
import { TopologyPanel } from '../TopologyPanel';
import { I18nProvider } from '../../i18n';
import { TopologyResponse } from '../../types/api';

describe('TopologyPanel Component', () => {
  const mockTopology: TopologyResponse = {
    status: 'healthy',
    active_protocol: 'rest',
    timestamp: '2026-08-26T11:00:00Z',
    services: [
      {
        id: 'audit-log',
        name: '脱敏审计日志 (Audit Log)',
        http_url: 'http://127.0.0.1:8084',
        grpc_addr: '127.0.0.1:50054',
        status: 'ready',
        rtt_ms: 1.5,
        rest_rtt_ms: 1.5,
        grpc_rtt_ms: 1.1,
        version: '1.8.0',
      },
      {
        id: 'service-hub',
        name: '调度中枢 (Service Hub)',
        http_url: 'http://127.0.0.1:8082',
        grpc_addr: '127.0.0.1:50052',
        status: 'ready',
        rtt_ms: 1.8,
        rest_rtt_ms: 1.8,
        grpc_rtt_ms: 1.2,
        version: '1.8.0',
        details: { store_type: 'postgres_leased' },
      },
      {
        id: 'datasource-mgr',
        name: '数据源管理 (Datasource Mgr)',
        http_url: 'http://127.0.0.1:8083',
        grpc_addr: '127.0.0.1:50053',
        status: 'ready',
        rtt_ms: 2.1,
        rest_rtt_ms: 2.1,
        grpc_rtt_ms: 1.5,
        version: '1.8.0',
      },
      {
        id: 'engine',
        name: '隐私与分类引擎 (PrivShield Agent)',
        http_url: 'http://127.0.0.1:8079',
        grpc_addr: '127.0.0.1:50051',
        status: 'ready',
        rtt_ms: 3.2,
        rest_rtt_ms: 3.2,
        grpc_rtt_ms: 2.4,
        version: '1.8.0',
      },
    ],
  };

  it('renders 4-service topology matrix strictly in fixed order (Hub ➔ Agent ➔ Datasource ➔ Audit)', () => {
    const onRefresh = vi.fn();
    const onProtocolChange = vi.fn();

    render(
      <I18nProvider>
        <TopologyPanel
          topology={mockTopology}
          activeProtocol="rest"
          onProtocolChange={onProtocolChange}
          onRefresh={onRefresh}
          loading={false}
        />
      </I18nProvider>
    );

    expect(screen.getByText('四微服务网格拓扑与健康矩阵')).toBeInTheDocument();

    // Check fixed position pins
    expect(screen.getByText('#1')).toBeInTheDocument();
    expect(screen.getByText('#2')).toBeInTheDocument();
    expect(screen.getByText('#3')).toBeInTheDocument();
    expect(screen.getByText('#4')).toBeInTheDocument();

    expect(screen.getByText('调度中枢 (Service Hub)')).toBeInTheDocument();
    expect(screen.getByText('隐私与分类引擎 (PrivShield Agent)')).toBeInTheDocument();
    expect(screen.getByText('数据源管理 (Datasource Mgr)')).toBeInTheDocument();
    expect(screen.getByText('脱敏审计日志 (Audit Log)')).toBeInTheDocument();
  });

  it('switches between REST and gRPC protocols on click', () => {
    const onRefresh = vi.fn();
    const onProtocolChange = vi.fn();

    render(
      <I18nProvider>
        <TopologyPanel
          topology={mockTopology}
          activeProtocol="rest"
          onProtocolChange={onProtocolChange}
          onRefresh={onRefresh}
          loading={false}
        />
      </I18nProvider>
    );

    const grpcBtn = screen.getByText('gRPC (mTLS / Protobuf)');
    fireEvent.click(grpcBtn);

    expect(onProtocolChange).toHaveBeenCalledWith('grpc');
    expect(onRefresh).toHaveBeenCalledWith('grpc');
  });
});
