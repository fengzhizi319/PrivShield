import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import React from 'react';
import { PipelineVisualizer } from '../PipelineVisualizer';
import { I18nProvider } from '../../i18n';

describe('PipelineVisualizer Component', () => {
  it('renders 6 pipeline stages and dispatch button', () => {
    const onDispatch = vi.fn();
    const onClassifyDispatch = vi.fn();

    render(
      <I18nProvider>
        <PipelineVisualizer
          status={null}
          onDispatch={onDispatch}
          onClassifyDispatch={onClassifyDispatch}
        />
      </I18nProvider>
    );

    expect(screen.getByText('6 阶段流水线动态流转大屏')).toBeInTheDocument();
    expect(screen.getByText('1. Ingest')).toBeInTheDocument();
    expect(screen.getByText('2. Fetch')).toBeInTheDocument();
    expect(screen.getByText('3. Classify')).toBeInTheDocument();
    expect(screen.getByText('4. Desensitize')).toBeInTheDocument();
    expect(screen.getByText('5. Return')).toBeInTheDocument();
    expect(screen.getByText('6. Audit')).toBeInTheDocument();
    expect(screen.getByText('触发 6 阶段流水线')).toBeInTheDocument();
  });

  it('switches preset sample when clicking preset buttons', () => {
    const onDispatch = vi.fn();
    const onClassifyDispatch = vi.fn();

    render(
      <I18nProvider>
        <PipelineVisualizer
          status={null}
          onDispatch={onDispatch}
          onClassifyDispatch={onClassifyDispatch}
        />
      </I18nProvider>
    );

    const kangyangBtn = screen.getByText('康养预设');
    fireEvent.click(kangyangBtn);
    expect(screen.getByDisplayValue(/ds_kangyang/)).toBeInTheDocument();
  });
});
