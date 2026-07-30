# TypeScript 技术栈说明 / TypeScript Technology Stack

## 1. 技术简介 / Introduction

TypeScript 是由 Microsoft 开发的开源编程语言，是 JavaScript 的超集，添加了静态类型系统与面向对象特性。
TypeScript is an open-source programming language developed by Microsoft, a superset of JavaScript that adds static typing and object-oriented features.

核心特性 / Core Features：
- **静态类型检查（Static Type Checking）**：编译期捕获类型错误，减少运行时异常。
- **类型推断（Type Inference）**：无需显式标注所有类型，编译器自动推断。
- **接口与泛型（Interfaces & Generics）**：定义数据契约与可复用组件。
- **ES Module 支持**：原生支持 `import/export` 模块化。
- **与 JavaScript 完全兼容**：所有合法 JS 代码都是合法 TS 代码。

本项目使用版本 / Version Used：`TypeScript ^5.7.3`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 类型定义文件 / Type Definition File

文件 / File：`console/web/src/types/api.ts`

本文件是前后端的"单一事实来源"（Single Source of Truth），定义了所有 API 交互的数据结构：
This file is the "Single Source of Truth" for frontend-backend API interactions:

```typescript
// 通用代理请求体（发往 /api/proxy）
// Generic proxy request body (sent to /api/proxy)
export interface ProxyRequest {
  method: string;
  path: string;
  body?: Record<string, any> | null;
  raw_payload_b64?: string | null;
  content_type?: string | null;
}

// 通用代理统一响应包装
// Generic proxy unified response wrapper
export interface ProxyResponse {
  status: number;
  duration_ms: number;
  data: any;
  via?: string;      // 后端标识 / Backend identifier
  protocol?: string; // 通信协议 / Communication protocol
}
```

### 2.2 泛型请求封装 / Generic Request Wrapper

文件 / File：`console/web/src/api/client.ts`

```typescript
// 统一请求入口：泛型 T 确保返回值类型安全
// Unified request entry: generic T ensures type-safe return values
async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  // ... 超时控制、鉴权头附加、错误处理
  return JSON.parse(text) as T;
}

// 调用时指定具体类型，获得完整的类型提示
// Specify concrete type at call site for full type hints
export async function fetchHealth(): Promise<ConsoleHealth> {
  return request<ConsoleHealth>('/api/health');
}
```

### 2.3 联合类型视图切换 / Union Type View Switching

文件 / File：`console/web/src/App.tsx`

```typescript
// 使用可辨识联合（Discriminated Union）实现类型安全的视图切换
// Use discriminated union for type-safe view switching
type View =
  | { type: 'overview' }
  | { type: 'endpoint'; sample: EndpointSample }
  | { type: 'batch' }
  | { type: 'filetest' }
  | { type: 'lbtest' }
  | { type: 'dynclassification' };
```

### 2.4 TypeScript 配置详解 / TypeScript Configuration Details

文件 / File：`console/web/tsconfig.json`

```jsonc
{
  "compilerOptions": {
    // === 编译目标与模块系统 / Compile Target & Module System ===
    "target": "ES2020",              // 编译目标（支持 ?. ?? BigInt 等）
    "module": "ESNext",              // 模块系统（Vite 原生 ESM）
    "moduleResolution": "bundler",   // bundler 模式（Vite 推荐，支持 package.json exports）
    "lib": ["ES2020", "DOM", "DOM.Iterable"], // 类型库：ES2020 + 浏览器 DOM

    // === JSX 与 React / JSX & React ===
    "jsx": "react-jsx",              // React 17+ 自动 JSX 转换（无需 import React）
    "useDefineForClassFields": true, // 标准 class field 语义

    // === 严格类型检查 / Strict Type Checking ===
    "strict": true,                  // 启用所有严格检查（见下表）
    "noUnusedLocals": true,          // 禁止未使用的局部变量
    "noUnusedParameters": true,      // 禁止未使用的函数参数
    "noFallthroughCasesInSwitch": true, // 禁止 switch 穿透

    // === 模块解析 / Module Resolution ===
    "baseUrl": ".",                  // 路径解析基准目录
    "paths": { "@/*": ["src/*"] },   // '@' 路径别名映射到 src/
    "allowImportingTsExtensions": true, // 允许导入带 .ts 扩展名的文件
    "resolveJsonModule": true,       // 允许导入 JSON 文件

    // === 构建与输出 / Build & Output ===
    "noEmit": true,                  // 不输出编译结果（仅类型检查）
    "isolatedModules": true,         // 确保每个文件可独立编译（Vite 要求）
    "skipLibCheck": true             // 跳过 .d.ts 检查（加速编译）
  },
  "include": ["src"],                // 仅编译 src 目录
  "references": [{ "path": "./tsconfig.node.json" }] // 引用 Node 端配置
}
```

**`strict: true` 包含的检查项 / Checks included in `strict: true`**：

| 检查项 / Check | 作用 / Purpose | 示例 / Example |
|---|---|---|
| `strictNullChecks` | null/undefined 不可赋给其他类型 | `const x: string = null` → 报错 |
| `strictFunctionTypes` | 函数参数类型逆变检查 | 防止不安全的函数赋值 |
| `strictBindCallApply` | bind/call/apply 参数类型检查 | `fn.call(null, wrongArg)` → 报错 |
| `strictPropertyInitialization` | 类属性必须初始化 | 未初始化的属性 → 报错 |
| `noImplicitAny` | 禁止隐式 any 类型 | 未标注类型的参数 → 报错 |
| `noImplicitThis` | 禁止隐式 this 类型 | 函数中未绑定的 this → 报错 |
| `alwaysStrict` | 输出 "use strict" | 所有文件严格模式 |

**`tsconfig.node.json`（Node 端配置）/ Node-side Configuration**：

```jsonc
// 用于 vite.config.ts 等 Node 端脚本的类型检查
// Type checking for Node-side scripts like vite.config.ts
{
  "compilerOptions": {
    "composite": true,     // 允许被其他 tsconfig 引用
    "module": "ESNext",    // Node 端也使用 ESM
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]  // 仅包含 Vite 配置文件
}
```

### 2.5 路径别名机制 / Path Alias Mechanism

TypeScript 的 `paths` 配置与 Vite 的 `resolve.alias` 配合工作：

```text
┌─────────────────────────────────────────────────────────────┐
│  源代码 / Source Code                                        │
│  import { Header } from '@/components/Header'               │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        ▼                                     ▼
┌───────────────────────┐         ┌───────────────────────┐
│  TypeScript 类型检查   │         │  Vite 构建解析         │
│  tsconfig.json paths  │         │  vite.config.ts alias │
│  "@/*" → "src/*"      │         │  '@' → './src'        │
│  （仅用于类型推断）    │         │  （实际模块解析）      │
└───────────────────────┘         └───────────────────────┘
```

**注意**：TypeScript 的 `paths` 仅用于类型检查，不影响运行时模块解析。Vite 的 `resolve.alias` 才是实际打包时的路径映射。两者必须保持一致。

### 2.6 类型工具与模式 / Type Utilities & Patterns

本项目中使用的 TypeScript 高级类型特性：

```typescript
// 1. 泛型约束：确保 API 客户端返回类型安全
// Generic constraints: ensure API client returns are type-safe
async function request<T>(path: string, init?: RequestInit): Promise<T>

// 2. 可辨识联合：类型安全的视图切换
// Discriminated union: type-safe view switching
type View =
  | { type: 'overview' }
  | { type: 'endpoint'; sample: EndpointSample }
  | { type: 'batch' }
  | { type: 'filetest' }
  | { type: 'lbtest' }
  | { type: 'dynclassification' };

// 3. 可选属性与空值合并：处理可选 API 字段
// Optional properties & nullish coalescing: handle optional API fields
interface ProxyResponse {
  via?: string;      // 可选：后端标识
  protocol?: string; // 可选：通信协议
}
const via = response.via ?? 'unknown';  // 空值合并运算符

// 4. Record 类型：动态键值对
// Record type: dynamic key-value pairs
body?: Record<string, any> | null;

// 5. 类型断言：解析 JSON 后断言为具体类型
// Type assertion: assert parsed JSON to concrete type
return JSON.parse(text) as T;
```

### 2.7 类型安全收益 / Type Safety Benefits

| 场景 / Scenario | 类型系统作用 / Type System Role |
|---|---|
| API 响应解析 | 接口定义确保字段名拼写正确 / Interface ensures correct field names |
| 组件 Props | 编译期检查必传属性 / Compile-time check for required props |
| 视图切换 | 联合类型穷举所有可能 / Union type exhausts all possibilities |
| 重构安全 | 修改接口定义时编译器标出所有受影响代码 / Compiler flags all affected code |

### 2.8 类型收窄与判别联合 / Type Narrowing & Discriminated Unions

本项目中视图切换使用可判别联合，TypeScript 自动收窄类型：

```typescript
// App.tsx 中的视图渲染逻辑 / View rendering logic in App.tsx
type View =
  | { type: 'overview' }
  | { type: 'endpoint'; sample: EndpointSample }
  | { type: 'batch' }
  | { type: 'filetest' }
  | { type: 'lbtest' }
  | { type: 'dynclassification' };

function renderView(view: View) {
  switch (view.type) {
    case 'overview':
      // TypeScript 知道此处 view 是 { type: 'overview' }
      return <Overview />;
    case 'endpoint':
      // TypeScript 自动收窄：view.sample 可安全访问
      // TypeScript auto-narrows: view.sample is safely accessible
      return <EndpointView sample={view.sample} />;
    case 'batch':
      return <BatchTest />;
    // ... 其他 case
  }
}
```

**类型收窄的常见方式 / Common Type Narrowing Methods**：

| 收窄方式 / Method | 示例 / Example | 适用场景 / Use Case |
|---|---|---|
| 判别属性 / Discriminant | `if (view.type === 'endpoint')` | 联合类型切换 / Union switching |
| typeof | `if (typeof x === 'string')` | 原始类型判断 / Primitive check |
| instanceof | `if (err instanceof Error)` | 类实例判断 / Class instance check |
| in 操作符 / in operator | `if ('sample' in view)` | 属性存在性 / Property existence |
| 自定义守卫 / Custom guard | `function isEndpoint(v): v is ...` | 复杂条件 / Complex conditions |

### 2.9 接口设计与前后端契约 / Interface Design & Frontend-Backend Contract

本项目 `types/api.ts` 定义了 17 个接口，与后端 Pydantic 模型一一对应：

```text
┌─────────────────────────────────────────────────────────────┐
│  前端 TypeScript 接口 / Frontend TypeScript Interfaces        │
│  console/web/src/types/api.ts                               │
├─────────────────────────────────────────────────────────────┤
│  EndpointSample    ←→  后端 samples.py 中的示例结构          │
│  ProxyRequest      ←→  后端 routers/proxy.py 请求模型       │
│  ProxyResponse     ←→  后端统一响应包装                     │
│  ConsoleHealth     ←→  后端 /api/health 响应                │
│  BatchRequestItem  ←→  后端批量测试请求项                   │
│  BatchResponse     ←→  后端批量测试响应                     │
│  UploadResponse    ←→  后端文件上传响应                     │
│  LbTestRequest     ←→  后端负载均衡测试请求                 │
│  LbTestResponse    ←→  后端负载均衡测试响应                 │
│  HistoryEntry      ←→  前端 localStorage 结构               │
└─────────────────────────────────────────────────────────────┘
```

**契约维护规则 / Contract Maintenance Rules**：

| 规则 / Rule | 说明 / Description |
|---|---|
| 字段命名一致 / Consistent naming | 后端 snake_case，前端接口也用 snake_case（代理透传）/ Both use snake_case (proxy passthrough) |
| 可选字段用 `?` / Optional with `?` | 对应后端 Pydantic `Optional[...]` / Maps to Pydantic Optional |
| 修改同步 / Sync changes | 改后端模型时必须同步更新 api.ts / Must update api.ts when changing backend models |
| 类型而非 any / Types over any | `data: any` 仅用于动态代理响应 / `data: any` only for dynamic proxy responses |

### 2.10 编译性能与工程化 / Compilation Performance & Engineering

**本项目 TypeScript 编译优化策略 / Project's TS Compilation Optimization**：

| 配置 / Config | 作用 / Effect |
|---|---|
| `noEmit: true` | 仅类型检查，不输出 JS（Vite 负责编译）/ Type check only, no JS output |
| `skipLibCheck: true` | 跳过 node_modules 中 .d.ts 检查 / Skip .d.ts in node_modules |
| `isolatedModules: true` | 确保每个文件可独立编译（esbuild 要求）/ Each file independently compilable |
| `incremental`（隐含）| Vite 内部缓存编译结果 / Vite caches compilation internally |

**编译流程 / Compilation Flow**：

```text
┌─────────────────────────────────────────────────────────────┐
│  开发模式 / Development Mode                                  │
│                                                             │
│  Vite Dev Server                                            │
│    └─ esbuild 转换 TS → JS（单文件，无类型检查）          │
│       └─ 速度极快（~1ms/文件）但无类型安全保证          │
│                                                             │
│  类型检查（可选）/ Type Check (optional)                      │
│    └─ pnpm tsc --noEmit（全量检查，~2-5s）                 │
├─────────────────────────────────────────────────────────────┤
│  生产构建 / Production Build                                  │
│                                                             │
│  pnpm build = tsc && vite build                             │
│    ├─ tsc：全量类型检查（失败则中断构建）                  │
│    └─ vite build：Rollup 打包 + esbuild 压缩               │
└─────────────────────────────────────────────────────────────┘
```

### 2.11 常见类型模式与最佳实践 / Common Type Patterns & Best Practices

```typescript
// 1. 字面量类型联合：限制可选值 / Literal union: restrict allowed values
export type FileOperation = 'mask_dataframe' | 'k_anonymize';
export type LbStrategy = 'round_robin' | 'random' | 'least_connections';
export type BackendType = 'rest' | 'grpc' | 'both';

// 2. 泛型函数：类型安全的 API 客户端 / Generic function: type-safe API client
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  return JSON.parse(await res.text()) as T;
}
// 调用时自动推断返回类型 / Return type auto-inferred at call site
const health = await request<ConsoleHealth>('/api/health');
health.backend; // ✅ 类型安全 / Type-safe
health.nonexist; // ❌ 编译错误 / Compile error

// 3. Record 类型：动态键值对 / Record type: dynamic key-value
body?: Record<string, any> | null;
// 等价于 / Equivalent to: { [key: string]: any } | null

// 4. 空值合并与可选链 / Nullish coalescing & optional chaining
const via = response.via ?? 'unknown';       // null/undefined → 'unknown'
const latency = health.latency_ms?.toFixed(1); // undefined 安全 / undefined-safe

// 5. 类型断言（谨慎使用）/ Type assertion (use cautiously)
const data = JSON.parse(text) as ProxyResponse;
// 注意：as 不做运行时检查，仅告诉编译器“信任我”
// Note: as does no runtime check, just tells compiler "trust me"
```

### 2.12 TypeScript 与生态系统协作 / TypeScript & Ecosystem Collaboration

| 工具 / Tool | TS 集成方式 / Integration | 作用 / Role |
|---|---|---|
| Vite | esbuild 转换 + tsc 检查 | 开发时快速编译，构建时类型安全 |
| ESLint | typescript-eslint 插件 | 类型感知规则（no-explicit-any）|
| Vitest | 原生 TS 支持 | 测试文件无需额外配置 |
| React | @types/react | JSX 元素类型检查 |
| Tailwind | 无直接集成 | 类名为字符串，无类型检查 |

### 2.13 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| `strict: true` 全量开启 | 从项目开始就保证类型安全，避免后补 / Type safety from day one |
| 接口用 snake_case | 与后端代理透传保持一致，无需转换层 / Consistent with backend proxy passthrough |
| `data: any` 保留 | 代理 30+ 端点，响应结构各异，无法统一定义 / 30+ endpoints with varied responses |
| 无 enum（用字面量联合）| 更轻量、Tree-shaking 友好、无运行时开销 / Lighter, tree-shakeable, no runtime cost |
| `noEmit: true` | Vite/esbuild 负责编译，tsc 仅做检查 / Vite/esbuild compiles, tsc only checks |
| 集中定义 types/api.ts | 单一事实来源，修改一处全局生效 / Single source of truth |

## 3. 声明文件与类型包 / Declaration Files & Type Packages

### 3.1 .d.ts 声明文件作用 / .d.ts Declaration File Purpose

声明文件仅包含类型信息，不包含实现代码，用于为 JS 库提供类型提示：

```text
┌─────────────────────────────────────────────────────────────┐
│  声明文件解析流程 / Declaration File Resolution Flow           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  import React from 'react'                                  │
│       │                                                     │
│       ▼                                                     │
│  TypeScript 查找类型定义的顺序 / TS type lookup order:       │
│    1. node_modules/react/index.d.ts                         │
│    2. node_modules/@types/react/index.d.ts                  │
│    3. tsconfig.json "typeRoots" 配置的目录              │
│    4. 默认 node_modules/@types/ 目录                    │
│                                                             │
│  本项目使用的 @types 包 / @types packages used:             │
│    - @types/react        → React 组件类型                  │
│    - @types/react-dom    → ReactDOM 类型                   │
│                                                             │
│  自带类型的包（无需 @types）/ Packages with built-in types:  │
│    - typescript          → 编译器自带                       │
│    - vitest              → 内置 .d.ts                       │
│    - @testing-library/*  → 内置 .d.ts                       │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 自定义声明文件 / Custom Declaration Files

```typescript
// 为无类型的第三方库创建声明 / Create declaration for untyped library
// src/types/untyped-lib.d.ts
declare module 'some-untyped-lib' {
  export function doSomething(input: string): number;
  export interface Config {
    timeout: number;
    retries: number;
  }
}

// 为静态资源声明类型 / Declare types for static assets
// src/vite-env.d.ts (Vite 自动生成 / Vite auto-generated)
/// <reference types="vite/client" />
// 提供 import.meta.env 类型、静态资源导入类型等
// Provides import.meta.env types, static asset import types, etc.
```

### 3.3 本项目中的类型引用链 / Type Reference Chain in This Project

```text
console/web/
├── src/
│   ├── vite-env.d.ts          ← Vite 环境类型（import.meta.env）
│   ├── types/
│   │   └── api.ts             ← 业务接口定义（17 个 interface）
│   └── test/
│       └── setup.ts           ← vitest + jest-dom 类型增强
├── node_modules/
│   ├── @types/react/          ← React 组件类型
│   ├── @types/react-dom/      ← ReactDOM 类型
│   └── vitest/                ← 内置测试类型
└── tsconfig.json              ← "types": [] 配置控制全局类型
```

## 4. 条件类型与 infer / Conditional Types & infer

### 4.1 条件类型基础 / Conditional Type Basics

条件类型根据类型关系进行分支判断，类似三元表达式：

```typescript
// 基本语法 / Basic syntax
type IsString<T> = T extends string ? true : false;

type A = IsString<'hello'>;  // true
type B = IsString<42>;       // false

// 实际应用：提取 Promise 内部类型 / Practical: extract Promise inner type
type UnwrapPromise<T> = T extends Promise<infer U> ? U : T;

type C = UnwrapPromise<Promise<string>>;  // string
type D = UnwrapPromise<number>;           // number

// 提取函数返回类型 / Extract function return type
type MyReturnType<T> = T extends (...args: any[]) => infer R ? R : never;
type E = MyReturnType<() => Promise<string>>;  // Promise<string>
```

### 4.2 infer 关键字详解 / infer Keyword Details

`infer` 在条件类型中声明待推断的类型变量：

```typescript
// 提取数组元素类型 / Extract array element type
type ElementOf<T> = T extends (infer E)[] ? E : never;
type F = ElementOf<string[]>;   // string
type G = ElementOf<number[][]>; // number[]

// 提取第一个参数类型 / Extract first parameter type
type FirstArg<T> = T extends (first: infer F, ...rest: any[]) => any ? F : never;
type H = FirstArg<(name: string, age: number) => void>;  // string

// 本项目中的应用：API 响应解包 / Project usage: API response unwrap
// request<T> 返回 Promise<T>，调用方获得解包后的 T
// request<T> returns Promise<T>, caller gets unwrapped T
async function request<T>(path: string): Promise<T> { ... }
const health = await request<ConsoleHealth>('/api/health');
// health 的类型是 ConsoleHealth（而非 Promise<ConsoleHealth>）
// health type is ConsoleHealth (not Promise<ConsoleHealth>)
```

### 4.3 分布式条件类型 / Distributive Conditional Types

当条件类型作用于联合类型时，自动对每个成员分别应用：

```typescript
// 分布式行为 / Distributive behavior
type ToArray<T> = T extends any ? T[] : never;

type I = ToArray<string | number>;
// = (string extends any ? string[] : never) | (number extends any ? number[] : never)
// = string[] | number[]  （而非 (string|number)[]）

// 禁止分布（用方括号包裹）/ Prevent distribution (wrap in brackets)
type ToArrayNoDistribute<T> = [T] extends [any] ? T[] : never;
type J = ToArrayNoDistribute<string | number>;  // (string | number)[]
```

## 5. 映射类型与 keyof / Mapped Types & keyof

### 5.1 映射类型基础 / Mapped Types Basics

映射类型通过遍历键集合生成新类型：

```typescript
// 基本语法 / Basic syntax
type MyPartial<T> = {
  [K in keyof T]?: T[K];  // 将所有属性变为可选 / Make all properties optional
};

type MyReadonly<T> = {
  readonly [K in keyof T]: T[K];  // 将所有属性变为只读 / Make all readonly
};

// 实际应用：API 响应处理 / Practical: API response handling
interface ProxyResponse {
  status: number;
  duration_ms: number;
  data: any;
  via?: string;
  protocol?: string;
}

// 生成“所有字段必填”版本 / Generate "all required" version
type StrictResponse = Required<ProxyResponse>;
// via 和 protocol 变为必填 / via and protocol become required

// 生成“仅保留部分字段”版本 / Generate "pick subset" version
type HealthInfo = Pick<ProxyResponse, 'status' | 'duration_ms'>;
// 仅包含 status 和 duration_ms / Only status and duration_ms
```

### 5.2 内置映射工具类型 / Built-in Mapped Utility Types

| 工具类型 / Utility | 作用 / Effect | 示例 / Example |
|---|---|---|
| `Partial<T>` | 所有属性可选 / All optional | 表单初始状态 / Form initial state |
| `Required<T>` | 所有属性必填 / All required | 配置验证后 / After config validation |
| `Readonly<T>` | 所有属性只读 / All readonly | 不可变状态 / Immutable state |
| `Pick<T, K>` | 选取部分属性 / Select subset | 组件 Props 子集 / Component Props subset |
| `Omit<T, K>` | 排除部分属性 / Exclude subset | 去除内部字段 / Remove internal fields |
| `Record<K, V>` | 构造键值对类型 / Build key-value type | 动态对象 / Dynamic objects |
| `Exclude<T, U>` | 从联合类型排除 / Exclude from union | 过滤视图类型 / Filter view types |
| `Extract<T, U>` | 从联合类型提取 / Extract from union | 提取特定视图 / Extract specific view |
| `ReturnType<T>` | 提取函数返回类型 / Extract return type | API 函数结果 / API function result |
| `Parameters<T>` | 提取函数参数元组 / Extract params tuple | 事件处理器 / Event handlers |

### 5.3 本项目中的映射类型应用 / Mapped Types Usage in This Project

```typescript
// Record 类型用于动态请求体 / Record type for dynamic request body
interface ProxyRequest {
  method: string;
  path: string;
  body?: Record<string, any> | null;  // 动态键值对 / Dynamic key-value
}

// 字面量联合代替 enum（映射为 UI 显示）
// Literal union instead of enum (mapped to UI display)
type LbStrategy = 'round_robin' | 'random' | 'least_connections';

// 策略显示名映射 / Strategy display name mapping
const strategyLabels: Record<LbStrategy, string> = {
  round_robin: '轮询',
  random: '随机',
  least_connections: '最少连接',
};
// Record<LbStrategy, string> 确保每个策略都有对应标签
// Record<LbStrategy, string> ensures every strategy has a label
```

## 6. 模板字面量类型 / Template Literal Types

### 6.1 基本用法 / Basic Usage

TypeScript 4.1+ 支持在类型层面使用模板字符串：

```typescript
// 基本拼接 / Basic concatenation
type EventName = `on${Capitalize<'click' | 'focus' | 'blur'>}`;
// = 'onClick' | 'onFocus' | 'onBlur'

// 生成所有组合 / Generate all combinations
type Method = 'GET' | 'POST' | 'PUT' | 'DELETE';
type ApiPath = `/api/${'health' | 'proxy' | 'samples'}`;
type Endpoint = `${Method} ${ApiPath}`;
// = 'GET /api/health' | 'GET /api/proxy' | ... (12 种组合 / 12 combinations)
```

### 6.2 内置字符串操作类型 / Built-in String Manipulation Types

| 类型 / Type | 作用 / Effect | 示例 / Example |
|---|---|---|
| `Uppercase<S>` | 转大写 / To uppercase | `'abc'` → `'ABC'` |
| `Lowercase<S>` | 转小写 / To lowercase | `'ABC'` → `'abc'` |
| `Capitalize<S>` | 首字母大写 / Capitalize first | `'abc'` → `'Abc'` |
| `Uncapitalize<S>` | 首字母小写 / Uncapitalize first | `'Abc'` → `'abc'` |

### 6.3 实际应用场景 / Practical Application Scenarios

```typescript
// 类型安全的事件处理器命名 / Type-safe event handler naming
type Handler<T extends string> = `handle${Capitalize<T>}`;
type ClickHandler = Handler<'click'>;  // 'handleClick'

// 类型安全的 CSS 类名生成 / Type-safe CSS class name generation
type Color = 'red' | 'green' | 'blue';
type Shade = 50 | 100 | 200 | 500;
type TailwindClass = `bg-${Color}-${Shade}`;
// = 'bg-red-50' | 'bg-red-100' | ... (12 种 / 12 combinations)

// 本项目中 Tailwind 类名为字符串，未使用模板字面量类型约束
// This project uses plain strings for Tailwind classes, no template literal constraint
// 原因：Tailwind 类名太多，约束反而降低灵活性
// Reason: too many Tailwind classes, constraint reduces flexibility
```

## 7. 模块增强与全局类型扩展 / Module Augmentation & Global Type Extension

### 7.1 模块增强 / Module Augmentation

```typescript
// 扩展已有模块的类型 / Extend existing module types
// 例如：为 import.meta.env 添加自定义环境变量类型
// Example: add custom env variable types to import.meta.env

/// <reference types="vite/client" />
interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string;
  readonly VITE_API_KEY: string;
  readonly VITE_ENABLE_MOCK: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
```

### 7.2 全局类型扩展 / Global Type Extension

```typescript
// 扩展 Window 对象 / Extend Window object
declare global {
  interface Window {
    __APP_VERSION__: string;
    __BUILD_TIME__: string;
  }
}

// 扩展 vitest 匹配器（由 @testing-library/jest-dom 自动完成）
// Extend vitest matchers (auto-done by @testing-library/jest-dom)
// setup.ts 中 import '@testing-library/jest-dom' 即自动扩展 expect()
// import '@testing-library/jest-dom' in setup.ts auto-extends expect()
```

### 7.3 类型扩展的安全规则 / Safety Rules for Type Extension

| 规则 / Rule | 说明 / Description |
|---|---|
| 仅添加新属性 / Only add new properties | 不可修改已有属性的类型 / Cannot modify existing property types |
| 使用 declare global | 在模块文件中扩展全局类型必须用 / Required for global extension in module files |
| 避免污染 / Avoid pollution | 尽量用模块增强而非全局扩展 / Prefer module augmentation over global |
| 与运行时一致 / Match runtime | 类型扩展必须与运行时实际行为一致 / Must match actual runtime behavior |

## 8. TypeScript 5.x 新特性 / TypeScript 5.x New Features

### 8.1 本项目使用的 TS 5.x 特性 / TS 5.x Features Used in This Project

| 特性 / Feature | 版本 / Version | 本项目应用 / Project Usage |
|---|---|---|
| `const` 类型参数 / const type params | 5.0 | 未使用 / Not used |
| `satisfies` 操作符 | 4.9 | 可用于配置对象验证 / Usable for config validation |
| `moduleResolution: "bundler"` | 5.0 | ✅ tsconfig.json 中使用 |
| 装饰器（Stage 3）/ Decorators | 5.0 | 未使用（React 无需）/ Not used (React doesn't need) |
| `NoInfer<T>` 工具类型 | 5.4 | 未使用 / Not used |

### 8.2 satisfies 操作符详解 / satisfies Operator Details

```typescript
// satisfies 验证表达式满足某类型，但保留更精确的推断类型
// satisfies validates expression meets a type, but keeps more precise inferred type

// 本项目配置对象示例 / Project config object example
const tailwindColors = {
  primary: '#4f46e5',
  secondary: '#6b7280',
  danger: '#dc2626',
} satisfies Record<string, string>;

// 保留精确类型（而非宽泛的 Record<string, string>）
// Keeps precise type (not broad Record<string, string>)
tailwindColors.primary;   // ✅ 类型是 '#4f46e5' 而非 string
tailwindColors.nonexist;  // ❌ 编译错误 / Compile error

// 对比：类型注解会丢失精确性 / Contrast: type annotation loses precision
const colors2: Record<string, string> = { primary: '#4f46e5' };
colors2.primary;   // 类型是 string（而非字面量）/ Type is string (not literal)
colors2.nonexist;  // ✅ 不报错（因为 Record 允许任意键）/ No error (Record allows any key)
```

### 8.3 装饰器支持（了解）/ Decorator Support (FYI)

```typescript
// TypeScript 5.0 支持 TC39 Stage 3 装饰器（无需 experimentalDecorators）
// TypeScript 5.0 supports TC39 Stage 3 decorators (no experimentalDecorators needed)

// 本项目未使用装饰器，因为 React 生态主要用函数组件 + Hooks
// This project doesn't use decorators, as React ecosystem uses function components + Hooks
// 装饰器主要用于 Angular / NestJS / TypeORM 等框架
// Decorators mainly used in Angular / NestJS / TypeORM frameworks

// 示例（仅供了解）/ Example (FYI only):
function logged(originalMethod: any, context: ClassMethodDecoratorContext) {
  const methodName = String(context.name);
  function replacementMethod(this: any, ...args: any[]) {
    console.log(`调用 ${methodName}`);
    return originalMethod.call(this, ...args);
  }
  return replacementMethod;
}

class ApiService {
  @logged
  async fetchData() { /* ... */ }
}
```

## 9. 类型系统进阶模式 / Advanced Type System Patterns

### 9.1 递归类型 / Recursive Types

```typescript
// JSON 值类型（递归定义）/ JSON value type (recursive definition)
type JsonValue =
  | string
  | number
  | boolean
  | null
  | JsonValue[]
  | { [key: string]: JsonValue };

// 本项目中 ProxyResponse.data 为 any，等价于 JsonValue
// ProxyResponse.data is any in this project, equivalent to JsonValue
// 使用 any 是因为 30+ 端点响应结构各异，无法统一定义
// Using any because 30+ endpoints have varied response structures
```

### 9.2 类型编程实战 / Type Programming in Practice

```typescript
// 深度只读（递归）/ Deep readonly (recursive)
type DeepReadonly<T> = {
  readonly [K in keyof T]: T[K] extends object ? DeepReadonly<T[K]> : T[K];
};

// 提取对象中所有函数属性 / Extract all function properties from object
type FunctionKeys<T> = {
  [K in keyof T]: T[K] extends (...args: any[]) => any ? K : never;
}[keyof T];

// 可选属性提取 / Extract optional properties
type OptionalKeys<T> = {
  [K in keyof T]-?: {} extends Pick<T, K> ? K : never;
}[keyof T];

// 应用：查看 ProxyResponse 的可选字段 / Usage: see optional fields of ProxyResponse
type OptionalFields = OptionalKeys<ProxyResponse>;  // 'via' | 'protocol'
```

### 9.3 类型安全事件系统 / Type-safe Event System

```typescript
// 类型安全的事件发射器 / Type-safe event emitter
interface EventMap {
  'health:updated': { backend: string; agent: string };
  'request:sent': { path: string; method: string };
  'request:completed': { path: string; duration_ms: number };
  'error': { message: string; code: number };
}

class TypedEmitter<T extends Record<string, any>> {
  on<K extends keyof T>(event: K, handler: (payload: T[K]) => void): void { /*...*/ }
  emit<K extends keyof T>(event: K, payload: T[K]): void { /*...*/ }
}

const emitter = new TypedEmitter<EventMap>();
emitter.on('health:updated', (data) => {
  data.backend;  // ✅ 类型安全 / Type-safe
  data.unknown;  // ❌ 编译错误 / Compile error
});
```

## 10. 泛型约束与高级泛型 / Generic Constraints & Advanced Generics

### 10.1 泛型约束基础 / Generic Constraint Basics

```typescript
// extends 约束泛型边界 / extends constrains generic bounds

// 1. 基本约束 / Basic constraint
function getProperty<T, K extends keyof T>(obj: T, key: K): T[K] {
  return obj[key];  // 类型安全的属性访问 / Type-safe property access
}

const endpoint = { id: '1', method: 'POST', path: '/mask' };
getProperty(endpoint, 'method');   // ✅ string
getProperty(endpoint, 'unknown');  // ❌ 编译错误 / Compile error

// 2. 多约束 / Multiple constraints
interface HasId { id: string }
interface HasTimestamp { createdAt: number }

function merge<T extends HasId & HasTimestamp>(a: T, b: Partial<T>): T {
  return { ...a, ...b };
}

// 3. 条件泛型 / Conditional generics
type ApiResponse<T> = T extends 'list'
  ? { data: Endpoint[]; total: number }
  : { data: Endpoint };

function fetchApi<T extends 'list' | 'detail'>(type: T): ApiResponse<T> {
  // 返回类型根据 T 自动推断 / Return type auto-inferred from T
  ...
}
```

### 10.2 本项目的泛型实践 / This Project's Generic Practice

```typescript
// API 客户端泛型 / API client generics
async function apiRequest<T>(
  path: string,
  options?: RequestInit
): Promise<ApiResult<T>> {
  const response = await fetch(`${BASE_URL}${path}`, options);
  if (!response.ok) {
    throw new ApiError(response.status, await response.text());
  }
  return { data: await response.json() as T, status: response.status };
}

// 类型安全的使用 / Type-safe usage
interface MaskResponse {
  masked: Record<string, string>;
  rules_applied: string[];
}

const result = await apiRequest<MaskResponse>('/api/proxy', {
  method: 'POST',
  body: JSON.stringify(payload),
});
result.data.masked;        // ✅ Record<string, string>
result.data.rules_applied; // ✅ string[]
result.data.unknown;       // ❌ 编译错误 / Compile error

// 通用列表组件泛型 / Generic list component
interface ListProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => ReactNode;
  keyExtractor: (item: T) => string;
  emptyMessage?: string;
}

function DataList<T>({ items, renderItem, keyExtractor, emptyMessage }: ListProps<T>) {
  if (items.length === 0) return <p>{emptyMessage ?? '无数据 / No data'}</p>;
  return (
    <ul>
      {items.map((item, i) => (
        <li key={keyExtractor(item)}>{renderItem(item, i)}</li>
      ))}
    </ul>
  );
}
```

### 10.3 泛型设计模式 / Generic Design Patterns

| 模式 / Pattern | 用途 / Purpose | 示例 / Example |
|---|---|---|
| keyof 约束 / keyof constraint | 属性安全访问 / Safe property access | `K extends keyof T` |
| 条件类型 / Conditional type | 类型级分支 / Type-level branching | `T extends X ? A : B` |
| 推断 / infer | 提取嵌套类型 / Extract nested type | `T extends Promise<infer U>` |
| 递归泛型 / Recursive generic | 深层类型操作 / Deep type ops | `DeepPartial<T>` |
| 模板字面量 / Template literal | 字符串类型操作 / String type ops | `` `on${Capitalize<K>}` `` |

## 11. 类型守卫与窄化 / Type Guards & Narrowing

### 11.1 类型守卫分类 / Type Guard Categories

```typescript
// 1. typeof 守卫 / typeof guard
function formatValue(value: string | number | boolean): string {
  if (typeof value === 'string') return value.toUpperCase();
  if (typeof value === 'number') return value.toFixed(2);
  return value ? 'Yes' : 'No';  // 此处 value 已窄化为 boolean / Narrowed to boolean
}

// 2. instanceof 守卫 / instanceof guard
class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

function handleError(err: unknown): string {
  if (err instanceof ApiError) {
    return `API 错误 / API Error ${err.status}: ${err.message}`;  // ✅ 安全访问 status
  }
  if (err instanceof Error) {
    return `错误 / Error: ${err.message}`;
  }
  return `未知错误 / Unknown: ${String(err)}`;
}

// 3. in 操作符守卫 / in operator guard
interface SuccessResult { data: unknown; error?: never }
interface ErrorResult { data?: never; error: string }
type Result = SuccessResult | ErrorResult;

function processResult(result: Result) {
  if ('error' in result && result.error) {
    console.error(result.error);   // ✅ ErrorResult
  } else {
    console.log(result.data);      // ✅ SuccessResult
  }
}
```

### 11.2 自定义类型守卫 / Custom Type Guards (is)

```typescript
// 本项目中的自定义类型守卫 / Custom type guards in this project

// 端点类型守卫 / Endpoint type guard
interface RestEndpoint { type: 'rest'; path: string; method: string }
interface GrpcEndpoint { type: 'grpc'; service: string; method: string }
type Endpoint = RestEndpoint | GrpcEndpoint;

function isRestEndpoint(ep: Endpoint): ep is RestEndpoint {
  return ep.type === 'rest';
}

function buildUrl(ep: Endpoint): string {
  if (isRestEndpoint(ep)) {
    return `${ep.method} ${ep.path}`;  // ✅ 安全访问 path
  }
  return `${ep.service}/${ep.method}`;  // ✅ 安全访问 service
}

// 非空断言守卫 / Non-null assertion guard
function isDefined<T>(value: T | null | undefined): value is T {
  return value !== null && value !== undefined;
}

// 使用 / Usage:
const endpoints: (Endpoint | null)[] = [...];
const valid = endpoints.filter(isDefined);  // Endpoint[] (无 null) / no null
```

### 11.3 可辨识联合类型 / Discriminated Unions

```typescript
// 本项目 API 响应的可辨识联合 / Discriminated union for API responses
type ApiState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string; retryable: boolean };

function renderState<T>(state: ApiState<T>): ReactNode {
  switch (state.status) {
    case 'idle':
      return <span>等待中 / Idle</span>;
    case 'loading':
      return <Spinner />;
    case 'success':
      return <DataView data={state.data} />;  // ✅ T 类型安全 / Type-safe T
    case 'error':
      return (
        <ErrorAlert
          message={state.error}              // ✅ string
          canRetry={state.retryable}         // ✅ boolean
        />
      );
  }
}

// 穷举检查 / Exhaustiveness check
function assertNever(x: never): never {
  throw new Error(`Unexpected value: ${x}`);
}
```

## 12. 编译器配置与工程化 / Compiler Configuration & Engineering

### 12.1 本项目 tsconfig 详解 / This Project's tsconfig Details

```jsonc
// console/web/tsconfig.json — 关键配置解析 / Key config explanation
{
  "compilerOptions": {
    // === 类型检查严格度 / Type checking strictness ===
    "strict": true,              // 启用所有严格检查 / Enable all strict checks
    "noUnusedLocals": true,      // 禁止未使用变量 / No unused variables
    "noUnusedParameters": true,  // 禁止未使用参数 / No unused parameters
    "noFallthroughCasesInSwitch": true,  // switch 必须 break

    // === 模块解析 / Module resolution ===
    "module": "ESNext",          // 使用 ESM / Use ESM
    "moduleResolution": "bundler",  // Vite 兼容解析 / Vite-compatible
    "resolveJsonModule": true,   // 支持 JSON 导入 / Support JSON imports

    // === JSX 处理 / JSX handling ===
    "jsx": "react-jsx",          // React 17+ 自动 JSX 转换 / Auto JSX transform

    // === 输出控制 / Output control ===
    "noEmit": true,              // 仅检查不输出 / Check only, no emit
    "isolatedModules": true,     // 单文件可编译 / Single-file compilable

    // === 路径别名 / Path aliases ===
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]         // @/ 映射到 src/ / Map @/ to src/
    }
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]  // Vite 配置用 / For Vite config
}
```

### 12.2 严格模式各选项作用 / Strict Mode Options Breakdown

| 选项 / Option | 作用 / Effect | 本项目影响 / Project Impact |
|---|---|---|
| `strictNullChecks` | null/undefined 不可赋给其他类型 / Not assignable | 强制空值检查 / Force null checks |
| `strictFunctionTypes` | 函数参数逆变检查 / Contravariant params | 回调类型安全 / Callback safety |
| `strictBindCallApply` | bind/call/apply 参数检查 / Param check | 方法调用安全 / Method call safety |
| `noImplicitAny` | 禁止隐式 any / No implicit any | 必须显式标注 / Must annotate |
| `noImplicitThis` | 禁止隐式 this / No implicit this | 组件 this 安全 / Component safety |
| `alwaysStrict` | 输出 "use strict" / Emit "use strict" | ESM 默认严格 / ESM default |
| `useUnknownInCatchVariables` | catch 变量为 unknown / catch var unknown | 强制类型检查 / Force type check |

### 12.3 构建与检查命令 / Build & Check Commands

```bash
# 类型检查（不输出文件）/ Type check (no emit)
npx tsc --noEmit

# 构建生产版本（Vite 内部调用 tsc）/ Production build (Vite calls tsc)
corepack pnpm build

# 监听模式检查 / Watch mode check
npx tsc --noEmit --watch

# 检查单个文件（快速验证）/ Check single file (quick verify)
npx tsc --noEmit src/api/client.ts

# 生成声明文件（库模式）/ Generate declarations (library mode)
npx tsc --declaration --emitDeclarationOnly --outDir dist/types
```

### 12.4 常见编译错误排查 / Common Compilation Error Troubleshooting

| 错误 / Error | 原因 / Cause | 解决 / Solution |
|---|---|---|
| TS2345: Argument not assignable | 类型不匹配 / Type mismatch | 检查泛型参数 / Check generic params |
| TS2322: Type not assignable | 赋值类型错误 / Wrong assign type | 添加类型守卫 / Add type guard |
| TS18048: Possibly undefined | 未检查空值 / Null not checked | 添加 `?.` 或 `!` / Add `?.` or `!` |
| TS7006: Parameter implicitly any | 缺少类型标注 / Missing annotation | 添加参数类型 / Add param type |
| TS2307: Cannot find module | 模块解析失败 / Module resolution | 检查 paths 配置 / Check paths config |
| TS2786: Cannot be used as JSX | 组件类型错误 / Component type error | 检查返回类型 / Check return type |

---

## 13. 装饰器与元数据编程 / Decorators & Metadata Programming

### 13.1 TC39 标准装饰器 / TC39 Standard Decorators

TypeScript 5.0 引入了符合 TC39 Stage 3 提案的新装饰器语法，与旧的实验性装饰器有本质区别：

```typescript
// ===== 新标准装饰器（TS 5.0+）/ New Standard Decorators =====
// tsconfig.json: { "compilerOptions": { "target": "ES2022" } }
// 无需 experimentalDecorators 选项 / No experimentalDecorators needed

// 类装饰器 / Class decorator
function logged<T extends new (...args: any[]) => any>(
  OriginalClass: T,
  context: ClassDecoratorContext
) {
  const className = String(context.name);

  // 返回新类替换原始类 / Return new class to replace original
  return class extends OriginalClass {
    constructor(...args: any[]) {
      console.log(`[LOG] Creating instance of ${className}`);
      super(...args);
      console.log(`[LOG] Instance created with args:`, args);
    }
  };
}

// 方法装饰器 / Method decorator
function measureTiming(
  originalMethod: Function,
  context: ClassMethodDecoratorContext
) {
  const methodName = String(context.name);

  // 返回新方法替换原方法 / Return new method to replace original
  return function (this: any, ...args: any[]) {
    const start = performance.now();
    const result = originalMethod.apply(this, args);
    const end = performance.now();
    console.log(`[TIMING] ${methodName} took ${(end - start).toFixed(2)}ms`);
    return result;
  };
}

// 访问器装饰器 / Accessor decorator
function cacheable(
  _target: undefined,
  context: ClassGetterDecoratorContext
) {
  let cached: any;
  let hasCache = false;

  context.addInitializer(function () {
    // 实例初始化时重置缓存 / Reset cache on instance init
    hasCache = false;
  });

  return function (this: any) {
    if (!hasCache) {
      cached = (this as any)[`__original_${String(context.name)}`]();
      hasCache = true;
    }
    return cached;
  };
}

// 使用示例 / Usage example
@logged
class PrivacyApiClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  @measureTiming
  async sendRequest(path: string, body: unknown) {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return response.json();
  }
}
```

### 13.2 装饰器组合与执行顺序 / Decorator Composition & Execution Order

```typescript
// ===== 多装饰器执行顺序 / Multiple Decorator Execution Order =====

function first() {
  console.log('first(): 工厂函数执行 / factory evaluated');
  return function (target: any, context: ClassMethodDecoratorContext) {
    console.log('first(): 装饰器应用 / decorator applied');
    return function (this: any, ...args: any[]) {
      console.log('first(): 方法调用前 / before method call');
      const result = (target as Function).apply(this, args);
      console.log('first(): 方法调用后 / after method call');
      return result;
    };
  };
}

function second() {
  console.log('second(): 工厂函数执行 / factory evaluated');
  return function (target: any, context: ClassMethodDecoratorContext) {
    console.log('second(): 装饰器应用 / decorator applied');
    return function (this: any, ...args: any[]) {
      console.log('second(): 方法调用前 / before method call');
      const result = (target as Function).apply(this, args);
      console.log('second(): 方法调用后 / after method call');
      return result;
    };
  };
}

class Example {
  // 执行顺序 / Execution order:
  // 1. 工厂函数: 从上到下 / Factories: top to bottom
  // 2. 装饰器应用: 从下到上 / Applied: bottom to top
  // 3. 方法调用: 从外到内 / Call: outer to inner
  @first()
  @second()
  method() {
    console.log('method(): 实际执行 / actual execution');
  }
}

// 输出顺序 / Output order:
// first(): 工厂函数执行
// second(): 工厂函数执行
// second(): 装饰器应用
// first(): 装饰器应用
// --- 调用时 / When called ---
// first(): 方法调用前
// second(): 方法调用前
// method(): 实际执行
// second(): 方法调用后
// first(): 方法调用后
```

### 13.3 参数装饰器与依赖注入 / Parameter Decorators & DI

```typescript
// ===== 使用旧装饰器实现 DI（实验性）/ DI with Legacy Decorators =====
// tsconfig: { "experimentalDecorators": true, "emitDecoratorMetadata": true }

import 'reflect-metadata';

const INJECT_TOKEN = Symbol('inject');

// 参数装饰器：标记注入点 / Parameter decorator: mark injection point
function Inject(token: string) {
  return function (target: any, propertyKey: string | symbol | undefined, parameterIndex: number) {
    const existingInjections: Map<number, string> =
      Reflect.getMetadata(INJECT_TOKEN, target) || new Map();
    existingInjections.set(parameterIndex, token);
    Reflect.defineMetadata(INJECT_TOKEN, existingInjections, target);
  };
}

// 简单 DI 容器 / Simple DI container
class Container {
  private services = new Map<string, any>();

  register(token: string, instance: any) {
    this.services.set(token, instance);
  }

  resolve<T>(TargetClass: new (...args: any[]) => T): T {
    const injections: Map<number, string> =
      Reflect.getMetadata(INJECT_TOKEN, TargetClass) || new Map();

    const args: any[] = [];
    injections.forEach((token, index) => {
      args[index] = this.services.get(token);
    });

    return new TargetClass(...args);
  }
}

// 使用 / Usage
const container = new Container();
container.register('API_BASE_URL', 'http://localhost:8079');
container.register('HTTP_CLIENT', { fetch: globalThis.fetch });

class ApiService {
  constructor(
    @Inject('API_BASE_URL') private baseUrl: string,
    @Inject('HTTP_CLIENT') private http: { fetch: typeof fetch }
  ) {}

  async get(path: string) {
    return this.http.fetch(`${this.baseUrl}${path}`);
  }
}

const service = container.resolve(ApiService);
```

---

## 14. 类型编程与元类型 / Type-level Programming & Meta-types

### 14.1 类型级别计算 / Type-level Computation

TypeScript 的类型系统是图灵完备的，可以在类型层面进行复杂计算：

```typescript
// ===== 类型级别算术 / Type-level Arithmetic =====

// 构建元组长度工具 / Tuple length utilities
type BuildTuple<N extends number, T extends any[] = []> =
  T['length'] extends N ? T : BuildTuple<N, [...T, any]>;

// 类型加法 / Type addition
type Add<A extends number, B extends number> =
  [...BuildTuple<A>, ...BuildTuple<B>]['length'];

type Result1 = Add<3, 5>;  // 8

// 类型级别字符串操作 / Type-level string manipulation
type CamelToSnake<S extends string> =
  S extends `${infer Head}${infer Tail}`
    ? Head extends Uppercase<Head>
      ? `_${Lowercase<Head>}${CamelToSnake<Tail>}`
      : `${Head}${CamelToSnake<Tail>}`
    : S;

type Snake1 = CamelToSnake<'firstName'>;    // "first_name"
type Snake2 = CamelToSnake<'httpResponse'>; // "http_response"

// 深度 Readonly / Deep Readonly
type DeepReadonly<T> = {
  readonly [K in keyof T]: T[K] extends object
    ? T[K] extends Function
      ? T[K]  // 函数不变 / Functions unchanged
      : DeepReadonly<T[K]>
    : T[K];
};

// 深度 Partial / Deep Partial
type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends object
    ? T[K] extends Array<infer U>
      ? Array<DeepPartial<U>>
      : DeepPartial<T[K]>
    : T[K];
};

// 应用示例 / Application example
interface PrivacyConfig {
  masking: {
    enabled: boolean;
    rules: Array<{ field: string; strategy: string }>;
  };
  dp: {
    epsilon: number;
    mechanism: 'laplace' | 'gaussian';
  };
}

// 所有字段变为可选 / All fields become optional
type PartialConfig = DeepPartial<PrivacyConfig>;
// { masking?: { enabled?: boolean; rules?: Array<{field?: string; strategy?: string}> } }
```

### 14.2 类型安全的事件系统 / Type-safe Event System

```typescript
// ===== 类型安全事件发射器 / Type-safe Event Emitter =====

// 事件映射定义 / Event map definition
interface AppEvents {
  'request:start': { method: string; path: string };
  'request:success': { status: number; duration: number };
  'request:error': { error: string; retryable: boolean };
  'connection:change': { connected: boolean };
}

// 类型安全的 EventEmitter / Type-safe EventEmitter
class TypedEmitter<Events extends Record<string, any>> {
  private handlers = new Map<keyof Events, Set<Function>>();

  on<K extends keyof Events>(
    event: K,
    handler: (payload: Events[K]) => void
  ): () => void {
    if (!this.handlers.has(event)) {
      this.handlers.set(event, new Set());
    }
    this.handlers.get(event)!.add(handler);

    // 返回取消订阅函数 / Return unsubscribe function
    return () => this.handlers.get(event)?.delete(handler);
  }

  emit<K extends keyof Events>(event: K, payload: Events[K]): void {
    this.handlers.get(event)?.forEach(handler => handler(payload));
  }

  once<K extends keyof Events>(
    event: K,
    handler: (payload: Events[K]) => void
  ): void {
    const wrapper = (payload: Events[K]) => {
      handler(payload);
      this.handlers.get(event)?.delete(wrapper);
    };
    this.on(event, wrapper);
  }
}

// 使用 - 完全类型安全 / Usage - fully type-safe
const emitter = new TypedEmitter<AppEvents>();

emitter.on('request:start', (payload) => {
  // payload 自动推断为 { method: string; path: string }
  console.log(`${payload.method} ${payload.path}`);
});

emitter.emit('request:success', {
  status: 200,
  duration: 42,
  // error: 'x'  // ❌ 编译错误：不存在此属性 / Compile error
});
```

### 14.3 类型体操实战 / Type Gymnastics in Practice

```typescript
// ===== 实用类型工具集 / Utility Type Toolkit =====

// 提取 Promise 内部类型 / Extract Promise inner type
type Awaited<T> = T extends Promise<infer U> ? Awaited<U> : T;

// 提取函数参数类型 / Extract function parameter types
type FunctionParams<T extends Function> = T extends (...args: infer P) => any ? P : never;

// 提取函数返回类型 / Extract function return type
type FunctionReturn<T extends Function> = T extends (...args: any[]) => infer R ? R : never;

// 对象键路径类型（嵌套访问）/ Object key path type (nested access)
type KeyPaths<T, Prefix extends string = ''> = {
  [K in keyof T & string]: T[K] extends object
    ? `${Prefix}${K}` | KeyPaths<T[K], `${Prefix}${K}.`>
    : `${Prefix}${K}`;
}[keyof T & string];

interface ApiResponse {
  data: {
    user: { name: string; email: string };
    items: Array<{ id: number }>;
  };
  meta: { total: number; page: number };
}

type Paths = KeyPaths<ApiResponse>;
// "data" | "data.user" | "data.user.name" | "data.user.email" |
// "data.items" | "meta" | "meta.total" | "meta.page"

// 类型安全的路径访问函数 / Type-safe path access function
declare function getPath<T, P extends KeyPaths<T>>(obj: T, path: P): unknown;

const resp: ApiResponse = { data: { user: { name: 'Alice', email: 'a@b.com' }, items: [] }, meta: { total: 1, page: 1 } };
getPath(resp, 'data.user.name');  // ✅ 合法路径 / Valid path
// getPath(resp, 'data.foo');     // ❌ 编译错误 / Compile error
```

---

## 15. 声明合并与模块扩展 / Declaration Merging & Module Extension

### 15.1 接口声明合并 / Interface Declaration Merging

TypeScript 中同名接口会自动合并，这是扩展第三方库类型的核心机制：

```typescript
// ===== 接口合并规则 / Interface Merging Rules =====

// 第一个声明 / First declaration
interface Window {
  __APP_VERSION__: string;
}

// 第二个声明 - 自动合并 / Second declaration - auto merged
interface Window {
  __API_BASE__: string;
  __DEBUG__: boolean;
}

// 合并后的 Window 同时拥有三个属性 / Merged Window has all three props
console.log(window.__APP_VERSION__);  // ✅
console.log(window.__API_BASE__);     // ✅
console.log(window.__DEBUG__);        // ✅

// 函数重载合并 / Function overload merging
interface ApiClient {
  request(path: string): Promise<any>;
}

interface ApiClient {
  request(path: string, options: { method: string }): Promise<any>;
  request(path: string, options: { method: string; body: unknown }): Promise<any>;
}

// 合并后形成重载签名 / Merged forms overload signatures
// 注意：后声明的重载优先级更高 / Note: later declarations have higher priority
```

### 15.2 模块增强 / Module Augmentation

```typescript
// ===== 扩展第三方模块类型 / Extend Third-party Module Types =====

// 文件: types/express-augment.d.ts
// 扩展 Express Request 对象 / Extend Express Request object
import 'express';

declare module 'express' {
  interface Request {
    // 添加自定义属性 / Add custom properties
    userId?: string;
    requestId: string;
    startTime: number;
  }
}

// 扩展 React 类型 / Extend React types
// 文件: types/react-augment.d.ts
import 'react';

declare module 'react' {
  interface CSSProperties {
    // 允许 CSS 自定义属性 / Allow CSS custom properties
    [key: `--${string}`]: string | number;
  }
}

// 使用 / Usage
function MyComponent() {
  return (
    <div style={{
      color: 'blue',
      '--custom-spacing': '16px',  // ✅ 现在合法 / Now valid
    }}>
      Hello
    </div>
  );
}

// 扩展 import.meta / Extend import.meta
// 文件: types/vite-env.d.ts
interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string;
  readonly VITE_APP_VERSION: string;
  readonly VITE_ENABLE_MOCK: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

// 使用 / Usage
const apiUrl = import.meta.env.VITE_API_BASE_URL;  // ✅ 类型安全 / Type-safe
```

### 15.3 全局类型声明与命名空间 / Global Type Declarations & Namespaces

```typescript
// ===== 全局类型文件组织 / Global Type File Organization =====

// 文件: types/global.d.ts
// 无需 import/export 的文件自动成为全局声明 / Files without import/export are global

// 全局类型别名 / Global type aliases
type Nullable<T> = T | null;
type AsyncResult<T, E = Error> = Promise<{ ok: true; data: T } | { ok: false; error: E }>;

// 全局接口 / Global interfaces
interface PaginationParams {
  page: number;
  pageSize: number;
  sortBy?: string;
  order?: 'asc' | 'desc';
}

interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  hasMore: boolean;
}

// 命名空间组织相关类型 / Namespace for related types
declare namespace API {
  namespace Masking {
    interface Request {
      data: Record<string, unknown>;
      fields?: string[];
      strategy?: 'partial' | 'full' | 'hash';
    }
    interface Response {
      masked: Record<string, unknown>;
      appliedRules: string[];
    }
  }

  namespace DP {
    interface Query {
      mechanism: 'laplace' | 'gaussian';
      epsilon: number;
      sensitivity?: number;
    }
    interface Result {
      value: number;
      noiseAdded: number;
      budgetRemaining: number;
    }
  }
}

// 在任何文件中使用（无需 import）/ Use in any file (no import needed)
async function callMaskingApi(req: API.Masking.Request): Promise<API.Masking.Response> {
  const resp = await fetch('/api/mask', {
    method: 'POST',
    body: JSON.stringify(req),
  });
  return resp.json();
}
```

### 15.4 声明合并实践总结 / Declaration Merging Practice Summary

| 场景 / Scenario | 技术 / Technique | 文件位置 / File Location | 本项目 / Project |
|---|---|---|---|
| 扩展 Window | interface 合并 / merge | `types/global.d.ts` | ✅ 使用 / Used |
| 扩展 import.meta.env | interface 合并 / merge | `types/vite-env.d.ts` | ✅ 使用 / Used |
| CSS 自定义属性 | module augmentation | `types/react-augment.d.ts` | 可选 / Optional |
| API 类型命名空间 | declare namespace | `types/api.d.ts` | ✅ 使用 / Used |
| 第三方库补丁 / Lib patch | module augmentation | `types/*.d.ts` | 按需 / As needed |

## 16. 类型体操实战 / Type Gymnastics in Practice

### 16.1 递归类型操作 / Recursive Type Operations

```typescript
// 深层只读：递归将所有属性变为 readonly
// Deep Readonly: recursively make all properties readonly
type DeepReadonly<T> = {
  readonly [K in keyof T]: T[K] extends object
    ? T[K] extends Function
      ? T[K]  // 函数不变 / Functions unchanged
      : DeepReadonly<T[K]>
    : T[K]
}

// 深层 Partial：递归将所有属性变为可选
// Deep Partial: recursively make all properties optional
type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends object
    ? T[K] extends Array<infer U>
      ? Array<DeepPartial<U>>
      : DeepPartial<T[K]>
    : T[K]
}

// 应用示例：隐私配置
// Usage: Privacy configuration
interface PrivacyConfig {
  masking: {
    strategy: 'full' | 'partial' | 'hash'
    fields: string[]
    nested: {
      depth: number
      separator: string
    }
  }
  dp: {
    epsilon: number
    mechanism: 'laplace' | 'gaussian'
  }
}

// 更新配置时只需传部分字段
// Only need partial fields when updating config
function updateConfig(patch: DeepPartial<PrivacyConfig>) {
  // patch.masking?.nested?.depth 可选访问
  // patch.masking?.nested?.depth optional access
}
```

### 16.2 字符串类型操作 / String Type Manipulation

```typescript
// 将 snake_case 转为 camelCase
// Convert snake_case to camelCase
type SnakeToCamel<S extends string> =
  S extends `${infer Head}_${infer Tail}`
    ? `${Head}${Capitalize<SnakeToCamel<Tail>>}`
    : S

// 将 camelCase 转为 snake_case
// Convert camelCase to snake_case
type CamelToSnake<S extends string> =
  S extends `${infer Head}${infer Tail}`
    ? Tail extends Uncapitalize<Tail>
      ? `${Lowercase<Head>}${CamelToSnake<Tail>}`
      : `${Lowercase<Head>}_${CamelToSnake<Uncapitalize<Tail>>}`
    : S

// 测试
// Tests
type A = SnakeToCamel<'masked_field_name'>  // 'maskedFieldName'
type B = CamelToSnake<'maskedFieldName'>    // 'masked_field_name'

// API 响应自动转换类型
// API response auto-conversion type
type CamelCaseKeys<T> = {
  [K in keyof T as SnakeToCamel<K & string>]: T[K]
}

// 后端返回 snake_case，前端使用 camelCase
// Backend returns snake_case, frontend uses camelCase
interface ApiMaskResponse {
  masked_data: Record<string, string>
  mask_details: Array<{ field_name: string; strategy: string }>
}

type FrontendMaskResponse = CamelCaseKeys<ApiMaskResponse>
// { maskedData: ..., maskDetails: ... }
```

### 16.3 联合类型分发 / Union Type Distribution

```typescript
// 分发条件类型：将联合类型每个成员单独处理
// Distributive conditional type: process each union member individually
type ToNullable<T> = T extends any ? T | null : never

type Result = ToNullable<string | number>
// string | null | number | null

// 实用：提取 Promise 内部类型
// Utility: Extract Promise inner type
type UnwrapPromise<T> = T extends Promise<infer U> ? U : T
type UnwrapAll<T> = T extends Promise<infer U> ? UnwrapAll<U> : T

type A = UnwrapAll<Promise<Promise<string>>>  // string

// 实用：提取数组元素类型
// Utility: Extract array element type
type ElementOf<T> = T extends readonly (infer E)[] ? E : never

// 实用：提取函数返回类型（包括异步）
// Utility: Extract function return type (including async)
type AsyncReturnType<T extends (...args: any[]) => any> =
  UnwrapPromise<ReturnType<T>>

async function fetchData(): Promise<{ name: string }> {
  return { name: 'test' }
}

type Data = AsyncReturnType<typeof fetchData>  // { name: string }
```

### 16.4 类型安全的事件系统 / Type-safe Event System

```typescript
// 类型安全的事件发射器
// Type-safe event emitter
interface EventMap {
  'mask:complete': { fields: string[]; duration: number }
  'dp:budget': { remaining: number; consumed: number }
  'classify:progress': { stage: string; percent: number }
  'error': { code: number; message: string }
}

class TypedEmitter<T extends Record<string, any>> {
  private handlers = new Map<keyof T, Set<Function>>()
  
  on<K extends keyof T>(event: K, handler: (payload: T[K]) => void): void {
    if (!this.handlers.has(event)) {
      this.handlers.set(event, new Set())
    }
    this.handlers.get(event)!.add(handler)
  }
  
  emit<K extends keyof T>(event: K, payload: T[K]): void {
    this.handlers.get(event)?.forEach(fn => fn(payload))
  }
  
  off<K extends keyof T>(event: K, handler: (payload: T[K]) => void): void {
    this.handlers.get(event)?.delete(handler)
  }
}

// 使用：完全类型安全
// Usage: Fully type-safe
const emitter = new TypedEmitter<EventMap>()

emitter.on('mask:complete', (payload) => {
  // payload 自动推断为 { fields: string[]; duration: number }
  // payload auto-inferred as { fields: string[]; duration: number }
  console.log(payload.fields.length)
})

emitter.emit('dp:budget', { remaining: 0.5, consumed: 0.5 })
// emitter.emit('dp:budget', { wrong: true })  // ✘ 编译错误 / Compile error
```

## 17. 模块系统与包管理 / Module System & Package Management

### 17.1 ESM 与 CJS 互操作 / ESM & CJS Interoperability

```typescript
// tsconfig.json 模块配置
// tsconfig.json module configuration
{
  "compilerOptions": {
    // 现代项目推荐
    // Recommended for modern projects
    "module": "ESNext",          // 输出 ESM / Output ESM
    "moduleResolution": "bundler", // Vite/esbuild 解析 / Resolution
    
    // Node.js 项目
    // Node.js projects
    // "module": "NodeNext",
    // "moduleResolution": "NodeNext",
  }
}
```

```typescript
// ESM 导入规则
// ESM import rules

// ✔ 命名导入（推荐）
// ✔ Named imports (recommended)
import { useState, useEffect } from 'react'

// ✔ 类型导入（编译时擦除）
// ✔ Type imports (erased at compile time)
import type { MaskRequest } from './types'

// ✔ 内联类型导入
// ✔ Inline type imports
import { type Config, loadConfig } from './config'

// ✘ 避免默认导入 CJS 包（可能出问题）
// ✘ Avoid default importing CJS packages (may cause issues)
// import lodash from 'lodash'  // 可能失败 / May fail
import { debounce } from 'lodash-es'  // ✔ ESM 版本 / ESM version
```

### 17.2 package.json exports 字段 / package.json exports Field

```json
// 现代包的导出配置
// Modern package export configuration
{
  "name": "@privacy/console-utils",
  "type": "module",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js",
      "require": "./dist/index.cjs"
    },
    "./react": {
      "types": "./dist/react/index.d.ts",
      "import": "./dist/react/index.js"
    },
    "./package.json": "./package.json"
  },
  "main": "./dist/index.cjs",
  "module": "./dist/index.js",
  "types": "./dist/index.d.ts"
}
```

### 17.3 路径别名与项目引用 / Path Aliases & Project References

```typescript
// tsconfig.json 路径别名
// tsconfig.json path aliases
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "@components/*": ["src/components/*"],
      "@hooks/*": ["src/hooks/*"],
      "@utils/*": ["src/utils/*"],
      "@types/*": ["src/types/*"]
    }
  }
}

// vite.config.ts 中同步配置
// Sync configuration in vite.config.ts
import { resolve } from 'path'

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@components': resolve(__dirname, 'src/components'),
      '@hooks': resolve(__dirname, 'src/hooks'),
    },
  },
})

// 使用
// Usage
import { MaskForm } from '@/components/MaskForm'
import { usePrivacyApi } from '@hooks/usePrivacyApi'
```

### 17.4 模块解析策略对比 / Module Resolution Strategy Comparison

| 策略 / Strategy | 配置 / Config | 适用场景 / Use Case | 特点 / Features |
|---|---|---|---|
| bundler | moduleResolution: "bundler" | Vite/webpack 项目 | 支持 exports + 无后缀 / Supports exports + no ext |
| NodeNext | moduleResolution: "NodeNext" | Node.js ESM | 严格 .js 后缀 / Strict .js ext |
| node16 | moduleResolution: "node16" | Node 16+ | 同 NodeNext / Same as NodeNext |
| classic | moduleResolution: "classic" | 已废弃 / Deprecated | 不建议使用 / Not recommended |

## 18. 编译优化与构建性能 / Compilation Optimization & Build Performance

### 18.1 增量编译与缓存 / Incremental Compilation & Caching

```json
// tsconfig.json 性能优化配置
// tsconfig.json performance optimization
{
  "compilerOptions": {
    // 增量编译：缓存上次编译结果
    // Incremental: cache last compilation
    "incremental": true,
    "tsBuildInfoFile": "./.tsbuildinfo",
    
    // 跳过声明文件检查
    // Skip declaration file checking
    "skipLibCheck": true,
    
    // 禁用源映射（生产）
    // Disable source maps (production)
    "sourceMap": false,
    
    // 仅发出声明（不编译 JS，由 Vite 处理）
    // Emit declarations only (Vite handles JS)
    "emitDeclarationOnly": true,
    "declaration": true,
    "declarationDir": "./dist/types"
  }
}
```

### 18.2 项目引用与并行构建 / Project References & Parallel Builds

```json
// 大型项目拆分为多个子项目
// Split large projects into sub-projects
// tsconfig.json (root)
{
  "references": [
    { "path": "./packages/core" },
    { "path": "./packages/ui" },
    { "path": "./packages/app" }
  ],
  "files": []
}

// packages/core/tsconfig.json
{
  "compilerOptions": {
    "composite": true,  // 必须启用 / Must enable
    "outDir": "./dist",
    "rootDir": "./src"
  }
}
```

```bash
# 并行构建所有子项目
# Build all sub-projects in parallel
tsc --build --verbose

# 清理并重建
# Clean and rebuild
tsc --build --clean && tsc --build
```

### 18.3 类型检查性能分析 / Type Checking Performance Analysis

```bash
# 生成编译性能跟踪
# Generate compilation performance trace
tsc --extendedDiagnostics

# 输出示例 / Output example:
# Files:                         234
# Lines of Library:            28456
# Lines of Definitions:        45678
# Lines of TypeScript:          8901
# Identifiers:                 12345
# Symbols:                     23456
# Types:                        5678
# Memory used:               123456K
# Assignability cache size:     1234
# Identity cache size:           567
# Subtype cache size:            890
# Strict subtype cache size:     123
# I/O Read time:               0.12s
# Parse time:                  0.45s
# Bind time:                   0.23s
# Check time:                  2.34s   ← 最耗时 / Most time-consuming
# Emit time:                   0.56s
# Total time:                  3.70s
```

### 18.4 常见性能陷阱与解决 / Common Performance Pitfalls & Solutions

```typescript
// ✘ 陷阱 1：过度使用条件类型递归
// ✘ Pitfall 1: Excessive conditional type recursion
type Bad<S extends string> =
  S extends `${infer A}${infer B}${infer C}${infer D}${infer E}${infer Rest}`
    ? [A, B, C, D, E, ...Bad<Rest>]
    : S extends `${infer A}${infer Rest}`
      ? [A, ...Bad<Rest>]
      : []
// 对长字符串极慢 / Extremely slow for long strings

// ✔ 解决：限制递归深度
// ✔ Solution: Limit recursion depth
type Safe<S extends string, Depth extends any[] = []> =
  Depth['length'] extends 50 ? string[] :  // 深度限制 / Depth limit
  S extends `${infer A}${infer Rest}`
    ? [A, ...Safe<Rest, [...Depth, any]>]
    : []

// ✘ 陷阱 2：巨大的联合类型
// ✘ Pitfall 2: Huge union types
type AllPaths = PathsOf<DeepNestedObject>  // 可能生成数千个联合 / May generate thousands

// ✔ 解决：使用 string 模板约束
// ✔ Solution: Use string template constraints
type SafePaths = `/${string}`  // 简单模板 / Simple template

// ✘ 陷阱 3：循环引用类型
// ✘ Pitfall 3: Circular reference types
interface Node {
  children: Node[]  // OK，但深层操作可能爆炸 / OK, but deep ops may explode
}
// DeepReadonly<Node> 会无限递归 / Will recurse infinitely

// ✔ 解决：添加深度限制
// ✔ Solution: Add depth limit
type SafeDeepReadonly<T, D extends number = 5> = ...
```

### 18.5 构建性能优化检查清单 / Build Performance Checklist

| 优化项 / Optimization | 效果 / Effect | 配置 / Config |
|---|---|---|
| skipLibCheck: true | 检查时间 -40% / Check -40% | tsconfig.json |
| incremental: true | 重建时间 -60% / Rebuild -60% | tsconfig.json |
| isolatedModules: true | 支持并行转译 / Parallel transpile | tsconfig.json |
| 避免 barrel files | 减少无关模块加载 / Reduce unused loads | 直接导入 / Direct imports |
| 类型导入用 import type | 编译时擦除 / Erased at compile | import type { X } |
| Vite 替代 tsc 编译 | 构建时间 -90% / Build -90% | esbuild 转译 / Transpile |
