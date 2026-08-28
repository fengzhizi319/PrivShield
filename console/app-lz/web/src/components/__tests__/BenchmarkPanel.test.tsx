import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import React from 'react';
import { BenchmarkPanel } from '../BenchmarkPanel';
import { I18nProvider } from '../../i18n';
import { DataApiDef } from '../../types/api';
import * as clientModule from '../../api/client';

const mockApis: DataApiDef[] = [
  {
    id: 1,
    seq: 1,
    name: '柳州市医保结算数据查询 API',
    description: '提供医保结算流水号、就诊医院、诊断编码等字段的合规脱敏查询',
    datasource_id: 'ds_yibao',
    category: '医疗健康',
    fields: ['insurance_settlement_id', 'person_id', 'gender', 'icd10_code'],
    status: 'active',
  },
  {
    id: 2,
    seq: 2,
    name: '柳州市康养中心长者健康档案 API',
    description: '提供康养中心入住长者的体征体检与病历信息的合规脱敏查询',
    datasource_id: 'ds_kangyang',
    category: '康养服务',
    fields: ['elder_id', 'name', 'age', 'gender', 'chronic_conditions'],
    status: 'active',
  },
];

describe('BenchmarkPanel Component', () => {
  it('renders benchmark dashboard with presets and KPI cards', () => {
    render(
      <I18nProvider>
        <BenchmarkPanel apis={mockApis} />
      </I18nProvider>
    );

    // 验证主标题与预设场景
    expect(screen.getByText(/全栈微服务性能与吞吐量基准压测工作台/i)).toBeInTheDocument();
    expect(screen.getByText(/医保结算/i)).toBeInTheDocument();
    expect(screen.getByText(/康养体征/i)).toBeInTheDocument();
    expect(screen.getByText(/突发脉冲/i)).toBeInTheDocument();
    expect(screen.getByText(/启动全链路压测/i)).toBeInTheDocument();
  });

  it('triggers benchmark run and calculates metrics', async () => {
    vi.spyOn(clientModule.api, 'invokeDataApi').mockResolvedValue({
      session_id: 'session-bench-1',
      api_id: 1,
      api_name: '医保结算数据 API',
      status: 'completed',
      raw_records: [{ a: '1' }],
      sanitized_data: [{ a: '*' }],
      stages: [
        { name: 'ingest', title: '入站校验', status: 'success', duration_ms: 1 },
        { name: 'fetch', title: '数据抽取', status: 'success', duration_ms: 2 },
        { name: 'classify_desensitize', title: '评级脱敏', status: 'success', duration_ms: 25 },
        { name: 'return', title: '装配交付', status: 'success', duration_ms: 1 },
        { name: 'audit', title: '审计存证', status: 'success', duration_ms: 3 },
      ],
      audit_entry_id: 'audit-test-123',
      total_duration_ms: 32,
    });

    render(
      <I18nProvider>
        <BenchmarkPanel apis={mockApis} />
      </I18nProvider>
    );

    const startBtn = screen.getByText(/启动全链路压测/i);
    fireEvent.click(startBtn);

    await waitFor(() => {
      expect(screen.getByText(/实时吞吐量 \(QPS\)/i)).toBeInTheDocument();
      expect(screen.getByText(/中位数延迟 \(P50\)/i)).toBeInTheDocument();
      expect(screen.getByText(/5阶段端到端全流程单笔耗时瀑布流/i)).toBeInTheDocument();
    });
  });
});
