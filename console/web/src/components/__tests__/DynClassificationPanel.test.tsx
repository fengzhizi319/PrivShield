/**
 * DynClassificationPanel 全局标准切换器测试。
 *
 * 覆盖：
 *   1. 挂载时拉取标准列表并渲染三个标准选项（四川/金融/广东）；
 *   2. 切换标准后展示对应等级体系徽章与默认等级；
 *   3. 切换后字段级/记录级评估请求携带新标准 ID（全链路切换）；
 *   4. 未选择标准时请求不携带 standard 字段；
 *   5. 标准列表拉取失败时面板降级可用。
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import DynClassificationPanel from '../DynClassificationPanel';
import * as apiClient from '@/api/client';

// Mock API 客户端模块（面板仅使用 proxyRequest 与 fetchStandards）
vi.mock('@/api/client', () => ({
  proxyRequest: vi.fn(),
  fetchStandards: vi.fn(),
}));

/** 三标准详情 mock 数据（与后端 GET /v1/dynclassification/standards 结构一致）。 */
const mockStandardsResponse = {
  standards: ['gd_health', 'jrt0197', 'sc_health_db51'],
  details: [
    {
      standard_id: 'gd_health',
      description: '广东省健康医疗数据安全分类分级管理技术规范',
      taxonomy: 'gd_health',
      domains: ['gd_health'],
      default_level: 'G2',
      levels: [
        { id: 'G1', name: '第1级（低敏感）', rank: 1 },
        { id: 'G2', name: '第2级（较低敏感）', rank: 2 },
        { id: 'G3', name: '第3级（敏感）', rank: 3 },
        { id: 'G4', name: '第4级（高敏感）', rank: 4 },
      ],
    },
    {
      standard_id: 'jrt0197',
      description: '金融数据安全分级指南',
      taxonomy: 'jrt0197',
      domains: ['finance', 'general-pii'],
      default_level: 'C3',
      levels: [
        { id: 'C1', name: 'C1 级', rank: 1 },
        { id: 'C2', name: 'C2 级', rank: 2 },
        { id: 'C3', name: 'C3 级', rank: 3 },
        { id: 'C4', name: 'C4 级', rank: 4 },
      ],
    },
    {
      standard_id: 'sc_health_db51',
      description: '四川省健康医疗大数据应用指南',
      taxonomy: 'sc_health_db51',
      domains: ['general-pii', 'medical'],
      default_level: 'L3',
      levels: [
        { id: 'L1', name: '一级', rank: 1 },
        { id: 'L2', name: '二级', rank: 2 },
        { id: 'L3', name: '三级', rank: 3 },
        { id: 'L4', name: '四级', rank: 4 },
        { id: 'L5', name: '五级', rank: 5 },
      ],
    },
  ],
};

describe('DynClassificationPanel 全局标准切换器', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiClient.fetchStandards).mockResolvedValue(mockStandardsResponse as any);
  });

  it('挂载时拉取标准列表并渲染三个标准选项', async () => {
    render(<DynClassificationPanel />);

    await waitFor(() => expect(apiClient.fetchStandards).toHaveBeenCalledTimes(1));

    const select = screen.getByRole('combobox');
    // 默认选项 + 三个标准选项
    const options = screen.getAllByRole('option');
    expect(options).toHaveLength(4);
    expect(options[0]).toHaveTextContent('默认（通用规则引擎）');
    expect(select).toHaveTextContent('sc_health_db51 — 四川省健康医疗大数据应用指南');
    expect(select).toHaveTextContent('gd_health — 广东省健康医疗数据安全分类分级管理技术规范');
    expect(select).toHaveTextContent('jrt0197 — 金融数据安全分级指南');
  });

  it('初始未选择标准时展示通用引擎提示', async () => {
    render(<DynClassificationPanel />);
    await waitFor(() =>
      expect(screen.getByText('使用通用规则引擎（未选择标准）')).toBeInTheDocument()
    );
  });

  it('切换到广东标准后展示 G1~G4 等级体系与默认等级', async () => {
    render(<DynClassificationPanel />);
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled());

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'gd_health' } });

    // 描述徽章 + 等级 chips + 默认等级
    expect(screen.getByText('广东省健康医疗数据安全分类分级管理技术规范')).toBeInTheDocument();
    for (const lv of ['G1', 'G2', 'G3', 'G4']) {
      expect(screen.getByText(lv)).toBeInTheDocument();
    }
    expect(screen.getByText('默认等级: G2')).toBeInTheDocument();
    // 输入区提示同步更新
    expect(
      screen.getAllByText(/分类标准由顶部切换器控制：gd_health（广东省健康医疗数据安全分类分级管理技术规范）/)
        .length
    ).toBeGreaterThan(0);
  });

  it('切换到四川标准后展示 L1~L5 等级体系', async () => {
    render(<DynClassificationPanel />);
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled());

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'sc_health_db51' } });

    for (const lv of ['L1', 'L2', 'L3', 'L4', 'L5']) {
      expect(screen.getByText(lv)).toBeInTheDocument();
    }
    expect(screen.getByText('默认等级: L3')).toBeInTheDocument();
  });

  it('字段评估请求携带当前选中的标准 ID', async () => {
    vi.mocked(apiClient.proxyRequest).mockResolvedValue({
      data: { fieldResult: { finalLevel: 'G4', confidence: 0.95 } },
    } as any);

    render(<DynClassificationPanel />);
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled());

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'gd_health' } });
    fireEvent.click(screen.getByText('执行动态分类评估'));

    await waitFor(() => expect(apiClient.proxyRequest).toHaveBeenCalledTimes(1));
    const req = vi.mocked(apiClient.proxyRequest).mock.calls[0][0] as any;
    expect(req.method).toBe('POST');
    expect(req.path).toBe('/v1/dynclassification/eval');
    expect(req.body.standard).toBe('gd_health');
    expect(req.body.fieldName).toBe('mobile_phone');
  });

  it('切换为四川标准后记录级分类请求携带 sc_health_db51', async () => {
    vi.mocked(apiClient.proxyRequest).mockResolvedValue({
      data: { recordResult: { finalLevel: 'L3' } },
    } as any);

    render(<DynClassificationPanel />);
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled());

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'sc_health_db51' } });
    fireEvent.click(screen.getByText('记录级分类 (Record)'));
    fireEvent.click(screen.getByText('执行记录级分类'));

    await waitFor(() => expect(apiClient.proxyRequest).toHaveBeenCalledTimes(1));
    const req = vi.mocked(apiClient.proxyRequest).mock.calls[0][0] as any;
    expect(req.path).toBe('/v1/dynclassification/eval_record');
    expect(req.body.standard).toBe('sc_health_db51');
    expect(req.body.record).toEqual({
      name: '张三',
      id_card: '110101199001011237',
      phone: '13800138000',
    });
  });

  it('未选择标准时评估请求不携带 standard 字段', async () => {
    vi.mocked(apiClient.proxyRequest).mockResolvedValue({
      data: { fieldResult: { finalLevel: 'L3' } },
    } as any);

    render(<DynClassificationPanel />);
    await waitFor(() => expect(screen.getByRole('combobox')).toBeEnabled());

    fireEvent.click(screen.getByText('执行动态分类评估'));

    await waitFor(() => expect(apiClient.proxyRequest).toHaveBeenCalledTimes(1));
    const req = vi.mocked(apiClient.proxyRequest).mock.calls[0][0] as any;
    expect(req.body).not.toHaveProperty('standard');
  });

  it('标准列表拉取失败时面板降级可用（仅默认选项）', async () => {
    vi.mocked(apiClient.fetchStandards).mockRejectedValue(new Error('网络错误'));

    render(<DynClassificationPanel />);

    await waitFor(() =>
      expect(screen.getByText('使用通用规则引擎（未选择标准）')).toBeInTheDocument()
    );
    // 仅默认选项
    expect(screen.getAllByRole('option')).toHaveLength(1);
    // 面板其余功能仍可操作
    expect(screen.getByText('执行动态分类评估')).toBeInTheDocument();
  });
});
