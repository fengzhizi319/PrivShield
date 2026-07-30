# React 技术栈说明 / React Technology Stack

## 1. 技术简介 / Introduction

React 是由 Meta（Facebook）开发并维护的开源 JavaScript UI 库，用于构建用户界面。
React is an open-source JavaScript UI library developed and maintained by Meta (Facebook) for building user interfaces.

核心特性 / Core Features：
- **组件化（Component-based）**：UI 被拆分为独立、可复用的组件，每个组件管理自己的状态与渲染逻辑。
- **声明式渲染（Declarative Rendering）**：开发者描述"UI 应该是什么样"，React 负责高效更新 DOM。
- **虚拟 DOM（Virtual DOM）**：通过 diff 算法最小化真实 DOM 操作，提升渲染性能。
- **单向数据流（One-way Data Flow）**：数据从父组件通过 props 流向子组件，状态变化通过 setState 触发重渲染。
- **Hooks API**：React 16.8+ 引入的函数组件状态管理能力（useState、useEffect、useCallback、useMemo 等）。

本项目使用版本 / Version Used：`React ^18.3.1` + `ReactDOM ^18.3.1`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 应用入口 / Application Entry

文件 / File：`console/web/src/main.tsx`

```tsx
// 使用 React 18 的 createRoot API 挂载根组件
// Mount root component using React 18's createRoot API
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </React.StrictMode>,
);
```

- `React.StrictMode`：开发环境下双重渲染以检测副作用问题，不影响生产构建。
- `I18nProvider`：国际化上下文提供者，包裹整个应用。

### 2.2 组件架构 / Component Architecture

```
App.tsx                    # 根组件：全局状态 + 三栏布局 / Root: global state + 3-column layout
├── Header.tsx             # 顶部导航：品牌 + 状态灯 + 后端切换 / Top nav: brand + health + backend switch
├── Sidebar.tsx            # 侧边栏：接口导航树 + 搜索 / Sidebar: API navigation tree + search
├── Overview.tsx           # 总览页：分类卡片网格 / Overview: category card grid
├── EndpointView.tsx       # 端点测试：请求编辑 + 响应查看 / Endpoint test: request editor + response viewer
├── BatchTest.tsx          # 批量测试 / Batch testing
├── FileTest.tsx           # 文件上传处理 / File upload processing
├── LbTest.tsx             # 负载均衡测试 / Load balancing test
├── DynClassificationPanel.tsx  # 动态分类分级 / Dynamic classification
├── ResponsePanel.tsx      # 响应展示面板 / Response display panel
├── HistoryPanel.tsx       # 请求历史面板 / Request history panel
├── BackendSelector.tsx    # 后端切换器 / Backend selector
├── ErrorBoundary.tsx      # 错误边界（降级 UI）/ Error boundary (fallback UI)
└── icons.tsx              # SVG 图标集 / SVG icon set
```

### 2.3 状态管理模式 / State Management Pattern

本项目使用 React 内置 Hooks 进行状态管理，无需 Redux 等外部状态库：
This project uses React's built-in Hooks for state management, no external state library needed:

- `useState`：组件局部状态（如当前视图、请求体文本、响应数据）
- `useEffect`：副作用处理（如后端切换时重新拉取数据、键盘快捷键注册）
- `useCallback`：缓存回调函数避免不必要的子组件重渲染
- `useMemo`：缓存计算结果（如分类分组、搜索过滤）
- `useRef`：保存不触发重渲染的可变引用（如快捷键 handler）

### 2.4 Hooks 使用详解 / Hooks Usage Details

本项目大量使用 React Hooks 实现状态管理与副作用控制：
This project heavily uses React Hooks for state management and side effect control:

#### useState — 组件局部状态 / Component Local State

```tsx
// App.tsx 中的核心状态声明 / Core state declarations in App.tsx
const [samples, setSamples] = useState<EndpointSample[]>([]);  // 端点列表 / Endpoint list
const [view, setView] = useState<View>({ type: 'overview' });  // 当前视图 / Current view
const [health, setHealth] = useState<ConsoleHealth | null>(null); // 健康状态 / Health status
const [loading, setLoading] = useState(true);                  // 加载标志 / Loading flag
const [error, setError] = useState<string | null>(null);       // 错误信息 / Error message
const [backend, setBackend] = useState<BackendOption>(DEFAULT_BACKEND); // 后端选择 / Backend selection
```

#### useEffect — 副作用处理 / Side Effect Handling

```tsx
// 后端切换时更新 API 基址并重新加载数据 / Update API base URL and reload data on backend switch
useEffect(() => {
  setBaseUrl(backend.value); // 切换全局 fetch 基址 / Switch global fetch base URL
  load();                    // 并行拉取 samples + health / Fetch samples + health in parallel
}, [backend, load]);         // 依赖数组：backend 或 load 变化时触发 / Trigger on backend or load change
```

#### useCallback — 稳定回调引用 / Stable Callback Reference

```tsx
// load() 使用 useCallback 缓存，避免 useEffect 无限循环 / load() cached with useCallback to prevent infinite loop
const load = useCallback(async () => {
  setLoading(true);
  setError(null);
  try {
    const [samplesData, healthData] = await Promise.all([fetchSamples(), fetchHealth()]);
    setSamples(samplesData);
    setHealth(healthData);
    setView({ type: 'overview' });
  } catch (e) {
    setError((e as Error).message);
  } finally {
    setLoading(false);
  }
}, []); // 空依赖：函数引用永不变 / Empty deps: function reference never changes
```

#### useMemo — 缓存计算结果 / Cached Computation

```tsx
// Sidebar 中按分类分组端点（避免每次渲染重新计算）
// Group endpoints by category in Sidebar (avoid recomputation each render)
const grouped = useMemo(() => {
  const map = new Map<string, EndpointSample[]>();
  samples.forEach(s => {
    const list = map.get(s.category) ?? [];
    list.push(s);
    map.set(s.category, list);
  });
  return map;
}, [samples]); // 仅 samples 变化时重新分组 / Regroup only when samples change
```

#### useRef — 不触发重渲染的可变引用 / Mutable Ref Without Re-render

```tsx
// EndpointView 中保存快捷键 handler 引用 / Save keyboard shortcut handler ref in EndpointView
const handlerRef = useRef<((e: KeyboardEvent) => void) | null>(null);
handlerRef.current = handleKeyDown; // 每次渲染更新引用但不触发重渲染 / Update ref each render without re-render
```

### 2.5 国际化（i18n）实现 / Internationalization (i18n) Implementation

文件 / File：`console/web/src/i18n/index.tsx`

本项目采用**自建轻量 i18n 方案**（无 react-i18next / react-intl 外部依赖）：
This project uses a **self-built lightweight i18n solution** (no react-i18next / react-intl dependency):

```tsx
// 架构 / Architecture：
//   I18nProvider (Context Provider) → useI18n() Hook → t() 翻译函数

// 使用方式 / Usage：
const { t, lang, setLang } = useI18n();
<span>{t('header.health_ok')}</span>           // 简单翻译 / Simple translation
<span>{t('batch.summary', 10, 8, 2)}</span>   // 占位符替换 {0},{1},{2} / Placeholder replacement
```

实现要点 / Implementation Highlights：

| 特性 / Feature | 实现方式 / Implementation |
|---|---|
| 双语字典 / Bilingual dictionaries | `zh` / `en` 两个 `Record<string, string>` 对象 / Two Record objects |
| 占位符替换 / Placeholder replacement | `t(key, ...args)` 中 `{0}` → `args[0]` / `{0}` → `args[0]` in t() |
| 语言持久化 / Language persistence | `localStorage('console-lang')` 存储偏好 / Store preference |
| 性能优化 / Performance | `useCallback` 缓存 `t` 函数，仅 `lang` 变化时重建 / Cache t(), rebuild only on lang change |
| 回退策略 / Fallback | key 不存在时原样返回 key / Return key as-is when not found |
| 默认语言 / Default language | 中文（'zh'）/ Chinese |

### 2.6 错误边界 / Error Boundary

```tsx
// ErrorBoundary.tsx：基于 Class 组件的 React 错误边界
// ErrorBoundary.tsx: Class component-based React error boundary
class ErrorBoundary extends React.Component<Props, State> {
  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };  // 捕获子组件渲染异常 / Catch child render exceptions
  }
  render() {
    if (this.state.hasError) return <FallbackUI />;  // 降级 UI / Fallback UI
    return this.props.children;                      // 正常渲染 / Normal render
  }
}
```

设计目的：单个组件（如 EndpointView）崩溃时不导致整页白屏，仅影响局部区域。
Design purpose: Single component crash (e.g. EndpointView) won't blank the entire page.

### 2.7 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 函数组件 + Hooks（无 Class 组件）| 代码更简洁、逻辑复用更灵活 / Cleaner code, flexible logic reuse |
| 无路由库（状态驱动视图切换）| 单页工具无需 URL 路由，减少依赖 / Single-page tool needs no URL routing |
| ErrorBoundary 包裹主区域 | 单组件崩溃不导致整页白屏 / Component crash won't blank the page |
| key 强制重建 EndpointView | 切换端点时清除上一端点的残留状态 / Clear stale state on endpoint switch |
| 自建 i18n 而非 react-i18next | 仅需双语切换，无需复数/性别/ICU 等高级特性 / Only need bilingual, no plural/gender/ICU |
| Promise.all 并行加载 | samples + health 无依赖关系，并行提升加载速度 / No dependency, parallel for faster loading |
| View 判别联合类型 | TypeScript 编译期保证视图切换的类型安全 / Compile-time type safety for view switching |

### 2.8 渲染管线与协调算法 / Rendering Pipeline & Reconciliation

```text
┌─────────────────────────────────────────────────────────────┐
│  1. 状态更新触发 / State update triggers                     │
│     setState() / setSamples() / setView()                    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Render 阶段 / Render Phase                               │
│     - 调用组件函数，生成新 Virtual DOM / Call component, new VDOM│
│     - 可中断（Concurrent Mode）/ Interruptible                │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. Reconciliation（协调）/ Reconciliation                    │
│     - Diff 新旧 Virtual DOM / Diff old vs new VDOM           │
│     - 计算最小变更集 / Calculate minimal changeset           │
│     - key 属性优化列表 diff / key prop optimizes list diff   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Commit 阶段 / Commit Phase                               │
│     - 批量更新真实 DOM / Batch update real DOM               │
│     - 不可中断（同步执行）/ Non-interruptible (sync)         │
│     - 触发 useEffect / Trigger useEffect                     │
└─────────────────────────────────────────────────────────────┘
```

**Virtual DOM Diff 策略 / Virtual DOM Diff Strategy**：

| 策略 / Strategy | 说明 / Description |
|---|---|
| 同层比较 / Same-level comparison | 仅比较同一层级的节点 / Only compare nodes at same level |
| 类型判断 / Type check | 不同类型直接替换整棵子树 / Different type = replace entire subtree |
| key 优化 / key optimization | 列表中用 key 识别节点移动 / Identify node moves in lists by key |
| 批量更新 / Batch updates | 多次 setState 合并为一次渲染 / Multiple setState merged into one render |

### 2.9 React 18 并发特性 / React 18 Concurrent Features

本项目使用 React 18，具备以下并发能力：

```tsx
// 1. 自动批处理 / Automatic Batching
// React 18 自动合并多次 setState（包括异步回调中）
// React 18 auto-batches multiple setState (including in async callbacks)
const handleClick = async () => {
  setLoading(true);   // 不会立即渲染 / Won't render immediately
  setError(null);     // 不会立即渲染 / Won't render immediately
  await fetchData();  // 异步操作 / Async operation
  setData(result);    // 不会立即渲染 / Won't render immediately
  setLoading(false);  // ← 仅此处触发一次渲染 / Only here triggers one render
};

// 2. createRoot API / createRoot API
// 启用并发模式的入口 / Entry point enabling concurrent mode
ReactDOM.createRoot(document.getElementById('root')!).render(<App />);

// 3. StrictMode 双重渲染 / StrictMode double render
// 开发环境检测副作用问题（生产无影响）
// Dev environment detects side effect issues (no production impact)
<React.StrictMode>
  <App />
</React.StrictMode>
```

### 2.10 性能优化策略 / Performance Optimization Strategies

| 策略 / Strategy | 本项目应用 / Application in Project |
|---|---|
| `useCallback` | 缓存 load() 函数，避免 useEffect 无限循环 / Cache load() to prevent infinite loop |
| `useMemo` | 缓存分类分组计算，仅 samples 变化时重算 / Cache grouping, recompute only on samples change |
| `key` 属性 | EndpointView 切换时强制重建，清除残留状态 / Force rebuild on switch, clear stale state |
| 条件渲染 / Conditional rendering | 加载中显示 Spinner，避免渲染空数据 / Show spinner while loading |
| 事件委托 / Event delegation | React 自动在根节点委托事件 / React auto-delegates events at root |

**避免不必要渲染 / Avoid Unnecessary Re-renders**：

```tsx
// ✗ 错误：每次渲染创建新函数引用 / Wrong: new function ref each render
<Sidebar onSelect={() => setView({ type: 'endpoint', sample })} />

// ✓ 正确：useCallback 缓存回调 / Correct: useCallback caches callback
const handleSelect = useCallback((sample: EndpointSample) => {
  setView({ type: 'endpoint', sample });
}, []);  // 空依赖：永不重建 / Empty deps: never rebuild
<Sidebar onSelect={handleSelect} />
```

### 2.11 组件通信模式 / Component Communication Patterns

```text
┌─────────────────────────────────────────────────────────────┐
│  App.tsx（状态中心）/ App.tsx (State Center)                  │
│  - samples, view, health, loading, error, backend            │
└───────┬───────────────┬───────────────┬──────────────────┘
        │ props ↓        │ props ↓        │ props ↓
        ▼               ▼               ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────────┐
│  Header     │ │  Sidebar    │ │  Main Content   │
│  - health   │ │  - samples  │ │  - view         │
│  - backend  │ │  - onSelect │ │  - samples      │
│  - onSwitch │ │  (callback) │ │  - backend      │
└─────────────┘ └─────────────┘ └─────────────────┘

通信方式 / Communication:
- 父 → 子：props 传递数据 / Parent → Child: props pass data
- 子 → 父：回调函数 / Child → Parent: callback functions
- 跨层级：Context (i18n) / Cross-level: Context (i18n)
```

## 3. Context API 详解 / Context API Details

### 3.1 本项目的 i18n Context / i18n Context in This Project

```tsx
// i18n/index.tsx 中的 Context 实现 / Context implementation in i18n/index.tsx

// 1. 创建 Context 对象 / Create Context object
const I18nContext = createContext<I18nContextValue | null>(null);

// 2. Provider 组件 / Provider component
export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLang] = useState<Lang>(() => {
    // 从 localStorage 恢复语言偏好 / Restore language preference from localStorage
    return (localStorage.getItem('console-lang') as Lang) || 'zh';
  });

  // 缓存翻译函数（仅 lang 变化时重建）/ Cache translation function (rebuild only on lang change)
  const t = useCallback((key: string, ...args: unknown[]) => {
    let text = dictionaries[lang][key] ?? key;
    args.forEach((arg, i) => { text = text.replace(`{${i}}`, String(arg)); });
    return text;
  }, [lang]);

  return (
    <I18nContext.Provider value={{ t, lang, setLang }}>
      {children}
    </I18nContext.Provider>
  );
}

// 3. 自定义 Hook 封装 / Custom Hook wrapper
export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used within I18nProvider');
  return ctx;
}
```

### 3.2 Context vs Props Drilling / Context vs Props Drilling

```text
Props Drilling 问题 / Props Drilling problem:
  App → Layout → Sidebar → NavItem → Label
  （每层都需传递 lang/t）/ Each level must pass lang/t

Context 解决 / Context solution:
  App (Provider) → ... → Label (useContext)
  （中间层无需感知）/ Intermediate layers unaware
```

| 方式 / Approach | 适用 / Suitable | 本项目 / This Project |
|---|---|---|
| Props | 1-2 层传递 / 1-2 levels | ✅ 主要方式 / Primary |
| Context | 跨多层共享 / Cross-level sharing | ✅ i18n 语言 / i18n language |
| 状态库 / State lib | 复杂全局状态 / Complex global state | ❌ 不需要 / Not needed |

## 4. React 事件系统 / React Event System

### 4.1 合成事件机制 / Synthetic Event Mechanism

```text
┌─────────────────────────────────────────────────────────────┐
│  浏览器原生事件 / Browser native events                    │
│  click, keydown, input, submit...                           │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  React 事件委托 / React event delegation                    │
│  - 所有事件统一委托到根节点 / All events delegated to root  │
│  - React 17+: 委托到 root container                         │
│  - React 16: 委托到 document                                │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  SyntheticEvent 包装 / SyntheticEvent wrapper               │
│  - 跨浏览器一致性 / Cross-browser consistency              │
│  - 事件池复用（React 16）/ Event pooling (React 16)       │
│  - 自动绑定 this / Auto-bind this                           │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 本项目中的事件处理 / Event Handling in This Project

```tsx
// 键盘快捷键（Ctrl+Enter 发送请求）/ Keyboard shortcut (Ctrl+Enter to send)
function EndpointView() {
  const handlerRef = useRef<(e: KeyboardEvent) => void>();

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        sendRequest();
      }
    };
    handlerRef.current = handler;
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);  // 清理 / Cleanup
  }, [sendRequest]);
}

// 表单提交 / Form submission
<form onSubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
  <input onChange={(e) => setValue(e.target.value)} />
</form>
```

## 5. 性能优化模式 / Performance Optimization Patterns

### 5.1 React.memo 与 useMemo / React.memo & useMemo

```tsx
// React.memo：跳过 props 未变的子组件渲染 / Skip re-render when props unchanged
const Sidebar = React.memo(function Sidebar({ samples, onSelect }: Props) {
  // 仅当 samples 或 onSelect 变化时重新渲染
  // Only re-render when samples or onSelect changes
  return <nav>{/* ... */}</nav>;
});

// useMemo：缓存计算结果 / Cache computation results
const filtered = useMemo(() => {
  return samples.filter(s =>
    s.name.toLowerCase().includes(search.toLowerCase())
  );
}, [samples, search]);  // 仅依赖变化时重新计算 / Recompute only on dep change
```

### 5.2 何时使用优化 / When to Optimize

| 场景 / Scenario | 优化手段 / Optimization | 必要性 / Necessity |
|---|---|---|
| 大列表渲染 / Large list render | React.memo + key | 高 / High |
| 复杂计算 / Complex computation | useMemo | 中 / Medium |
| 回调传递给子组件 / Callback to child | useCallback | 中 / Medium |
| 简单组件 / Simple component | 无需优化 / No optimization | 低 / Low |
| 本项目（<20 组件）/ This project | useCallback + useMemo 已足够 | 低 / Low |

### 5.3 key 属性的正确使用 / Correct key Usage

```tsx
// ✅ 正确：稳定唯一 key / Correct: stable unique key
{samples.map(s => (
  <EndpointCard key={s.id} sample={s} />
))}

// ✅ 正确：强制重建（本项目用于切换端点）/ Force rebuild (endpoint switch)
<EndpointView key={selectedSample.id} sample={selectedSample} />
// key 变化 → React 销毁旧组件 + 创建新组件（清除内部状态）
// key change → React destroys old + creates new (clears internal state)

// ❌ 错误：使用索引作为 key（列表重排时出问题）/ Wrong: index as key
{items.map((item, i) => <Card key={i} />)}  // 重排时状态错乱
```

## 6. 自定义 Hook 模式 / Custom Hook Patterns

### 6.1 本项目中的自定义 Hook / Custom Hooks in This Project

```tsx
// useI18n：国际化 / Internationalization
const { t, lang, setLang } = useI18n();

// 可抽取的自定义 Hook 示例 / Extractable custom Hook example
function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    fn()
      .then(result => { if (!cancelled) setData(result); })
      .catch(err => { if (!cancelled) setError(err.message); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };  // 防止内存泄漏 / Prevent memory leak
  }, deps);

  return { data, loading, error };
}

// 使用 / Usage:
const { data: samples, loading } = useAsync(fetchSamples, [backend]);
```

### 6.2 Hook 规则 / Rules of Hooks

| 规则 / Rule | 说明 / Description |
|---|---|
| 仅在顶层调用 / Only call at top level | 不在条件/循环/嵌套函数中 / Not in conditions/loops/nested |
| 仅在组件/Hook 中调用 / Only in components/Hooks | 不在普通函数中 / Not in regular functions |
| 自定义 Hook 以 use 开头 / Custom Hooks start with use | 便于 ESLint 检测 / Enables ESLint detection |
| 每次渲染顺序一致 / Same order every render | React 依赖调用顺序 / React relies on call order |

## 7. React 18 并发特性 / React 18 Concurrent Features

### 7.1 并发渲染概述 / Concurrent Rendering Overview

React 18 引入并发渲染，允许 UI 更新可中断、可恢复、可拆分：

```text
┌─────────────────────────────────────────────────────────────┐
│  React 渲染模式对比 / React Rendering Modes                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  React 17（同步渲染）/ React 17 (Synchronous):               │
│    setState → 立即重新渲染整棵树 → 阻塞主线程       │
│    用户输入卡顿（大组件树时）/ Input jank (large tree)    │
│                                                             │
│  React 18（并发渲染）/ React 18 (Concurrent):                │
│    setState → 开始渲染 → 可中断 → 处理高优先级更新     │
│    → 恢复低优先级渲染 → 提交到 DOM                    │
│    用户输入始终流畅 / Input always smooth                │
│                                                             │
│  本项目使用 createRoot → 自动启用并发模式             │
│  This project uses createRoot → concurrent mode auto-enabled│
└─────────────────────────────────────────────────────────────┘
```

### 7.2 useTransition 低优先级更新 / useTransition Low-priority Updates

```tsx
import { useTransition, useState } from 'react';

function SearchPanel() {
  const [input, setInput] = useState('');
  const [results, setResults] = useState<EndpointSample[]>([]);
  const [isPending, startTransition] = useTransition();

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    // 高优先级：立即更新输入框 / High priority: update input immediately
    setInput(e.target.value);

    // 低优先级：过滤结果（可被中断）/ Low priority: filter results (interruptible)
    startTransition(() => {
      const filtered = allSamples.filter(s =>
        s.name.toLowerCase().includes(e.target.value.toLowerCase())
      );
      setResults(filtered);
    });
  };

  return (
    <div>
      <input value={input} onChange={handleChange} />
      {isPending && <span>搜索中...</span>}
      <ResultList items={results} />
    </div>
  );
}
```

### 7.3 useDeferredValue 延迟值 / useDeferredValue Deferred Value

```tsx
import { useDeferredValue, useMemo } from 'react';

function Sidebar({ samples, query }: Props) {
  // 延迟更新：输入时先用旧值渲染，空闲后再更新
  // Deferred: use old value first, update when idle
  const deferredQuery = useDeferredValue(query);
  const isStale = query !== deferredQuery;

  const filtered = useMemo(() => {
    return samples.filter(s => s.name.includes(deferredQuery));
  }, [samples, deferredQuery]);

  return (
    <div style={{ opacity: isStale ? 0.7 : 1 }}>
      {filtered.map(s => <NavItem key={s.id} sample={s} />)}
    </div>
  );
}
```

### 7.4 本项目并发特性使用情况 / Project Concurrent Feature Usage

| 特性 / Feature | 是否使用 / Used? | 原因 / Reason |
|---|---|---|
| createRoot | ✅ | 入口文件使用，启用并发模式 / Entry file, enables concurrent |
| useTransition | ❌ | 组件树小，无性能问题 / Small tree, no perf issues |
| useDeferredValue | ❌ | 搜索列表短（<50 项）/ Short search list (<50 items) |
| Suspense | ❌ | 无懒加载组件 / No lazy components |
| automatic batching | ✅ | React 18 自动启用 / Auto-enabled in React 18 |

## 8. Suspense 与懒加载 / Suspense & Lazy Loading

### 8.1 Suspense 工作机制 / Suspense Mechanism

```tsx
// Suspense 允许组件"等待"异步操作完成后再渲染
// Suspense allows components to "wait" for async operations before rendering

const HeavyPanel = React.lazy(() => import('./HeavyPanel'));

function App() {
  return (
    // fallback 在异步组件加载期间显示 / fallback shown while async component loads
    <React.Suspense fallback={<LoadingSpinner />}>
      <HeavyPanel />
    </React.Suspense>
  );
}

// 嵌套 Suspense：内层优先显示 / Nested Suspense: inner shows first
<React.Suspense fallback={<PageSkeleton />}>
  <Header />  {/* 立即渲染 / Renders immediately */}
  <React.Suspense fallback={<ContentSkeleton />}>
    <HeavyContent />  {/* 异步加载 / Loads async */}
  </React.Suspense>
</React.Suspense>
```

### 8.2 本项目未使用 Suspense 的原因 / Why This Project Doesn't Use Suspense

```text
1. 组件总量小（~15 个），无需懒加载
   Few components (~15), no lazy loading needed
2. 所有数据通过 API 获取，已有 loading 状态
   All data fetched via API, already has loading states
3. 无路由，无页面级分割需求
   No routing, no page-level splitting needed
4. 初始加载已经很快（~60KB gzip）
   Initial load already fast (~60KB gzip)
```

## 9. 性能优化模式 / Performance Optimization Patterns

### 9.1 React.memo 组件缓存 / React.memo Component Caching

```tsx
// 仅当 props 变化时重新渲染 / Only re-render when props change
const NavItem = React.memo(function NavItem({ sample, isActive, onClick }: Props) {
  return (
    <button
      className={isActive ? 'bg-indigo-50' : ''}
      onClick={() => onClick(sample)}
    >
      {sample.name}
    </button>
  );
});

// 本项目中的使用 / Usage in this project:
// - Sidebar 中的列表项（避免父组件重渲染时所有子项重渲染）
// - List items in Sidebar (avoid all items re-rendering when parent re-renders)
```

### 9.2 性能分析工具 / Performance Profiling Tools

| 工具 / Tool | 用途 / Purpose | 使用方式 / Usage |
|---|---|---|
| React DevTools Profiler | 组件渲染时间、重渲染原因 / Render time, re-render reasons | 浏览器扩展 / Browser extension |
| `React.Profiler` | 编程式性能测量 / Programmatic perf measurement | 包裹组件 / Wrap component |
| Chrome Performance | 主线程活动、帧率 / Main thread, frame rate | DevTools → Performance |
| `why-did-you-render` | 自动检测不必要重渲染 / Auto-detect unnecessary re-renders | 开发依赖 / Dev dependency |

### 9.3 React.Profiler 编程式测量 / Programmatic Measurement

```tsx
// 开发环境性能监控（生产环境自动禁用）
// Dev environment perf monitoring (auto-disabled in production)
function onRenderCallback(
  id: string, phase: string, actualDuration: number,
  baseDuration: number, startTime: number, commitTime: number,
) {
  if (actualDuration > 16) {  // 超过一帧时间 / Exceeds one frame
    console.warn(`[Perf] ${id} 渲染耗时 ${actualDuration.toFixed(1)}ms`);
  }
}

// 包裹目标组件 / Wrap target component
<React.Profiler id="Sidebar" onRender={onRenderCallback}>
  <Sidebar samples={samples} />
</React.Profiler>
```

### 9.4 本项目性能优化实践 / Project Performance Practices

| 优化技术 / Technique | 应用位置 / Location | 效果 / Effect |
|---|---|---|
| useCallback | App.tsx load() | 避免 useEffect 无限循环 / Prevent infinite loop |
| useMemo | Sidebar 分组计算 | 仅 samples 变化时重新分组 / Regroup only on change |
| useRef | 快捷键 handler | 避免每次渲染重新绑定 / Avoid re-binding each render |
| key 强制重建 | EndpointView | 切换端点时清除旧状态 / Clear stale state on switch |
| Promise.all | 初始加载 | samples + health 并行 / Parallel fetch |
| 条件渲染 | loading/error | 避免无效 DOM 节点 / Avoid unnecessary DOM nodes |

## 10. 设计模式补充 / Additional Design Patterns

### 10.1 状态提升模式 / State Lifting Pattern

```text
本项目状态管理架构 / Project State Management Architecture:

App.tsx (全局状态 / Global state)
  │  samples, view, health, backend, loading, error
  │
  ├──▶ Header (props: health, backend)
  │      └── 无自己的状态 / No own state
  │
  ├──▶ Sidebar (props: samples, view, setView)
  │      └── 局部状态: searchQuery, expandedCategories
  │
  └──▶ EndpointView (props: sample, backend)
         └── 局部状态: requestBody, response, loading

原则：状态尽可能放在最低层级，仅必要时提升
Principle: state at lowest level possible, lift only when necessary
```

### 10.2 组合模式 / Composition Pattern

```tsx
// 本项目使用 children 组合而非 props 传递组件
// This project uses children composition over component props

// ErrorBoundary 包裹任意子树 / ErrorBoundary wraps any subtree
<ErrorBoundary>
  <EndpointView sample={sample} />
</ErrorBoundary>

// I18nProvider 包裹整个应用 / I18nProvider wraps entire app
<I18nProvider>
  <App />
</I18nProvider>
```

### 10.3 条件渲染模式 / Conditional Rendering Patterns

```tsx
// 本项目中的条件渲染层次 / Conditional rendering layers in this project
function App() {
  if (loading) return <LoadingScreen />;       // 全局加载 / Global loading
  if (error) return <ErrorScreen msg={error} />; // 全局错误 / Global error

  return (
    <Layout>
      {view.type === 'overview' && <Overview />}
      {view.type === 'endpoint' && <EndpointView key={view.sample.id} />}
      {/* ... 其他视图 / other views */}
    </Layout>
  );
}
```

## 11. 错误边界与容错 / Error Boundaries & Fault Tolerance

### 11.1 ErrorBoundary 实现原理 / ErrorBoundary Implementation

React 错误边界是类组件，用于捕获子组件树的渲染错误：

```tsx
// 本项目的 ErrorBoundary 实现 / This project's ErrorBoundary implementation
import { Component, type ReactNode, type ErrorInfo } from 'react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;           // 自定义回退 UI / Custom fallback UI
  onError?: (error: Error) => void;  // 错误回调 / Error callback
}

interface State {
  hasError: boolean;
  error: Error | null;
}

class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error): State {
    // 更新 state 使下次渲染显示回退 UI / Update state to show fallback
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // 记录错误日志 / Log error
    console.error('[ErrorBoundary]', error, errorInfo.componentStack);
    this.props.onError?.(error);
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback ?? (
        <div className="p-4 bg-red-50 border border-red-200 rounded-lg">
          <h3 className="text-red-800 font-medium">组件加载失败 / Component failed</h3>
          <p className="text-red-600 text-sm mt-1">{this.state.error?.message}</p>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            className="mt-2 px-3 py-1 bg-red-600 text-white rounded text-sm"
          >
            重试 / Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
```

### 11.2 错误边界布局策略 / Error Boundary Placement Strategy

```text
应用错误边界层次 / Application error boundary hierarchy:

<ErrorBoundary fallback={<AppCrashScreen />}>     ← 应用级 / App-level
  <App>
    <ErrorBoundary fallback={<PanelError />}>     ← 面板级 / Panel-level
      <ResponsePanel />
    </ErrorBoundary>
    <ErrorBoundary fallback={<PanelError />}>     ← 面板级 / Panel-level
      <HistoryPanel />
    </ErrorBoundary>
    <ErrorBoundary>                               ← 组件级 / Component-level
      <LazyLoadedChart />
    </ErrorBoundary>
  </App>
</ErrorBoundary>

设计原则 / Design principles:
- 顶层边界防止白屏 / Top boundary prevents white screen
- 面板级边界隔离故障 / Panel boundary isolates failures
- 懒加载组件必须有边界 / Lazy components must have boundary
- 边界不捕获事件处理错误 / Boundaries don't catch event errors
```

### 11.3 异步错误处理 / Async Error Handling

```tsx
// 异步操作的错误处理模式 / Error handling pattern for async operations
function useApiCall<T>(fetcher: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(false);

  const execute = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fetcher();
      setData(result);
    } catch (err) {
      // 统一错误转换 / Unified error transformation
      setError(err instanceof Error ? err : new Error(String(err)));
    } finally {
      setLoading(false);
    }
  }, [fetcher]);

  return { data, error, loading, execute };
}

// 组件中使用 / Usage in component
function EndpointView() {
  const { data, error, loading, execute } = useApiCall(
    () => api.sendRequest(currentEndpoint)
  );

  if (error) return <ErrorAlert error={error} onRetry={execute} />;
  if (loading) return <Spinner />;
  return <ResponsePanel data={data} />;
}
```

## 12. 状态管理架构 / State Management Architecture

### 12.1 本项目状态分层 / This Project's State Layers

```text
状态分类与存储位置 / State classification & storage:

┌─────────────────────────────────────────────────────────┐
│  服务器状态 / Server state                               │
│  - 端点列表、响应数据 / Endpoint list, response data     │
│  - 存储：useState + 自定义 Hook / In useState + hooks    │
│  - 特点：异步加载、可失效 / Async loaded, invalidatable  │
├─────────────────────────────────────────────────────────┤
│  UI 状态 / UI state                                     │
│  - 当前视图、侧边栏展开 / Current view, sidebar open    │
│  - 存储：useState / In useState                         │
│  - 特点：同步、组件局部 / Sync, component-local          │
├─────────────────────────────────────────────────────────┤
│  共享状态 / Shared state                                 │
│  - 主题、全局配置 / Theme, global config                 │
│  - 存储：Context / In Context                           │
│  - 特点：跨组件、低频更新 / Cross-component, low-freq    │
├─────────────────────────────────────────────────────────┤
│  持久化状态 / Persisted state                            │
│  - 请求历史、用户偏好 / Request history, user prefs      │
│  - 存储：localStorage / In localStorage                 │
│  - 特点：跨会话保持 / Persists across sessions          │
└─────────────────────────────────────────────────────────┘
```

### 12.2 状态提升与组合 / State Lifting & Composition

```tsx
// 本项目的状态提升示例 / State lifting example in this project
function App() {
  // 状态提升到 App 层 / State lifted to App level
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [response, setResponse] = useState<ResponseData | null>(null);

  // 通过 props 下发 / Pass down via props
  return (
    <Layout>
      <Sidebar
        endpoints={endpoints}           // 数据下发 / Data down
        selectedId={selectedId}
        onSelect={setSelectedId}        // 事件上报 / Events up
      />
      <EndpointView
        endpoint={endpoints.find(e => e.id === selectedId)}
        onResponse={setResponse}        // 回调上报 / Callback up
      />
      <ResponsePanel response={response} />
    </Layout>
  );
}
```

### 12.3 状态管理方案对比 / State Management Solution Comparison

| 方案 / Solution | 复杂度 / Complexity | 适用规模 / Scale | 本项目 / This Project |
|---|---|---|---|
| useState | 低 / Low | 组件局部 / Component-local | ✅ 主要 / Primary |
| useReducer | 中 / Medium | 复杂状态转换 / Complex transitions | ✅ 部分 / Partial |
| Context | 中 / Medium | 跨组件共享 / Cross-component | ✅ 主题 / Theme |
| Zustand | 中 / Medium | 中型应用 / Medium apps | 未使用 / Not used |
| Redux Toolkit | 高 / High | 大型应用 / Large apps | 未使用 / Not used |

## 13. 组件通信模式 / Component Communication Patterns

### 13.1 通信方向分类 / Communication Direction Classification

```text
父 → 子 / Parent → Child:  Props 传递 / Props passing
子 → 父 / Child → Parent:  回调函数 / Callback functions
兄 → 弟 / Sibling:          状态提升 / State lifting
跨层级 / Cross-level:       Context / Context API
全局事件 / Global events:   EventEmitter / Custom events
```

### 13.2 本项目的通信实践 / This Project's Communication Practice

```tsx
// 1. Props 下发 + 回调上报 / Props down + Callbacks up
function Sidebar({ endpoints, onSelect }: {
  endpoints: Endpoint[];
  onSelect: (id: string) => void;  // 回调上报 / Callback up
}) {
  return (
    <nav>
      {endpoints.map(ep => (
        <button key={ep.id} onClick={() => onSelect(ep.id)}>
          {ep.name}
        </button>
      ))}
    </nav>
  );
}

// 2. Context 跨层通信 / Context cross-level communication
const ThemeContext = createContext<ThemeContextType>(null!);

function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<'light' | 'dark'>('light');
  const toggle = useCallback(() =>
    setTheme(t => t === 'light' ? 'dark' : 'light'), []);

  return (
    <ThemeContext.Provider value={{ theme, toggle }}>
      {children}
    </ThemeContext.Provider>
  );
}

// 任意深层组件访问 / Any deep component can access
function DeepNestedButton() {
  const { theme, toggle } = useContext(ThemeContext);
  return <button onClick={toggle}>当前: {theme} / Current: {theme}</button>;
}

// 3. 自定义事件（跨模块）/ Custom events (cross-module)
// 用于无父子关系的组件通信 / For unrelated component communication
const eventBus = new EventTarget();

// 发布 / Publish
eventBus.dispatchEvent(new CustomEvent('request:sent', { detail: { id } }));

// 订阅 / Subscribe
useEffect(() => {
  const handler = (e: Event) => {
    const { id } = (e as CustomEvent).detail;
    refreshHistory();
  };
  eventBus.addEventListener('request:sent', handler);
  return () => eventBus.removeEventListener('request:sent', handler);
}, []);
```

### 13.3 通信模式选择指南 / Communication Pattern Selection Guide

| 场景 / Scenario | 推荐模式 / Recommended | 原因 / Reason |
|---|---|---|
| 父传子数据 / Parent→child data | Props | 简单直接 / Simple & direct |
| 子通知父 / Child→parent notify | 回调 Props / Callback props | 保持单向数据流 / Unidirectional flow |
| 兄弟组件 / Sibling components | 状态提升 / State lifting | 共同父级管理 / Common parent manages |
| 深层嵌套 / Deep nesting (3+层) | Context | 避免 prop drilling |
| 无关组件 / Unrelated components | 事件总线 / Event bus | 解耦 / Decoupled |
| 服务器数据 / Server data | 自定义 Hook / Custom hook | 封装异步逻辑 / Encapsulate async |

---

## 14. React Server Components 与渲染架构 / RSC & Rendering Architecture

### 14.1 渲染模式演进 / Rendering Mode Evolution

React 的渲染模式经历了从纯客户端到混合渲染的演进，理解这些模式对架构选型至关重要：

```
┌─────────────────────────────────────────────────────────────────────┐
│                    React 渲染模式演进 / Rendering Evolution           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  CSR (Client-Side Rendering)                                        │
│  ┌───────────────────────────────────────────────────────────┐      │
│  │  Browser: HTML Shell → JS Bundle → Fetch Data → Render    │      │
│  │  优点 / Pros: 简单、交互丰富 / Simple, rich interaction    │      │
│  │  缺点 / Cons: 白屏、SEO差 / White flash, poor SEO         │      │
│  └───────────────────────────────────────────────────────────┘      │
│                          ↓ 演进 / evolves                           │
│  SSR (Server-Side Rendering)                                        │
│  ┌───────────────────────────────────────────────────────────┐      │
│  │  Server: Render HTML → Send to Client → Hydrate → Ready   │      │
│  │  优点 / Pros: 首屏快、SEO好 / Fast FCP, good SEO          │      │
│  │  缺点 / Cons: 服务器负载、TTI慢 / Server load, slow TTI   │      │
│  └───────────────────────────────────────────────────────────┘      │
│                          ↓ 演进 / evolves                           │
│  RSC (React Server Components)                                      │
│  ┌───────────────────────────────────────────────────────────┐      │
│  │  Server: 渲染服务端组件（零JS）/ Render server comps (0 JS)│      │
│  │  Client: 仅接收交互组件 / Only receive interactive comps   │      │
│  │  优点 / Pros: 零bundle增长、直接DB访问 / Zero bundle, DB   │      │
│  │  缺点 / Cons: 框架依赖、心智模型复杂 / Framework, complex  │      │
│  └───────────────────────────────────────────────────────────┘      │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 14.2 Server Components 工作原理 / How RSC Works

Server Components 在服务器上执行，其输出以特殊序列化格式（RSC Payload）发送到客户端：

```tsx
// ===== Server Component（默认）/ Server Component (default) =====
// 文件: app/endpoints/page.tsx (Next.js App Router)
// 特征 / Characteristics:
//   - 不能使用 useState/useEffect 等客户端 Hook
//   - 可以直接访问数据库、文件系统
//   - 输出不包含在 JS bundle 中

import { db } from '@/lib/database';  // 直接导入服务端模块 / Direct server import

// 异步组件 - Server Components 支持 async/await
async function EndpointList() {
  // 直接在组件中查询数据库 / Query DB directly in component
  const endpoints = await db.query('SELECT * FROM endpoints ORDER BY name');

  return (
    <div className="space-y-2">
      {endpoints.map((ep) => (
        <div key={ep.id} className="p-3 border rounded">
          <span className="font-mono">{ep.method} {ep.path}</span>
          {/* 嵌套客户端组件 / Nested client component */}
          <EndpointActions endpointId={ep.id} />
        </div>
      ))}
    </div>
  );
}

// ===== Client Component（需要 'use client' 指令）=====
'use client';

import { useState } from 'react';

function EndpointActions({ endpointId }: { endpointId: string }) {
  const [loading, setLoading] = useState(false);

  const handleTest = async () => {
    setLoading(true);
    await fetch(`/api/endpoints/${endpointId}/test`, { method: 'POST' });
    setLoading(false);
  };

  return (
    <button onClick={handleTest} disabled={loading}>
      {loading ? 'Testing...' : 'Test'}
    </button>
  );
}
```

### 14.3 RSC Payload 与数据流 / RSC Payload & Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│              RSC 数据流 / RSC Data Flow                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Server                        Client                           │
│  ┌──────────────┐              ┌──────────────────────┐        │
│  │ Server Comp  │              │  React Reconciler    │        │
│  │ (async)      │              │                      │        │
│  │              │   RSC        │  ┌────────────────┐  │        │
│  │ DB Query ────┼──Payload────►│  │ Virtual DOM    │  │        │
│  │ File Read    │  (JSON-like) │  │ Diff & Patch   │  │        │
│  │ API Call     │              │  └────────────────┘  │        │
│  └──────────────┘              │                      │        │
│                                │  ┌────────────────┐  │        │
│  ┌──────────────┐              │  │ Client Comps   │  │        │
│  │ Client Comp  │   JS Bundle  │  │ (useState etc) │  │        │
│  │ Source Code ─┼──────────────►│  │ Event Handlers │  │        │
│  └──────────────┘              │  └────────────────┘  │        │
│                                └──────────────────────┘        │
│                                                                 │
│  RSC Payload 示例 / Example:                                    │
│  {                                                              │
│    "type": "div",                                               │
│    "props": {                                                   │
│      "children": [                                              │
│        { "type": "h1", "props": { "children": "Endpoints" } }, │
│        { "type": "$L1", "props": { "endpointId": "ep_123" } }  │
│      ]                                                          │
│    }                                                            │
│  }                                                              │
│  // $L1 引用客户端组件模块 / references client component module │
└─────────────────────────────────────────────────────────────────┘
```

### 14.4 本项目渲染策略分析 / This Project's Rendering Strategy

本项目（Console 测试控制台）选择纯 CSR 的决策分析：

| 考量因素 / Factor | 分析 / Analysis | 决策 / Decision |
|---|---|---|
| 部署环境 / Deployment | 内网本地工具 / Intranet local tool | CSR 足够 / CSR sufficient |
| SEO 需求 / SEO need | 无搜索引擎索引 / No search indexing | 无需 SSR / No SSR needed |
| 交互密度 / Interaction density | 高频表单操作 / High-frequency forms | CSR 最优 / CSR optimal |
| 数据源 / Data source | 后端 REST/gRPC API | 客户端 fetch 即可 / Client fetch OK |
| 首屏要求 / First paint | 内网低延迟 / Low latency intranet | CSR 可接受 / CSR acceptable |
| 团队复杂度 / Team complexity | 小团队维护 / Small team | 避免 SSR 复杂度 / Avoid SSR complexity |

---

## 15. Hooks 高级模式与状态机 / Advanced Hook Patterns & State Machines

### 15.1 useReducer 管理复杂状态 / useReducer for Complex State

当组件状态逻辑复杂（多个相关联的状态变更）时，`useReducer` 比 `useState` 更可维护：

```tsx
// ===== 请求状态机 / Request State Machine =====
// 将异步请求建模为有限状态机 / Model async request as FSM

// 状态定义 / State definition
type RequestState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string; retryCount: number };

// 动作定义 / Action definition
type RequestAction<T> =
  | { type: 'FETCH_START' }
  | { type: 'FETCH_SUCCESS'; payload: T }
  | { type: 'FETCH_ERROR'; error: string }
  | { type: 'RETRY' }
  | { type: 'RESET' };

// 纯函数 Reducer / Pure function reducer
function requestReducer<T>(
  state: RequestState<T>,
  action: RequestAction<T>
): RequestState<T> {
  switch (state.status) {
    case 'idle':
      if (action.type === 'FETCH_START') return { status: 'loading' };
      return state;

    case 'loading':
      if (action.type === 'FETCH_SUCCESS')
        return { status: 'success', data: action.payload };
      if (action.type === 'FETCH_ERROR')
        return { status: 'error', error: action.error, retryCount: 0 };
      return state;

    case 'success':
      if (action.type === 'FETCH_START') return { status: 'loading' };
      if (action.type === 'RESET') return { status: 'idle' };
      return state;

    case 'error':
      if (action.type === 'RETRY')
        return { status: 'loading' };  // 重试时重置为 loading
      if (action.type === 'RESET') return { status: 'idle' };
      return state;

    default:
      return state;
  }
}

// 自定义 Hook 封装 / Custom hook wrapper
function useRequest<T>(fetcher: () => Promise<T>) {
  const [state, dispatch] = useReducer(requestReducer<T>, { status: 'idle' });

  const execute = useCallback(async () => {
    dispatch({ type: 'FETCH_START' });
    try {
      const data = await fetcher();
      dispatch({ type: 'FETCH_SUCCESS', payload: data });
    } catch (err) {
      dispatch({ type: 'FETCH_ERROR', error: (err as Error).message });
    }
  }, [fetcher]);

  const retry = useCallback(() => dispatch({ type: 'RETRY' }), []);
  const reset = useCallback(() => dispatch({ type: 'RESET' }), []);

  return { state, execute, retry, reset };
}

// 使用示例 / Usage example
function EndpointTester() {
  const { state, execute, retry } = useRequest(() =>
    fetch('/api/mask').then(r => r.json())
  );

  return (
    <div>
      <button onClick={execute}>Send Request</button>
      {state.status === 'loading' && <Spinner />}
      {state.status === 'success' && <pre>{JSON.stringify(state.data)}</pre>}
      {state.status === 'error' && (
        <div className="text-red-500">
          {state.error}
          <button onClick={retry}>Retry</button>
        </div>
      )}
    </div>
  );
}
```

### 15.2 useSyncExternalStore 外部状态同步 / External Store Sync

`useSyncExternalStore` 是 React 18 引入的底层 Hook，用于安全地订阅外部数据源（避免 tearing 问题）：

```tsx
// ===== 外部存储订阅 / External Store Subscription =====
import { useSyncExternalStore } from 'react';

// 简单事件总线实现 / Simple event bus implementation
class EventBus {
  private listeners = new Map<string, Set<() => void>>();
  private store = new Map<string, unknown>();

  subscribe(event: string, callback: () => void): () => void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(callback);

    // 返回取消订阅函数 / Return unsubscribe function
    return () => this.listeners.get(event)?.delete(callback);
  }

  emit(event: string, data: unknown) {
    this.store.set(event, data);
    this.listeners.get(event)?.forEach(cb => cb());
  }

  getSnapshot<T>(event: string): T | undefined {
    return this.store.get(event) as T | undefined;
  }
}

const bus = new EventBus();

// 在组件中安全订阅 / Safely subscribe in component
function useBusEvent<T>(event: string): T | undefined {
  return useSyncExternalStore(
    // subscribe: 注册监听器 / Register listener
    (onStoreChange) => bus.subscribe(event, onStoreChange),
    // getSnapshot: 返回当前值（必须是不可变的）/ Return current value (immutable)
    () => bus.getSnapshot<T>(event),
    // getServerSnapshot: SSR 时的快照 / Snapshot for SSR
    () => undefined
  );
}

// 使用 / Usage
function ConnectionStatus() {
  const status = useBusEvent<{ connected: boolean }>('connection');
  return (
    <span className={status?.connected ? 'text-green-500' : 'text-red-500'}>
      {status?.connected ? '● Connected' : '○ Disconnected'}
    </span>
  );
}
```

### 15.3 useTransition 与并发更新 / useTransition & Concurrent Updates

`useTransition` 允许将某些状态更新标记为「非紧急」，让 React 优先处理用户交互：

```tsx
// ===== 并发搜索过滤 / Concurrent Search Filtering =====
import { useState, useTransition, useMemo } from 'react';

function EndpointSearch({ endpoints }: { endpoints: Endpoint[] }) {
  const [query, setQuery] = useState('');           // 紧急：输入框值 / Urgent: input value
  const [filteredList, setFilteredList] = useState(endpoints);  // 非紧急：列表 / Non-urgent
  const [isPending, startTransition] = useTransition();

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;

    // 紧急更新：立即反映输入 / Urgent: reflect input immediately
    setQuery(value);

    // 非紧急更新：可被中断的过滤计算 / Non-urgent: interruptible filtering
    startTransition(() => {
      const filtered = endpoints.filter(ep =>
        ep.path.toLowerCase().includes(value.toLowerCase()) ||
        ep.method.toLowerCase().includes(value.toLowerCase())
      );
      setFilteredList(filtered);
    });
  };

  return (
    <div>
      <input
        value={query}
        onChange={handleChange}
        placeholder="Search endpoints..."
        className="w-full px-3 py-2 border rounded"
      />
      {/* 过渡期间显示加载指示 / Show indicator during transition */}
      <div className={isPending ? 'opacity-50' : 'opacity-100'}>
        {filteredList.map(ep => (
          <EndpointRow key={ep.id} endpoint={ep} />
        ))}
      </div>
    </div>
  );
}
```

### 15.4 Hooks 组合模式总结 / Hook Composition Patterns Summary

| 模式 / Pattern | 适用场景 / Use Case | 核心 Hook / Core Hook | 本项目应用 / Project Usage |
|---|---|---|---|
| 状态机 / State machine | 多状态流转 / Multi-state flow | useReducer | 请求生命周期 / Request lifecycle |
| 外部订阅 / External sub | 全局事件 / Global events | useSyncExternalStore | WebSocket 状态 / WS status |
| 并发过渡 / Concurrent | 大列表过滤 / Large list filter | useTransition | 端点搜索 / Endpoint search |
| 乐观更新 / Optimistic | 表单提交 / Form submit | useOptimistic (React 19) | 未使用 / Not used |
| 动作绑定 / Action binding | 异步操作 / Async ops | useActionState (React 19) | 未使用 / Not used |

---

## 16. 组件测试策略与模式 / Component Testing Strategy & Patterns

### 16.1 测试金字塔与 React / Testing Pyramid & React

```
┌─────────────────────────────────────────────────────────────────┐
│              React 测试金字塔 / React Testing Pyramid            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│                    ╱╲                                           │
│                   ╱ E2E ╲         少量 / Few                    │
│                  ╱ (Playwright) ╲  完整用户流程 / Full flows     │
│                 ╱────────────────╲                               │
│                ╱  Integration     ╲   适量 / Moderate            │
│               ╱ (Testing Library)  ╲  组件交互 / Component UX    │
│              ╱──────────────────────╲                            │
│             ╱      Unit Tests        ╲  大量 / Many              │
│            ╱    (Vitest + RTL)        ╲ 逻辑验证 / Logic verify  │
│           ╱────────────────────────────╲                         │
│                                                                 │
│  本项目策略 / This project's strategy:                           │
│  - 重点: 组件集成测试（用户行为驱动）/ Focus: integration (UX)    │
│  - 辅助: 工具函数单元测试 / Support: util unit tests             │
│  - 工具: Vitest + @testing-library/react / Tools                 │
└─────────────────────────────────────────────────────────────────┘
```

### 16.2 组件测试实战 / Component Testing in Practice

```tsx
// ===== 测试文件: EndpointView.test.tsx =====
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { EndpointView } from '../components/EndpointView';

// Mock API 模块 / Mock API module
vi.mock('../api/client', () => ({
  sendRequest: vi.fn(),
}));

import { sendRequest } from '../api/client';
const mockSendRequest = vi.mocked(sendRequest);

describe('EndpointView', () => {
  const mockEndpoint = {
    id: 'mask',
    name: 'Data Masking',
    method: 'POST' as const,
    path: '/api/mask',
    description: 'Mask sensitive fields',
    sampleBody: { data: { email: 'test@example.com' } },
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('渲染端点信息 / renders endpoint info', () => {
    render(<EndpointView endpoint={mockEndpoint} />);

    // 按用户可见文本查询 / Query by visible text
    expect(screen.getByText('Data Masking')).toBeInTheDocument();
    expect(screen.getByText('POST')).toBeInTheDocument();
    expect(screen.getByText('/api/mask')).toBeInTheDocument();
  });

  it('发送请求并显示响应 / sends request and shows response', async () => {
    const mockResponse = {
      status: 200,
      data: { masked: { email: 't***@example.com' } },
      duration: 42,
    };
    mockSendRequest.mockResolvedValue(mockResponse);

    render(<EndpointView endpoint={mockEndpoint} />);

    // 模拟用户点击 / Simulate user click
    fireEvent.click(screen.getByRole('button', { name: /send/i }));

    // 等待异步完成 / Wait for async completion
    await waitFor(() => {
      expect(screen.getByText(/200/)).toBeInTheDocument();
    });

    // 验证 API 调用参数 / Verify API call params
    expect(mockSendRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        method: 'POST',
        path: '/api/mask',
      })
    );
  });

  it('处理请求错误 / handles request error', async () => {
    mockSendRequest.mockRejectedValue(new Error('Network error'));

    render(<EndpointView endpoint={mockEndpoint} />);
    fireEvent.click(screen.getByRole('button', { name: /send/i }));

    await waitFor(() => {
      expect(screen.getByText(/network error/i)).toBeInTheDocument();
    });
  });
});
```

### 16.3 自定义 Hook 测试 / Custom Hook Testing

```tsx
// ===== 使用 renderHook 测试自定义 Hook / Test custom hooks =====
import { renderHook, act } from '@testing-library/react';
import { useRequestHistory } from '../hooks/useRequestHistory';

describe('useRequestHistory', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('添加和获取历史记录 / adds and retrieves history', () => {
    const { result } = renderHook(() => useRequestHistory());

    // 使用 act 包裹状态更新 / Wrap state updates in act
    act(() => {
      result.current.addEntry({
        method: 'POST',
        path: '/api/mask',
        status: 200,
        duration: 35,
      });
    });

    expect(result.current.entries).toHaveLength(1);
    expect(result.current.entries[0]).toMatchObject({
      method: 'POST',
      path: '/api/mask',
    });
  });

  it('限制最大历史条数 / limits max history entries', () => {
    const { result } = renderHook(() => useRequestHistory(5));

    // 添加超过限制的条目 / Add entries exceeding limit
    act(() => {
      for (let i = 0; i < 8; i++) {
        result.current.addEntry({
          method: 'GET',
          path: `/api/test/${i}`,
          status: 200,
          duration: 10 + i,
        });
      }
    });

    // 应只保留最新 5 条 / Should keep only latest 5
    expect(result.current.entries).toHaveLength(5);
    expect(result.current.entries[0].path).toBe('/api/test/7');
  });

  it('持久化到 localStorage / persists to localStorage', () => {
    const { result } = renderHook(() => useRequestHistory());

    act(() => {
      result.current.addEntry({
        method: 'POST',
        path: '/api/dp',
        status: 200,
        duration: 50,
      });
    });

    // 重新渲染 Hook 应从 localStorage 恢复 / Re-render should restore
    const { result: result2 } = renderHook(() => useRequestHistory());
    expect(result2.current.entries).toHaveLength(1);
  });
});
```

### 16.4 测试最佳实践 / Testing Best Practices

| 原则 / Principle | 做法 / Do | 避免 / Avoid |
|---|---|---|
| 查询优先级 / Query priority | `getByRole` > `getByText` > `getByTestId` | 直接查 DOM class |
| 用户视角 / User perspective | 模拟点击、输入 / Simulate click, type | 直接调用组件方法 |
| 异步等待 / Async wait | `waitFor` / `findBy*` | 固定 `setTimeout` |
| Mock 范围 / Mock scope | 仅 mock 网络层 / Only mock network | mock 内部实现细节 |
| 断言方式 / Assertion | 验证可见输出 / Verify visible output | 验证内部 state |
| 测试隔离 / Isolation | `beforeEach` 清理 / Clean in beforeEach | 测试间共享可变状态 |
| 快照测试 / Snapshot | 仅用于稳定 UI / Only for stable UI | 频繁变化的组件 |

## 17. 并发渲染与 Transition / Concurrent Rendering & Transitions

### 17.1 useTransition 实战 / useTransition in Practice

```tsx
// 非阻塞搜索：输入不卡顿，结果延迟更新
// Non-blocking search: input stays responsive, results update deferred
import { useState, useTransition, useMemo } from 'react'

function PrivacyFieldSearch({ allFields }: { allFields: Field[] }) {
  const [query, setQuery] = useState('')
  const [isPending, startTransition] = useTransition()
  const [results, setResults] = useState<Field[]>([])
  
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    // 紧急更新：输入框立即响应
    // Urgent update: input responds immediately
    setQuery(value)
    
    // 非紧急更新：搜索结果延迟计算
    // Non-urgent update: search results deferred
    startTransition(() => {
      const filtered = allFields.filter(f =>
        f.name.toLowerCase().includes(value.toLowerCase())
      )
      setResults(filtered)
    })
  }
  
  return (
    <div>
      <input
        value={query}
        onChange={handleChange}
        placeholder="搜索字段... / Search fields..."
        className="w-full rounded border px-3 py-2"
      />
      {isPending && <span className="text-sm text-gray-400">搜索中... / Searching...</span>}
      <ul>
        {results.map(f => (
          <li key={f.id}>{f.name} - {f.classification}</li>
        ))}
      </ul>
    </div>
  )
}
```

### 17.2 useDeferredValue 与优先级 / useDeferredValue & Priority

```tsx
// 大列表渲染优化：输入优先，列表延迟
// Large list rendering: input priority, list deferred
import { useDeferredValue, useMemo } from 'react'

function ClassificationTable({ fields, filter }: {
  fields: ClassificationField[]
  filter: string
}) {
  // 延迟值：低优先级渲染
  // Deferred value: low priority rendering
  const deferredFilter = useDeferredValue(filter)
  const isStale = filter !== deferredFilter
  
  const filtered = useMemo(() => {
    return fields.filter(f =>
      f.name.includes(deferredFilter) ||
      f.classification.includes(deferredFilter)
    )
  }, [fields, deferredFilter])
  
  return (
    <div className={isStale ? 'opacity-50 transition-opacity' : ''}>
      <table>
        <tbody>
          {filtered.map(f => (
            <tr key={f.id}>
              <td>{f.name}</td>
              <td>{f.classification}</td>
              <td>{f.confidence}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

### 17.3 并发特性对比 / Concurrent Features Comparison

| Hook | 优先级 / Priority | 用途 / Use Case | 本项目场景 / Project Scenario |
|---|---|---|---|
| useTransition | 低 / Low | 触发非紧急更新 / Trigger non-urgent | 搜索过滤 / Search filter |
| useDeferredValue | 低 / Low | 延迟值渲染 / Defer value render | 大表格过滤 / Large table filter |
| Suspense | - | 等待异步 / Wait for async | 数据加载 / Data loading |
| startTransition | 低 / Low | 包裹状态更新 / Wrap state update | 页面切换 / Page switch |

## 18. 表单架构与数据流 / Form Architecture & Data Flow

### 18.1 受控表单模式 / Controlled Form Pattern

```tsx
// 类型安全的表单 Hook
// Type-safe form Hook
import { useState, useCallback } from 'react'

interface MaskFormValues {
  strategy: 'full' | 'partial' | 'hash'
  fields: string[]
  customPattern: string
  preserveLength: boolean
}

const initialMaskForm: MaskFormValues = {
  strategy: 'partial',
  fields: [],
  customPattern: '',
  preserveLength: true,
}

function useMaskForm() {
  const [values, setValues] = useState<MaskFormValues>(initialMaskForm)
  const [errors, setErrors] = useState<Partial<Record<keyof MaskFormValues, string>>>({})
  const [isSubmitting, setIsSubmitting] = useState(false)
  
  const setField = useCallback(<K extends keyof MaskFormValues>(
    key: K,
    value: MaskFormValues[K]
  ) => {
    setValues(prev => ({ ...prev, [key]: value }))
    // 清除该字段错误
    // Clear field error
    setErrors(prev => ({ ...prev, [key]: undefined }))
  }, [])
  
  const validate = useCallback((): boolean => {
    const newErrors: typeof errors = {}
    if (values.fields.length === 0) {
      newErrors.fields = '至少选择一个字段 / Select at least one field'
    }
    if (values.strategy === 'hash' && !values.customPattern) {
      newErrors.customPattern = '哈希模式需要自定义模式 / Hash needs pattern'
    }
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }, [values])
  
  const submit = useCallback(async (onSuccess: (data: MaskFormValues) => void) => {
    if (!validate()) return
    setIsSubmitting(true)
    try {
      await onSuccess(values)
    } finally {
      setIsSubmitting(false)
    }
  }, [values, validate])
  
  return { values, errors, isSubmitting, setField, submit }
}
```

### 18.2 表单组件组合 / Form Component Composition

```tsx
// 可复用表单字段组件
// Reusable form field component
function FormField({ label, error, children }: {
  label: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
        {label}
      </label>
      {children}
      {error && (
        <p className="text-xs text-red-500 animate-shake">{error}</p>
      )}
    </div>
  )
}

// 组装表单
// Assemble form
function MaskConfigForm() {
  const { values, errors, isSubmitting, setField, submit } = useMaskForm()
  
  return (
    <form onSubmit={(e) => { e.preventDefault(); submit(handleMask) }} className="space-y-4">
      <FormField label="脱敏策略 / Strategy" error={errors.strategy}>
        <select
          value={values.strategy}
          onChange={e => setField('strategy', e.target.value as any)}
          className="w-full rounded border px-3 py-2"
        >
          <option value="partial">部分遮盖 / Partial</option>
          <option value="full">完全遮盖 / Full</option>
          <option value="hash">哈希 / Hash</option>
        </select>
      </FormField>
      
      <FormField label="保留长度 / Preserve Length">
        <input
          type="checkbox"
          checked={values.preserveLength}
          onChange={e => setField('preserveLength', e.target.checked)}
        />
      </FormField>
      
      <button type="submit" disabled={isSubmitting}>
        {isSubmitting ? '处理中... / Processing...' : '执行脱敏 / Apply Mask'}
      </button>
    </form>
  )
}
```

## 19. 应用架构与代码组织 / App Architecture & Code Organization

### 19.1 功能模块划分 / Feature Module Structure

```
src/
├── app/                        # 应用层 / App layer
│   ├── App.tsx                 # 根组件 / Root component
│   ├── Router.tsx              # 路由配置 / Route config
│   └── providers.tsx           # Provider 组装 / Provider assembly
├── features/                   # 功能模块 / Feature modules
│   ├── masking/                # 脱敏功能 / Masking feature
│   │   ├── MaskPage.tsx        # 页面 / Page
│   │   ├── MaskForm.tsx        # 表单 / Form
│   │   ├── MaskResult.tsx      # 结果展示 / Result display
│   │   ├── useMaskApi.ts       # API Hook
│   │   └── types.ts            # 类型 / Types
│   ├── dp/                     # 差分隐私 / DP feature
│   │   ├── DPPage.tsx
│   │   ├── DPConfigForm.tsx
│   │   └── useDpApi.ts
│   └── classification/         # 分类功能 / Classification
│       ├── ClassifyPage.tsx
│       ├── FieldTable.tsx
│       └── useClassifyApi.ts
├── shared/                     # 共享层 / Shared layer
│   ├── components/ui/          # UI 组件库 / UI components
│   ├── hooks/                  # 通用 Hooks
│   ├── lib/                    # 工具函数 / Utilities
│   └── types/                  # 全局类型 / Global types
└── services/                   # 服务层 / Service layer
    └── api.ts                  # HTTP 客户端 / HTTP client
```

### 19.2 数据流架构 / Data Flow Architecture

```
┌─────────────────────────────────────────────────────────┐
│  UI Layer (Components)                                  │
│  • 纯展示 + 事件触发 / Pure display + event trigger    │
├─────────────────────────────────────────────────────────┤
│  Hook Layer (useXxxApi)                                 │
│  • 状态管理 + 副作用 / State + side effects            │
│  • 加载/错误状态 / Loading/error states                │
├─────────────────────────────────────────────────────────┤
│  Service Layer (api.ts)                                 │
│  • HTTP 请求封装 / HTTP request wrapper                │
│  • 错误转换 / Error transformation                     │
├─────────────────────────────────────────────────────────┤
│  Backend (Console Backend / Go Proxy)                   │
│  • 路由转发 / Route forwarding                         │
│  • 协议转换 / Protocol conversion                      │
└─────────────────────────────────────────────────────────┘
```

### 19.3 架构原则总结 / Architecture Principles Summary

| 原则 / Principle | 实践 / Practice | 原因 / Reason |
|---|---|---|
| 功能内聚 / Feature cohesion | 按功能分目录 / By feature dir | 可维护 / Maintainable |
| 单向数据流 / Unidirectional flow | UI → Hook → Service | 可预测 / Predictable |
| 类型安全 / Type safety | 全链路 TS | 早发现问题 / Catch early |
| 组件纯化 / Component purity | UI 无副作用 / No side effects | 可测试 / Testable |
| 懒加载 / Lazy loading | 路由级分割 / Route-level split | 首屏快 / Fast FCP |
