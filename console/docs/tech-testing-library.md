# Testing Library 技术栈说明 / Testing Library Technology Stack

## 1. 技术简介 / Introduction

Testing Library 是一套以用户行为为中心的测试工具库家族，核心理念是"测试越接近用户使用方式，就能给你越多的信心"。
Testing Library is a family of testing utility libraries centered on user behavior, with the core philosophy that "the more your tests resemble the way your software is used, the more confidence they can give you."

本项目使用的 Testing Library 家族成员 / Family Members Used：
- **@testing-library/react ^16.2.0**：React 组件渲染与查询 / React component rendering and querying
- **@testing-library/jest-dom ^7.0.0**：DOM 专用断言匹配器 / DOM-specific assertion matchers
- **@testing-library/user-event ^14.6.1**：模拟真实用户交互 / Simulate real user interactions

核心特性 / Core Features：
- **用户视角查询（User-centric Queries）**：通过文本、角色、标签等用户可见属性查找元素，而非实现细节（class/id）。
- **自动等待（Auto-waiting）**：`findBy*` 查询自动等待异步渲染完成。
- **无实现耦合（No Implementation Coupling）**：测试不依赖组件内部状态，重构不破坏测试。
- **可访问性友好（Accessibility Friendly）**：`getByRole` 等查询天然验证 ARIA 语义。
- **框架无关（Framework Agnostic）**：核心 DOM Testing Library 适用于任何渲染到 DOM 的框架。

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 测试架构 / Test Architecture

```
Vitest (测试运行器 / test runner)
  ├── jsdom (浏览器 DOM 模拟 / browser DOM simulation)
  ├── @testing-library/react (组件渲染 + 查询 / render + query)
  │     ├── render() → 将组件挂载到虚拟 DOM / Mount component to virtual DOM
  │     ├── screen → 查询全局 DOM / Query global DOM
  │     └── fireEvent / userEvent → 模拟交互 / Simulate interactions
  ├── @testing-library/jest-dom (断言扩展 / assertion extensions)
  │     └── toBeInTheDocument / toHaveClass / toHaveTextContent ...
  └── setup.ts (全局注册 jest-dom 匹配器 / globally register jest-dom matchers)
```

### 2.2 测试前置配置 / Test Setup

文件 / File：`console/web/src/test/setup.ts`

```typescript
// 每个测试文件执行前自动加载，注册 DOM 断言扩展匹配器。
// Auto-loaded before each test file, registers DOM assertion extension matchers.
import '@testing-library/jest-dom';
```

在 `vite.config.ts` 中引用 / Referenced in `vite.config.ts`：

```typescript
test: {
  globals: true,
  environment: 'jsdom',
  setupFiles: './src/test/setup.ts',  // ← 此处引用 / referenced here
}
```

### 2.3 组件测试完整示例 / Complete Component Test Example

文件 / File：`console/web/src/components/__tests__/ErrorBoundary.test.tsx`

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ErrorBoundary from '../ErrorBoundary';

// Mock 依赖组件（隔离测试目标）/ Mock dependencies (isolate test target)
vi.mock('@/components/icons', () => ({
  Icon: ({ name }: { name: string }) => <span data-testid={`icon-${name}`} />,
}));

// 辅助组件：可控的"炸弹"（模拟渲染错误）/ Helper: controllable "bomb" (simulates render error)
function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) throw new Error('测试爆炸');
  return <div data-testid="safe-content">正常内容</div>;
}

describe('ErrorBoundary', () => {
  // 场景 1：正常渲染 / Scenario 1: normal rendering
  it('子组件正常时渲染 children', () => {
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={false} />
      </ErrorBoundary>,
    );
    // 用户视角断言：页面应显示正常内容 / User-perspective assertion
    expect(screen.getByTestId('safe-content')).toBeInTheDocument();
  });

  // 场景 2：错误降级 / Scenario 2: error fallback
  it('子组件抛错时展示降级界面', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    render(
      <ErrorBoundary>
        <Bomb shouldThrow={true} />
      </ErrorBoundary>,
    );
    // 断言降级 UI 出现 / Assert fallback UI appears
    expect(screen.getByText('界面渲染出错')).toBeInTheDocument();
    expect(screen.getByText('测试爆炸')).toBeInTheDocument();
    spy.mockRestore();
  });

  // 场景 3：重试恢复 / Scenario 3: retry recovery
  it('点击重试按钮后重新渲染子树', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    let throwFlag = true;
    function ControlledBomb() {
      if (throwFlag) throw new Error('临时错误');
      return <div data-testid="recovered">已恢复</div>;
    }

    render(<ErrorBoundary><ControlledBomb /></ErrorBoundary>);
    expect(screen.getByText('界面渲染出错')).toBeInTheDocument();

    // 模拟用户点击"重试" / Simulate user clicking "Retry"
    throwFlag = false;
    fireEvent.click(screen.getByText('重试'));
    expect(screen.getByTestId('recovered')).toBeInTheDocument();
    spy.mockRestore();
  });
});
```

### 2.4 查询方法选择指南 / Query Method Selection Guide

| 查询方法 / Query Method | 返回 / Returns | 未找到时 / When Not Found | 适用场景 / Use Case |
|---|---|---|---|
| `getByText('...')` | 元素 / Element | 抛错 / Throws | 确定存在的元素 / Element guaranteed to exist |
| `queryByText('...')` | 元素 \| null | null | 断言元素不存在 / Assert element absence |
| `findByText('...')` | Promise\<Element\> | 超时抛错 / Timeout throw | 异步渲染的元素 / Async rendered element |
| `getByTestId('...')` | 元素 / Element | 抛错 / Throws | 无可见文本的元素 / Element without visible text |
| `getByRole('button')` | 元素 / Element | 抛错 / Throws | 按语义角色查找（推荐）/ By semantic role (preferred) |

### 2.5 用户交互模拟 / User Interaction Simulation

```tsx
import { fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// fireEvent：底层 DOM 事件派发（快速，适合简单场景）
// fireEvent: low-level DOM event dispatch (fast, for simple scenarios)
fireEvent.click(screen.getByText('发送'));
fireEvent.change(input, { target: { value: '新值' } });

// userEvent：模拟完整用户行为序列（更真实，推荐）
// userEvent: simulates complete user behavior sequence (more realistic, preferred)
const user = userEvent.setup();
await user.type(input, 'hello');       // 逐字符输入 / character-by-character typing
await user.click(button);              // 完整点击（mousedown→mouseup→click）/ full click sequence
await user.clear(input);               // 全选+删除 / select-all + delete
```

### 2.6 jest-dom 断言匹配器 / jest-dom Assertion Matchers

| 匹配器 / Matcher | 用途 / Purpose | 示例 / Example |
|---|---|---|
| `toBeInTheDocument()` | 元素存在于 DOM / Element exists in DOM | `expect(el).toBeInTheDocument()` |
| `toHaveTextContent('...')` | 包含指定文本 / Contains specified text | `expect(el).toHaveTextContent('成功')` |
| `toHaveClass('...')` | 拥有指定 CSS 类 / Has specified CSS class | `expect(el).toHaveClass('bg-red-50')` |
| `toBeVisible()` | 元素可见 / Element is visible | `expect(el).toBeVisible()` |
| `toBeDisabled()` | 元素被禁用 / Element is disabled | `expect(btn).toBeDisabled()` |
| `toHaveValue('...')` | 表单元素值 / Form element value | `expect(input).toHaveValue('test')` |

### 2.7 Mock 策略 / Mocking Strategies

```tsx
// 1. Mock 模块（隔离外部依赖）/ Mock module (isolate external deps)
vi.mock('@/components/icons', () => ({
  Icon: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

// 2. Mock 全局 API（拦截网络请求）/ Mock global API (intercept network)
vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
  ok: true,
  text: () => Promise.resolve(JSON.stringify({ backend: 'ok' })),
}));

// 3. Spy + 抑制输出（避免测试日志污染）/ Spy + suppress output
const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
// ... 测试代码 / test code
spy.mockRestore();  // 恢复原始实现 / restore original
```

### 2.8 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 优先 `getByText` / `getByRole` | 测试更接近用户行为，重构不易破坏 / Tests closer to user behavior, refactor-resistant |
| `data-testid` 仅用于无文本元素 | 避免滥用实现细节查询 / Avoid overusing implementation detail queries |
| jest-dom 全局注册（setupFiles）| 所有测试文件自动获得 DOM 断言，无需逐个 import / All tests get DOM assertions automatically |
| userEvent 优先于 fireEvent | 更真实的事件序列（含 focus/blur），减少误报 / More realistic event sequence, fewer false positives |
| Mock 图标组件 | 避免 SVG 渲染开销，加速测试 / Avoid SVG rendering overhead, speed up tests |

### 2.9 render() 渲染机制详解 / render() Rendering Mechanism

`@testing-library/react` 的 `render()` 函数内部执行以下步骤：

```text
┌─────────────────────────────────────────────────────────────┐
│  render(<Component />) 调用                                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  1. 创建容器 div / Create container div                      │
│     const container = document.createElement('div')          │
│     document.body.appendChild(container)                     │
├─────────────────────────────────────────────────────────────┤
│  2. 调用 ReactDOM.createRoot(container).render(element)      │
│     触发 React 完整渲染管线 / Triggers full React pipeline    │
├─────────────────────────────────────────────────────────────┤
│  3. 包装 act() 确保同步刷新 / Wrap in act() for sync flush   │
│     所有 useEffect/useLayoutEffect 同步执行                   │
├─────────────────────────────────────────────────────────────┤
│  4. 返回工具对象 / Return utilities object                    │
│     { container, rerender, unmount, ...queries }             │
└─────────────────────────────────────────────────────────────┘
```

**自动清理机制 / Auto-cleanup Mechanism**：

```typescript
// Vitest + Testing Library 自动在每个测试后执行 cleanup()
// Vitest + Testing Library auto-runs cleanup() after each test
// 无需手动卸载组件，避免测试间 DOM 污染
// No need to manually unmount, prevents DOM pollution between tests

// 等价于 / Equivalent to:
afterEach(() => {
  cleanup(); // 卸载所有 render() 创建的组件 / Unmount all rendered components
});
```

**rerender 与 unmount 的使用 / Using rerender and unmount**：

```tsx
const { rerender, unmount } = render(<Counter count={1} />);
expect(screen.getByText('1')).toBeInTheDocument();

// 更新 props 重新渲染（模拟父组件状态变化）/ Re-render with new props
rerender(<Counter count={2} />);
expect(screen.getByText('2')).toBeInTheDocument();

// 手动卸载（测试 componentWillUnmount / useEffect cleanup）
unmount();
```

### 2.10 异步测试模式 / Async Testing Patterns

```tsx
import { render, screen, waitFor } from '@testing-library/react';

// 模式 1：findBy* 自动等待（推荐）/ Pattern 1: findBy* auto-wait (preferred)
it('异步加载数据后显示', async () => {
  render(<AsyncComponent />);
  // findByText 内部轮询 DOM 直到元素出现（默认超时 1000ms）
  // findByText polls DOM until element appears (default timeout 1000ms)
  const result = await screen.findByText('加载完成');
  expect(result).toBeInTheDocument();
});

// 模式 2：waitFor 等待条件 / Pattern 2: waitFor condition
it('等待状态变化', async () => {
  render(<StatusIndicator />);
  await waitFor(() => {
    expect(screen.getByRole('status')).toHaveTextContent('就绪');
  }, { timeout: 3000, interval: 100 });
});

// 模式 3：waitForElementToBeRemoved / Pattern 3: waitForElementToBeRemoved
it('加载指示器消失', async () => {
  render(<DataFetcher />);
  expect(screen.getByText('加载中...')).toBeInTheDocument();
  await waitForElementToBeRemoved(() => screen.queryByText('加载中...'));
  expect(screen.getByText('数据内容')).toBeInTheDocument();
});
```

**异步测试的超时配置 / Timeout Configuration for Async Tests**：

| 参数 / Parameter | 默认值 / Default | 说明 / Description |
|---|---|---|
| `timeout` | 1000ms | findBy*/waitFor 最大等待时间 / Max wait time |
| `interval` | 50ms | DOM 轮询间隔 / DOM polling interval |
| `onTimeout` | 抛出错误 / Throws | 超时回调（可自定义错误信息）/ Timeout callback |

### 2.11 可访问性测试 / Accessibility Testing

Testing Library 的查询 API 天然鼓励可访问性验证：

```tsx
// getByRole 查询验证 ARIA 语义是否正确 / getByRole validates ARIA semantics
it('按钮具有正确的可访问角色', () => {
  render(<SubmitButton />);
  // 如果元素没有 role="button" 或不是 <button>，此查询会失败
  // If element lacks role="button" or isn't <button>, this query fails
  const btn = screen.getByRole('button', { name: '提交' });
  expect(btn).toBeEnabled();
});

// 表单标签关联验证 / Form label association validation
it('输入框有关联标签', () => {
  render(<SearchForm />);
  // getByLabelText 要求 <label htmlFor> 正确关联 <input>
  // getByLabelText requires correct <label htmlFor> association
  const input = screen.getByLabelText('搜索关键词');
  expect(input).toHaveAttribute('type', 'text');
});
```

**查询优先级（从最推荐到最不推荐）/ Query Priority (most to least preferred)**：

```text
1. getByRole          → 语义角色（button/link/textbox）— 最接近用户感知
2. getByLabelText     → 表单元素（关联 <label>）
3. getByPlaceholderText → 无标签时的占位文本
4. getByText          → 非交互元素的可见文本
5. getByDisplayValue  → 表单当前值
6. getByAltText       → 图片替代文本
7. getByTitle         → title 属性
8. getByTestId        → 最后手段（无可见语义时）
```

### 2.12 调试工具 / Debugging Tools

```tsx
import { render, screen } from '@testing-library/react';

// 1. screen.debug()：打印当前 DOM 树到控制台
// Print current DOM tree to console
it('调试 DOM 结构', () => {
  render(<MyComponent />);
  screen.debug();              // 打印完整 DOM / Print full DOM
  screen.debug(undefined, 10); // 限制深度为 10 层 / Limit depth to 10
});

// 2. logRoles()：列出 DOM 中所有可访问角色
// List all accessible roles in DOM
import { logRoles } from '@testing-library/react';
it('查看可用角色', () => {
  const { container } = render(<Navigation />);
  logRoles(container);  // 输出: button, link, navigation, list...
});

// 3. 查询失败时的内置提示 / Built-in hints on query failure
// getByText 找不到元素时，自动打印可用元素列表
// When getByText fails, auto-prints available elements list
```

**常见调试流程 / Common Debugging Workflow**：

```text
测试失败 / Test fails
    │
    ▼
 screen.debug() 查看 DOM 结构 → 元素是否存在？
    │                              │
    │  不存在                       │ 存在但查询失败
    ▼                              ▼
检查渲染逻辑/异步等待      logRoles() 检查角色名称
    │                      screen.getByRole('xxx') 拼写
    ▼                              │
添加 await findBy*                 ▼
或 waitFor()               修正查询参数/添加 aria-label
```

### 2.13 本项目测试文件组织 / Project Test File Organization

```text
console/web/src/
├── api/
│   ├── client.ts                    # 被测模块 / Module under test
│   └── __tests__/
│       └── client.test.ts           # API 客户端单元测试 / API client unit tests
├── components/
│   ├── ErrorBoundary.tsx            # 被测组件 / Component under test
│   ├── FileTest.tsx                 # 被测组件（含导出函数）/ Component (with exported fn)
│   └── __tests__/
│       ├── ErrorBoundary.test.tsx   # 组件渲染测试 / Component render tests
│       └── FileTest.test.tsx        # 纯函数逻辑测试 / Pure function logic tests
└── test/
    └── setup.ts                     # 全局测试配置 / Global test setup
```

**测试命名约定 / Test Naming Conventions**：

| 约定 / Convention | 示例 / Example |
|---|---|
| 文件命名 / File naming | `<Module>.test.ts(x)` |
| 目录位置 / Directory | 与被测文件同级 `__tests__/` / Same-level `__tests__/` |
| describe 块 / describe block | 被测模块名 / Module under test name |
| it 描述 / it description | 中文行为描述（"当...时..."）/ Chinese behavior description |
| 辅助函数 / Helper functions | 测试文件内部定义（如 makeFile）/ Defined within test file |

### 2.14 Testing Library vs 其他方案 / Testing Library vs Alternatives

| 维度 / Dimension | Testing Library | Enzyme (已停维) | Cypress Component |
|---|---|---|---|
| 测试哲学 / Philosophy | 用户行为驱动 / User behavior | 实现细节驱动 / Implementation | E2E 驱动 / E2E driven |
| 访问内部状态 / Access internals | ❌ 不鼓励 / Discouraged | ✅ state()/instance() | ❌ 不鼓励 |
| 重构抗性 / Refactor resistance | 高 / High | 低 / Low | 高 / High |
| 运行环境 / Runtime | jsdom (Node) | jsdom (Node) | 真实浏览器 / Real browser |
| 速度 / Speed | 快（毫秒级）/ Fast (ms) | 快 / Fast | 慢（秒级）/ Slow (s) |
| React 18 支持 / React 18 support | ✅ 完整 / Full | ❌ 不完整 / Incomplete | ✅ 完整 |
| 维护状态 / Maintenance | 活跃 / Active | 停维 / Deprecated | 活跃 / Active |

## 3. user-event 高级用法 / user-event Advanced Usage

### 3.1 user-event vs fireEvent 内部差异 / Internal Differences

```text
┌─────────────────────────────────────────────────────────────┐
│  fireEvent.click(button) 触发的事件 / Events fired             │
│    → click (单个事件) / click (single event)                  │
├─────────────────────────────────────────────────────────────┤
│  userEvent.click(button) 触发的事件 / Events fired             │
│    → pointerover → pointerenter → mouseover → mouseenter    │
│    → pointermove → mousemove                                │
│    → pointerdown → mousedown → focus                        │
│    → pointerup → mouseup → click                            │
│    （完整 11 个事件，模拟真实用户行为）
│    (Full 11 events, simulates real user behavior)            │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 键盘交互模拟 / Keyboard Interaction Simulation

```tsx
import userEvent from '@testing-library/user-event';

it('支持键盘快捷键', async () => {
  const user = userEvent.setup();
  render(<SearchPanel />);

  const input = screen.getByRole('textbox', { name: '搜索' });

  // 逐字符输入（触发 onChange 每次）/ Type char-by-char (fires onChange each time)
  await user.type(input, 'mask');
  expect(input).toHaveValue('mask');

  // 特殊按键 / Special keys
  await user.keyboard('{Enter}');          // 回车提交 / Enter to submit
  await user.keyboard('{Escape}');         // 取消 / Cancel
  await user.keyboard('{Backspace}');      // 删除 / Delete
  await user.keyboard('{Control>}a{/Control}');  // Ctrl+A 全选 / Select all
  await user.keyboard('{Tab}');            // 焦点切换 / Focus switch

  // 直接设置值（不触发逐字符事件）/ Set value directly (no per-char events)
  await user.clear(input);                 // 清空 / Clear
  await user.paste('粘贴内容');           // 粘贴（单次 onChange）/ Paste (single onChange)
});
```

### 3.3 指针与拖拽 / Pointer & Drag

```tsx
it('支持双击和悬停', async () => {
  const user = userEvent.setup();
  render(<DataTable />);

  const row = screen.getByText('测试数据');

  // 悬停（触发 tooltip）/ Hover (triggers tooltip)
  await user.hover(row);
  expect(await screen.findByRole('tooltip')).toBeInTheDocument();

  // 离开（隐藏 tooltip）/ Unhover (hides tooltip)
  await user.unhover(row);
  await waitFor(() => {
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  // 双击（进入编辑模式）/ Double-click (enter edit mode)
  await user.dblClick(row);
  expect(screen.getByRole('textbox')).toBeInTheDocument();
});
```

### 3.4 Tab 导航与焦点管理 / Tab Navigation & Focus Management

```tsx
it('表单 Tab 顺序正确', async () => {
  const user = userEvent.setup();
  render(<LoginForm />);

  // Tab 键按 DOM 顺序移动焦点 / Tab moves focus in DOM order
  await user.tab();
  expect(screen.getByLabelText('用户名')).toHaveFocus();

  await user.tab();
  expect(screen.getByLabelText('密码')).toHaveFocus();

  await user.tab();
  expect(screen.getByRole('button', { name: '登录' })).toHaveFocus();

  // Shift+Tab 反向 / Shift+Tab reverse
  await user.tab({ shift: true });
  expect(screen.getByLabelText('密码')).toHaveFocus();
});
```

### 3.5 user-event 配置选项 / user-event Configuration Options

```tsx
// 全局配置 / Global configuration
const user = userEvent.setup({
  delay: null,           // 禁用延迟（测试更快）/ Disable delay (faster tests)
  // delay: 10,          // 每次按键间隔 10ms（默认）/ 10ms between keystrokes (default)
  skipClick: false,      // 不跳过 click 事件 / Don't skip click events
  skipHover: false,      // 不跳过 hover 事件 / Don't skip hover events
});

// 高级：模拟慢速输入（测试防抖）/ Advanced: simulate slow typing (test debounce)
const slowUser = userEvent.setup({ delay: 200 });
await slowUser.type(input, 'test');  // 每字符间隔 200ms / 200ms between chars
```

## 4. 自定义 render 封装 / Custom render Wrapper

### 4.1 带 Provider 的自定义 render / Custom render with Providers

```tsx
// test/utils.tsx — 封装全局 Provider / Wrap global Providers
import { render, RenderOptions } from '@testing-library/react';
import { ReactElement } from 'react';

// 模拟全局上下文（如主题、认证、路由）
// Mock global context (theme, auth, router)
function AllProviders({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider theme="light">
      <AuthProvider user={{ name: '测试用户' }}>
        {children}
      </AuthProvider>
    </ThemeProvider>
  );
}

// 自定义 render：自动包裹 Provider / Custom render: auto-wrap Providers
function customRender(
  ui: ReactElement,
  options?: Omit<RenderOptions, 'wrapper'>,
) {
  return render(ui, { wrapper: AllProviders, ...options });
}

// 重新导出所有 Testing Library 工具 / Re-export all Testing Library utilities
export * from '@testing-library/react';
export { customRender as render };
```

### 4.2 使用自定义 render / Using Custom render

```tsx
// 测试文件中直接导入自定义 render / Import custom render in test files
import { render, screen } from '../test/utils';

it('显示当前用户名', () => {
  render(<UserBadge />);  // 自动包含 AuthProvider / Auto-includes AuthProvider
  expect(screen.getByText('测试用户')).toBeInTheDocument();
});
```

### 4.3 本项目为何无需自定义 render / Why This Project Doesn't Need Custom render

| 因素 / Factor | 本项目情况 / Project Status |
|---|---|
| Context Provider | 无全局 Context（状态在 App 层）/ No global Context (state at App level) |
| 路由 / Router | 无 React Router（单页视图切换）/ No React Router (single-page view switch) |
| 主题 / Theme | 无主题切换（固定 Tailwind 样式）/ No theme switch (fixed Tailwind styles) |
| 国际化 / i18n | 无（硬编码中文）/ None (hardcoded Chinese) |
| 结论 / Conclusion | 组件独立性强，直接 render 即可 / Components are independent, direct render suffices |

## 5. Hooks 测试 / Testing Hooks

### 5.1 renderHook 基础 / renderHook Basics

```tsx
import { renderHook, act } from '@testing-library/react';

// 测试自定义 Hook / Test custom hook
function useCounter(initial = 0) {
  const [count, setCount] = useState(initial);
  const increment = () => setCount(c => c + 1);
  const reset = () => setCount(initial);
  return { count, increment, reset };
}

it('计数器 Hook 正常工作', () => {
  const { result } = renderHook(() => useCounter(5));

  expect(result.current.count).toBe(5);

  // 状态更新必须包裹在 act() 中 / State updates must be wrapped in act()
  act(() => {
    result.current.increment();
  });
  expect(result.current.count).toBe(6);

  act(() => {
    result.current.reset();
  });
  expect(result.current.count).toBe(5);
});
```

### 5.2 带 Provider 的 Hook 测试 / Hook Testing with Providers

```tsx
it('使用 Context 的 Hook', () => {
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ThemeContext.Provider value="dark">
      {children}
    </ThemeContext.Provider>
  );

  const { result } = renderHook(() => useTheme(), { wrapper });
  expect(result.current.theme).toBe('dark');
});
```

### 5.3 Hook 生命周期测试 / Hook Lifecycle Testing

```tsx
it('组件卸载时清理定时器', () => {
  vi.useFakeTimers();
  const { unmount } = renderHook(() => usePolling('/api/health', 5000));

  // 验证定时器已创建 / Verify timer created
  expect(vi.getTimerCount()).toBe(1);

  // 卸载后定时器应被清除 / Timer should be cleared after unmount
  unmount();
  expect(vi.getTimerCount()).toBe(0);

  vi.useRealTimers();
});
```

## 6. API Mock 与 MSW 集成 / API Mocking & MSW Integration

### 6.1 当前项目 Mock 方式 / Current Project Mocking Approach

```tsx
// 本项目使用 vi.stubGlobal 直接模拟 fetch / This project uses vi.stubGlobal to mock fetch
// 文件: console/web/src/api/__tests__/client.test.ts

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

// 辅助函数：构造 Response 对象 / Helper: construct Response object
function jsonResponse(data: any, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  };
}

it('获取健康状态', async () => {
  mockFetch.mockResolvedValueOnce(jsonResponse({ backend: 'ok', agent: 'ok' }));
  const health = await fetchHealth();
  expect(health.backend).toBe('ok');
  expect(mockFetch).toHaveBeenCalledWith('/api/health', expect.any(Object));
});
```

### 6.2 MSW 集成方案（推荐升级）/ MSW Integration (Recommended Upgrade)

MSW（Mock Service Worker）在网络层拦截请求，比 vi.stubGlobal 更真实：

```tsx
// 安装 / Install: pnpm add -D msw

// test/mocks/handlers.ts — 定义 API 模拟 / Define API mocks
import { http, HttpResponse } from 'msw';

export const handlers = [
  http.get('/api/health', () => {
    return HttpResponse.json({ backend: 'ok', agent: 'ok', via: 'go-grpc' });
  }),

  http.post('/api/proxy', async ({ request }) => {
    const body = await request.json();
    return HttpResponse.json({
      status: 200,
      duration_ms: 12.5,
      data: { result: 'masked' },
      via: 'go-grpc',
    });
  }),
];

// test/mocks/server.ts — 启动 Mock 服务器 / Start Mock server
import { setupServer } from 'msw/node';
import { handlers } from './handlers';
export const server = setupServer(...handlers);

// test/setup.ts — 全局生命周期 / Global lifecycle
import { server } from './mocks/server';
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
```

### 6.3 Mock 方案对比 / Mocking Approach Comparison

| 维度 / Dimension | vi.stubGlobal | MSW | 真实后端 / Real backend |
|---|---|---|---|
| 模拟层级 / Mock level | 全局函数 / Global fn | 网络层 / Network layer | 无 / None |
| 真实度 / Realism | 低（跳过网络）/ Low | 高（拦截 HTTP）/ High | 最高 / Highest |
| 速度 / Speed | 极快 / Very fast | 快 / Fast | 慢 / Slow |
| 维护成本 / Maintenance | 低 / Low | 中 / Medium | 高 / High |
| 调试能力 / Debugging | 一般 / Fair | 好（可日志）/ Good | 最好 / Best |
| 本项目使用 / Project usage | ✅ 当前 / Current | 推荐升级 / Recommended | CI 集成测试 / CI integration |

## 7. 测试性能优化 / Test Performance Optimization

### 7.1 加速测试的策略 / Strategies to Speed Up Tests

| 策略 / Strategy | 说明 / Description | 效果 / Effect |
|---|---|---|
| Mock 重型组件 / Mock heavy components | SVG 图标、复杂图表 / SVG icons, charts | 减少渲染时间 / Reduce render time |
| vi.useFakeTimers() | 避免真实等待 / Avoid real waiting | 定时器测试即时完成 / Timer tests instant |
| 并行执行 / Parallel execution | Vitest 默认文件级并行 / File-level parallel | 充分利用多核 / Utilize multi-core |
| 精确查询 / Precise queries | getByRole 代替 getByTestId | 减少 DOM 遍历 / Reduce DOM traversal |
| 避免全量渲染 / Avoid full render | 只渲染目标组件 / Only render target | 减少无关组件开销 / Reduce unrelated overhead |

### 7.2 本项目测试优化实践 / Project Test Optimization Practices

```tsx
// 1. Mock 图标组件（避免 SVG 渲染开销）/ Mock icon components (avoid SVG overhead)
vi.mock('@/components/icons', () => ({
  Icon: ({ name }: { name: string }) => <span data-testid={`icon-${name}`} />,
}));

// 2. 使用 fake timers 避免真实延迟 / Use fake timers to avoid real delays
it('轮询健康状态', async () => {
  vi.useFakeTimers();
  render(<HealthPoller interval={5000} />);

  // 快进时间而非真实等待 / Advance time instead of real waiting
  vi.advanceTimersByTime(5000);
  await vi.runAllTimersAsync();

  expect(screen.getByText('ok')).toBeInTheDocument();
  vi.useRealTimers();
});

// 3. 抑制 console 输出（避免日志污染）/ Suppress console output (avoid log pollution)
const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
// ... 测试错误边界 / Test error boundary
spy.mockRestore();
```

### 7.3 测试覆盖率指南 / Test Coverage Guide

| 测试类型 / Test Type | 覆盖目标 / Coverage Target | 本项目示例 / Project Example |
|---|---|---|
| 单元测试 / Unit test | 纯函数、工具函数 / Pure functions | `formatBytes()`、`buildHeaders()` |
| 组件测试 / Component test | UI 交互逻辑 / UI interaction logic | ErrorBoundary、FileTest |
| 集成测试 / Integration test | API 客户端 + Mock / API client + Mock | client.test.ts |
| E2E 测试 / E2E test | 完整用户流程 / Full user flow | 未实现 / Not implemented |

**覆盖率目标建议 / Coverage Target Recommendation**：

```text
核心业务逻辑 / Core business logic:     > 90%
API 客户端 / API client:               > 80%
UI 组件 / UI components:               > 70%
工具函数 / Utility functions:           > 95%
配置文件 / Config files:               无需测试 / No test needed
```

## 8. 可访问性测试 / Accessibility Testing

### 8.1 Testing Library 的 A11y 理念 / A11y Philosophy

Testing Library 的查询 API 设计强制开发者使用可访问的方式查找元素：
Testing Library's query APIs force developers to find elements accessibly:

| 查询方式 / Query Method | A11y 意义 / A11y Meaning | 优先级 / Priority |
|---|---|---|
| `getByRole()` | 按 ARIA 角色查找 / Find by ARIA role | ★★★ 首选 / First choice |
| `getByLabelText()` | 按表单标签查找 / Find by form label | ★★★ 表单元素 / Form elements |
| `getByText()` | 按可见文本查找 / Find by visible text | ★★ 次选 / Secondary |
| `getByTestId()` | 按测试 ID 查找 / Find by test ID | ★ 最后手段 / Last resort |

### 8.2 本项目中的可访问查询 / Accessible Queries in This Project

```tsx
// ✓ 好：使用语义化查询 / Good: semantic queries
screen.getByRole('button', { name: /send request/i });
screen.getByRole('heading', { name: /overview/i });
screen.getByLabelText(/backend/i);

// ✗ 避免：使用实现细节 / Avoid: implementation details
// container.querySelector('.btn-primary')  // 依赖 CSS 类名 / Depends on CSS class
// container.querySelector('#submit-btn')   // 依赖 DOM ID / Depends on DOM ID
```

### 8.3 jest-axe 集成 / jest-axe Integration

```tsx
// 自动检测可访问性违规 / Auto-detect accessibility violations
import { axe, toHaveNoViolations } from 'jest-axe';
expect.extend(toHaveNoViolations);

test('component has no a11y violations', async () => {
  const { container } = render(<EndpointList />);
  const results = await axe(container);
  expect(results).toHaveNoViolations();
});

// 检测内容 / Detects:
// - 缺少 alt 属性的图片 / Images missing alt
// - 缺少 label 的表单元素 / Form elements without label
// - 颜色对比度不足 / Insufficient color contrast
// - 缺少 landmark 区域 / Missing landmark regions
```

## 9. 异步测试模式 / Async Testing Patterns

### 9.1 等待策略 / Waiting Strategies

```tsx
import { waitFor, waitForElementToBeRemoved } from '@testing-library/react';

// 策略 1：等待条件满足 / Strategy 1: Wait for condition
test('shows response after request', async () => {
  render(<App />);
  await userEvent.click(screen.getByRole('button', { name: /send/i }));

  await waitFor(() => {
    expect(screen.getByText(/response received/i)).toBeInTheDocument();
  }, { timeout: 5000 });  // 超时 5s / 5s timeout
});

// 策略 2：等待元素消失 / Strategy 2: Wait for element removal
test('loading disappears', async () => {
  render(<App />);
  const spinner = screen.getByTestId('loading');
  await waitForElementToBeRemoved(spinner);
  expect(screen.getByText(/done/i)).toBeInTheDocument();
});

// 策略 3：findBy 查询（内置 waitFor）/ Strategy 3: findBy (built-in waitFor)
test('shows data', async () => {
  render(<App />);
  const item = await screen.findByText(/result/i);  // 自动等待 / Auto-wait
  expect(item).toBeInTheDocument();
});
```

### 9.2 异步操作测试 / Testing Async Operations

```tsx
// 测试 API 请求 + 响应渲染 / Test API request + response render
test('proxy request shows response', async () => {
  // Mock API 响应 / Mock API response
  server.use(
    http.post('/api/proxy', () => {
      return HttpResponse.json({
        status: 200,
        duration_ms: 42.5,
        data: { masked: '***' },
      });
    })
  );

  render(<App />);
  await userEvent.click(screen.getByRole('button', { name: /send/i }));

  // 等待异步完成 / Wait for async completion
  expect(await screen.findByText(/42.5/)).toBeInTheDocument();
  expect(screen.getByText(/\*\*\*/)).toBeInTheDocument();
});
```

### 9.3 定时器与延迟 / Timers & Delays

```tsx
// 使用 fake timers 测试延迟逻辑 / Use fake timers for delay logic
test('debounced search', async () => {
  vi.useFakeTimers();
  render(<SearchBox />);

  await userEvent.type(screen.getByRole('textbox'), 'mask');

  // 快进定时器 / Advance timers
  vi.advanceTimersByTime(300);  // debounce 300ms

  expect(mockSearch).toHaveBeenCalledWith('mask');
  vi.useRealTimers();  // 恢复 / Restore
});
```

## 10. 测试调试技巧 / Testing Debugging Tips

### 10.1 screen.debug() 输出 / screen.debug() Output

```tsx
test('debug DOM state', () => {
  render(<App />);

  // 打印当前 DOM（调试用）/ Print current DOM (for debugging)
  screen.debug();                    // 全部 DOM / Full DOM
  screen.debug(screen.getByRole('main'));  // 特定元素 / Specific element

  // 输出示例 / Output example:
  // <body>
  //   <div>
  //     <main>
  //       <h1>Privacy Console</h1>
  //       ...
});
```

### 10.2 常见测试失败排查 / Common Test Failure Troubleshooting

| 错误 / Error | 原因 / Cause | 解决 / Solution |
|---|---|---|
| `Unable to find element` | 元素未渲染 / Element not rendered | 检查条件渲染 / Check conditional render |
| `Found multiple elements` | 多个匹配 / Multiple matches | 使用 `getAllBy*` 或更精确查询 / Use `getAllBy*` or more specific |
| `act(...) warning` | 未等待异步 / Async not awaited | 使用 `await` + `waitFor` |
| `Timeout exceeded` | 异步未完成 / Async not completed | 增加 timeout 或检查 Mock / Increase timeout or check Mock |
| `Not wrapped in act` | 状态更新在 act 外 / State update outside act | 确保所有交互用 await / Ensure all interactions awaited |

### 10.3 测试组织最佳实践 / Test Organization Best Practices

```tsx
// 测试文件结构 / Test file structure
describe('EndpointView', () => {
  // 共享 setup / Shared setup
  beforeEach(() => {
    server.resetHandlers();
  });

  // 按功能分组 / Group by feature
  describe('request sending', () => {
    it('sends POST to proxy endpoint', async () => { ... });
    it('shows loading state during request', async () => { ... });
  });

  describe('response display', () => {
    it('renders JSON response formatted', async () => { ... });
    it('shows error message on failure', async () => { ... });
  });

  describe('edge cases', () => {
    it('handles empty response body', async () => { ... });
    it('handles network timeout', async () => { ... });
  });
});
```

**命名规范 / Naming conventions**：
- `describe`：组件/模块名 / Component/module name
- `it`：动词开头的行为描述 / Verb-first behavior description
- 测试名应描述“期望行为”而非“实现细节” / Names describe "expected behavior" not "implementation"

## 11. 快照测试 / Snapshot Testing

### 11.1 快照测试原理 / Snapshot Testing Principles

```text
┌────────────────────────────────────────────────────────────────┐
│  快照测试工作流 / Snapshot Testing Workflow                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  第一次运行 / First run:                                       │
│    render(<Component />) → toMatchInlineSnapshot()            │
│    └── 生成快照文件 / Generate snapshot file                  │
│        └── __snapshots__/Component.test.tsx.snap              │
│                                                                │
│  后续运行 / Subsequent runs:                                   │
│    render(<Component />) → 与快照对比 / Compare with snapshot  │
│    ├── 匹配 / Match → ✅ 测试通过 / Test passes              │
│    └── 不匹配 / Mismatch → ❌ 显示 diff / Show diff           │
│                                                                │
│  更新快照 / Update snapshot:                                   │
│    vitest --update  (或 -u)                                   │
│    └── 重新生成所有快照 / Regenerate all snapshots            │
└────────────────────────────────────────────────────────────────┘
```

### 11.2 Vitest 快照测试示例 / Vitest Snapshot Test Examples

```typescript
import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { MaskingForm } from '../components/MaskingForm';
import { ResponseViewer } from '../components/ResponseViewer';

describe('MaskingForm snapshot', () => {
  it('matches default state snapshot', () => {
    const { container } = render(<MaskingForm />);
    // 内联快照（推荐小型组件）/ Inline snapshot (recommended for small)
    expect(container.firstChild).toMatchInlineSnapshot(`
      <form class="space-y-4">
        <div class="field-group">
          <label>Strategy</label>
          <select>...</select>
        </div>
        <button type="submit" class="btn-primary">
          Execute
        </button>
      </form>
    `);
  });

  it('matches file snapshot for complex UI', () => {
    const { container } = render(
      <ResponseViewer
        data={{ result: 'masked', fields: ['email', 'phone'] }}
        status="success"
        duration={42.5}
      />
    );
    // 文件快照（适合复杂组件）/ File snapshot (for complex components)
    expect(container).toMatchSnapshot();
  });
});
```

### 11.3 快照测试最佳实践 / Snapshot Testing Best Practices

| 实践 / Practice | 说明 / Description | 原因 / Reason |
|---|---|---|
| 小型组件用内联 / Inline for small | `toMatchInlineSnapshot()` | 可见性好，无需切换文件 / Visible, no file switch |
| 复杂 UI 用文件 / File for complex | `toMatchSnapshot()` | 避免测试文件过长 / Avoid test file bloat |
| 不快照整个页面 / Don't snapshot pages | 只快照独立组件 / Only isolated components | 减少脆弱性 / Reduce brittleness |
| CI 禁止自动更新 / No auto-update in CI | `--ci` 标志 / `--ci` flag | 防止意外变更 / Prevent accidental changes |
| 配合行为测试 / Pair with behavior tests | 快照不能替代交互测试 / Can't replace interaction | 快照只验证结构 / Snapshot only validates structure |

### 11.4 本项目快照策略 / This Project's Snapshot Strategy

| 组件 / Component | 是否快照 / Snapshot? | 原因 / Reason |
|---|---|---|
| ResponseViewer | ✅ | 复杂 JSON 渲染结构 / Complex JSON render structure |
| MaskingForm | ✅ | 表单布局稳定性 / Form layout stability |
| EndpointSelector | ❌ | 交互为主，用行为测试 / Interaction-focused |
| App (整体) / App (whole) | ❌ | 太大，快照脆弱 / Too large, brittle |

## 12. 组件组合测试 / Component Composition Testing

### 12.1 组合测试理念 / Composition Testing Philosophy

```text
┌────────────────────────────────────────────────────────────────┐
│  测试金字塔与组合测试 / Test Pyramid & Composition Testing      │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│         /\          E2E (Cypress/Playwright)                  │
│        /  \         少量关键流程 / Few critical flows          │
│       /────\                                                  │
│      /      \       集成测试 / Integration                    │
│     /  组合  \     组件组合 + API Mock / Composition + Mock    │
│    /──────────\                                                │
│   /            \     单元测试 / Unit                          │
│  /   单个组件   \   独立组件逻辑 / Isolated component logic   │
│ /────────────────\                                             │
│                                                                │
│  本项目重点 / This project focus:                              │
│  ★ 组合测试层：组件 + httpx mock + 用户交互                   │
│  ★ Composition layer: component + httpx mock + user interaction│
└────────────────────────────────────────────────────────────────┘
```

### 12.2 父子组件组合测试 / Parent-Child Composition Test

```typescript
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { describe, it, expect, beforeAll, afterAll } from 'vitest';

// 测试父子组件组合行为 / Test parent-child composition behavior
import { MaskingPage } from '../pages/MaskingPage';
// MaskingPage 包含: EndpointSelector + MaskingForm + ResponseViewer

const server = setupServer(
  http.post('/api/mask', () => {
    return HttpResponse.json({
      masked_data: { email: 'a***@example.com' },
      strategy: 'partial',
    });
  }),
);

beforeAll(() => server.listen());
afterAll(() => server.close());

describe('MaskingPage composition', () => {
  it('full workflow: select endpoint → fill form → view result', async () => {
    const user = userEvent.setup();
    render(<MaskingPage />);

    // 1. 选择端点 / Select endpoint
    await user.selectOptions(
      screen.getByRole('combobox', { name: /endpoint/i }),
      'mask'
    );

    // 2. 填写表单 / Fill form
    await user.type(
      screen.getByRole('textbox', { name: /data/i }),
      '{"email": "alice@example.com"}'
    );

    // 3. 提交 / Submit
    await user.click(screen.getByRole('button', { name: /execute/i }));

    // 4. 验证结果展示（跨组件状态流转）
    // Verify result display (cross-component state flow)
    await waitFor(() => {
      expect(screen.getByText(/a\*\*\*@example.com/)).toBeInTheDocument();
    });
  });

  it('error state propagates from ResponseViewer to parent', async () => {
    server.use(
      http.post('/api/mask', () => {
        return new HttpResponse(null, { status: 500 });
      }),
    );

    const user = userEvent.setup();
    render(<MaskingPage />);

    await user.click(screen.getByRole('button', { name: /execute/i }));

    await waitFor(() => {
      // 错误状态从子组件传播到父组件布局
      // Error state propagates from child to parent layout
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });
  });
});
```

### 12.3 测试替身策略 / Test Double Strategy

| 层级 / Layer | 替身类型 / Double Type | 工具 / Tool | 示例 / Example |
|---|---|---|---|
| 网络层 / Network | Mock Service Worker | MSW | 拦截 HTTP 请求 / Intercept HTTP |
| 组件 Props | 固定数据 / Fixed data | 直接传入 / Pass directly | `render(<Viewer data={mock} />)` |
| Context | 自定义 Provider | 测试用 wrapper | 模拟主题/认证 / Mock theme/auth |
| 时间 / Time | Fake timers | vi.useFakeTimers | 测试 debounce/timeout |
| 存储 / Storage | Memory mock | vi.spyOn | localStorage mock |

### 12.4 本项目组合测试实践 / This Project's Composition Practice

| 测试场景 / Test Scenario | 组合组件 / Composed Components | Mock 策略 / Mock Strategy |
|---|---|---|
| 脱敏工作流 / Masking workflow | Page + Form + Viewer | MSW 拦截 / MSW intercept |
| DP 查询流程 / DP query flow | Page + QueryForm + Chart | MSW + 固定数据 / Fixed data |
| 健康检查 / Health check | StatusBar + API | MSW 多状态 / Multi-status |
| 错误处理 / Error handling | 全局 ErrorBoundary | MSW 500 响应 / 500 response |

## 13. 测试覆盖率策略 / Test Coverage Strategy

### 13.1 Vitest 覆盖率配置 / Vitest Coverage Configuration

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom',
    coverage: {
      provider: 'v8',           // V8 原生覆盖率 / V8 native coverage
      reporter: [
        'text',                 // 终端表格 / Terminal table
        'html',                 // HTML 报告 / HTML report
        'lcov',                // CI 集成 / CI integration
      ],
      // 覆盖率阈值 / Coverage thresholds
      thresholds: {
        statements: 80,         // 语句覆盖 / Statement coverage
        branches: 75,           // 分支覆盖 / Branch coverage
        functions: 80,          // 函数覆盖 / Function coverage
        lines: 80,              // 行覆盖 / Line coverage
      },
      // 排除文件 / Exclude files
      exclude: [
        'node_modules/',
        'dist/',
        '**/*.config.*',
        '**/types/**',
        'src/main.tsx',         // 入口文件 / Entry file
      ],
    },
  },
});
```

### 13.2 覆盖率指标解读 / Coverage Metrics Explained

```text
┌────────────────────────────────────────────────────────────────┐
│  覆盖率指标含义 / Coverage Metrics Meaning                    │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Statements (语句): 每条可执行语句是否被执行                  │
│  ─────────────────────────────────────────────                  │
│  function example(x: number) {  // ← 语句 1                  │
│    if (x > 0) {                 // ← 语句 2 (分支)            │
│      return x * 2;              // ← 语句 3                  │
│    }                                                          │
│    return 0;                    // ← 语句 4                  │
│  }                                                            │
│  测试 example(5): 语句覆盖 75% (3/4)，分支覆盖 50% (1/2)  │
│                                                                │
│  Branches (分支): if/else/switch 的每个分支是否被覆盖         │
│  Functions (函数): 每个函数是否被调用                          │
│  Lines (行): 每行代码是否被执行                              │
└────────────────────────────────────────────────────────────────┘
```

### 13.3 覆盖率目标与优先级 / Coverage Goals & Priority

| 模块 / Module | 目标覆盖率 / Target | 优先级 / Priority | 原因 / Reason |
|---|---|---|---|
| API 调用层 / API layer | 90%+ | ★★★★★ | 核心业务逻辑 / Core business logic |
| 表单验证 / Form validation | 85%+ | ★★★★ | 用户输入边界 / User input boundaries |
| 状态管理 / State management | 80%+ | ★★★★ | 数据流正确性 / Data flow correctness |
| UI 组件 / UI components | 70%+ | ★★★ | 渲染正确性 / Render correctness |
| 工具函数 / Utilities | 95%+ | ★★★★★ | 纯函数易测 / Pure functions easy |
| 配置/类型 / Config/types | N/A | - | 排除在覆盖率外 / Excluded |

### 13.4 CI 覆盖率集成 / CI Coverage Integration

```yaml
# .github/workflows/ci.yml 片段 / Snippet
name: Test & Coverage
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v2
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: pnpm

      - name: Install & Test
        run: |
          cd console/web
          pnpm install --frozen-lockfile
          pnpm test -- --coverage --run

      - name: Check thresholds
        run: |
          cd console/web
          # Vitest 自动根据 thresholds 配置失败
          # Vitest auto-fails based on thresholds config
          echo "Coverage thresholds met"

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          directory: console/web/coverage
```

### 13.5 本项目覆盖率实践 / This Project's Coverage Practice

| 实践 / Practice | 说明 / Description |
|---|---|
| 关注行为覆盖而非数字 / Focus on behavior not numbers | 80% 是底线，不是目标 / 80% is floor, not goal |
| 优先测试边界情况 / Prioritize edge cases | 空输入、超时、错误 / Empty, timeout, error |
| 不追求 100% / Don't chase 100% |  diminishing returns，维护成本高 / High maintenance |
| 新增代码必须带测试 / New code must have tests | PR 审查检查 / PR review check |
| 覆盖率下降即失败 / Coverage drop = fail | CI 阈值强制 / CI threshold enforced |

---

## 14. 视觉回归测试 / Visual Regression Testing

### 14.1 视觉测试概念与工具 / Visual Testing Concepts & Tools

视觉回归测试通过截图对比检测 UI 变化，确保样式修改不会意外破坏界面：

```
┌─────────────────────────────────────────────────────────────────┐
│         视觉测试工作流 / Visual Testing Workflow                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 基准截图 / Baseline Screenshot                             │
│     ┌───────────────────────────────────────────────────┐   │
│     │  首次运行生成基准图像 / First run generates baseline │   │
│     │  存储于 __screenshots__/baseline/                   │   │
│     └───────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  2. 当前截图 / Current Screenshot                              │
│     ┌───────────────────────────────────────────────────┐   │
│     │  每次测试运行生成 / Generated each test run          │   │
│     │  存储于 __screenshots__/current/                    │   │
│     └───────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  3. 像素对比 / Pixel Comparison                                │
│     ┌───────────────────────────────────────────────────┐   │
│     │  逐像素比较 / Pixel-by-pixel comparison             │   │
│     │  生成差异图 / Generate diff image                   │   │
│     │  计算差异百分比 / Calculate diff percentage         │   │
│     └───────────────────────────────────────────────────┘   │
│                          │                                      │
│              ┌─────────┴─────────┐                         │
│              ▼                   ▼                         │
│     ┌────────────┐      ┌────────────┐                  │
│     │ 通过 / Pass │      │ 失败 / Fail │                  │
│     │ diff < 0.1% │      │ diff > 0.1% │                  │
│     └────────────┘      └────────────┘                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 14.2 Vitest + Playwright 视觉测试 / Vitest + Playwright Visual Testing

```typescript
// ===== 视觉测试配置 / Visual Test Configuration =====
// vitest.config.visual.ts
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    // 视觉测试单独配置 / Separate config for visual tests
    include: ['**/*.visual.test.tsx'],
    environment: 'node',  // Playwright 自带浏览器 / Playwright has own browser
    testTimeout: 30000,   // 视觉测试需要更多时间 / Visual tests need more time
  },
});

// ===== 视觉测试示例 / Visual Test Example =====
// EndpointView.visual.test.tsx
import { test, expect } from '@playwright/test';

test.describe('EndpointView Visual Tests', () => {
  test('默认状态 / default state', async ({ page }) => {
    await page.goto('/endpoints/mask');
    await page.waitForSelector('[data-testid="endpoint-view"]');

    // 全页截图对比 / Full page screenshot comparison
    await expect(page).toHaveScreenshot('endpoint-default.png', {
      maxDiffPixelRatio: 0.001,  // 允许 0.1% 差异 / Allow 0.1% diff
    });
  });

  test('加载状态 / loading state', async ({ page }) => {
    // 拦截 API 延迟响应 / Intercept API with delayed response
    await page.route('**/api/endpoints/*', async (route) => {
      await new Promise(r => setTimeout(r, 5000));
      await route.fulfill({ json: {} });
    });

    await page.goto('/endpoints/mask');
    const spinner = page.locator('[data-testid="loading-spinner"]');

    // 组件级截图 / Component-level screenshot
    await expect(spinner).toHaveScreenshot('loading-spinner.png');
  });

  test('响应面板 / response panel', async ({ page }) => {
    await page.goto('/endpoints/mask');
    await page.click('button:has-text("Send")');
    await page.waitForSelector('[data-testid="response-panel"]');

    const panel = page.locator('[data-testid="response-panel"]');
    await expect(panel).toHaveScreenshot('response-panel.png', {
      // 忽略动态内容区域 / Ignore dynamic content areas
      mask: [{
        selector: '[data-testid="timestamp"]',
        color: '#000',  // 用黑色遮盖 / Cover with black
      }],
    });
  });
});
```

### 14.3 视觉测试最佳实践 / Visual Testing Best Practices

| 实践 / Practice | 说明 / Description | 原因 / Reason |
|---|---|---|
| 固定视口尺寸 / Fixed viewport | 1280x720 或 1920x1080 | 避免响应式差异 / Avoid responsive diffs |
| 禁用动画 / Disable animations | `* { animation: none }` | 截图时机不确定 / Timing uncertain |
| 隐藏动态内容 / Hide dynamic | 时间戳、随机 ID | 每次运行不同 / Different each run |
| 字体加载等待 / Wait for fonts | `document.fonts.ready` | 字体回退差异 / Font fallback diffs |
| 阈值设置 / Threshold | 0.1% - 0.5% | 平衡敏感度和稳定性 / Balance |
| 基准更新 / Baseline update | 手动审核后更新 / Update after review | 避免意外变化 / Avoid accidental |

---

## 15. 端到端测试集成 / End-to-End Testing Integration

### 15.1 E2E 测试架构 / E2E Test Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│           E2E 测试架构 / E2E Test Architecture                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  测试层 / Test Layers:                                         │
│                                                                 │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Unit Tests (Vitest + RTL)                          │   │
│  │  - 组件逻辑 / Component logic                        │   │
│  │  - 工具函数 / Utility functions                      │   │
│  │  - 快速反馈 / Fast feedback (<1s)                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Integration Tests (Vitest + MSW)                   │   │
│  │  - API 交互 / API interactions                       │   │
│  │  - 状态管理 / State management                       │   │
│  │  - 中等速度 / Medium speed (1-5s)                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                      │
│                          ▼                                      │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  E2E Tests (Playwright)                             │   │
│  │  - 完整用户流程 / Complete user flows                │   │
│  │  - 真实浏览器 / Real browser                         │   │
│  │  - 慢但全面 / Slow but comprehensive (10-30s)        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 15.2 Playwright E2E 测试示例 / Playwright E2E Test Example

```typescript
// ===== E2E 测试: 完整用户流程 / E2E Test: Complete User Flow =====
// e2e/masking-flow.spec.ts
import { test, expect, Page } from '@playwright/test';

// 页面对象模式 / Page Object Pattern
class ConsolePage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('http://localhost:5173');
    await this.page.waitForLoadState('networkidle');
  }

  async selectEndpoint(name: string) {
    await this.page.click(`text=${name}`);
    await this.page.waitForSelector('[data-testid="endpoint-view"]');
  }

  async setRequestBody(body: object) {
    const editor = this.page.locator('[data-testid="request-body"] textarea');
    await editor.fill(JSON.stringify(body, null, 2));
  }

  async sendRequest() {
    await this.page.click('button:has-text("Send")');
    await this.page.waitForSelector('[data-testid="response-panel"]');
  }

  async getResponseStatus(): Promise<string> {
    return await this.page.textContent('[data-testid="status-code"]') || '';
  }

  async getResponseBody(): Promise<object> {
    const text = await this.page.textContent('[data-testid="response-body"]');
    return JSON.parse(text || '{}');
  }
}

test.describe('掌码功能流程 / Masking Feature Flow', () => {
  let console: ConsolePage;

  test.beforeEach(async ({ page }) => {
    console = new ConsolePage(page);
    await console.goto();
  });

  test('完整掌码流程 / Complete masking flow', async () => {
    // 1. 选择掌码端点 / Select masking endpoint
    await console.selectEndpoint('Data Masking');

    // 2. 设置请求体 / Set request body
    await console.setRequestBody({
      data: {
        email: 'john.doe@example.com',
        phone: '13800138000',
        name: '张三',
      },
    });

    // 3. 发送请求 / Send request
    await console.sendRequest();

    // 4. 验证响应 / Verify response
    const status = await console.getResponseStatus();
    expect(status).toBe('200');

    const body = await console.getResponseBody() as any;
    expect(body.masked_data.email).not.toBe('john.doe@example.com');
    expect(body.masked_data.email).toContain('***');
  });

  test('错误处理 / Error handling', async ({ page }) => {
    await console.selectEndpoint('Data Masking');

    // 设置无效 JSON / Set invalid JSON
    const editor = page.locator('[data-testid="request-body"] textarea');
    await editor.fill('{ invalid json }');

    await console.sendRequest();

    // 应显示错误提示 / Should show error message
    await expect(page.locator('.error-message')).toBeVisible();
  });
});
```

### 15.3 测试分层策略 / Test Layering Strategy

| 测试类型 / Test Type | 工具 / Tool | 数量 / Quantity | 运行时机 / When | 本项目 / Project |
|---|---|---|---|---|
| 单元测试 / Unit | Vitest + RTL | ~100 | 每次保存 / Every save | ✅ 主要 / Primary |
| 集成测试 / Integration | Vitest + MSW | ~30 | PR 提交 / PR submit | ✅ 使用 / Used |
| E2E 测试 / E2E | Playwright | ~10 | 合并前 / Before merge | 可选 / Optional |
| 视觉测试 / Visual | Playwright | ~20 | 定期 / Periodic | 可选 / Optional |

---

## 16. 测试反模式与重构 / Testing Anti-patterns & Refactoring

### 16.1 常见反模式识别 / Common Anti-pattern Identification

```typescript
// ===== ❌ 反模式 1: 测试实现细节 / Anti-pattern 1: Testing Implementation =====

// 错误: 测试内部 state / Wrong: testing internal state
test('错误: 检查 state 值 / Wrong: check state value', () => {
  const { result } = renderHook(() => useCounter());

  act(() => result.current.increment());

  // ❌ 直接访问内部实现 / Directly access internal implementation
  expect(result.current.state.count).toBe(1);
  expect(result.current.state.history).toHaveLength(1);
});

// 正确: 测试可观察行为 / Correct: test observable behavior
test('正确: 检查渲染输出 / Correct: check rendered output', () => {
  render(<Counter />);

  fireEvent.click(screen.getByRole('button', { name: /increment/i }));

  // ✅ 检查用户可见的输出 / Check user-visible output
  expect(screen.getByText('Count: 1')).toBeInTheDocument();
});


// ===== ❌ 反模式 2: 过度 Mock / Anti-pattern 2: Over-mocking =====

// 错误: mock 所有东西 / Wrong: mock everything
vi.mock('../utils/format');
vi.mock('../utils/validate');
vi.mock('../utils/transform');
vi.mock('../hooks/useData');
vi.mock('../components/Child');

// 正确: 只 mock 边界 / Correct: mock only boundaries
vi.mock('../api/client');  // 只 mock 网络层 / Only mock network layer


// ===== ❌ 反模式 3: 条件逻辑测试 / Anti-pattern 3: Conditional Test Logic =====

// 错误: 测试中有 if/else / Wrong: if/else in tests
test('错误 / Wrong', () => {
  const result = calculate(10);

  if (result > 100) {
    expect(result).toBe(150);
  } else {
    expect(result).toBe(50);
  }
});

// 正确: 明确期望 / Correct: explicit expectations
test('正确 / Correct', () => {
  expect(calculate(10)).toBe(50);
});
```

### 16.2 测试代码重构技巧 / Test Code Refactoring Tips

```typescript
// ===== 测试工厂模式 / Test Factory Pattern =====

// 定义工厂函数 / Define factory function
function createMockEndpoint(overrides: Partial<Endpoint> = {}): Endpoint {
  return {
    id: 'test-endpoint',
    name: 'Test Endpoint',
    method: 'POST',
    path: '/api/test',
    description: 'A test endpoint',
    sampleBody: { data: 'test' },
    ...overrides,
  };
}

// 使用工厂 / Use factory
test('渲染 GET 端点 / renders GET endpoint', () => {
  const endpoint = createMockEndpoint({ method: 'GET', path: '/api/users' });
  render(<EndpointView endpoint={endpoint} />);
  expect(screen.getByText('GET')).toBeInTheDocument();
});


// ===== Arrange-Act-Assert 模式 / AAA Pattern =====
test('发送请求显示响应 / send request shows response', async () => {
  // Arrange: 准备测试数据 / Prepare test data
  mockSendRequest.mockResolvedValue({ status: 200, data: { result: 'ok' } });
  render(<EndpointView endpoint={createMockEndpoint()} />);

  // Act: 执行用户操作 / Perform user action
  fireEvent.click(screen.getByRole('button', { name: /send/i }));

  // Assert: 验证结果 / Verify result
  await waitFor(() => {
    expect(screen.getByText(/200/)).toBeInTheDocument();
  });
});
```

### 16.3 测试质量检查清单 / Test Quality Checklist

| 检查项 / Check Item | 标准 / Standard | 说明 / Description |
|---|---|---|
| 测试名称描述行为 / Name describes behavior | ✅ 必须 / Required | "当...时应该..." / "should...when..." |
| 无实现细节 / No implementation details | ✅ 必须 / Required | 只测试公共 API / Only test public API |
| 单一职责 / Single responsibility | ✅ 推荐 / Recommended | 一个测试一个行为 / One test one behavior |
| 独立性 / Independence | ✅ 必须 / Required | 无测试间依赖 / No inter-test deps |
| 确定性 / Deterministic | ✅ 必须 / Required | 无随机/时间依赖 / No random/time deps |
| 快速执行 / Fast execution | ✅ 推荐 / Recommended | 单元测试 <100ms / Unit test <100ms |
| 可读性 / Readability | ✅ 推荐 / Recommended | AAA 结构清晰 / Clear AAA structure |

## 17. 组件库测试策略 / Component Library Testing Strategy

组件库（如 UI 组件库）的测试与普通应用测试有显著不同。组件库需要关注 API 稳定性、可访问性、主题定制、边界情况等，并且需要保证向后兼容。

Component library testing differs significantly from application testing. Libraries must focus on API stability, accessibility, theming, edge cases, and backward compatibility guarantees.

### 17.1 组件 API 测试 / Component API Testing

```tsx
/**
 * 组件库 API 测试模式 / Component library API testing patterns
 *
 * 核心原则 / Core principles:
 * 1. 测试公开 API，不测内部实现 / Test public API, not internals
 * 2. 每个 prop 都有对应测试 / Every prop has corresponding test
 * 3. 默认行为必须测试 / Default behavior must be tested
 * 4. 边界值和错误输入 / Boundary values and invalid inputs
 */
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Button } from '../src/components/Button';

describe('Button 组件 API / Button Component API', () => {
  // === 默认行为 / Default behavior ===
  it('默认渲染为 button 元素 / renders as button element by default', () => {
    render(<Button>Click me</Button>);
    const btn = screen.getByRole('button', { name: 'Click me' });
    expect(btn.tagName).toBe('BUTTON');
    expect(btn).toHaveAttribute('type', 'button');
  });

  // === Props 测试 / Props testing ===
  describe('variant prop', () => {
    it.each([
      ['primary', 'btn-primary'],
      ['secondary', 'btn-secondary'],
      ['danger', 'btn-danger'],
      ['ghost', 'btn-ghost'],
    ] as const)('variant="%s" 应用类名 "%s" / applies class "%s"',
      (variant, expectedClass) => {
        render(<Button variant={variant}>Test</Button>);
        expect(screen.getByRole('button')).toHaveClass(expectedClass);
      }
    );
  });

  describe('size prop', () => {
    it.each(['sm', 'md', 'lg'] as const)(
      'size="%s" 渲染正确尺寸 / renders correct size',
      (size) => {
        render(<Button size={size}>Test</Button>);
        expect(screen.getByRole('button')).toHaveClass(`btn-${size}`);
      }
    );
  });

  describe('disabled prop', () => {
    it('禁用时不可点击 / not clickable when disabled', async () => {
      const onClick = vi.fn();
      render(<Button disabled onClick={onClick}>Disabled</Button>);

      const btn = screen.getByRole('button');
      expect(btn).toBeDisabled();

      await userEvent.click(btn);
      expect(onClick).not.toHaveBeenCalled();
    });
  });

  describe('loading prop', () => {
    it('加载中显示 spinner 并禁用 / shows spinner and disables when loading', () => {
      render(<Button loading>Saving</Button>);
      const btn = screen.getByRole('button');

      expect(btn).toBeDisabled();
      expect(screen.getByTestId('loading-spinner')).toBeInTheDocument();
      expect(btn).toHaveAttribute('aria-busy', 'true');
    });
  });

  // === 事件测试 / Event testing ===
  describe('事件处理 / Event handling', () => {
    it('点击触发 onClick / click triggers onClick', async () => {
      const onClick = vi.fn();
      render(<Button onClick={onClick}>Click</Button>);

      await userEvent.click(screen.getByRole('button'));
      expect(onClick).toHaveBeenCalledTimes(1);
      expect(onClick).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'click' })
      );
    });

    it('键盘 Enter/Space 触发点击 / keyboard Enter/Space triggers click', async () => {
      const onClick = vi.fn();
      render(<Button onClick={onClick}>KB</Button>);

      const btn = screen.getByRole('button');
      btn.focus();

      await userEvent.keyboard('{Enter}');
      expect(onClick).toHaveBeenCalledTimes(1);

      await userEvent.keyboard(' ');
      expect(onClick).toHaveBeenCalledTimes(2);
    });
  });

  // === Ref 转发 / Ref forwarding ===
  it('支持 ref 转发 / supports ref forwarding', () => {
    const ref = React.createRef<HTMLButtonElement>();
    render(<Button ref={ref}>Ref</Button>);
    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
    expect(ref.current?.textContent).toBe('Ref');
  });

  // === 组合性 / Composition ===
  it('支持自定义 className 合并 / supports custom className merge', () => {
    render(<Button className="custom-class">Styled</Button>);
    const btn = screen.getByRole('button');
    expect(btn).toHaveClass('btn');         // 基础类 / Base class
    expect(btn).toHaveClass('custom-class'); // 自定义类 / Custom class
  });
});
```

### 17.2 可访问性测试矩阵 / Accessibility Testing Matrix

```tsx
/**
 * 组件库可访问性测试 / Component library accessibility testing
 *
 * 每个组件必须覆盖的 a11y 检查项：
 * Accessibility checks every component must cover:
 * - ARIA 角色和属性 / ARIA roles and attributes
 * - 键盘导航 / Keyboard navigation
 * - 焦点管理 / Focus management
 * - 屏幕阅读器兼容 / Screen reader compatibility
 * - 颜色对比度 / Color contrast
 */
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe, toHaveNoViolations } from 'jest-axe';
import { Select } from '../src/components/Select';

expect.extend(toHaveNoViolations);

describe('Select 可访问性 / Select Accessibility', () => {
  const options = [
    { value: 'apple', label: 'Apple' },
    { value: 'banana', label: 'Banana' },
    { value: 'cherry', label: 'Cherry' },
  ];

  it('无 axe 违规 / no axe violations', async () => {
    const { container } = render(
      <Select options={options} label="Fruit" value="apple" onChange={() => {}} />
    );
    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  it('具有正确的 ARIA 角色 / has correct ARIA roles', () => {
    render(<Select options={options} label="Fruit" value="" onChange={() => {}} />);

    // 触发器应为 combobox 角色 / Trigger should have combobox role
    expect(screen.getByRole('combobox')).toBeInTheDocument();
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByRole('combobox')).toHaveAttribute('aria-haspopup', 'listbox');
  });

  it('键盘导航完整流程 / full keyboard navigation flow', async () => {
    const onChange = vi.fn();
    render(<Select options={options} label="Fruit" value="" onChange={onChange} />);

    const combobox = screen.getByRole('combobox');

    // Tab 聚焦 / Tab to focus
    await userEvent.tab();
    expect(combobox).toHaveFocus();

    // Enter 打开下拉 / Enter opens dropdown
    await userEvent.keyboard('{Enter}');
    expect(screen.getByRole('listbox')).toBeInTheDocument();

    // 箭头键导航 / Arrow key navigation
    await userEvent.keyboard('{ArrowDown}');
    expect(screen.getByRole('option', { name: 'Apple' })).toHaveAttribute(
      'aria-selected', 'true'
    );

    await userEvent.keyboard('{ArrowDown}');
    expect(screen.getByRole('option', { name: 'Banana' })).toHaveAttribute(
      'aria-selected', 'true'
    );

    // Enter 选择 / Enter selects
    await userEvent.keyboard('{Enter}');
    expect(onChange).toHaveBeenCalledWith('banana');

    // 下拉关闭，焦点回到触发器 / Dropdown closes, focus returns
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(combobox).toHaveFocus();
  });

  it('Escape 关闭不改变值 / Escape closes without changing value', async () => {
    const onChange = vi.fn();
    render(<Select options={options} label="Fruit" value="apple" onChange={onChange} />);

    await userEvent.click(screen.getByRole('combobox'));
    expect(screen.getByRole('listbox')).toBeInTheDocument();

    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('焦点陷阱在下拉内 / focus is trapped within dropdown', async () => {
    render(<Select options={options} label="Fruit" value="" onChange={() => {}} />);

    await userEvent.click(screen.getByRole('combobox'));
    const listbox = screen.getByRole('listbox');

    // Tab 不应离开下拉 / Tab should not leave dropdown
    await userEvent.tab();
    expect(within(listbox).getByRole('option', { name: 'Apple' })).toHaveFocus();
  });
});
```

### 17.3 组件快照与回归测试 / Component Snapshot & Regression Testing

```tsx
/**
 * 组件库快照测试策略 / Component library snapshot testing strategy
 *
 * 组件库中的快照测试用途：
 * Snapshot testing use in component libraries:
 * 1. 检测意外的 DOM 结构变化 / Detect unexpected DOM structure changes
 * 2. 保证 CSS 类名稳定 / Ensure CSS class name stability
 * 3. API 变更的早期预警 / Early warning for API changes
 *
 * 注意：快照测试是补充，不是替代行为测试！
 * Note: Snapshot tests complement, NOT replace behavioral tests!
 */
import { render } from '@testing-library/react';
import { Card, CardHeader, CardBody, CardFooter } from '../src/components/Card';

describe('Card 快照测试 / Card Snapshot Tests', () => {
  it('基础 Card 结构稳定 / basic Card structure is stable', () => {
    const { container } = render(
      <Card>
        <CardHeader>Title</CardHeader>
        <CardBody>Content</CardBody>
        <CardFooter>Actions</CardFooter>
      </Card>
    );
    expect(container.firstChild).toMatchSnapshot();
  });

  it('各 variant 快照 / variant snapshots', () => {
    const variants = ['default', 'outlined', 'elevated', 'interactive'] as const;

    variants.forEach((variant) => {
      const { container } = render(<Card variant={variant}>Test</Card>);
      expect(container.firstChild).toMatchSnapshot(`card-${variant}`);
    });
  });

  it('响应式属性快照 / responsive props snapshot', () => {
    const { container } = render(
      <Card padding={{ base: 'sm', md: 'md', lg: 'lg' }}>
        Responsive padding
      </Card>
    );
    expect(container.firstChild).toMatchSnapshot();
  });
});

// 组件库测试分层策略 / Component library test layering strategy
//
// ┌─────────────────────────────────────────┐
// │  E2E (Playwright)     ~5%   视觉回归   │
// ├─────────────────────────────────────────┤
// │  集成测试             ~15%  组件组合    │
// ├─────────────────────────────────────────┤
// │  行为测试 (RTL)       ~50%  用户交互    │
// ├─────────────────────────────────────────┤
// │  单元测试             ~20%  工具函数    │
// ├─────────────────────────────────────────┤
// │  快照 + a11y          ~10%  结构稳定    │
// └─────────────────────────────────────────┘
```

### 17.4 组件库测试检查清单 / Component Library Test Checklist

| 检查项 / Check Item | 优先级 / Priority | 说明 / Description |
|---|---|---|
| 默认渲染 / Default render | P0 | 无 props 时的默认输出 / Output with no props |
| 每个 prop 变体 / Each prop variant | P0 | 所有合法值 / All valid values |
| 事件回调 / Event callbacks | P0 | onClick, onChange 等 / All handlers |
| 键盘操作 / Keyboard ops | P0 | Tab, Enter, Escape, Arrows |
| ARIA 属性 / ARIA attributes | P0 | role, aria-*, 关联 / Relations |
| 焦点管理 / Focus management | P1 | 焦点陷阱、恢复 / Trap, restore |
| 禁用状态 / Disabled state | P1 | 不可交互 + 视觉 / No interaction |
| 错误状态 / Error state | P1 | 错误提示可访问 / Accessible errors |
| ref 转发 / Ref forwarding | P1 | 支持外部 ref / External ref |
| 组合性 / Composition | P2 | children, render props |
| 主题定制 / Theming | P2 | CSS 变量、主题切换 / Theme switch |
| 响应式 / Responsive | P2 | 断点行为 / Breakpoint behavior |

## 18. 测试自动化与持续集成 / Test Automation & Continuous Integration

测试自动化是将测试执行集成到开发工作流和 CI/CD 管线中的实践。良好的自动化策略能在代码提交后几分钟内反馈质量问题。

Test automation is the practice of integrating test execution into development workflows and CI/CD pipelines. Good automation strategies provide quality feedback within minutes of code commits.

### 18.1 CI 管线测试分层 / CI Pipeline Test Layering

```yaml
# .github/workflows/test-pipeline.yml
# 分层测试管线 / Layered test pipeline
#
# 设计原则 / Design principles:
# 1. 快速反馈优先 / Fast feedback first
# 2. 失败早停止 / Fail early
# 3. 并行执行 / Parallel execution
# 4. 缓存加速 / Cache acceleration

name: Test Pipeline

on:
  pull_request:
    branches: [main, develop]
  push:
    branches: [main]

env:
  NODE_VERSION: '20'
  PNPM_VERSION: '9'

jobs:
  # === 第 1 层：静态检查（~30s）/ Layer 1: Static checks ===
  static-analysis:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: ${{ env.PNPM_VERSION }}
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'pnpm'
      - run: pnpm install --frozen-lockfile

      - name: TypeScript 类型检查 / Type check
        run: pnpm tsc --noEmit

      - name: ESLint 检查 / Lint
        run: pnpm eslint . --max-warnings=0

      - name: Prettier 格式 / Format check
        run: pnpm prettier --check "src/**/*.{ts,tsx}"

  # === 第 2 层：单元测试（~2min）/ Layer 2: Unit tests ===
  unit-tests:
    needs: static-analysis
    runs-on: ubuntu-latest
    strategy:
      matrix:
        shard: [1, 2, 3, 4]  # 4 路并行 / 4-way parallel
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'pnpm'
      - run: pnpm install --frozen-lockfile

      - name: 执行分片测试 / Run sharded tests
        run: |
          pnpm vitest run \
            --shard=${{ matrix.shard }}/4 \
            --coverage \
            --reporter=junit \
            --outputFile=test-results/shard-${{ matrix.shard }}.xml

      - uses: actions/upload-artifact@v4
        with:
          name: coverage-shard-${{ matrix.shard }}
          path: coverage/

  # === 第 3 层：集成测试（~5min）/ Layer 3: Integration tests ===
  integration-tests:
    needs: unit-tests
    runs-on: ubuntu-latest
    services:
      backend:
        image: PrivShield:latest
        ports: ['8079:8079']
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'pnpm'
      - run: pnpm install --frozen-lockfile

      - name: 等待后端就绪 / Wait for backend
        run: |
          timeout 30 bash -c 'until curl -s http://localhost:8079/health; do sleep 1; done'

      - name: 执行集成测试 / Run integration tests
        run: pnpm vitest run --config vitest.integration.config.ts
        env:
          API_BASE_URL: http://localhost:8079

  # === 第 4 层：E2E 测试（~10min）/ Layer 4: E2E tests ===
  e2e-tests:
    needs: integration-tests
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'pnpm'
      - run: pnpm install --frozen-lockfile
      - run: pnpm exec playwright install --with-deps chromium

      - name: 构建应用 / Build app
        run: pnpm build

      - name: E2E 测试 / E2E tests
        run: pnpm exec playwright test --project=chromium

      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: playwright-report
          path: playwright-report/
```

### 18.2 测试数据工厂 / Test Data Factory

```typescript
/**
 * 测试数据工厂模式 / Test data factory pattern
 *
 * 优势 / Benefits:
 * 1. 减少测试中的重复数据构造 / Reduce repetitive data construction
 * 2. 默认值合理，覆盖可选 / Sensible defaults, optional overrides
 * 3. 类型安全 / Type-safe
 * 4. 可组合 / Composable
 */
import { faker } from '@faker-js/faker';

// 基础工厂函数类型 / Base factory function type
type Factory<T> = (overrides?: Partial<T>) => T;

// === 用户工厂 / User factory ===
export const createUser: Factory<User> = (overrides = {}) => ({
  id: faker.string.uuid(),
  name: faker.person.fullName(),
  email: faker.internet.email(),
  role: 'viewer',
  createdAt: faker.date.recent().toISOString(),
  ...overrides,
});

// === API 响应工厂 / API response factory ===
export const createApiResponse: Factory<ApiResponse<unknown>> = (overrides = {}) => ({
  success: true,
  data: null,
  error: null,
  timestamp: new Date().toISOString(),
  requestId: faker.string.uuid(),
  ...overrides,
});

// === 隐私分类结果工厂 / Privacy classification result factory ===
export const createClassificationResult: Factory<ClassificationResult> = (overrides = {}) => ({
  field: 'user_email',
  category: 'PII',
  subcategory: 'email_address',
  confidence: 0.95,
  sensitivityLevel: 3,
  suggestedMask: 'partial',
  ...overrides,
});

// === 组合工厂 / Composed factories ===
export const createUserList = (count: number, overrides?: Partial<User>): User[] =>
  Array.from({ length: count }, () => createUser(overrides));

export const createPaginatedResponse = <T>(
  items: T[],
  page = 1,
  pageSize = 20
): PaginatedResponse<T> => ({
  items,
  total: items.length,
  page,
  pageSize,
  totalPages: Math.ceil(items.length / pageSize),
});

// === 测试中使用 / Usage in tests ===
describe('UserList 组件 / UserList component', () => {
  it('渲染用户列表 / renders user list', () => {
    const users = createUserList(5, { role: 'admin' });
    render(<UserList users={users} />);

    expect(screen.getAllByRole('listitem')).toHaveLength(5);
    users.forEach(user => {
      expect(screen.getByText(user.name)).toBeInTheDocument();
    });
  });

  it('空列表显示提示 / empty list shows message', () => {
    render(<UserList users={[]} />);
    expect(screen.getByText(/no users/i)).toBeInTheDocument();
  });
});
```

### 18.3 测试环境管理 / Test Environment Management

```typescript
/**
 * 测试环境配置与隔离 / Test environment configuration & isolation
 *
 * 关键原则 / Key principles:
 * 1. 每个测试文件独立环境 / Independent env per test file
 * 2. 无全局状态泄漏 / No global state leakage
 * 3. 确定性时间/随机 / Deterministic time/random
 * 4. 网络层完全 mock / Fully mocked network layer
 */

// vitest.setup.ts - 全局设置 / Global setup
import { afterEach, beforeEach, vi } from 'vitest';
import { cleanup } from '@testing-library/react';
import { server } from './mocks/server';

// MSW 服务器生命周期 / MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => {
  server.resetHandlers();  // 重置 handler / Reset handlers
  cleanup();              // 清理 DOM / Clean DOM
  vi.clearAllMocks();     // 清理 mock / Clear mocks
});
afterAll(() => server.close());

// === 时间控制 / Time control ===
export function mockTime(isoString: string = '2024-06-15T10:30:00Z') {
  const date = new Date(isoString);
  vi.useFakeTimers();
  vi.setSystemTime(date);
  return {
    advance: (ms: number) => vi.advanceTimersByTime(ms),
    advanceTo: (iso: string) => vi.setSystemTime(new Date(iso)),
    restore: () => vi.useRealTimers(),
  };
}

// === 环境变量控制 / Environment variable control ===
export function withEnv(vars: Record<string, string>) {
  const original = { ...process.env };

  beforeEach(() => {
    Object.assign(process.env, vars);
  });

  afterEach(() => {
    process.env = original;
  });
}

// === 控制台输出控制 / Console output control ===
export function silenceConsole(...methods: ('log' | 'warn' | 'error')[]) {
  const originals: Record<string, any> = {};

  beforeEach(() => {
    methods.forEach(m => {
      originals[m] = console[m];
      console[m] = vi.fn();
    });
  });

  afterEach(() => {
    methods.forEach(m => {
      console[m] = originals[m];
    });
  });
}
```

### 18.4 测试自动化成熟度模型 / Test Automation Maturity Model

| 级别 / Level | 名称 / Name | 特征 / Characteristics | 反馈时间 / Feedback Time |
|---|---|---|---|
| L1 | 手动 / Manual | 开发者本地运行 / Dev runs locally | 无保证 / No guarantee |
| L2 | 基础 CI / Basic CI | 提交触发单元测试 / Commit triggers unit | ~5min |
| L3 | 分层管线 / Layered pipeline | 静态+单元+集成 / Static+unit+integration | ~10min |
| L4 | 并行+分片 / Parallel+shard | 多路并行执行 / Multi-way parallel | ~3min |
| L5 | 智能选择 / Smart selection | 只跑受影响测试 / Only affected tests | ~1min |
| L6 | 预测分析 / Predictive | ML 预测失败风险 / ML predicts failure risk | 实时 / Real-time |

## 19. 契约测试与 API 验证 / Contract Testing & API Validation

契约测试验证服务间的接口约定是否被双方正确实现。在前后端分离架构中，契约测试能及早发现接口不匹配问题，避免集成阶段的意外。

Contract testing verifies that interface agreements between services are correctly implemented by both parties. In separated frontend/backend architectures, contract tests catch interface mismatches early, avoiding integration surprises.

### 19.1 前端契约测试 / Frontend Contract Testing

```typescript
/**
 * API 契约测试 / API contract testing
 *
 * 验证前端期望的 API 响应格式与后端实际返回一致。
 * Verifies frontend-expected API response format matches backend actual output.
 *
 * 策略 / Strategy:
 * 1. 从 OpenAPI spec 生成类型 / Generate types from OpenAPI spec
 * 2. 用真实后端响应做快照 / Snapshot real backend responses
 * 3. CI 中对比 mock 与实际 / Compare mock vs actual in CI
 */
import { describe, it, expect } from 'vitest';
import { z } from 'zod';

// === 定义 API 契约 Schema / Define API contract schemas ===

const MaskingRequestSchema = z.object({
  data: z.record(z.unknown()),
  profile: z.string().optional(),
  fields: z.array(z.string()).optional(),
});

const MaskingResponseSchema = z.object({
  masked_data: z.record(z.unknown()),
  masked_fields: z.array(z.string()),
  profile_used: z.string(),
});

const ClassificationFieldSchema = z.object({
  field: z.string(),
  category: z.string(),
  level: z.number().int().min(1).max(5),
  confidence: z.number().min(0).max(1),
  rule_matched: z.string().optional(),
});

const ClassificationResponseSchema = z.object({
  results: z.array(ClassificationFieldSchema),
  summary: z.object({
    total_fields: z.number(),
    sensitive_fields: z.number(),
    max_level: z.number(),
  }),
});

const DPQueryResponseSchema = z.object({
  result: z.number(),
  epsilon: z.number().positive(),
  mechanism: z.enum(['laplace', 'gaussian']),
  noise_added: z.number(),
  budget_remaining: z.number(),
});

// === 契约测试 / Contract tests ===

describe('API 契约验证 / API Contract Validation', () => {
  describe('Masking API 契约 / Masking API contract', () => {
    it('请求格式符合契约 / request format matches contract', () => {
      const request = {
        data: { name: '张三', email: 'zhang@example.com', age: 30 },
        profile: 'default',
      };

      const result = MaskingRequestSchema.safeParse(request);
      expect(result.success).toBe(true);
    });

    it('响应格式符合契约 / response format matches contract', () => {
      // 模拟后端实际响应 / Simulate actual backend response
      const backendResponse = {
        masked_data: { name: '张*', email: 'z***@example.com', age: 30 },
        masked_fields: ['name', 'email'],
        profile_used: 'default',
      };

      const result = MaskingResponseSchema.safeParse(backendResponse);
      expect(result.success).toBe(true);
    });

    it('拒绝缺少必填字段的响应 / rejects response missing required fields', () => {
      const invalidResponse = {
        masked_data: { name: '张*' },
        // 缺少 masked_fields 和 profile_used / Missing required fields
      };

      const result = MaskingResponseSchema.safeParse(invalidResponse);
      expect(result.success).toBe(false);
    });
  });

  describe('Classification API 契约 / Classification API contract', () => {
    it('分类结果包含所有必要字段 / results contain all required fields', () => {
      const response = {
        results: [
          {
            field: 'id_card',
            category: 'PII',
            level: 4,
            confidence: 0.98,
            rule_matched: 'regex_id_card',
          },
        ],
        summary: {
          total_fields: 5,
          sensitive_fields: 2,
          max_level: 4,
        },
      };

      const result = ClassificationResponseSchema.safeParse(response);
      expect(result.success).toBe(true);

      // 验证业务约束 / Verify business constraints
      if (result.success) {
        expect(result.data.summary.max_level).toBeGreaterThanOrEqual(
          Math.max(...result.data.results.map(r => r.level))
        );
      }
    });

    it('level 必须在 1-5 范围 / level must be in range 1-5', () => {
      const invalidResult = {
        field: 'test',
        category: 'PII',
        level: 6,  // 超出范围 / Out of range
        confidence: 0.9,
      };

      const result = ClassificationFieldSchema.safeParse(invalidResult);
      expect(result.success).toBe(false);
    });
  });

  describe('DP API 契约 / DP API contract', () => {
    it('epsilon 必须为正数 / epsilon must be positive', () => {
      const invalidResponse = {
        result: 42.5,
        epsilon: -1,  // 无效 / Invalid
        mechanism: 'laplace',
        noise_added: 3.2,
        budget_remaining: 0.5,
      };

      const result = DPQueryResponseSchema.safeParse(invalidResponse);
      expect(result.success).toBe(false);
    });

    it('mechanism 只接受枚举值 / mechanism only accepts enum values', () => {
      const validResponse = {
        result: 100.3,
        epsilon: 0.5,
        mechanism: 'gaussian',
        noise_added: -2.1,
        budget_remaining: 1.5,
      };

      const result = DPQueryResponseSchema.safeParse(validResponse);
      expect(result.success).toBe(true);
    });
  });
});
```

### 19.2 契约变更检测 / Contract Change Detection

```typescript
/**
 * 契约变更检测工具 / Contract change detection utility
 *
 * 在 CI 中自动检测 API 契约变更，防止破坏性修改。
 * Automatically detects API contract changes in CI, preventing breaking modifications.
 */
import { readFileSync } from 'fs';
import { resolve } from 'path';

interface ContractField {
  name: string;
  type: string;
  required: boolean;
  description?: string;
}

interface ContractSnapshot {
  version: string;
  timestamp: string;
  endpoints: Record<string, {
    method: string;
    requestFields: ContractField[];
    responseFields: ContractField[];
  }>;
}

class ContractChangeDetector {
  private baseline: ContractSnapshot;
  private current: ContractSnapshot;

  constructor(baselinePath: string, currentPath: string) {
    this.baseline = JSON.parse(readFileSync(resolve(baselinePath), 'utf-8'));
    this.current = JSON.parse(readFileSync(resolve(currentPath), 'utf-8'));
  }

  detectBreakingChanges(): string[] {
    const changes: string[] = [];

    for (const [endpoint, baseSpec] of Object.entries(this.baseline.endpoints)) {
      const currentSpec = this.current.endpoints[endpoint];

      // 1. 端点删除 = 破坏性 / Endpoint removal = breaking
      if (!currentSpec) {
        changes.push(`BREAKING: Endpoint removed: ${endpoint}`);
        continue;
      }

      // 2. 必填字段删除 = 破坏性 / Required field removal = breaking
      for (const field of baseSpec.responseFields) {
        if (field.required) {
          const stillExists = currentSpec.responseFields.some(f => f.name === field.name);
          if (!stillExists) {
            changes.push(`BREAKING: Required response field removed: ${endpoint}.${field.name}`);
          }
        }
      }

      // 3. 新增必填请求字段 = 破坏性 / New required request field = breaking
      for (const field of currentSpec.requestFields) {
        if (field.required) {
          const existedBefore = baseSpec.requestFields.some(f => f.name === field.name);
          if (!existedBefore) {
            changes.push(`BREAKING: New required request field: ${endpoint}.${field.name}`);
          }
        }
      }

      // 4. 字段类型变更 = 破坏性 / Field type change = breaking
      for (const baseField of baseSpec.responseFields) {
        const currentField = currentSpec.responseFields.find(f => f.name === baseField.name);
        if (currentField && currentField.type !== baseField.type) {
          changes.push(
            `BREAKING: Type changed: ${endpoint}.${baseField.name} ` +
            `${baseField.type} -> ${currentField.type}`
          );
        }
      }
    }

    return changes;
  }

  detectNonBreakingChanges(): string[] {
    const changes: string[] = [];

    for (const [endpoint, currentSpec] of Object.entries(this.current.endpoints)) {
      const baseSpec = this.baseline.endpoints[endpoint];

      // 新增端点 = 非破坏性 / New endpoint = non-breaking
      if (!baseSpec) {
        changes.push(`INFO: New endpoint added: ${endpoint}`);
        continue;
      }

      // 新增可选响应字段 = 非破坏性 / New optional response field = non-breaking
      for (const field of currentSpec.responseFields) {
        const existed = baseSpec.responseFields.some(f => f.name === field.name);
        if (!existed && !field.required) {
          changes.push(`INFO: Optional field added: ${endpoint}.${field.name}`);
        }
      }
    }

    return changes;
  }
}
```

### 19.3 契约测试策略对比 / Contract Testing Strategy Comparison

| 策略 / Strategy | 工具 / Tools | 优势 / Pros | 劣势 / Cons | 适用 / Suitable |
|---|---|---|---|---|
| Schema 验证 / Schema validation | Zod, JSON Schema | 简单直接 / Simple | 手动维护 / Manual | ✅ 本项目 / This project |
| OpenAPI 生成 / OpenAPI gen | openapi-typescript | 自动同步 / Auto-sync | 依赖 spec 准确 / Depends on spec | 推荐 / Recommended |
| Consumer-driven / CDC | Pact | 双向验证 / Bidirectional | 复杂度高 / Complex | 微服务 / Microservices |
| 快照对比 / Snapshot diff | 自定义 / Custom | 灵活 / Flexible | 无标准化 / Non-standard | 内部 API / Internal |
| E2E 验证 / E2E validation | Playwright + API | 真实环境 / Real env | 慢 / Slow | 发布前 / Pre-release |
