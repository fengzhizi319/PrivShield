# Vitest 技术栈说明 / Vitest Technology Stack

## 1. 技术简介 / Introduction

Vitest 是由 Vite 团队开发的下一代前端测试框架，与 Vite 共享配置与转换管道。
Vitest is a next-generation frontend testing framework developed by the Vite team, sharing config and transform pipeline with Vite.

核心特性 / Core Features：
- **与 Vite 共享配置（Shared Config）**：无需重复配置 babel/ts 转换，直接复用 vite.config.ts。
- **极速执行（Fast Execution）**：基于 Vite 的模块转换，HMR 级别的测试热重载。
- **Jest 兼容 API（Jest-compatible）**：describe/it/expect/vi 等 API 与 Jest 一致，迁移零成本。
- **原生 ESM 支持**：无需额外配置即可测试 ES Module 代码。
- **内置覆盖率（Built-in Coverage）**：通过 c8/istanbul 生成覆盖率报告。

本项目使用版本 / Version Used：`vitest ^3.0.5` + `@testing-library/react ^16.2.0` + `jsdom ^26.0.0`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 测试配置 / Test Configuration

文件 / File：`console/web/vite.config.ts`（test 字段）

```typescript
export default defineConfig({
  // ... Vite 配置 / Vite config
  test: {
    globals: true,                        // 全局注入 describe/it/expect，无需手动 import
    environment: 'jsdom',                 // 模拟浏览器 DOM 环境 / Simulate browser DOM
    setupFiles: './src/test/setup.ts',    // 测试前置脚本 / Test setup script
  },
})
```

### 2.2 测试前置脚本 / Test Setup

文件 / File：`console/web/src/test/setup.ts`

```typescript
// 引入 jest-dom 扩展匹配器（如 toBeInTheDocument、toHaveClass）
// Import jest-dom extended matchers
import '@testing-library/jest-dom';
```

### 2.3 组件测试示例 / Component Test Example

文件 / File：`console/web/src/components/__tests__/ErrorBoundary.test.tsx`

```tsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import ErrorBoundary from '../ErrorBoundary';

describe('ErrorBoundary', () => {
  it('renders children when no error', () => {
    render(<ErrorBoundary><div>OK</div></ErrorBoundary>);
    expect(screen.getByText('OK')).toBeInTheDocument();
  });
});
```

### 2.4 API 客户端测试 / API Client Test

文件 / File：`console/web/src/api/__tests__/client.test.ts`

```typescript
import { describe, it, expect, vi } from 'vitest';
import { fetchHealth } from '../client';

// Mock fetch API 模拟后端响应 / Mock fetch to simulate backend
vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
  ok: true,
  text: () => Promise.resolve(JSON.stringify({ backend: 'ok' })),
}));
```

### 2.5 运行测试 / Running Tests

```bash
# 运行所有测试 / Run all tests
cd console/web && pnpm test

# 监听模式（开发时）/ Watch mode (during development)
pnpm test -- --watch

# 单次运行 / Single run
pnpm test -- --run
```

### 2.6 Mock 策略详解 / Mock Strategy Details

本项目使用 Vitest 内置的 `vi` 对象实现多种 Mock 模式：
This project uses Vitest's built-in `vi` object for multiple mock patterns:

#### 全局 fetch Mock / Global fetch Mock

```typescript
// client.test.ts：模拟全局 fetch API / Mock global fetch API
const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);  // 替换全局 fetch 为 mock 函数 / Replace global fetch with mock

// 构造模拟响应 / Construct mock response
function jsonResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(data),
  });
}

// 使用示例 / Usage example
mockFetch.mockReturnValue(jsonResponse({ status: 'ok' }));
await fetchHealth();
expect(mockFetch).toHaveBeenCalledWith(
  'http://localhost:8081/api/health',
  expect.objectContaining({ signal: expect.any(AbortSignal) }),
);
```

#### 定时器 Mock / Timer Mock

```typescript
// 测试超时逻辑：快进时间触发 AbortController / Test timeout: advance time to trigger AbortController
vi.useFakeTimers();                  // 启用假定时器 / Enable fake timers
mockFetch.mockReturnValue(new Promise(() => {})); // 永不 resolve 的 promise / Never-resolving promise

void fetchHealth();
vi.advanceTimersByTime(60_000);      // 快进 60s 触发超时 / Advance 60s to trigger timeout

vi.useRealTimers();                  // 测试后恢复真实定时器 / Restore real timers after test
```

#### 文件对象 Mock / File Object Mock

```typescript
// FileTest.test.tsx：构造指定大小的 File 对象 / Construct File with specific size
function makeFile(name: string, sizeBytes: number): File {
  const content = sizeBytes > 0 ? 'x'.repeat(Math.min(sizeBytes, 1024)) : '';
  const file = new File([content], name, { type: 'text/csv' });
  // jsdom 中 File.size 由 content 长度决定，需 mock 大文件场景
  // In jsdom, File.size is determined by content length, need mock for large files
  if (sizeBytes > 1024) {
    Object.defineProperty(file, 'size', { value: sizeBytes });
  }
  return file;
}
```

#### 测试生命周期钩子 / Test Lifecycle Hooks

```typescript
beforeEach(() => {
  vi.clearAllMocks();    // 每个测试前清除所有 mock 调用记录 / Clear all mock call records
  setBaseUrl('');        // 重置全局状态 / Reset global state
  setApiKey('');
});

afterEach(() => {
  vi.useRealTimers();    // 确保定时器不泄漏到下一个测试 / Ensure timers don't leak
});
```

### 2.7 测试用例覆盖范围 / Test Coverage Scope

| 测试文件 / Test File | 覆盖内容 / Coverage |
|---|---|
| `client.test.ts` | URL 拼接、尾部斜杠去除、API Key 头、错误处理、超时、POST body 序列化 |
| `ErrorBoundary.test.tsx` | 正常渲染子组件、崩溃时显示降级 UI、错误信息展示 |
| `FileTest.test.tsx` | 文件扩展名校验、大小上限、大小写不敏感、边界值 |

### 2.8 测试最佳实践 / Testing Best Practices

| 实践 / Practice | 说明 / Description |
|---|---|
| 每个测试独立 / Each test independent | beforeEach 重置全局状态，测试间无依赖 / Reset global state, no inter-test deps |
| 测试用户行为 / Test user behavior | 验证“用户看到什么”而非内部实现 / Verify "what user sees" not internals |
| 边界值测试 / Boundary value testing | 恰好等于上限、空值、超大文件 / Exact limit, empty, oversized |
| 异步测试 / Async testing | async/await + rejects.toThrow 验证异常 / Verify exceptions |
| 不依赖网络 / No network dependency | 全部通过 vi.stubGlobal mock / All mocked via vi.stubGlobal |

### 2.9 测试工具链 / Testing Toolchain

| 工具 / Tool | 作用 / Purpose |
|---|---|
| Vitest | 测试运行器 + 断言库 + Mock 工具 / Test runner + assertion + mock |
| @testing-library/react | React 组件渲染与交互测试 / Component render & interaction |
| @testing-library/user-event | 模拟真实用户操作 / Simulate real user events |
| @testing-library/jest-dom | DOM 断言扩展 / DOM assertion extensions |
| jsdom | 浏览器 DOM 模拟 / Browser DOM simulation |
| vi.stubGlobal | 全局变量替换（fetch、localStorage）/ Global variable replacement |
| vi.useFakeTimers | 定时器控制（测试超时逻辑）/ Timer control (test timeout logic) |

### 2.10 测试运行器架构 / Test Runner Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  Vitest 运行器 / Vitest Runner                               │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  Vite 转换管道 / Vite Transform Pipeline              │ │
│  │  - TypeScript 编译 / TypeScript compilation           │ │
│  │  - JSX 转换 / JSX transform                          │ │
│  │  - ESM 模块解析 / ESM module resolution               │ │
│  │  - 路径别名解析 / Path alias resolution               │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  测试环境 / Test Environment                          │ │
│  │  - jsdom: 模拟浏览器 DOM / Simulate browser DOM       │ │
│  │  - node: 纯 Node.js 环境 / Pure Node.js environment   │ │
│  └─────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────┐ │
│  │  测试收集与执行 / Test Collection & Execution         │ │
│  │  - 发现 *.test.ts(x) 文件 / Discover test files       │ │
│  │  - 并行执行测试套件 / Parallel test suite execution   │ │
│  │  - HMR 级别热重载 / HMR-level hot reload              │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

**Vitest vs Jest 架构对比 / Vitest vs Jest Architecture**：

| 维度 / Dimension | Vitest | Jest |
|---|---|---|
| 转换管道 / Transform | 复用 Vite（零配置）/ Reuses Vite | 需配置 babel/ts-jest |
| 模块系统 / Module system | 原生 ESM | CommonJS 为主 / Mostly CJS |
| 热重载 / Hot reload | HMR 级别 / HMR-level | 需重新运行 / Re-run needed |
| 配置文件 / Config file | 复用 vite.config.ts | 独立 jest.config.js |
| 启动速度 / Startup speed | 极快 / Very fast | 较慢 / Slower |

### 2.11 覆盖率配置 / Coverage Configuration

```typescript
// vite.config.ts 中配置覆盖率 / Configure coverage in vite.config.ts
export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',           // 使用 V8 原生覆盖率 / Use V8 native coverage
      reporter: ['text', 'html', 'lcov'],  // 报告格式 / Report formats
      include: ['src/**/*.{ts,tsx}'],       // 覆盖范围 / Coverage scope
      exclude: [
        'src/test/**',          // 排除测试文件 / Exclude test files
        'src/**/*.d.ts',        // 排除类型声明 / Exclude type declarations
        'src/main.tsx',         // 排除入口文件 / Exclude entry file
      ],
      thresholds: {
        statements: 80,         // 语句覆盖率阈值 / Statement coverage threshold
        branches: 70,           // 分支覆盖率阈值 / Branch coverage threshold
        functions: 80,          // 函数覆盖率阈值 / Function coverage threshold
        lines: 80,              // 行覆盖率阈值 / Line coverage threshold
      },
    },
  },
})
```

```bash
# 生成覆盖率报告 / Generate coverage report
cd console/web && pnpm test -- --coverage

# 查看 HTML 报告 / View HTML report
open coverage/index.html
```

### 2.12 快照测试 / Snapshot Testing

Vitest 支持快照测试（本项目未使用，但可用于 UI 回归检测）：

```tsx
import { render } from '@testing-library/react';
import { expect, it } from 'vitest';

it('renders correctly', () => {
  const { container } = render(<MyComponent />);
  // 首次运行创建快照，后续运行对比
  // First run creates snapshot, subsequent runs compare
  expect(container.innerHTML).toMatchSnapshot();
});

// 更新快照（UI 有意更改时）/ Update snapshots (when UI intentionally changed)
// pnpm test -- --update
```

### 2.13 测试模式与过滤 / Test Modes & Filtering

```bash
# 监听模式（默认）：文件变更自动重跑相关测试
# Watch mode (default): auto re-run related tests on file change
pnpm test

# 单次运行（CI 环境）/ Single run (CI environment)
pnpm test -- --run

# 仅运行匹配的测试文件 / Run only matching test files
pnpm test -- client.test

# 仅运行匹配的测试名称 / Run only matching test names
pnpm test -- -t "timeout"

# UI 模式（可视化测试面板）/ UI mode (visual test panel)
pnpm test -- --ui
```

### 2.14 测试文件组织 / Test File Organization

```text
console/web/src/
├── api/
│   ├── client.ts              # 源代码 / Source code
│   └── __tests__/
│       └── client.test.ts     # 测试文件（同名 + .test）/ Test file (same name + .test)
├── components/
│   ├── ErrorBoundary.tsx
│   ├── FileTest.tsx
│   └── __tests__/
│       ├── ErrorBoundary.test.tsx
│       └── FileTest.test.tsx
└── test/
    └── setup.ts               # 全局测试初始化 / Global test setup
```

**命名约定 / Naming Conventions**：

| 模式 / Pattern | 说明 / Description |
|---|---|
| `*.test.ts(x)` | 测试文件（Vitest 自动发现）/ Test files (auto-discovered) |
| `__tests__/` 目录 | 测试文件集中存放 / Centralized test storage |
| `setup.ts` | 全局初始化（匹配器注册）/ Global init (matcher registration) |

## 3. 模块模拟详解 / Module Mocking Details

### 3.1 vi.mock 模块级模拟 / Module-level Mocking

```typescript
// 模拟整个模块 / Mock entire module
vi.mock('../api/client', () => ({
  fetchHealth: vi.fn().mockResolvedValue({ backend: 'ok', agent: 'ok' }),
  fetchSamples: vi.fn().mockResolvedValue([
    { id: 'mask', name: 'Mask', category: 'privacy' },
  ]),
  setBaseUrl: vi.fn(),
  setApiKey: vi.fn(),
}));

// 在测试中获取模拟引用 / Get mock reference in tests
import { fetchHealth } from '../api/client';
const mockFetchHealth = vi.mocked(fetchHealth);

it('loads health data', async () => {
  mockFetchHealth.mockResolvedValueOnce({ backend: 'error', agent: 'ok' });
  // ... 测试逻辑 / test logic
});
```

### 3.2 vi.mock 提升机制 / vi.mock Hoisting

```typescript
// ❗ 重要：vi.mock 会被自动提升到文件顶部
// Important: vi.mock is auto-hoisted to file top

import { fetchHealth } from '../api/client';  // 实际导入 / Actual import
import { describe, it, expect, vi } from 'vitest';

// 这个调用会被提升到 import 之前执行 / This call is hoisted before imports
vi.mock('../api/client', () => ({
  fetchHealth: vi.fn(),  // 模拟实现 / Mock implementation
}));

// 因此 import 获取的已经是模拟版本 / So import gets the mocked version
describe('App', () => {
  it('works', () => {
    // fetchHealth 已经是 vi.fn() / Already vi.fn()
  });
});
```

### 3.3 部分模拟 / Partial Mocking

```typescript
// 保留原始实现，仅模拟部分导出 / Keep original, mock only some exports
vi.mock('../utils/format', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../utils/format')>();
  return {
    ...actual,                    // 保留其他导出 / Keep other exports
    formatDate: vi.fn().mockReturnValue('2024-01-01'),  // 仅模拟这个 / Only mock this
  };
});
```

### 3.4 Spy 与 Mock 的区别 / Spy vs Mock Difference

| 特性 / Feature | vi.fn() (Mock) | vi.spyOn() (Spy) |
|---|---|---|
| 原始实现 / Original impl | 无（需手动提供）/ None | 保留 / Preserved |
| 用途 / Purpose | 替换函数 / Replace function | 监听调用 / Observe calls |
| 恢复 / Restore | 不需要 / Not needed | vi.restoreAllMocks() |
| 典型场景 / Typical use | 模拟 API 调用 / Mock API calls | 验证方法被调用 / Verify method called |

```typescript
// Spy 示例：监听但不替换 / Spy: observe without replacing
const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

render(<ErrorBoundary><BrokenComponent /></ErrorBoundary>);

expect(consoleSpy).toHaveBeenCalledWith(
  expect.stringContaining('ErrorBoundary caught')
);
consoleSpy.mockRestore();  // 恢复原始 console.error / Restore original
```

## 4. 并行执行与性能 / Parallel Execution & Performance

### 4.1 并行策略 / Parallelism Strategy

```text
┌─────────────────────────────────────────────────────────────┐
│  Vitest 并行执行模型 / Vitest Parallel Execution Model     │
│                                                             │
│  测试文件级别：多进程并行 / File level: multi-process       │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐       │
│  │ Worker #1  │ │ Worker #2  │ │ Worker #3  │       │
│  │ client.    │ │ ErrorBound │ │ FileTest.  │       │
│  │ test.ts    │ │ test.tsx   │ │ test.tsx   │       │
│  └───────────┘ └───────────┘ └───────────┘       │
│                                                             │
│  测试用例级别：同一文件内串行 / Case level: serial in file  │
│  describe('client')                                         │
│    it('test 1') → it('test 2') → it('test 3')  // 顺序执行  │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 并行配置 / Parallelism Configuration

```typescript
// vite.config.ts 中的并行配置 / Parallelism config in vite.config.ts
export default defineConfig({
  test: {
    // 并行工作线程数 / Parallel worker count
    maxWorkers: 4,          // 默认: CPU 核心数 - 1 / Default: CPU cores - 1
    minWorkers: 1,          // 最小工作线程 / Minimum workers

    // 文件内测试顺序 / In-file test order
    sequence: {
      shuffle: false,       // 不随机打乱（保证可重现）/ No shuffle (reproducible)
    },

    // 隔离模式 / Isolation mode
    isolate: true,          // 每个文件独立环境 / Independent env per file
    pool: 'threads',        // 'threads' | 'forks' | 'vmThreads'
  },
})
```

**池模式对比 / Pool Mode Comparison**：

| 模式 / Mode | 隔离性 / Isolation | 速度 / Speed | 适用 / Use Case |
|---|---|---|---|
| `threads` | 中（共享进程）/ Medium | 最快 / Fastest | 默认推荐 / Default recommended |
| `forks` | 高（独立进程）/ High | 较慢 / Slower | 全局状态污染 / Global state pollution |
| `vmThreads` | 极高（VM 隔离）/ Very high | 最慢 / Slowest | 安全敏感 / Security sensitive |

### 4.3 性能优化技巧 / Performance Optimization Tips

| 技巧 / Tip | 说明 / Description |
|---|---|
| 缩小测试范围 / Narrow test scope | `pnpm test -- client` 仅运行匹配文件 / Run only matching |
| 监听模式 / Watch mode | 仅重跑受影响测试 / Only re-run affected tests |
| 避免全局 Mock / Avoid global mocks | 减少模块重新加载 / Reduce module reloads |
| 合理使用 isolate / Use isolate wisely | 无状态测试可关闭隔离 / Stateless tests can disable |
| 懒加载重型依赖 / Lazy-load heavy deps | 避免每个 worker 加载 torch 等 / Avoid loading torch per worker |

## 5. 基准测试 / Benchmarking

### 5.1 Vitest Bench API / Vitest Bench API

```typescript
// src/utils/__bench__/format.bench.ts
import { bench, describe } from 'vitest';
import { formatDuration, formatBytes } from '../format';

describe('formatDuration benchmark', () => {
  bench('format milliseconds', () => {
    formatDuration(123.456);
  });

  bench('format seconds', () => {
    formatDuration(5432.1);
  });

  // 配置选项 / Configuration options
  bench('format with options', () => {
    formatDuration(999.999);
  }, {
    iterations: 10000,    // 最小迭代次数 / Minimum iterations
    time: 1000,           // 最大运行时间 (ms) / Max run time
    warmupTime: 100,      // 预热时间 / Warmup time
  });
});
```

### 5.2 运行基准测试 / Running Benchmarks

```bash
# 运行所有基准测试 / Run all benchmarks
cd console/web && pnpm vitest bench

# 运行特定基准 / Run specific benchmark
pnpm vitest bench format.bench

# 输出示例 / Example output:
# ✓ formatDuration benchmark > format milliseconds  123ns ± 5ns
# ✓ formatDuration benchmark > format seconds       145ns ± 8ns
```

## 6. 工作区与单仓库 / Workspace & Monorepo

### 6.1 Vitest 工作区配置 / Vitest Workspace Configuration

```typescript
// vitest.workspace.ts（单仓库场景）/ Monorepo scenario
import { defineWorkspace } from 'vitest/config';

export default defineWorkspace([
  // 前端项目 / Frontend project
  {
    test: {
      name: 'web',
      root: './console/web',
      environment: 'jsdom',
      globals: true,
    },
  },
  // 后端测试（如果有）/ Backend tests (if any)
  // 历史示例：原 Python REST BFF 已移除，当前 BFF 为 Go 实现（console/bff-go）
  {
    test: {
      name: 'backend',
      root: './console/bff-go',
      environment: 'node',
    },
  },
]);
```

### 6.2 项目引用与路径解析 / Project References & Path Resolution

```typescript
// vite.config.ts 中的路径解析 / Path resolution in vite.config.ts
export default defineConfig({
  resolve: {
    alias: {
      '@': '/src',           // 路径别名 / Path alias
      '@api': '/src/api',    // API 模块别名 / API module alias
    },
  },
  test: {
    // 测试中的路径解析与生产一致 / Path resolution same as production
    alias: {
      '@': '/src',
    },
  },
})
```

## 7. 调试与故障排查 / Debugging & Troubleshooting

### 7.1 常见错误与解决 / Common Errors & Solutions

| 错误 / Error | 原因 / Cause | 解决 / Solution |
|---|---|---|
| `ReferenceError: document is not defined` | 缺少 jsdom 环境 / Missing jsdom env | 添加 `environment: 'jsdom'` |
| `vi.mock is not defined` | 未启用 globals / globals not enabled | 添加 `globals: true` 或手动 import |
| `Cannot find module '@/...'` | 路径别名未配置 / Path alias not configured | 在 test.alias 中配置 |
| `Timeout - Async callback` | 异步操作未完成 / Async op not completed | 增加 timeout 或检查 await |
| `Mock function called more times` | 测试间状态泄漏 / State leak between tests | beforeEach 中 vi.clearAllMocks() |

### 7.2 调试模式 / Debug Mode

```bash
# 单线程运行（便于调试）/ Single thread (easier debugging)
pnpm test -- --run --no-threads

# 详细输出 / Verbose output
pnpm test -- --reporter=verbose

# 仅运行失败测试 / Run only failed tests
pnpm test -- --changed

# Node.js 调试器 / Node.js debugger
node --inspect-brk ./node_modules/vitest/vitest.mjs run --no-threads
```

### 7.3 测试设计原则 / Test Design Principles

```text
测试金字塔 / Test Pyramid:

        /\          E2E 测试 (Playwright/Cypress)
       /  \         - 完整用户流程 / Full user flows
      /____\        - 少量但关键 / Few but critical
     /      \
    / 集成测试 \     组件交互测试 (Testing Library)
   /__________\    - 组件组合行为 / Component composition
  /            \   - 适量 / Moderate amount
 /______________\
/  单元测试      \  纯函数/工具测试 (Vitest)
/________________\ - 大量且快速 / Many and fast
                 - 本项目主要层次 / This project's main layer
```

## 8. 快照测试 / Snapshot Testing

### 8.1 快照测试原理 / Snapshot Testing Principle

快照测试将组件渲染结果序列化为文本文件，后续运行时对比差异：

```tsx
import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';

it('组件渲染快照', () => {
  const { container } = render(<HealthBadge status="ok" />);
  // 首次运行：创建 __snapshots__/xxx.test.tsx.snap 文件
  // First run: creates __snapshots__/xxx.test.tsx.snap file
  // 后续运行：对比当前渲染与快照差异
  // Subsequent runs: compares current render with snapshot
  expect(container.innerHTML).toMatchSnapshot();
});

// 内联快照（直接嵌入测试文件）/ Inline snapshot (embedded in test file)
it('内联快照示例', () => {
  const result = formatBytes(1024);
  expect(result).toMatchInlineSnapshot('"1.0 KB"');
});
```

### 8.2 快照更新策略 / Snapshot Update Strategy

```bash
# 更新所有快照（组件 UI 有意变更时）/ Update all snapshots (when UI intentionally changes)
pnpm test -- --update
# 或简写 / Or shorthand
pnpm test -- -u

# 交互式更新（逐个确认）/ Interactive update (confirm one by one)
pnpm test -- --watch -u
```

**快照测试适用场景 / Snapshot Test Use Cases**：

| 适用 / Suitable | 不适用 / Not Suitable |
|---|---|
| UI 回归检测 / UI regression | 频繁变化的组件 / Frequently changing components |
| 组件库文档 / Component library docs | 包含随机值/时间戳 / Contains random/timestamp |
| 序列化输出验证 / Serialization output | 大型复杂 DOM / Large complex DOM |
| 配置对象结构 / Config object structure | 逻辑正确性验证 / Logic correctness |

### 8.3 本项目快照测试建议 / Project Snapshot Recommendations

```text
本项目当前未使用快照测试，原因：
This project currently doesn't use snapshot tests, reasons:

1. 组件数量少（~15 个），手动断言已足够
   Few components (~15), manual assertions suffice
2. UI 迭代频繁，快照更新成本高
   Frequent UI iteration, high snapshot update cost
3. 用户行为测试（Testing Library）提供更强信心
   User behavior tests (Testing Library) provide stronger confidence

推荐场景：若未来组件库化，可引入快照防止意外 UI 变更
Recommended: if component library in future, use snapshots to prevent accidental UI changes
```

## 9. 覆盖率配置详解 / Coverage Configuration Details

### 9.1 启用覆盖率收集 / Enabling Coverage Collection

```typescript
// vite.config.ts 中配置 / Configure in vite.config.ts
export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',              // 覆盖率引擎 / Coverage engine (v8 | istanbul)
      reporter: ['text', 'html', 'lcov'],  // 报告格式 / Report formats
      reportsDirectory: './coverage',       // 输出目录 / Output directory
      include: ['src/**/*.{ts,tsx}'],       // 包含文件 / Included files
      exclude: [
        'src/test/**',             // 测试文件 / Test files
        'src/**/*.d.ts',           // 类型声明 / Type declarations
        'src/main.tsx',            // 入口文件 / Entry file
      ],
      thresholds: {
        statements: 80,            // 语句覆盖率阈值 / Statement coverage threshold
        branches: 70,              // 分支覆盖率阈值 / Branch coverage threshold
        functions: 80,             // 函数覆盖率阈值 / Function coverage threshold
        lines: 80,                 // 行覆盖率阈值 / Line coverage threshold
      },
    },
  },
});
```

### 9.2 运行覆盖率 / Running Coverage

```bash
# 生成覆盖率报告 / Generate coverage report
pnpm test -- --coverage

# 输出示例 / Output example:
# ----------|---------|----------|---------|---------|
# File      | % Stmts | % Branch | % Funcs | % Lines |
# ----------|---------|----------|---------|---------|
# client.ts |   92.5  |    85.7  |   100   |   92.5  |
# App.tsx   |   78.3  |    65.2  |   80.0  |   78.3  |
# ----------|---------|----------|---------|---------|
```

### 9.3 覆盖率指标解读 / Coverage Metrics Explained

| 指标 / Metric | 含义 / Meaning | 未覆盖示例 / Uncovered Example |
|---|---|---|
| Statements | 每条语句是否执行 / Each statement executed | 未进入的 if 分支体 / Unentered if branch body |
| Branches | 每个条件分支是否覆盖 / Each conditional branch | 未触发的 catch 块 / Untriggered catch block |
| Functions | 每个函数是否调用 / Each function called | 未使用的工具函数 / Unused utility function |
| Lines | 每行是否执行 / Each line executed | 与 Statements 类似 / Similar to Statements |

### 9.4 v8 vs istanbul 对比 / v8 vs istanbul Comparison

| 维度 / Dimension | v8 (默认) | istanbul |
|---|---|---|
| 速度 / Speed | 快（原生 V8 覆盖）/ Fast (native V8) | 较慢（代码插桩）/ Slower (instrumentation) |
| 准确性 / Accuracy | 行级 / Line-level | 语句级（更精确）/ Statement-level (more precise) |
| 配置复杂度 / Config | 简单 / Simple | 较复杂 / More complex |
| 源码映射 / Source maps | 依赖 Vite 转换 / Depends on Vite transform | 独立处理 / Independent |
| 推荐场景 / Recommended | 日常开发 / Daily dev | CI 精确报告 / CI precise reports |

## 10. 基准测试 API / Benchmark API

### 10.1 Vitest Bench 功能 / Vitest Bench Feature

Vitest 内置基准测试支持（基于 Tinybench）：

```typescript
// src/utils/__bench__/format.bench.ts
import { bench, describe } from 'vitest';
import { formatBytes } from '../format';

describe('formatBytes 性能', () => {
  bench('小数值', () => {
    formatBytes(1024);
  });

  bench('大数值', () => {
    formatBytes(1024 * 1024 * 1024 * 5.5);
  });

  // 带选项的基准 / Bench with options
  bench('正则匹配', () => {
    /\.(csv|xlsx|parquet)$/i.test('data.csv');
  }, { iterations: 10000, time: 5000 });
});
```

### 10.2 运行基准测试 / Running Benchmarks

```bash
# 运行所有基准测试 / Run all benchmarks
pnpm vitest bench

# 输出示例 / Output example:
#  ✓ formatBytes 性能 (2) 1234ms
#    name        hz     min     max    mean     p75     p99
#    小数值   892,345  0.001   0.003   0.001   0.001   0.002
#    大数值   876,123  0.001   0.004   0.001   0.001   0.003
```

### 10.3 基准测试选项 / Benchmark Options

| 选项 / Option | 默认值 / Default | 说明 / Description |
|---|---|---|
| `iterations` | 自动 / Auto | 最小运行次数 / Minimum runs |
| `time` | 5000ms | 最大运行时间 / Maximum run time |
| `warmupTime` | 100ms | 预热时间 / Warmup time |
| `warmupIterations` | 5 | 预热次数 / Warmup iterations |
| `throws` | false | 是否抛出异常 / Whether to throw |

## 11. 重试与容错机制 / Retry & Fault Tolerance

### 11.1 测试重试配置 / Test Retry Configuration

```typescript
// vite.config.ts
export default defineConfig({
  test: {
    retry: 2,  // 失败测试最多重试 2 次 / Retry failed tests up to 2 times
    // 适用场景：偶发性异步超时、网络抖动
    // Use case: sporadic async timeouts, network jitter
  },
});

// 或在单个测试中设置 / Or set per test
it('可能不稳定的测试', { retry: 3 }, async () => {
  // 最多重试 3 次 / Retry up to 3 times
  const result = await flakyOperation();
  expect(result).toBe('success');
});
```

### 11.2 测试超时控制 / Test Timeout Control

```typescript
// 全局超时（默认 5000ms）/ Global timeout (default 5000ms)
export default defineConfig({
  test: {
    testTimeout: 10000,     // 单个测试超时 / Per-test timeout
    hookTimeout: 10000,     // beforeAll/afterAll 超时 / Hook timeout
    teardownTimeout: 5000,  // 清理超时 / Teardown timeout
  },
});

// 单个测试超时 / Per-test timeout
it('慢速操作', { timeout: 30000 }, async () => {
  await slowOperation();  // 最多等待 30s / Wait up to 30s
});
```

### 11.3 测试隔离与并发 / Test Isolation & Concurrency

```typescript
// 文件级隔离（默认）/ File-level isolation (default)
// 每个测试文件在独立 worker 中运行 / Each test file runs in separate worker

// 测试级并发控制 / Test-level concurrency control
it.concurrent('并发测试 1', async () => { /* ... */ });
it.concurrent('并发测试 2', async () => { /* ... */ });

// 顺序执行（有依赖关系时）/ Sequential execution (when dependent)
it.sequential('步骤 1', async () => { /* ... */ });
it.sequential('步骤 2', async () => { /* ... */ });

// 跳过与仅运行 / Skip and only
it.skip('暂时跳过', () => { /* ... */ });
it.only('仅运行此测试', () => { /* ... */ });  // ⚠️ 提交前移除 / Remove before commit
it.todo('待实现', () => { /* ... */ });        // 标记为待办 / Mark as todo
```

## 12. 浏览器模式（实验性）/ Browser Mode (Experimental)

### 12.1 浏览器模式概述 / Browser Mode Overview

```text
┌─────────────────────────────────────────────────────────────┐
│  Vitest 运行环境对比 / Vitest Runtime Environments             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  jsdom（本项目使用）/ jsdom (used by this project):          │
│    - Node.js 中模拟 DOM / Simulates DOM in Node.js          │
│    - 速度快，无真实浏览器 / Fast, no real browser            │
│    - 不支持：布局计算、真实事件、Canvas / No layout/events   │
│                                                             │
│  happy-dom:                                                  │
│    - 更轻量的 DOM 模拟 / Lighter DOM simulation              │
│    - 比 jsdom 快 ~2x / ~2x faster than jsdom                │
│    - API 兼容性稍差 / Slightly less API compatible           │
│                                                             │
│  browser（实验性）/ browser (experimental):                   │
│    - 真实浏览器（Chrome/Firefox）/ Real browser              │
│    - 完全真实的 DOM/CSS/事件 / Fully real DOM/CSS/events     │
│    - 需要 Playwright/WebdriverIO / Requires Playwright       │
│    - 速度较慢但最准确 / Slower but most accurate             │
└─────────────────────────────────────────────────────────────┘
```

### 12.2 环境选择指南 / Environment Selection Guide

| 场景 / Scenario | 推荐环境 / Recommended | 原因 / Reason |
|---|---|---|
| 组件逻辑测试 / Component logic | jsdom | 快速、足够 / Fast, sufficient |
| CSS/布局相关 / CSS/layout related | browser | 需真实渲染 / Needs real rendering |
| 纯函数/工具 / Pure functions | node | 无需 DOM / No DOM needed |
| 表单交互 / Form interaction | jsdom | 事件模拟足够 / Event simulation suffices |
| Canvas/WebGL | browser | jsdom 不支持 / jsdom doesn't support |
| 本项目 / This project | jsdom | 代理工具 UI，无复杂渲染 / Proxy tool UI, no complex render |

## 13. 测试工厂与 Fixture / Test Factories & Fixtures

### 13.1 工厂函数模式 / Factory Function Pattern

```typescript
// src/test/factories.ts — 测试数据工厂 / Test data factories
import type { Endpoint, ProxyResponse } from '../types/api';

// 端点工厂 / Endpoint factory
export function createEndpoint(overrides: Partial<Endpoint> = {}): Endpoint {
  return {
    id: 'test-001',
    name: 'Mask PII',
    method: 'POST',
    path: '/api/v1/mask',
    category: 'masking',
    description: '测试端点 / Test endpoint',
    ...overrides,  // 允许覆盖任意字段 / Allow overriding any field
  };
}

// 响应工厂 / Response factory
export function createResponse(overrides: Partial<ProxyResponse> = {}): ProxyResponse {
  return {
    status: 200,
    duration_ms: 42,
    body: { masked: { email: 'j***@example.com' } },
    headers: { 'content-type': 'application/json' },
    ...overrides,
  };
}

// 使用示例 / Usage example:
describe('EndpointView', () => {
  it('显示端点名称 / displays endpoint name', () => {
    const endpoint = createEndpoint({ name: 'Custom Name' });
    render(<EndpointView endpoint={endpoint} />);
    expect(screen.getByText('Custom Name')).toBeInTheDocument();
  });

  it('处理错误响应 / handles error response', () => {
    const response = createResponse({ status: 500, body: { error: 'fail' } });
    // ...
  });
});
```

### 13.2 Fixture 文件组织 / Fixture File Organization

```text
测试文件结构 / Test file structure:

src/
├── test/
│   ├── setup.ts          # 全局设置 / Global setup
│   ├── factories.ts      # 数据工厂 / Data factories
│   ├── helpers.tsx       # 自定义 render / Custom render
│   └── mocks/
│       ├── handlers.ts   # MSW 处理器 / MSW handlers
│       └── server.ts     # MSW 服务器 / MSW server
├── api/
│   ├── client.ts
│   └── client.test.ts    # 就近放置 / Co-located
└── components/
    ├── Button.tsx
    └── Button.test.tsx   # 就近放置 / Co-located
```

### 13.3 测试辅助工具 / Test Helper Utilities

```typescript
// src/test/helpers.tsx — 自定义测试工具 / Custom test utilities
import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

// 包装 Provider 的自定义 render / Custom render with providers
export function renderWithProviders(
  ui: React.ReactElement,
  options?: RenderOptions
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },  // 测试中禁用重试 / Disable retry
  });

  function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  }

  return { queryClient, ...render(ui, { wrapper: Wrapper, ...options }) };
}

// 等待异步更新的工具 / Utility for waiting async updates
export async function waitForLoadingToFinish() {
  await waitFor(() => {
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });
}
```

## 14. 异步测试模式 / Async Testing Patterns

### 14.1 异步断言策略 / Async Assertion Strategies

```typescript
import { waitFor, findBy, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// 策略 1: findBy* (推荐) / Strategy 1: findBy* (recommended)
it('加载并显示数据 / loads and displays data', async () => {
  render(<DataPanel />);
  // findBy = getBy + waitFor / findBy = getBy + waitFor
  const item = await screen.findByText('Masked Email');
  expect(item).toBeVisible();
});

// 策略 2: waitFor 显式等待 / Strategy 2: explicit waitFor
it('更新状态 / updates state', async () => {
  render(<StatusIndicator />);
  await userEvent.click(screen.getByRole('button'));

  await waitFor(() => {
    expect(screen.getByText('Success')).toBeInTheDocument();
  }, { timeout: 3000 });  // 自定义超时 / Custom timeout
});

// 策略 3: waitForElementToBeRemoved / Strategy 3: waitForElementToBeRemoved
it('加载完成后移除 spinner / removes spinner after load', async () => {
  render(<AsyncComponent />);
  expect(screen.getByRole('progressbar')).toBeInTheDocument();

  await waitForElementToBeRemoved(() => screen.queryByRole('progressbar'));
  expect(screen.getByText('Data loaded')).toBeInTheDocument();
});
```

### 14.2 Mock 异步 API / Mocking Async APIs

```typescript
// 使用 vi.fn() mock 异步函数 / Mock async functions with vi.fn()
import { vi } from 'vitest';
import * as api from '../api/client';

// 方法 1: mockResolvedValue / Method 1: mockResolvedValue
vi.spyOn(api, 'sendRequest').mockResolvedValue({
  status: 200,
  body: { result: 'ok' },
  duration_ms: 10,
});

// 方法 2: mockImplementation / Method 2: mockImplementation
vi.spyOn(api, 'sendRequest').mockImplementation(async (req) => {
  if (req.path === '/error') throw new Error('Network error');
  return { status: 200, body: {}, duration_ms: 5 };
});

// 方法 3: MSW 服务器级 mock / Method 3: MSW server-level mock
import { http, HttpResponse } from 'msw';
import { server } from '../test/mocks/server';

it('处理服务器错误 / handles server error', async () => {
  server.use(
    http.post('/api/proxy', () => {
      return HttpResponse.json({ error: 'timeout' }, { status: 504 });
    })
  );

  render(<ProxyForm />);
  await userEvent.click(screen.getByText('Send'));
  expect(await screen.findByText(/timeout/)).toBeInTheDocument();
});
```

### 14.3 定时器与延迟测试 / Timer & Delay Testing

```typescript
// 使用 fake timers 测试延迟逻辑 / Test delayed logic with fake timers
it('防抖搜索 / debounced search', async () => {
  vi.useFakeTimers();
  const onSearch = vi.fn();
  render(<SearchInput onSearch={onSearch} debounceMs={300} />);

  await userEvent.type(screen.getByRole('textbox'), 'mask', {
    delay: null,  // fake timers 时禁用延迟 / Disable delay with fake timers
  });

  expect(onSearch).not.toHaveBeenCalled();  // 尚未触发 / Not yet triggered
  vi.advanceTimersByTime(300);               // 前进 300ms / Advance 300ms
  expect(onSearch).toHaveBeenCalledWith('mask');

  vi.useRealTimers();  // 恢复真实定时器 / Restore real timers
});
```

## 15. 类型测试与静态验证 / Type Testing & Static Validation

### 15.1 Vitest 类型测试 API / Vitest Type Testing API

```typescript
// Vitest 支持编译时类型测试 / Vitest supports compile-time type tests
import { expectTypeOf, test } from 'vitest';
import { formatBytes } from '../utils/format';
import type { Endpoint } from '../types/api';

test('formatBytes 类型 / formatBytes types', () => {
  // 验证返回类型 / Verify return type
  expectTypeOf(formatBytes).returns.toBeString();

  // 验证参数类型 / Verify parameter types
  expectTypeOf(formatBytes).parameter(0).toBeNumber();

  // 验证类型等价 / Verify type equality
  expectTypeOf<Endpoint['method']>().toEqualTypeOf<'GET' | 'POST' | 'PUT' | 'DELETE'>();
});

test('API 响应类型 / API response types', () => {
  expectTypeOf<Endpoint>().toHaveProperty('id');
  expectTypeOf<Endpoint>().toHaveProperty('path');
  expectTypeOf<Endpoint['id']>().toBeString();

  // 可调用性检查 / Callability check
  expectTypeOf(formatBytes).toBeCallableWith(1024);
  // @ts-expect-error — 不接受字符串 / Doesn't accept string
  expectTypeOf(formatBytes).toBeCallableWith('invalid');
});
```

### 15.2 运行类型测试 / Running Type Tests

```bash
# 类型测试与运行时测试一起执行 / Type tests run alongside runtime tests
pnpm vitest --typecheck

# 配置 / Configuration:
# vite.config.ts
export default defineConfig({
  test: {
    typecheck: {
      enabled: true,           // 启用类型测试 / Enable type testing
      tsconfig: './tsconfig.json',
      include: ['**/*.test-d.ts'],  // 类型测试文件 / Type test files
    },
  },
});
```

### 15.3 本项目类型安全实践 / This Project's Type Safety Practices

| 实践 / Practice | 实现 / Implementation | 作用 / Purpose |
|---|---|---|
| 严格模式 / Strict mode | tsconfig `strict: true` | 编译时捕获错误 / Catch errors at compile |
| API 类型 / API types | `types/api.ts` 集中定义 / Centralized | 前后端契约 / Frontend-backend contract |
| 工厂函数类型 / Factory types | `Partial<T>` 覆盖 / Override | 类型安全的测试数据 / Type-safe test data |
| noUncheckedIndexedAccess | 索引访问返回 `T | undefined` | 防止越界 / Prevent out-of-bounds |
| 泛型组件 / Generic components | `DataList<T>` | 复用且类型安全 / Reusable & type-safe |

## 16. 自定义 Reporter / Custom Reporter

### 16.1 Reporter 架构 / Reporter Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  Vitest Reporter 架构 / Vitest Reporter Architecture           │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Test Runner                                                   │
│    │                                                           │
│    ├── 事件发射 / Event emission                               │
│    │   ├── onInit         ← 初始化 / Initialize               │
│    │   ├── onCollected    ← 收集完成 / Collection done        │
│    │   ├── onTaskUpdate   ← 任务状态变化 / Task state change  │
│    │   ├── onFinished     ← 全部完成 / All finished           │
│    │   └── onWatcherRerun ← 重新运行 / Rerun                  │
│    │                                                           │
│    └── Reporter 实例 / Reporter instances                      │
│        ├── default (终端输出 / Terminal output)                │
│        ├── json (机器可读 / Machine readable)                  │
│        ├── junit (CI 集成 / CI integration)                    │
│        ├── html (可视化报告 / Visual report)                   │
│        └── custom (自定义 / Custom)                            │
└────────────────────────────────────────────────────────────────┘
```

### 16.2 自定义 Reporter 实现 / Custom Reporter Implementation

```typescript
// vitest.reporter.ts - 自定义测试报告器
// Custom test reporter
import type { Reporter, File, TaskResultPack } from 'vitest';

export default class SlackReporter implements Reporter {
  private startTime = 0;
  private passed = 0;
  private failed = 0;

  onInit() {
    this.startTime = Date.now();
  }

  onFinished(files: File[]) {
    const duration = Date.now() - this.startTime;

    for (const file of files) {
      for (const task of file.tasks) {
        if (task.result?.state === 'pass') this.passed++;
        if (task.result?.state === 'fail') this.failed++;
      }
    }

    // 输出摘要 / Output summary
    const summary = [
      `✅ Passed: ${this.passed}`,
      `❌ Failed: ${this.failed}`,
      `⏱️  Duration: ${(duration / 1000).toFixed(1)}s`,
    ].join('\n');

    console.log('\n' + '='.repeat(50));
    console.log('Test Summary / 测试摘要');
    console.log('='.repeat(50));
    console.log(summary);

    // 可选：发送到外部服务 / Optional: send to external service
    // await sendToSlack(summary);
  }
}
```

### 16.3 Reporter 配置 / Reporter Configuration

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // 多 Reporter 并行 / Multiple reporters in parallel
    reporters: [
      'default',                          // 终端输出 / Terminal
      'json',                             // JSON 文件 / JSON file
      ['json', { outputFile: 'results.json' }],
      './vitest.reporter.ts',             // 自定义 / Custom
    ],
    outputFile: {
      json: './coverage/results.json',
      junit: './coverage/junit.xml',
    },
  },
});
```

### 16.4 本项目 Reporter 实践 / This Project's Reporter Practice

| Reporter | 用途 / Purpose | 环境 / Environment |
|---|---|---|
| default | 开发时终端反馈 / Dev terminal feedback | 本地 / Local |
| json | CI 结果解析 / CI result parsing | CI |
| verbose | 调试时详细输出 / Debug detailed output | 调试 / Debug |
| 自定义 / Custom | 未使用 / Not used | - |

## 17. 全局 Setup 与 Teardown / Global Setup & Teardown

### 17.1 全局设置架构 / Global Setup Architecture

```text
┌────────────────────────────────────────────────────────────────┐
│  Vitest 全局设置生命周期 / Vitest Global Setup Lifecycle        │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  globalSetup (一次 / once)                                     │
│    │                                                           │
│    ├── setup()    ← 所有测试前 / Before all tests              │
│    │   ├── 启动 mock 服务器 / Start mock server                │
│    │   ├── 初始化数据库 / Initialize database                  │
│    │   └── 设置环境变量 / Set env variables                    │
│    │                                                           │
│    ├── [测试执行 / Tests execute]                              │
│    │   ├── setupFiles (每个文件 / per file)                    │
│    │   ├── beforeEach (每个测试 / per test)                    │
│    │   └── afterEach (每个测试 / per test)                     │
│    │                                                           │
│    └── teardown() ← 所有测试后 / After all tests               │
│        ├── 关闭服务器 / Close servers                          │
│        ├── 清理临时文件 / Clean temp files                     │
│        └── 生成报告 / Generate reports                         │
└────────────────────────────────────────────────────────────────┘
```

### 17.2 全局设置实现 / Global Setup Implementation

```typescript
// vitest.global-setup.ts - 全局设置文件
// Global setup file
import type { GlobalSetupContext } from 'vitest/node';

export async function setup({ provide }: GlobalSetupContext) {
  // 在所有测试前执行 / Execute before all tests
  console.log('[Global Setup] Starting test environment...');

  // 示例：启动 MSW 服务器 / Example: start MSW server
  // const server = setupServer(...handlers);
  // server.listen();

  // 示例：设置全局配置 / Set global config
  provide('apiBaseUrl', 'http://localhost:8080');
  provide('testStartTime', Date.now());

  // 返回 teardown 函数 / Return teardown function
  return async () => {
    console.log('[Global Teardown] Cleaning up...');
    // server.close();
  };
}

// vitest.setup.ts - 每个测试文件的 setup
// Per-test-file setup
import { afterEach, beforeEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// 每个测试后自动清理 DOM
// Auto-cleanup DOM after each test
afterEach(() => {
  cleanup();
});

// 可选：重置所有 mock / Optional: reset all mocks
afterEach(() => {
  vi.clearAllMocks();
});
```

### 17.3 配置集成 / Configuration Integration

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // 全局设置（一次）/ Global setup (once)
    globalSetup: './vitest.global-setup.ts',

    // 每个测试文件的 setup / Per-file setup
    setupFiles: ['./vitest.setup.ts'],

    // 环境 / Environment
    environment: 'jsdom',

    // 超时 / Timeout
    testTimeout: 10000,
    hookTimeout: 10000,
  },
});
```

### 17.4 本项目 Setup 实践 / This Project's Setup Practice

| 层级 / Layer | 文件 / File | 内容 / Content |
|---|---|---|
| 全局 / Global | vitest.global-setup.ts | MSW 服务器初始化 / MSW server init |
| 文件级 / File-level | vitest.setup.ts | cleanup + mock reset |
| 测试级 / Test-level | beforeEach/afterEach | 特定状态重置 / Specific state reset |

## 18. 测试驱动开发工作流 / Test-Driven Development Workflow

### 18.1 TDD 循环 / TDD Cycle

```text
┌────────────────────────────────────────────────────────────────┐
│  TDD 红-绿-重构循环 / TDD Red-Green-Refactor Cycle             │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│       ┌───────────┐                                          │
│       │  🔴 RED   │  写失败测试 / Write failing test           │
│       └─────┬─────┘                                          │
│             │                                                 │
│             ▼                                                 │
│       ┌───────────┐                                          │
│       │ 🟢 GREEN  │  最小实现 / Minimal implementation         │
│       └─────┬─────┘                                          │
│             │                                                 │
│             ▼                                                 │
│       ┌───────────┐                                          │
│       │ 🔵 REFACTOR│  优化代码 / Optimize code                 │
│       └─────┬─────┘                                          │
│             │                                                 │
│             └─────────── 循环 / Loop ───────────────┐    │
│                                                 │    │
│                                                 ▼    │
│                                              完成 / Done │
└────────────────────────────────────────────────────────────────┘
```

### 18.2 Vitest Watch 模式 / Vitest Watch Mode

```bash
# TDD 工作流命令 / TDD workflow commands

# 1. 启动 watch 模式 / Start watch mode
pnpm vitest --watch
# 文件变更时自动重新运行相关测试
# Auto-rerun related tests on file change

# 2. 仅运行特定文件 / Run specific file only
pnpm vitest --watch src/components/MaskingForm.test.tsx

# 3. 仅运行失败的测试 / Run only failed tests
# 在 watch 模式中按 'f' / Press 'f' in watch mode

# 4. UI 模式（可视化）/ UI mode (visual)
pnpm vitest --ui
# 打开浏览器界面 / Opens browser interface
```

### 18.3 TDD 实战示例 / TDD Practical Example

```typescript
// 步骤 1 (RED): 写失败测试 / Step 1 (RED): Write failing test
describe('formatDuration', () => {
  it('formats milliseconds to human-readable string', () => {
    expect(formatDuration(42)).toBe('42ms');
    expect(formatDuration(1500)).toBe('1.5s');
    expect(formatDuration(65000)).toBe('1m 5s');
  });
});
// → 测试失败：formatDuration 未定义 / Test fails: undefined

// 步骤 2 (GREEN): 最小实现 / Step 2 (GREEN): Minimal implementation
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.round((ms % 60000) / 1000);
  return `${minutes}m ${seconds}s`;
}
// → 测试通过 / Test passes

// 步骤 3 (REFACTOR): 优化 / Step 3 (REFACTOR): Optimize
// - 提取常量 / Extract constants
// - 添加边界测试 / Add edge case tests
// - 确保重构后测试仍通过 / Ensure tests still pass after refactor
```

### 18.4 本项目 TDD 实践 / This Project's TDD Practice

| 实践 / Practice | 说明 / Description |
|---|---|
| 新功能先写测试 / Test first for new features | 确保需求明确 / Ensure clear requirements |
| Bug 修复先写回归测试 / Regression test before fix | 防止复发 / Prevent recurrence |
| watch 模式开发 / Watch mode development | 即时反馈 / Instant feedback |
| 小步提交 / Small commits | 每个红-绿循环一次提交 / Commit per red-green cycle |
| 重构时保持测试通过 / Keep tests green during refactor | 安全重构 / Safe refactoring |

## 19. Vitest 与 CI/CD 集成 / Vitest & CI/CD Integration

### 19.1 GitHub Actions 工作流 / GitHub Actions Workflow

```yaml
# .github/workflows/test.yml
name: Test Suite

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        node-version: [18, 20, 22]
    steps:
      - uses: actions/checkout@v4
      
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ matrix.node-version }}
          cache: 'pnpm'
          cache-dependency-path: console/web/pnpm-lock.yaml
      
      - name: Install dependencies
        run: cd console/web && pnpm install --frozen-lockfile
      
      - name: Run tests with coverage
        run: cd console/web && pnpm vitest run --coverage --reporter=json --outputFile=coverage.json
      
      - name: Upload coverage
        if: matrix.node-version == 20
        uses: codecov/codecov-action@v4
        with:
          files: console/web/coverage.json
          flags: unittests

  e2e-tests:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: 'pnpm'
      
      - name: Install & Build
        run: |
          cd console/web
          pnpm install --frozen-lockfile
          pnpm build
      
      - name: Start backend
        run: |
          cd console/bff-go
          go run ./cmd/server &
      
      - name: Run E2E
        run: cd console/web && pnpm test:e2e
```

### 19.2 覆盖率门控与报告 / Coverage Gating & Reporting

```typescript
// vitest.config.ts 中的覆盖率配置
// Coverage configuration in vitest.config.ts
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html', 'lcov'],
      // 覆盖率阈值门控
      // Coverage threshold gates
      thresholds: {
        global: {
          branches: 80,
          functions: 80,
          lines: 80,
          statements: 80,
        },
        // 每个文件的最低要求
        // Per-file minimum requirements
        perFile: true,
      },
      // 排除不需要覆盖的文件
      // Exclude files that don't need coverage
      exclude: [
        'node_modules/**',
        'dist/**',
        '**/*.d.ts',
        '**/vitest.config.*',
        'src/main.tsx',  // 入口文件 / Entry file
      ],
    },
  },
})
```

### 19.3 测试分片与并行 / Test Sharding & Parallelism

```bash
# 大型项目中将测试分片到多个 CI 节点
# Shard tests across multiple CI nodes in large projects

# 节点 1：运行第 1/4 片
# Node 1: Run shard 1/4
vitest run --shard=1/4

# 节点 2：运行第 2/4 片
# Node 2: Run shard 2/4
vitest run --shard=2/4

# 节点 3：运行第 3/4 片
# Node 3: Run shard 3/4
vitest run --shard=3/4

# 节点 4：运行第 4/4 片
# Node 4: Run shard 4/4
vitest run --shard=4/4
```

```yaml
# GitHub Actions 分片矩阵
# GitHub Actions sharding matrix
test-sharded:
  runs-on: ubuntu-latest
  strategy:
    matrix:
      shard: [1/4, 2/4, 3/4, 4/4]
  steps:
    - uses: actions/checkout@v4
    - run: pnpm install --frozen-lockfile
    - run: pnpm vitest run --shard=${{ matrix.shard }}
```

### 19.4 失败重试与 Flaky 检测 / Failure Retry & Flaky Detection

```typescript
// vitest.config.ts
export default defineConfig({
  test: {
    // 全局重试配置
    // Global retry configuration
    retry: 2,  // 失败后重试 2 次 / Retry 2 times on failure
    
    // 测试超时
    // Test timeout
    testTimeout: 10000,  // 10s
    hookTimeout: 15000,  // 15s
  },
})
```

```typescript
// 单个测试的重试控制
// Per-test retry control
import { test, expect } from 'vitest'

// 已知不稳定的测试标记重试
// Mark known flaky tests with retry
test('network dependent test', { retry: 3 }, async () => {
  const response = await fetch('/api/data')
  expect(response.ok).toBe(true)
})

// 使用 fails 标记已知失败的测试
// Use fails to mark known failing tests
test.fails('known bug #123', () => {
  expect(buggyFunction()).toBe('expected')
})
```

### 19.5 CI 性能优化策略 / CI Performance Optimization

| 策略 / Strategy | 配置 / Config | 效果 / Effect |
|---|---|---|
| 依赖缓存 / Dependency cache | pnpm store cache | 安装时间 -60% / Install -60% |
| 测试分片 / Test sharding | --shard=N/M | 总时间 -75% / Total -75% |
| 变更检测 / Change detection | --changed=main | 仅跑受影响测试 / Only affected |
| 并行执行 / Parallel execution | 默认开启 / On by default | CPU 充分利用 / Full CPU use |
| 覆盖率按需 / Coverage on demand | 仅主分支 / Main branch only | PR 更快 / Faster PRs |

## 20. 测试架构设计 / Test Architecture Design

### 20.1 测试金字塔实践 / Testing Pyramid Practice

```
        ╱╲
       ╱  ╲        E2E 测试 (5%)
      ╱────╲       • 完整用户流程 / Full user flows
     ╱      ╲      • Playwright / Playwright
    ╱────────╲
   ╱          ╲    集成测试 (25%)
  ╱────────────╲   • API 调用链 / API call chains
 ╱              ╲  • MSW Mock / MSW Mock
╱────────────────╲
╱                ╲  单元测试 (70%)
╱──────────────────╲ • 纯函数 / Pure functions
                    • 组件逻辑 / Component logic
                    • Hooks / Hooks
```

### 20.2 测试文件组织模式 / Test File Organization Patterns

```
src/
├── components/
│   ├── MaskForm/
│   │   ├── MaskForm.tsx          # 组件 / Component
│   │   ├── MaskForm.test.tsx     # 单元测试 / Unit test
│   │   ├── MaskForm.stories.tsx  # Storybook (可选)
│   │   └── index.ts              # 导出 / Export
│   └── DPPanel/
│       ├── DPPanel.tsx
│       └── DPPanel.test.tsx
├── hooks/
│   ├── usePrivacyApi.ts
│   ├── usePrivacyApi.test.ts     # Hook 测试 / Hook test
│   └── __mocks__/                # Hook 专用 mock
├── utils/
│   ├── format.ts
│   └── format.test.ts
├── services/
│   ├── api.ts
│   └── api.test.ts               # 集成测试 / Integration test
└── __tests__/                    # 跨模块集成测试
    └── e2e-flows.test.ts
```

### 20.3 测试工具函数库 / Test Utility Library

```typescript
// src/test-utils/index.ts
import { render, RenderOptions } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ReactElement, ReactNode } from 'react'

// 自定义 render 包含所有 Provider
// Custom render includes all Providers
export function renderWithProviders(
  ui: ReactElement,
  options?: {
    queryClient?: QueryClient
    initialEntries?: string[]
  } & Omit<RenderOptions, 'wrapper'>
) {
  const {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },  // 测试中禁用重试 / Disable retry in tests
      },
    }),
    ...renderOptions
  } = options ?? {}

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )
  }

  return {
    queryClient,
    ...render(ui, { wrapper: Wrapper, ...renderOptions }),
  }
}

// 测试数据工厂
// Test data factory
export function createMockMaskResponse(overrides = {}) {
  return {
    masked_data: { name: '张**', phone: '138****1234' },
    mask_details: [
      { field: 'name', strategy: 'partial', original_length: 3 },
      { field: 'phone', strategy: 'partial', original_length: 11 },
    ],
    ...overrides,
  }
}

// 等待异步操作完成
// Wait for async operations to complete
export async function waitForApiCalls(ms = 100) {
  await new Promise(resolve => setTimeout(resolve, ms))
}
```

### 20.4 Mock 服务层设计 / Mock Service Layer Design

```typescript
// src/test-utils/mocks/handlers.ts
import { http, HttpResponse } from 'msw'

// 统一的 API Mock 处理器
// Unified API Mock handlers
export const handlers = [
  // 脱敏接口
  // Mask endpoint
  http.post('/api/v1/mask', async ({ request }) => {
    const body = await request.json()
    
    // 模拟错误场景
    // Simulate error scenarios
    if (body.data?.trigger_error) {
      return HttpResponse.json(
        { error: 'Internal Server Error' },
        { status: 500 }
      )
    }
    
    // 模拟延迟
    // Simulate latency
    await delay(100)
    
    return HttpResponse.json({
      masked_data: { name: '张**' },
      mask_details: [{ field: 'name', strategy: 'partial' }],
    })
  }),
  
  // 差分隐私接口
  // DP endpoint
  http.post('/api/v1/dp/count', async () => {
    return HttpResponse.json({
      noisy_count: 1042,
      epsilon: 1.0,
      noise_added: 42,
    })
  }),
]

// src/test-utils/mocks/server.ts
import { setupServer } from 'msw/node'
import { handlers } from './handlers'

export const server = setupServer(...handlers)
```

### 20.5 测试架构原则 / Test Architecture Principles

| 原则 / Principle | 说明 / Description | 实践 / Practice |
|---|---|---|
| 测试独立性 / Test isolation | 每个测试独立运行 / Each test runs independently | beforeEach 重置 / Reset |
| 确定性 / Determinism | 无随机失败 / No random failures | Mock 时间/随机数 / Mock time/random |
| 快速反馈 / Fast feedback | 单元测试 < 5s / Unit tests < 5s | 并行 + 分片 / Parallel + shard |
| 可读性 / Readability | 测试即文档 / Tests as documentation | AAA 模式 / AAA pattern |
| 维护性 / Maintainability | 低耦合 / Low coupling | 工具函数复用 / Utility reuse |

## 21. 性能测试与基准分析 / Performance Testing & Benchmarking

### 21.1 Vitest Bench API 详解 / Vitest Bench API Details

```typescript
// src/utils/__bench__/format.bench.ts
import { bench, describe } from 'vitest'
import { formatBytes, formatDuration, formatNumber } from '../format'

describe('formatBytes performance', () => {
  const sizes = [0, 1024, 1048576, 1073741824, 1099511627776]
  
  bench('formatBytes - small values', () => {
    formatBytes(1024)
    formatBytes(2048)
    formatBytes(512)
  })
  
  bench('formatBytes - large values', () => {
    formatBytes(1073741824)
    formatBytes(1099511627776)
  })
  
  bench('formatBytes - batch 1000', () => {
    for (let i = 0; i < 1000; i++) {
      formatBytes(sizes[i % sizes.length])
    }
  })
})

describe('formatNumber algorithms', () => {
  const data = Array.from({ length: 10000 }, () => Math.random())
  
  bench('Intl.NumberFormat', () => {
    const fmt = new Intl.NumberFormat('zh-CN')
    data.forEach(n => fmt.format(n))
  })
  
  bench('manual toLocaleString', () => {
    data.forEach(n => n.toLocaleString('zh-CN'))
  })
  
  bench('regex formatting', () => {
    data.forEach(n => n.toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, ','))
  })
})
```

### 21.2 组件渲染性能测试 / Component Rendering Performance

```typescript
// src/components/__bench__/Table.bench.tsx
import { bench, describe } from 'vitest'
import { renderToString } from 'react-dom/server'
import { DataTable } from '../DataTable'

describe('DataTable render performance', () => {
  const generateRows = (count: number) =>
    Array.from({ length: count }, (_, i) => ({
      id: i,
      field: `field_${i}`,
      classification: 'P' + (i % 4 + 1),
      confidence: Math.random(),
    }))
  
  bench('render 10 rows', () => {
    renderToString(<DataTable rows={generateRows(10)} />)
  })
  
  bench('render 100 rows', () => {
    renderToString(<DataTable rows={generateRows(100)} />)
  })
  
  bench('render 1000 rows', () => {
    renderToString(<DataTable rows={generateRows(1000)} />)
  })
})
```

### 21.3 内存泄漏检测 / Memory Leak Detection

```typescript
// src/utils/__tests__/memory.test.ts
import { test, expect, afterEach } from 'vitest'

test('event listener cleanup - no memory leak', () => {
  const addSpy = vi.spyOn(window, 'addEventListener')
  const removeSpy = vi.spyOn(window, 'removeEventListener')
  
  const { unmount } = render(<ResizeObserverComponent />)
  
  // 确认添加了监听器
  // Confirm listener was added
  expect(addSpy).toHaveBeenCalledWith('resize', expect.any(Function))
  
  // 卸载后必须移除
  // Must remove after unmount
  unmount()
  expect(removeSpy).toHaveBeenCalledWith('resize', expect.any(Function))
  
  addSpy.mockRestore()
  removeSpy.mockRestore()
})

test('interval cleanup - no dangling timers', () => {
  vi.useFakeTimers()
  
  const { unmount } = render(<PollingComponent interval={5000} />)
  
  // 确认定时器已创建
  // Confirm timer was created
  expect(vi.getTimerCount()).toBe(1)
  
  // 卸载后定时器必须清除
  // Timer must be cleared after unmount
  unmount()
  expect(vi.getTimerCount()).toBe(0)
  
  vi.useRealTimers()
})
```

### 21.4 性能回归防护 / Performance Regression Prevention

```typescript
// src/utils/__tests__/perf-regression.test.ts
import { test, expect } from 'vitest'
import { classifyFields } from '../classification'

test('classifyFields processes 1000 fields within 50ms', () => {
  const fields = Array.from({ length: 1000 }, (_, i) => ({
    name: `field_${i}`,
    value: `value_${i}`,
    type: i % 3 === 0 ? 'string' : 'number',
  }))
  
  const start = performance.now()
  const result = classifyFields(fields)
  const elapsed = performance.now() - start
  
  // 性能断言：必须在 50ms 内完成
  // Performance assertion: must complete within 50ms
  expect(elapsed).toBeLessThan(50)
  expect(result).toHaveLength(1000)
})

test('debounce does not fire more than expected', () => {
  vi.useFakeTimers()
  const fn = vi.fn()
  const debounced = debounce(fn, 300)
  
  // 快速连续调用 100 次
  // Rapidly call 100 times
  for (let i = 0; i < 100; i++) {
    debounced(i)
  }
  
  vi.advanceTimersByTime(300)
  
  // 只应触发一次
  // Should only fire once
  expect(fn).toHaveBeenCalledTimes(1)
  expect(fn).toHaveBeenCalledWith(99)
  
  vi.useRealTimers()
})
```

### 21.5 性能测试最佳实践 / Performance Testing Best Practices

| 实践 / Practice | 说明 / Description | 示例 / Example |
|---|---|---|
| 基线建立 / Baseline establishment | 首次运行记录基准 / Record baseline on first run | bench --reporter=json |
| 回归阈值 / Regression threshold | 允许 ±10% 波动 / Allow ±10% variance | expect(elapsed).toBeLessThan(baseline * 1.1) |
| 环境隔离 / Environment isolation | CI 专用 runner / Dedicated CI runner | 避免共享资源竞争 / Avoid resource contention |
| 预热运行 / Warmup runs | 排除 JIT 影响 / Exclude JIT effects | bench 自动预热 / bench auto-warmup |
| 多次采样 / Multiple samples | 统计显著性 / Statistical significance | iterations: 1000 |

## 22. Mock 服务与外部依赖集成 / Mock Services & External Dependency Integration

在复杂应用中，测试经常需要与外部服务交互（API、数据库、消息队列）。Vitest 提供了多层次的 mock 机制，从函数级到网络级，确保测试的独立性和确定性。

In complex applications, tests often need to interact with external services (APIs, databases, message queues). Vitest provides multi-level mock mechanisms, from function-level to network-level, ensuring test independence and determinism.

### 22.1 MSW 网络层 Mock / MSW Network Layer Mock

```typescript
// src/mocks/handlers.ts
// MSW (Mock Service Worker) 处理器定义
// MSW handler definitions for API mocking
import { http, HttpResponse, delay } from 'msw'

// 模拟隐私服务 API / Mock privacy service API
export const handlers = [
  // === Masking API ===
  http.post('/api/v1/mask', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>
    
    // 模拟网络延迟 / Simulate network latency
    await delay(50)
    
    // 根据输入生成模拟响应 / Generate mock response based on input
    const data = (body as any).data || {}
    const maskedData: Record<string, string> = {}
    for (const [key, value] of Object.entries(data)) {
      maskedData[key] = String(value).slice(0, 2) + '***'
    }
    
    return HttpResponse.json({
      masked_data: maskedData,
      masked_fields: Object.keys(data),
      profile_used: 'default',
    })
  }),

  // === Classification API ===
  http.post('/api/v1/classify', async ({ request }) => {
    await delay(80)
    
    return HttpResponse.json({
      results: [
        { field: 'email', category: 'PII', level: 3, confidence: 0.95 },
        { field: 'id_card', category: 'PII', level: 4, confidence: 0.99 },
      ],
      summary: { total_fields: 5, sensitive_fields: 2, max_level: 4 },
    })
  }),

  // === DP Query API ===
  http.post('/api/v1/dp/query', async () => {
    await delay(30)
    
    return HttpResponse.json({
      result: 42.7,
      epsilon: 0.5,
      mechanism: 'laplace',
      noise_added: 2.3,
      budget_remaining: 1.5,
    })
  }),

  // === 错误场景 / Error scenarios ===
  http.post('/api/v1/mask', async ({ request }) => {
    const body = await request.json() as any
    
    // 模拟预算耗尽 / Simulate budget exhaustion
    if (body?.data?.trigger === 'budget_exhausted') {
      return HttpResponse.json(
        { error: 'Privacy budget exhausted', code: 'BUDGET_EXHAUSTED' },
        { status: 429 }
      )
    }
    
    // 模拟服务不可用 / Simulate service unavailable
    if (body?.data?.trigger === 'server_error') {
      return HttpResponse.json(
        { error: 'Internal server error' },
        { status: 500 }
      )
    }
    
    return HttpResponse.json({ masked_data: {}, masked_fields: [], profile_used: 'default' })
  }),
]

// src/mocks/server.ts
import { setupServer } from 'msw/node'
import { handlers } from './handlers'

export const server = setupServer(...handlers)
```

### 22.2 模块级 Mock 策略 / Module-level Mock Strategy

```typescript
// src/services/__tests__/privacy-client.test.ts
import { describe, test, expect, vi, beforeEach } from 'vitest'

// === 策略 1: vi.mock 整个模块 / Strategy 1: Mock entire module ===
vi.mock('../crypto-utils', () => ({
  encrypt: vi.fn((data: string) => `encrypted_${data}`),
  decrypt: vi.fn((data: string) => data.replace('encrypted_', '')),
  generateKey: vi.fn(() => 'mock-key-12345'),
}))

// === 策略 2: 部分 mock / Strategy 2: Partial mock ===
vi.mock('../logger', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../logger')>()
  return {
    ...actual,           // 保留真实实现 / Keep real implementation
    logger: {            // 只 mock logger / Only mock logger
      info: vi.fn(),
      error: vi.fn(),
      warn: vi.fn(),
    },
  }
})

// === 策略 3: 动态 mock / Strategy 3: Dynamic mock ===
import { PrivacyClient } from '../privacy-client'
import { encrypt } from '../crypto-utils'

describe('PrivacyClient', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('使用加密工具处理敏感数据 / uses crypto utils for sensitive data', async () => {
    const client = new PrivacyClient({ baseUrl: 'http://localhost:8079' })
    
    await client.maskField('id_card', '110101199001011234')
    
    // 验证加密被调用 / Verify encryption was called
    expect(encrypt).toHaveBeenCalledWith('110101199001011234')
  })

  test('加密失败时回退到明文 / falls back to plaintext on encryption failure', async () => {
    // 动态修改 mock 行为 / Dynamically modify mock behavior
    vi.mocked(encrypt).mockImplementationOnce(() => {
      throw new Error('Encryption service unavailable')
    })
    
    const client = new PrivacyClient({ baseUrl: 'http://localhost:8079' })
    const result = await client.maskField('name', '张三')
    
    // 应该回退而不是抛异常 / Should fallback, not throw
    expect(result).toBeDefined()
  })
})
```

### 22.3 定时器与异步 Mock / Timer & Async Mock

```typescript
// 定时器密集组件测试 / Timer-intensive component testing
import { describe, test, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { AutoRefreshPanel } from '../components/AutoRefreshPanel'

describe('AutoRefreshPanel 定时器测试 / Timer tests', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  test('每 5s 自动刷新 / auto-refreshes every 5s', async () => {
    vi.useFakeTimers()
    const onRefresh = vi.fn()
    
    render(<AutoRefreshPanel interval={5000} onRefresh={onRefresh} />)
    
    // 初始不触发 / No initial trigger
    expect(onRefresh).not.toHaveBeenCalled()
    
    // 前进 5s / Advance 5s
    await act(async () => {
      vi.advanceTimersByTime(5000)
    })
    expect(onRefresh).toHaveBeenCalledTimes(1)
    
    // 再前进 10s = 又触发 2 次 / Advance 10s = 2 more triggers
    await act(async () => {
      vi.advanceTimersByTime(10000)
    })
    expect(onRefresh).toHaveBeenCalledTimes(3)
  })

  test('组件卸载后停止刷新 / stops refreshing after unmount', async () => {
    vi.useFakeTimers()
    const onRefresh = vi.fn()
    
    const { unmount } = render(
      <AutoRefreshPanel interval={3000} onRefresh={onRefresh} />
    )
    
    // 卸载 / Unmount
    unmount()
    
    // 前进时间，不应触发 / Advance time, should not trigger
    await act(async () => {
      vi.advanceTimersByTime(30000)
    })
    expect(onRefresh).not.toHaveBeenCalled()
  })

  test('重试逻辑使用指数退避 / retry uses exponential backoff', async () => {
    vi.useFakeTimers()
    const fetchFn = vi.fn()
      .mockRejectedValueOnce(new Error('fail 1'))
      .mockRejectedValueOnce(new Error('fail 2'))
      .mockResolvedValueOnce({ data: 'success' })
    
    render(<AutoRefreshPanel fetchFn={fetchFn} retryBackoff />)
    
    // 第 1 次失败，等待 1s / 1st failure, wait 1s
    await act(async () => { vi.advanceTimersByTime(1000) })
    expect(fetchFn).toHaveBeenCalledTimes(2)
    
    // 第 2 次失败，等待 2s / 2nd failure, wait 2s
    await act(async () => { vi.advanceTimersByTime(2000) })
    expect(fetchFn).toHaveBeenCalledTimes(3)
    
    // 第 3 次成功 / 3rd succeeds
    await screen.findByText('success')
  })
})
```

### 22.4 Mock 策略选择指南 / Mock Strategy Selection Guide

| 场景 / Scenario | 推荐策略 / Recommended | 工具 / Tool | 原因 / Reason |
|---|---|---|---|
| HTTP API 调用 / HTTP API calls | 网络层 mock / Network mock | MSW | 最接近真实 / Most realistic |
| 工具函数 / Utility functions | 模块 mock / Module mock | vi.mock | 简单直接 / Simple |
| 定时器 / Timers | 假定时器 / Fake timers | vi.useFakeTimers | 确定性 / Deterministic |
| 随机数 / Random | 固定种子 / Fixed seed | vi.spyOn(Math) | 可重现 / Reproducible |
| 文件系统 / File system | 内存 FS / In-memory FS | memfs | 无副作用 / No side effects |
| 数据库 / Database | 内存 DB / In-memory DB | better-sqlite3 | 真实 SQL / Real SQL |
| 日期时间 / DateTime | 固定时间 / Fixed time | vi.setSystemTime | 确定性 / Deterministic |
| 第三方 SDK / 3rd-party SDK | 接口 mock / Interface mock | vi.fn | 隔离 / Isolation |

## 23. 测试可观测性与诊断 / Test Observability & Diagnostics

测试可观测性是指能够深入理解测试执行过程、快速定位失败原因、并持续监控测试健康度的能力。在大型测试套件中，良好的可观测性工具能显著减少调试时间。

Test observability is the ability to deeply understand test execution, quickly locate failure causes, and continuously monitor test suite health. In large test suites, good observability tools significantly reduce debugging time.

### 23.1 自定义 Reporter 详细实现 / Custom Reporter Detailed Implementation

```typescript
// vitest-reporters/detailed-reporter.ts
import type { Reporter, File, TaskResultPack } from 'vitest'
import { writeFileSync, mkdirSync } from 'fs'
import { resolve } from 'path'

interface TestMetrics {
  totalTests: number
  passed: number
  failed: number
  skipped: number
  duration: number
  slowTests: { name: string; duration: number }[]
  failureCategories: Record<string, number>
}

export class DetailedReporter implements Reporter {
  private startTime = 0
  private metrics: TestMetrics = {
    totalTests: 0,
    passed: 0,
    failed: 0,
    skipped: 0,
    duration: 0,
    slowTests: [],
    failureCategories: {},
  }

  onInit() {
    this.startTime = Date.now()
    console.log('\n🚀 测试执行开始 / Test execution started\n')
  }

  onTaskUpdate(packs: TaskResultPack[]) {
    for (const [id, result] of packs) {
      if (!result) continue
      
      // 记录慢测试 / Record slow tests
      if (result.duration && result.duration > 1000) {
        this.metrics.slowTests.push({
          name: id,
          duration: result.duration,
        })
      }
    }
  }

  onFinished(files?: File[]) {
    this.metrics.duration = Date.now() - this.startTime
    
    if (!files) return
    
    // 统计结果 / Aggregate results
    for (const file of files) {
      for (const task of file.tasks || []) {
        this.metrics.totalTests++
        if (task.result?.state === 'pass') this.metrics.passed++
        else if (task.result?.state === 'fail') {
          this.metrics.failed++
          // 分类失败原因 / Categorize failure reasons
          const error = task.result.errors?.[0]?.message || 'unknown'
          const category = this.categorizeError(error)
          this.metrics.failureCategories[category] = 
            (this.metrics.failureCategories[category] || 0) + 1
        }
        else this.metrics.skipped++
      }
    }
    
    this.printReport()
    this.saveReport()
  }

  private categorizeError(message: string): string {
    if (message.includes('timeout')) return '超时 / Timeout'
    if (message.includes('expect')) return '断言失败 / Assertion'
    if (message.includes('network') || message.includes('fetch')) return '网络 / Network'
    if (message.includes('permission')) return '权限 / Permission'
    return '其他 / Other'
  }

  private printReport() {
    const m = this.metrics
    console.log('\n' + '='.repeat(60))
    console.log('📊 测试报告 / Test Report')
    console.log('='.repeat(60))
    console.log(`总计 / Total: ${m.totalTests}`)
    console.log(`✅ 通过 / Passed: ${m.passed}`)
    console.log(`❌ 失败 / Failed: ${m.failed}`)
    console.log(`⏭️  跳过 / Skipped: ${m.skipped}`)
    console.log(`⏱️  耗时 / Duration: ${(m.duration / 1000).toFixed(2)}s`)
    
    if (m.slowTests.length > 0) {
      console.log('\n🐌 慢测试 / Slow tests (>1s):')
      m.slowTests
        .sort((a, b) => b.duration - a.duration)
        .slice(0, 5)
        .forEach(t => console.log(`   ${t.duration}ms - ${t.name}`))
    }
    
    if (Object.keys(m.failureCategories).length > 0) {
      console.log('\n📁 失败分类 / Failure categories:')
      Object.entries(m.failureCategories)
        .forEach(([cat, count]) => console.log(`   ${cat}: ${count}`))
    }
    console.log('='.repeat(60) + '\n')
  }

  private saveReport() {
    const dir = resolve(process.cwd(), 'test-reports')
    mkdirSync(dir, { recursive: true })
    writeFileSync(
      resolve(dir, `report-${Date.now()}.json`),
      JSON.stringify(this.metrics, null, 2)
    )
  }
}
```

### 23.2 测试执行追踪 / Test Execution Tracing

```typescript
// vitest.setup.tracing.ts
// 测试执行追踪插件 / Test execution tracing plugin
import { beforeEach, afterEach } from 'vitest'
import { performance, PerformanceObserver } from 'perf_hooks'

interface TraceEntry {
  testName: string
  file: string
  startTime: number
  endTime: number
  duration: number
  memoryBefore: number
  memoryAfter: number
  memoryDelta: number
}

const traces: TraceEntry[] = []
let currentTrace: Partial<TraceEntry> = {}

beforeEach((context) => {
  const mem = process.memoryUsage()
  currentTrace = {
    testName: context.task.name,
    file: context.task.file?.name || 'unknown',
    startTime: performance.now(),
    memoryBefore: mem.heapUsed,
  }
})

afterEach((context) => {
  const mem = process.memoryUsage()
  const endTime = performance.now()
  
  traces.push({
    testName: currentTrace.testName || context.task.name,
    file: currentTrace.file || 'unknown',
    startTime: currentTrace.startTime || 0,
    endTime,
    duration: endTime - (currentTrace.startTime || 0),
    memoryBefore: currentTrace.memoryBefore || 0,
    memoryAfter: mem.heapUsed,
    memoryDelta: mem.heapUsed - (currentTrace.memoryBefore || 0),
  })
})

// 在所有测试结束后输出追踪报告 / Output trace report after all tests
process.on('exit', () => {
  if (traces.length === 0) return
  
  // 检测内存泄漏 / Detect memory leaks
  const leakSuspects = traces
    .filter(t => t.memoryDelta > 10 * 1024 * 1024) // >10MB
    .sort((a, b) => b.memoryDelta - a.memoryDelta)
  
  if (leakSuspects.length > 0) {
    console.warn('\n⚠️  可能的内存泄漏 / Possible memory leaks:')
    leakSuspects.slice(0, 3).forEach(t => {
      console.warn(`   +${(t.memoryDelta / 1024 / 1024).toFixed(1)}MB - ${t.testName}`)
    })
  }
  
  // 输出最慢测试 / Output slowest tests
  const slowest = traces
    .sort((a, b) => b.duration - a.duration)
    .slice(0, 10)
  
  console.log('\n🐌 Top 10 慢测试 / Slowest tests:')
  slowest.forEach((t, i) => {
    console.log(`   ${i + 1}. ${t.duration.toFixed(0)}ms - ${t.testName}`)
  })
})
```

### 23.3 测试健康度指标 / Test Health Metrics

```typescript
// scripts/test-health-check.ts
// 测试健康度分析工具 / Test health analysis tool
import { readFileSync, readdirSync } from 'fs'
import { resolve, join } from 'path'

interface HealthMetrics {
  // 稳定性 / Stability
  flakyTestRate: number      // 不稳定测试比例 / Flaky test rate
  consecutiveFailures: number // 连续失败次数 / Consecutive failures
  
  // 速度 / Speed
  p50Duration: number        // 中位耗时 / Median duration
  p95Duration: number        // 95分位耗时 / 95th percentile
  totalSuiteTime: number     // 套件总时间 / Total suite time
  
  // 覆盖 / Coverage
  lineCoverage: number       // 行覆盖率 / Line coverage
  branchCoverage: number     // 分支覆盖率 / Branch coverage
  uncoveredCriticalPaths: string[] // 未覆盖关键路径 / Uncovered critical paths
  
  // 维护性 / Maintainability
  avgTestLength: number      // 平均测试长度 / Avg test length (lines)
  duplicateTestRatio: number // 重复测试比例 / Duplicate test ratio
  outdatedSnapshotCount: number // 过期快照数 / Outdated snapshot count
}

class TestHealthAnalyzer {
  private reports: any[] = []

  loadReports(reportDir: string) {
    const files = readdirSync(reportDir).filter(f => f.endsWith('.json'))
    this.reports = files.map(f => 
      JSON.parse(readFileSync(join(reportDir, f), 'utf-8'))
    )
  }

  computeHealthScore(metrics: HealthMetrics): number {
    // 加权健康分 (0-100) / Weighted health score
    let score = 100
    
    // 稳定性权重 40% / Stability weight 40%
    score -= metrics.flakyTestRate * 40
    score -= Math.min(metrics.consecutiveFailures * 5, 20)
    
    // 速度权重 25% / Speed weight 25%
    if (metrics.p95Duration > 5000) score -= 15
    else if (metrics.p95Duration > 2000) score -= 8
    
    // 覆盖权重 25% / Coverage weight 25%
    if (metrics.lineCoverage < 60) score -= 20
    else if (metrics.lineCoverage < 80) score -= 10
    
    // 维护性权重 10% / Maintainability weight 10%
    if (metrics.outdatedSnapshotCount > 10) score -= 10
    
    return Math.max(0, Math.round(score))
  }

  generateRecommendations(metrics: HealthMetrics): string[] {
    const recs: string[] = []
    
    if (metrics.flakyTestRate > 0.05) {
      recs.push('⚠️  不稳定测试率超过 5%，建议隔离或修复 / Flaky rate >5%, isolate or fix')
    }
    if (metrics.p95Duration > 3000) {
      recs.push('🐌 P95 耗时超过 3s，考虑并行化或分片 / P95 >3s, consider parallelization')
    }
    if (metrics.branchCoverage < 70) {
      recs.push('📉 分支覆盖率低于 70%，补充边界测试 / Branch coverage <70%, add edge cases')
    }
    if (metrics.avgTestLength > 50) {
      recs.push('📝 平均测试超过 50 行，考虑拆分 / Avg test >50 lines, consider splitting')
    }
    
    return recs
  }
}
```

### 23.4 可观测性工具链对比 / Observability Toolchain Comparison

| 工具 / Tool | 功能 / Function | 集成方式 / Integration | 适用场景 / Use Case |
|---|---|---|---|
| Vitest Reporter | 执行结果报告 / Execution report | 内置 / Built-in | 所有项目 / All projects |
| vitest-sonar-reporter | SonarQube 集成 / SonarQube | 插件 / Plugin | 企业质量门 / Enterprise gates |
| @vitest/coverage-v8 | V8 覆盖率 / V8 coverage | 内置 / Built-in | 快速覆盖 / Quick coverage |
| @vitest/coverage-istanbul | Istanbul 覆盖率 / Istanbul | 插件 / Plugin | 详细报告 / Detailed reports |
| 自定义追踪 / Custom tracing | 性能+内存 / Perf+memory | setup 文件 / Setup file | 深度诊断 / Deep diagnosis |
| GitHub Annotations | PR 内联注释 / PR inline notes | CI 集成 / CI integration | 代码审查 / Code review |

## 24. 变异测试与测试有效性 / Mutation Testing & Test Effectiveness

变异测试是评估测试套件有效性的终极手段。它通过向源代码注入微小变异（突变体），然后运行测试套件，检查测试是否能“杀死”这些突变体。未被杀死的突变体表示测试存在盲区。

Mutation testing is the ultimate means of evaluating test suite effectiveness. It injects small mutations (mutants) into source code, then runs the test suite to check if tests can "kill" these mutants. Surviving mutants indicate test blind spots.

### 24.1 变异测试原理 / Mutation Testing Principles

```typescript
/**
 * 变异测试概念演示 / Mutation testing concept demo
 * 
 * 突变体类型 / Mutant types:
 * 1. 条件突变 / Conditional: > 变为 >=, === 变为 !==
 * 2. 算术突变 / Arithmetic: + 变为 -, * 变为 /
 * 3. 返回值突变 / Return value: return x 变为 return null
 * 4. 删除突变 / Statement removal: 删除整行代码
 * 5. 布尔突变 / Boolean: true 变为 false
 * 
 * 结果分类 / Result classification:
 * - Killed: 测试检测到变异（测试通过变为失败）/ Test detects mutation
 * - Survived: 测试未检测到（测试仍通过）/ Test misses mutation
 * - Timeout: 变异导致无限循环 / Mutation causes infinite loop
 * - No coverage: 变异代码无测试覆盖 / No test coverage
 */

// 源代码示例 / Source code example
function calculateDiscount(price: number, quantity: number, isMember: boolean): number {
  if (quantity >= 10) {
    return price * 0.8  // 批量折扣 / Bulk discount
  }
  if (isMember) {
    return price * 0.9  // 会员折扣 / Member discount
  }
  return price  // 无折扣 / No discount
}

// 突变体示例 / Mutant examples:
// Mutant 1: quantity >= 10 → quantity > 10    (条件突变 / Conditional)
// Mutant 2: price * 0.8 → price * 0.2        (算术突变 / Arithmetic)
// Mutant 3: return price * 0.9 → return price (删除突变 / Removal)
// Mutant 4: isMember → !isMember              (布尔突变 / Boolean)

// 能杀死 Mutant 1 的测试 / Test that kills Mutant 1:
// test('恰好 10 件应用批量折扣 / exactly 10 items gets bulk discount', () => {
//   expect(calculateDiscount(100, 10, false)).toBe(80)  // 捕获 >= vs >
// })

// 无法杀死 Mutant 3 的弱测试 / Weak test that cannot kill Mutant 3:
// test('会员有折扣 / members get discount', () => {
//   const result = calculateDiscount(100, 1, true)
//   expect(result).toBeLessThan(100)  // 太宽松！/ Too loose!
// })
```

### 24.2 Stryker 集成配置 / Stryker Integration Config

```typescript
// stryker.config.mjs
// Stryker Mutator - JavaScript/TypeScript 变异测试框架
// Stryker Mutator - Mutation testing framework for JS/TS

/** @type {import('@stryker-mutator/api/core').PartialStrykerOptions} */
export default {
  // 变异目标 / Mutation targets
  mutate: [
    'src/**/*.ts',
    'src/**/*.tsx',
    // 排除测试文件和类型定义 / Exclude test files and type defs
    '!src/**/__tests__/**',
    '!src/**/*.d.ts',
    '!src/**/*.test.ts',
  ],

  // 测试运行器 / Test runner
  testRunner: 'vitest',
  vitest: {
    configFile: 'vitest.config.ts',
  },

  // 报告 / Reporting
  reporters: ['html', 'json', 'dashboard'],
  htmlReporter: { fileName: 'reports/mutation/mutation-report.html' },
  jsonReporter: { fileName: 'reports/mutation/mutation-report.json' },

  // 阈值 / Thresholds
  thresholds: {
    high: 80,    // 绿色 / Green
    low: 60,     // 黄色 / Yellow
    break: 50,   // 低于此分数 CI 失败 / Below this CI fails
  },

  // 性能配置 / Performance config
  concurrency: 4,              // 并行 worker 数 / Parallel workers
  timeoutMS: 30000,            // 单个突变体超时 / Per-mutant timeout
  incremental: true,           // 增量变异（只测变更文件）/ Incremental
  incrementalFile: '.stryker-cache/incremental.json',

  // 突变体插件 / Mutator plugins
  plugins: [
    '@stryker-mutator/vitest-runner',
    '@stryker-mutator/typescript-checker',
  ],

  // TypeScript 检查器 / TypeScript checker
  checkers: ['typescript'],
  tsconfigFile: 'tsconfig.json',

  // 忽略模式 / Ignore patterns
  ignorePatterns: [
    'node_modules',
    'dist',
    'coverage',
  ],
}
```

### 24.3 变异分数改进策略 / Mutation Score Improvement Strategy

```typescript
// 变异测试驱动的测试改进示例
// Mutation testing driven test improvement example

// 源代码 / Source code
export function classifySensitivity(
  fieldName: string,
  value: string,
  context: { isProduction: boolean }
): number {
  // 生产环境提高敏感度 / Production raises sensitivity
  const baseScore = fieldName.includes('id') ? 4 : 2
  const valueBonus = value.length > 15 ? 1 : 0
  const envMultiplier = context.isProduction ? 1.5 : 1.0
  
  const score = (baseScore + valueBonus) * envMultiplier
  return Math.min(5, Math.max(1, Math.round(score)))
}

// === 弱测试（低变异分数）/ Weak tests (low mutation score) ===
describe('classifySensitivity - 弱测试 / weak tests', () => {
  test('返回数字 / returns a number', () => {
    const result = classifySensitivity('name', 'John', { isProduction: false })
    expect(typeof result).toBe('number')  // 太弱！无法杀死任何突变体
  })
})

// === 强测试（高变异分数）/ Strong tests (high mutation score) ===
describe('classifySensitivity - 强测试 / strong tests', () => {
  // 杀死条件突变: includes('id') / Kills conditional mutant
  test('字段名含 id 时基础分为 4 / base score 4 when field contains id', () => {
    expect(classifySensitivity('user_id', 'x', { isProduction: false })).toBe(4)
    expect(classifySensitivity('name', 'x', { isProduction: false })).toBe(2)
  })

  // 杀死算术突变: length > 15 / Kills arithmetic mutant
  test('值长度恰好 15 无加分 / exactly 15 chars no bonus', () => {
    const val15 = 'a'.repeat(15)
    const val16 = 'a'.repeat(16)
    
    const score15 = classifySensitivity('name', val15, { isProduction: false })
    const score16 = classifySensitivity('name', val16, { isProduction: false })
    
    expect(score16).toBeGreaterThan(score15)  // 捕获 > vs >=
  })

  // 杀死布尔突变: isProduction / Kills boolean mutant
  test('生产环境乘以 1.5 / production multiplies by 1.5', () => {
    const devScore = classifySensitivity('user_id', 'short', { isProduction: false })
    const prodScore = classifySensitivity('user_id', 'short', { isProduction: true })
    
    expect(prodScore).toBeGreaterThan(devScore)  // 捕获 true vs false
    expect(prodScore).toBe(Math.min(5, Math.round(devScore * 1.5)))
  })

  // 杀死边界突变: Math.min/max / Kills boundary mutant
  test('分数限制在 1-5 范围 / score clamped to 1-5', () => {
    // 最高可能: (4+1)*1.5 = 7.5 → 应为 5 / Max possible: should be 5
    const maxScore = classifySensitivity('user_id', 'a'.repeat(20), { isProduction: true })
    expect(maxScore).toBe(5)
    
    // 最低可能: 2*1.0 = 2 → 应为 2 / Min possible: should be 2
    const minScore = classifySensitivity('name', 'x', { isProduction: false })
    expect(minScore).toBeGreaterThanOrEqual(1)
  })
})
```

### 24.4 变异测试 CI 集成 / Mutation Testing CI Integration

```yaml
# .github/workflows/mutation.yml
name: Mutation Testing

on:
  # 每周运行一次（耗时较长）/ Run weekly (time-consuming)
  schedule:
    - cron: '0 2 * * 1'  # 每周一凌晨 2 点 / Monday 2AM
  # 手动触发 / Manual trigger
  workflow_dispatch:

jobs:
  mutation:
    runs-on: ubuntu-latest
    timeout-minutes: 60
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'pnpm'
      - run: pnpm install --frozen-lockfile

      - name: 执行变异测试 / Run mutation testing
        run: |
          pnpm exec stryker run \
            --reporters html,json,dashboard \
            --concurrency 4

      - name: 上传报告 / Upload report
        uses: actions/upload-artifact@v4
        with:
          name: mutation-report
          path: reports/mutation/

      - name: 检查阈值 / Check threshold
        run: |
          SCORE=$(cat reports/mutation/mutation-report.json | jq '.thresholds.high')
          echo "变异分数 / Mutation score: $SCORE%"
          if [ "$SCORE" -lt 60 ]; then
            echo "❌ 变异分数低于 60% / Score below 60%"
            exit 1
          fi

      - name: 评论 PR / Comment PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@v7
        with:
          script: |
            const report = require('./reports/mutation/mutation-report.json')
            const body = `## 🧬 变异测试报告 / Mutation Report\n\n` +
              `- 变异分数 / Score: **${report.mutationScore}%**\n` +
              `- 杀死 / Killed: ${report.killed}\n` +
              `- 存活 / Survived: ${report.survived}\n` +
              `- 超时 / Timeout: ${report.timeout}\n`
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body,
            })
```

### 24.5 测试有效性度量对比 / Test Effectiveness Metrics Comparison

| 指标 / Metric | 定义 / Definition | 目标 / Target | 工具 / Tool |
|---|---|---|---|
| 行覆盖率 / Line coverage | 执行的代码行比例 / Executed line ratio | ≥80% | v8/istanbul |
| 分支覆盖率 / Branch coverage | 执行的分支比例 / Executed branch ratio | ≥70% | istanbul |
| 变异分数 / Mutation score | 被杀死的突变体比例 / Killed mutant ratio | ≥60% | Stryker |
| 缺陷检测率 / Defect detection | 发现真实 bug 的比例 / Real bug detection | 跟踪 / Track | 手动统计 / Manual |
| 测试稳定性 / Test stability | 无 flaky 的比例 / Non-flaky ratio | ≥99% | CI 统计 / CI stats |
| 测试速度 / Test speed | 套件总耗时 / Total suite time | <5min | Vitest |
