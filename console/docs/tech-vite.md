# Vite 技术栈说明 / Vite Technology Stack

## 1. 技术简介 / Introduction

Vite（法语"快"的意思）是由 Vue.js 作者尤雨溪创建的新一代前端构建工具。
Vite (French for "fast") is a next-generation frontend build tool created by Evan You, the author of Vue.js.

核心特性 / Core Features：
- **极速冷启动（Instant Server Start）**：基于原生 ES Module，开发服务器无需打包即可启动。
- **闪电热更新（Lightning HMR）**：模块热替换速度不随项目规模增长而变慢。
- **优化构建（Optimized Build）**：生产构建使用 Rollup，自动代码分割与 tree-shaking。
- **开箱即用（Out-of-the-box）**：内置 TypeScript、JSX、CSS 预处理器支持。
- **插件生态（Plugin Ecosystem）**：兼容 Rollup 插件，可扩展任意构建行为。

本项目使用版本 / Version Used：`Vite ^6.1.0` + `@vitejs/plugin-react ^4.3.4`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 Vite 配置文件 / Vite Configuration

文件 / File：`console/web/vite.config.ts`

```typescript
/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  // React 插件：启用 Fast Refresh + 自动 JSX 转换
  // React plugin: enables Fast Refresh + automatic JSX transform
  plugins: [react()],

  resolve: {
    alias: {
      // 路径别名：@/ 映射到 src/ 目录，简化导入路径
      // Path alias: @/ maps to src/ directory, simplifies imports
      '@': path.resolve(__dirname, './src'),
    },
  },

  server: {
    // 开发服务器监听端口 / Dev server listening port
    port: 5173,
    proxy: {
      // API 代理：开发时将 /api/* 请求转发到后端（避免 CORS）
      // API proxy: forward /api/* to backend during dev (avoid CORS)
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },

  build: {
    // 构建输出目录 / Build output directory
    outDir: 'dist',
    // 构建前清空输出目录 / Clean output dir before build
    emptyOutDir: true,
  },

  // Vitest 测试配置（复用 Vite 配置）/ Vitest config (reuses Vite config)
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
```

### 2.2 开发工作流 / Development Workflow

```bash
# 启动开发服务器（带 HMR 热更新）/ Start dev server (with HMR)
cd console/web && pnpm dev
# → 浏览器打开 http://localhost:5173

# 生产构建 / Production build
pnpm build
# → 输出到 console/web/dist/（带内容哈希的 JS/CSS）

# 预览生产构建 / Preview production build
pnpm preview
```

### 2.3 构建产物结构 / Build Output Structure

```
console/web/dist/
├── index.html              # 入口 HTML（不带哈希，no-cache）
└── assets/
    ├── index-[hash].js     # 主 JS bundle（带内容哈希，可强缓存）
    └── index-[hash].css    # 主 CSS bundle（带内容哈希，可强缓存）
```

### 2.4 与后端的集成 / Integration with Backend

构建产物 `dist/` 被两个后端托管为静态文件：
The build output `dist/` is served as static files by both backends:

- **Python 后端**：`FastAPI StaticFiles` 挂载 `/assets`，SPA 回退返回 `index.html`
- **Go 后端**：`Gin r.Static("/assets", ...)` + `NoRoute` 回退

### 2.5 关键配置说明 / Key Configuration Notes

| 配置项 / Config | 作用 / Purpose |
|---|---|
| `plugins: [react()]` | 启用 React Fast Refresh（保存即刷新，保留组件状态）|
| `resolve.alias.@` | 支持 `import X from '@/components/X'` 简写 |
| `server.proxy./api` | 开发时免 CORS 配置，直接代理到后端 |
| `build.emptyOutDir` | 每次构建清空旧产物，避免残留文件 |
| `test.environment: jsdom` | 测试中模拟浏览器 DOM 环境 |

### 2.6 HMR 热更新机制 / HMR Mechanism

Vite 的 HMR（Hot Module Replacement）基于原生 ES Module 实现：

```text
┌─────────────────────────────────────────────────────────────┐
│  开发者保存文件 / Developer saves file                      │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  Vite Dev Server 检测变更 / Detects change                  │
│  - 通过 chokidar 监听文件系统 / Watch via chokidar          │
│  - 仅重新编译变更的模块 / Only recompile changed module     │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  WebSocket 通知浏览器 / Notify browser via WebSocket        │
│  - 发送模块更新事件 / Send module update event              │
│  - 包含模块 ID 和时间戳 / Include module ID and timestamp   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  浏览器执行热替换 / Browser performs hot swap               │
│  - React Fast Refresh 保留组件状态 / Preserve component state│
│  - 仅替换变更模块，不整页刷新 / Only swap, no full reload   │
└─────────────────────────────────────────────────────────────┘
```

**HMR 与 Live Reload 的区别 / HMR vs Live Reload**：

| 特性 / Feature | HMR | Live Reload |
|---|---|---|
| 状态保留 | ✅ 保留组件状态 / Preserves state | ❌ 丢失所有状态 / Loses all state |
| 更新速度 | 毫秒级 / Milliseconds | 秒级（整页刷新）/ Seconds |
| 实现方式 | WebSocket + ESM | 整页重新加载 / Full page reload |

### 2.7 开发与生产模式对比 / Dev vs Production Mode

| 维度 / Dimension | 开发模式 / Dev | 生产模式 / Production |
|---|---|---|
| 模块加载 | 原生 ESM，按需加载 / Native ESM, on-demand | Rollup 打包，Tree-shaking |
| 启动速度 | 毫秒级（无打包）/ Milliseconds | 秒级（需打包）/ Seconds |
| 代码压缩 | 无 / None | Terser/esbuild 压缩 |
| Source Map | 内存中 / In-memory | 独立 .map 文件 |
| CSS 处理 | PostCSS 实时编译 / Real-time | 提取为独立文件 |
| 缓存策略 | 无缓存 / No cache | 内容哈希强缓存 |

### 2.8 代理配置详解 / Proxy Configuration Details

```typescript
server: {
  proxy: {
    '/api': {
      target: 'http://127.0.0.1:8080',  // 代理目标
      changeOrigin: true,               // 修改 Origin 头
      // 可选配置 / Optional configs:
      // rewrite: (path) => path.replace(/^\/api/, ''),  // 路径重写
      // secure: false,      // 允许自签名证书（HTTPS 目标）
      // ws: true,           // 代理 WebSocket
    },
  },
}
```

**代理工作原理 / Proxy Workflow**：

```text
浏览器 / Browser          Vite Dev Server          后端 / Backend
     │                        │                        │
     │  GET /api/health       │                        │
     ├───────────────────────▶│                        │
     │                        │  GET /api/health       │
     │                        ├───────────────────────▶│
     │                        │                        │
     │                        │  200 OK {status:...}   │
     │                        │◀───────────────────────┤
     │  200 OK {status:...}   │                        │
     │◀───────────────────────┤                        │
```

### 2.9 插件系统 / Plugin System

Vite 插件基于 Rollup 插件接口扩展：

```typescript
// @vitejs/plugin-react 插件功能 / Plugin features:
// 1. 自动 JSX 转换（无需手动 import React）
// 2. React Fast Refresh（保留组件状态的热更新）
// 3. Babel 转换支持（可选）
plugins: [react()]

// 插件执行顺序 / Plugin execution order:
// 1. Alias 解析 → 2. 插件 transform → 3. 模块加载 → 4. HMR 处理
```

### 2.10 环境变量 / Environment Variables

Vite 支持通过 `.env` 文件配置环境变量：

```bash
# .env.development（开发模式）/ Development mode
VITE_API_BASE_URL=http://localhost:8080

# .env.production（生产模式）/ Production mode
VITE_API_BASE_URL=/api
```

```typescript
// 在代码中访问（必须以 VITE_ 前缀）/ Access in code (must have VITE_ prefix)
const apiUrl = import.meta.env.VITE_API_BASE_URL;
const isDev = import.meta.env.DEV;   // 布尔值：是否开发模式
const isProd = import.meta.env.PROD; // 布尔值：是否生产模式
```

### 2.11 构建优化 / Build Optimizations

Vite 生产构建的自动优化：

| 优化项 / Optimization | 说明 / Description |
|---|---|
| Tree-shaking | 移除未使用的导出代码 |
| Code Splitting | 按路由/动态导入分割代码块 |
| CSS Extraction | 提取 CSS 为独立文件（并行加载）|
| Asset Hashing | 文件名含内容哈希（强缓存）|
| Minification | esbuild 压缩 JS/CSS（比 Terser 快 10-100x）|
| Preload Hints | 自动生成 `<link rel="modulepreload">` |

### 2.12 依赖预构建 / Dependency Pre-bundling

Vite 开发模式的核心优化——将 CommonJS/UMD 依赖转换为 ESM：

```text
┌─────────────────────────────────────────────────────────────┐
│  首次启动 dev server / First dev server start                  │
│                                                             │
│  1. 扫描 package.json 中的依赖                             │
│     Scan dependencies in package.json                       │
│                                                             │
│  2. 使用 esbuild 预构建（极快）                            │
│     Pre-bundle with esbuild (extremely fast)                │
│     - react, react-dom → 合并为单个 ESM 文件             │
│     - CommonJS → ESM 转换                                  │
│     - 内部模块合并（减少 HTTP 请求）                      │
│                                                             │
│  3. 缓存到 node_modules/.vite/                             │
│     Cache to node_modules/.vite/                            │
│     - 下次启动直接复用（秒级启动）                        │
│     - 依赖变更时自动重新预构建                            │
└─────────────────────────────────────────────────────────────┘
```

**预构建前后对比 / Before vs After Pre-bundling**：

| 状态 / State | 请求数 / Requests | 加载时间 / Load Time |
|---|---|---|
| 未预构建 (react-dom 内部) | ~200 个模块文件 | ~2s |
| 预构建后 | 1 个合并文件 | ~10ms |

### 2.13 Rollup 生产构建详解 / Rollup Production Build Details

```text
pnpm build 执行流程 / pnpm build execution flow:

1. tsc --noEmit
   全量 TypeScript 类型检查（失败则中断）
   Full TypeScript type check (abort on failure)
       │
       ▼
2. vite build → Rollup 打包
   ├─ 解析入口 / Resolve entry (index.html → main.tsx)
   ├─ 构建模块图 / Build module graph
   ├─ Tree-shaking / Remove dead code
   ├─ Code splitting / Split chunks
   ├─ esbuild 压缩 / Minify with esbuild
   └─ 输出带哈希文件 / Emit hashed files
       │
       ▼
3. 输出到 dist/
   ├── index.html          (no-cache)
   └── assets/
       ├── index-a1b2c3.js  (immutable, 强缓存 1年)
       └── index-d4e5f6.css (immutable, 强缓存 1年)
```

**缓存策略 / Caching Strategy**：

| 文件 / File | Cache-Control | 原因 / Reason |
|---|---|---|
| `index.html` | `no-cache` | 始终检查新版本（入口不带哈希）/ Always check for new version |
| `assets/*-[hash].js` | `immutable, max-age=31536000` | 内容哈希保证唯一，可永久缓存 / Content hash guarantees uniqueness |
| `assets/*-[hash].css` | `immutable, max-age=31536000` | 同上 / Same as above |

### 2.14 CSS 处理管道 / CSS Processing Pipeline

```text
源码中的 CSS 引用 / CSS references in source:

main.tsx
  └─ import './index.css'
       │
       ▼  Vite 开发模式 / Dev mode:
┌─────────────────────────────────────────────────────────────┐
│  index.css                                                  │
│  @tailwind base;        ─┐                                  │
│  @tailwind components;   ├─ PostCSS 处理                    │
│  @tailwind utilities;   ─┘                                  │
│       │                                                     │
│       ▼  PostCSS Pipeline                                    │
│  1. tailwindcss 插件：扫描 JSX 中的类名，生成实际 CSS   │
│  2. autoprefixer 插件：添加浏览器前缀 (-webkit- 等)     │
│       │                                                     │
│       ▼                                                     │
│  通过 <style> 标签注入 DOM（开发模式）                    │
│  Injected via <style> tag (dev mode)                        │
└─────────────────────────────────────────────────────────────┘

       ▼  Vite 生产模式 / Production mode:
┌─────────────────────────────────────────────────────────────┐
│  1. PostCSS 处理（同上）                                    │
│  2. 提取为独立 .css 文件（非 <style> 注入）               │
│  3. esbuild 压缩 CSS（移除空白/注释）                     │
│  4. 输出 index-[hash].css（带内容哈希）                    │
│  5. 在 index.html 中生成 <link rel="stylesheet">           │
└─────────────────────────────────────────────────────────────┘
```

### 2.15 模块解析顺序 / Module Resolution Order

当 Vite 遇到 `import X from '@/components/X'` 时的解析顺序：

```text
1. Alias 解析 / Alias resolution
   '@' → '/absolute/path/to/console/web/src'
       │
       ▼
2. 文件扩展名探测 / File extension probing
   尝试: X.ts → X.tsx → X.js → X.jsx → X/index.ts → X/index.tsx
       │
       ▼
3. package.json exports 解析（node_modules 包）
   package.json exports resolution (for node_modules packages)
       │
       ▼
4. 加载并转换 / Load and transform
   - .tsx → esbuild JSX 转换 + TS 剥离
   - .css → PostCSS 处理
   - .json → JSON 解析
```

### 2.16 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| Vite 而非 Webpack | 开发启动快 10-100x，HMR 不随项目增大而变慢 / 10-100x faster dev start |
| esbuild 压缩而非 Terser | 快 10-100x，本项目无需极致压缩率 / 10-100x faster, don't need max compression |
| 单 chunk 输出（无路由分割）| 单页工具体积小（~100KB），无需分割 / Small SPA (~100KB), no splitting needed |
| 代理 /api 而非 CORS | 开发时零配置，与生产部署一致 / Zero config dev, consistent with production |
| test 配置内嵌 vite.config | Vitest 复用 Vite 配置（alias/插件），无重复 / Vitest reuses Vite config |

## 3. 依赖预构建详解 / Dependency Pre-bundling Details

### 3.1 预构建机制 / Pre-bundling Mechanism

```text
┌─────────────────────────────────────────────────────────────┐
│  首次启动 dev server / First dev server start               │
│                                                             │
│  1. 扫描 package.json 依赖 / Scan package.json deps       │
│  2. 使用 esbuild 预构建 / Pre-bundle with esbuild          │
│     - CommonJS → ESM 转换 / CommonJS → ESM conversion     │
│     - 合并多模块为单文件 / Merge multiple modules          │
│     - 例: react + react-dom + scheduler → 1 个文件    │
│  3. 缓存到 node_modules/.vite/ / Cache to .vite/           │
│  4. 后续启动直接使用缓存 / Subsequent starts use cache     │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 预构建解决的问题 / Problems Pre-bundling Solves

| 问题 / Problem | 解决 / Solution |
|---|---|
| CommonJS 不兼容 ESM / CJS incompatible | esbuild 转换为 ESM / Convert to ESM |
| 大量小文件 HTTP 请求 / Many small file requests | 合并为单文件 / Merge into single file |
| node_modules 深层嵌套 / Deep nesting | 扁平化输出 / Flatten output |
| 每次启动重复处理 / Repeat processing | 缓存机制 / Cache mechanism |

### 3.3 缓存与失效 / Cache & Invalidation

```bash
# 缓存位置 / Cache location
node_modules/.vite/deps/

# 缓存失效条件 / Cache invalidation conditions:
# - package.json 依赖变更 / package.json deps changed
# - 锁文件变更 / Lockfile changed
# - vite.config.ts 变更 / vite.config.ts changed

# 手动清除缓存 / Manually clear cache
rm -rf node_modules/.vite
# 或 / Or
npx vite --force  # 强制重新预构建 / Force re-prebundle
```

## 4. Rollup 生产构建详解 / Rollup Production Build Details

### 4.1 构建流程 / Build Flow

```text
┌─────────────────────────────────────────────────────────────┐
│  1. 模块图构建 / Module graph construction                  │
│     从 index.html 入口开始 / Start from index.html entry    │
│     递归解析所有 import / Recursively resolve all imports   │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Tree-shaking                                             │
│     标记未使用的导出 / Mark unused exports                  │
│     删除死代码 / Remove dead code                           │
│     例: 未使用的 lodash 函数不会被打包                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 代码分割 / Code splitting                               │
│     本项目为单 chunk（无动态 import）/ Single chunk         │
│     多页应用可配置 manualChunks / Multi-page can configure  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  4. 压缩 / Minification                                     │
│     esbuild 压缩 JS/CSS / Minify JS/CSS with esbuild       │
│     移除空白、注释、缩短变量名 / Remove whitespace, comments│
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  5. 输出 / Output                                           │
│     内容哈希文件名 / Content-hashed filenames              │
│     index-a1b2c3.js + index-d4e5f6.css                     │
│     支持强缓存 (Cache-Control: max-age=31536000)           │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 构建产物分析 / Build Output Analysis

```bash
# 查看构建产物大小 / Check build output size
cd console/web && pnpm build

# 输出示例 / Example output:
# dist/index.html          0.45 kB │ gzip:  0.30 kB
# dist/assets/index-*.css  8.20 kB │ gzip:  2.50 kB
# dist/assets/index-*.js  95.00 kB │ gzip: 30.00 kB

# 可视化分析（需安装 rollup-plugin-visualizer）
# Visual analysis (requires rollup-plugin-visualizer)
npx vite-bundle-visualizer
```

## 5. CSS 处理管道 / CSS Processing Pipeline

### 5.1 开发模式 CSS / Dev Mode CSS

```text
src/index.css (@tailwind 指令)
    │
    ▼  PostCSS (tailwindcss + autoprefixer)
编译后的 CSS / Compiled CSS
    │
    ▼  注入为 <style> 标签 / Inject as <style> tag
浏览器渲染 / Browser render
    │
    HMR: 修改 CSS 时仅更新 <style>，不刷新页面
    HMR: CSS change only updates <style>, no page reload
```

### 5.2 生产模式 CSS / Production Mode CSS

```text
src/index.css
    │
    ▼  PostCSS (tailwindcss + autoprefixer)
编译后的 CSS / Compiled CSS
    │
    ▼  提取为独立文件 / Extract to standalone file
dist/assets/index-[hash].css
    │
    ▼  esbuild 压缩 / Minify with esbuild
压缩后的 CSS / Minified CSS (~8KB gzip)
    │
    ▼  <link> 标签引用 / Referenced via <link> tag
浏览器加载 / Browser loads
```

## 6. 环境变量 / Environment Variables

### 6.1 Vite 环境变量机制 / Vite Env Variable Mechanism

```bash
# .env 文件（所有模式）/ .env file (all modes)
VITE_API_BASE_URL=http://localhost:8080
VITE_APP_TITLE=Privacy Console

# .env.production（仅生产构建）/ .env.production (production only)
VITE_API_BASE_URL=/api

# 在代码中访问 / Access in code
const apiUrl = import.meta.env.VITE_API_BASE_URL;  // ✅ 可访问
const secret = import.meta.env.SECRET_KEY;          // ❌ 不可访问（无 VITE_ 前缀）
```

### 6.2 安全约束 / Security Constraints

| 规则 / Rule | 说明 / Description |
|---|---|
| 仅 `VITE_` 前缀暴露 / Only `VITE_` prefix exposed | 防止泄露服务端密钥 / Prevent server secret leakage |
| 编译时替换 / Compile-time replacement | 环境变量被内联到代码 / Env vars inlined into code |
| 不包含敏感信息 / No sensitive info | 前端代码可被查看 / Frontend code is viewable |
| 本项目未使用 / Not used in this project | API 地址由后端切换器控制 / API URL controlled by backend switcher |

## 7. 静态资源处理 / Static Asset Handling

### 7.1 资源导入方式 / Asset Import Methods

```typescript
// 方式 1：导入为 URL（小文件内联 base64）/ Import as URL (small files inlined as base64)
import logoUrl from './assets/logo.png';
// logoUrl = "/assets/logo-a1b2c3.png"  (大文件) 或 "data:image/png;base64,..."  (小文件)

// 方式 2：导入为原始字符串 / Import as raw string
import rawContent from './data/config.json?raw';
// rawContent = '{"key": "value"}'  (字符串)

// 方式 3：导入为 Worker / Import as Worker
import MyWorker from './worker.ts?worker';
const worker = new MyWorker();

// 方式 4：public 目录（不经过构建）/ public directory (not processed by build)
// public/favicon.ico → 直接复制，用 "/favicon.ico" 引用
```

### 7.2 资源内联阈值 / Asset Inline Threshold

```typescript
// vite.config.ts
export default defineConfig({
  build: {
    assetsInlineLimit: 4096,  // 默认 4KB，小于此值的资源内联为 base64
    // Default 4KB, assets smaller than this are inlined as base64
  },
});
```

**内联 vs 独立文件的权衡 / Inline vs Separate File Tradeoffs**：

| 方式 / Method | 优势 / Pros | 劣势 / Cons |
|---|---|---|
| 内联 base64 | 减少 HTTP 请求 / Fewer HTTP requests | 增大 JS 体积、无缓存 / Larger JS, no cache |
| 独立文件 | 可强缓存、并行加载 / Cacheable, parallel | 额外 HTTP 请求 / Extra HTTP request |
| 本项目 / This project | SVG 图标内联在 JSX 中 / SVG icons inline in JSX | 无图片资源 / No image assets |

## 8. 代码分割策略 / Code Splitting Strategy

### 8.1 自动代码分割 / Automatic Code Splitting

```text
┌─────────────────────────────────────────────────────────────┐
│  Vite/Rollup 自动分割规则 / Auto-splitting rules               │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. 动态 import() → 独立 chunk / Dynamic import → separate   │
│     const Comp = React.lazy(() => import('./Heavy'))        │
│     → 生成 Heavy-[hash].js                                  │
│                                                             │
│  2. node_modules → vendor chunk / Dependencies → vendor      │
│     react, react-dom → vendor-[hash].js                     │
│                                                             │
│  3. 共享模块 → 公共 chunk / Shared modules → common chunk    │
│     多个入口引用的模块 / Modules referenced by multiple entries│
│                                                             │
│  本项目当前为单入口 SPA，无代码分割需求
│  This project is single-entry SPA, no code splitting needed
└─────────────────────────────────────────────────────────────┘
```

### 8.2 手动分割配置 / Manual Split Configuration

```typescript
// vite.config.ts（本项目未使用，仅供参考）/ Not used, for reference only
export default defineConfig({
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom'],
          'vendor-utils': ['lodash-es', 'dayjs'],
        },
      },
    },
  },
});
```

### 8.3 懒加载与 React.lazy / Lazy Loading with React.lazy

```tsx
// 路由级懒加载（本项目未使用，因为无路由）
// Route-level lazy loading (not used, no routing)
const HeavyPanel = React.lazy(() => import('./components/HeavyPanel'));

function App() {
  return (
    <React.Suspense fallback={<div>加载中...</div>}>
      <HeavyPanel />
    </React.Suspense>
  );
}

// 本项目所有组件同步加载，原因：
// All components loaded synchronously in this project, reasons:
// 1. 组件总量小（~200KB gzip）/ Small total size (~200KB gzip)
// 2. 单页工具，无路由切换 / Single-page tool, no route switching
// 3. 懒加载引入的 loading 状态反而降低体验 / Lazy loading states hurt UX
```

## 9. 构建性能分析 / Build Performance Analysis

### 9.1 构建时间分析 / Build Time Analysis

```bash
# 详细构建日志 / Detailed build log
pnpm build --debug

# 使用 rollup-plugin-visualizer 分析包体积
# Use rollup-plugin-visualizer for bundle analysis
pnpm add -D rollup-plugin-visualizer
```

```typescript
// vite.config.ts
import { visualizer } from 'rollup-plugin-visualizer';

export default defineConfig({
  plugins: [
    react(),
    visualizer({
      filename: 'stats.html',     // 输出可视化报告 / Output visual report
      open: true,                 // 自动打开浏览器 / Auto-open browser
      gzipSize: true,             // 显示 gzip 后大小 / Show gzip size
    }),
  ],
});
```

### 9.2 包体积优化策略 / Bundle Size Optimization

| 策略 / Strategy | 说明 / Description | 本项目状态 / Project Status |
|---|---|---|
| Tree-shaking | 移除未使用导出 / Remove unused exports | ✅ 自动（ESM）|
| 压缩 / Minification | esbuild 压缩 JS/CSS | ✅ 默认启用 |
| 依赖预构建 / Dep pre-bundling | esbuild 合并 CJS→ESM | ✅ 开发模式 |
| 动态导入 / Dynamic import | 按需加载模块 / Load on demand | ❌ 未使用（无需）|
| 外部化 / Externalize | CDN 加载大库 / Load large libs from CDN | ❌ 未使用 |
| 图标内联 / Icon inline | SVG 直接在 JSX 中 / SVG directly in JSX | ✅ 无图标库依赖 |

### 9.3 本项目构建产物分析 / Project Build Output Analysis

```text
console/web/dist/ 典型构建产物 / Typical build output:

index.html           ~0.5 KB    入口 HTML / Entry HTML
assets/index-*.js    ~180 KB    主 JS（gzip ~60KB）/ Main JS
assets/index-*.css   ~15 KB     主 CSS（gzip ~4KB）/ Main CSS

总计约 195 KB（gzip ~64 KB）—— 对于单页工具已足够精简
Total ~195 KB (gzip ~64 KB) -- lean enough for a single-page tool

无额外 vendor chunk，因为依赖少（react + react-dom 已包含在主 bundle）
No extra vendor chunk, few deps (react + react-dom included in main bundle)
```

## 10. 库模式 / Library Mode

### 10.1 库模式概述 / Library Mode Overview

Vite 可以将项目构建为可发布的 npm 库（本项目未使用，仅供了解）：

```typescript
// vite.config.ts (库模式配置 / Library mode config)
import { resolve } from 'path';
import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),  // 库入口 / Library entry
      name: 'MyLib',                               // 全局变量名 / Global var name
      formats: ['es', 'cjs', 'umd'],              // 输出格式 / Output formats
      fileName: (format) => `my-lib.${format}.js`,
    },
    rollupOptions: {
      external: ['react', 'react-dom'],           // 外部化依赖 / Externalize deps
      output: {
        globals: { react: 'React', 'react-dom': 'ReactDOM' },
      },
    },
  },
});
```

### 10.2 库模式 vs 应用模式 / Library Mode vs App Mode

| 维度 / Dimension | 应用模式 / App Mode | 库模式 / Library Mode |
|---|---|---|
| 本项目使用 / Project usage | ✅ 是 / Yes | ❌ 否 / No |
| 入口 / Entry | index.html | 指定的 TS/JS 文件 |
| 依赖处理 / Dependencies | 打包进 bundle | 外部化（peer deps）|
| 输出格式 / Output | 单一 ES module | es/cjs/umd 多格式 |
| Tree-shaking | 应用级 / App-level | 由消费者决定 / Consumer decides |
| CSS 处理 / CSS handling | 提取为文件 / Extract to file | 可内联或提取 / Inline or extract |

## 11. Web Worker 支持 / Web Worker Support

### 11.1 Vite Worker 集成 / Vite Worker Integration

```typescript
// 方式 1：导入为 Worker 构造器 / Import as Worker constructor
import MyWorker from './heavy-compute.ts?worker';
const worker = new MyWorker();
worker.postMessage({ data: largeArray });
worker.onmessage = (e) => console.log(e.data.result);

// 方式 2：内联 Worker（小任务）/ Inline Worker (small tasks)
import { createWorker } from './utils';
const inlineWorker = createWorker(`
  self.onmessage = (e) => {
    self.postMessage(e.data * 2);
  };
`);
```

### 11.2 本项目 Worker 使用情况 / Project Worker Usage

```text
本项目未使用 Web Worker，原因：
This project doesn't use Web Workers, reasons:

1. 所有计算在后端完成（Python agent）/ All computation done backend (Python agent)
2. 前端仅做 UI 渲染 + API 调用 / Frontend only does UI rendering + API calls
3. 无 CPU 密集型前端任务 / No CPU-intensive frontend tasks

潜在使用场景（未来）/ Potential use cases (future):
- 大 JSON 格式化/高亮（>1MB 响应）/ Large JSON formatting (>1MB responses)
- 客户端文件解析（CSV/Parquet 预览）/ Client-side file parsing
- 实时图表数据转换 / Real-time chart data transformation
```

## 12. 设计决策补充 / Additional Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 无代码分割 / No code splitting | 单页工具，组件总量小 / Single-page tool, small total size |
| SVG 内联而非图标库 / SVG inline not icon lib | 避免额外依赖，图标数量少 / Avoid extra dep, few icons |
| 无 PWA / No PWA | 本地开发工具，无需离线 / Local dev tool, no offline need |
| 无 SSR / No SSR | 工具型应用无 SEO 需求 / Tool app has no SEO need |
| 单 bundle 输出 / Single bundle output | 依赖少，无需 vendor 分离 / Few deps, no vendor split needed |
| assetsInlineLimit 默认 / Default inline limit | 无图片资源，配置无影响 / No image assets, config irrelevant |

## 13. HMR 热模块替换机制 / HMR Hot Module Replacement Mechanism

### 13.1 HMR 工作原理 / HMR Working Principles

Vite 的 HMR 基于原生 ESM，仅更新变更模块而无需整页刷新：

```text
HMR 更新流程 / HMR update flow:

文件保存 / File saved
    │
    ▼
┌───────────────────────────────────────────────────────┐
│  Vite Dev Server 检测变更 / Detects change          │
│  - chokidar 监听文件系统 / File system watch       │
│  - 确定变更模块 ID / Identify changed module ID     │
└───────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────┐
│  通过 WebSocket 通知浏览器 / Notify browser via WS  │
│  - 发送 { type: 'update', path, timestamp }        │
│  - 浏览器收到后重新 fetch 变更模块 / Re-fetch module │
└───────────────────────────────────────────────────────┘
    │
    ▼
┌───────────────────────────────────────────────────────┐
│  浏览器执行模块热替换 / Browser executes HMR        │
│  - 调用 import.meta.hot.accept() 回调             │
│  - 替换模块绑定 / Replace module bindings          │
│  - React Fast Refresh 保留组件状态 / Preserve state │
└───────────────────────────────────────────────────────┘
    │
    ▼
  UI 更新（无刷新）/ UI updated (no reload)
```

### 13.2 React Fast Refresh 集成 / React Fast Refresh Integration

```typescript
// @vitejs/plugin-react 自动启用 Fast Refresh
// @vitejs/plugin-react enables Fast Refresh automatically
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [
    react({
      // Fast Refresh 配置 / Fast Refresh options
      fastRefresh: true,  // 默认启用 / Enabled by default
    }),
  ],
});

// Fast Refresh 保留规则 / Fast Refresh preservation rules:
// ✅ 保留：组件内部 state / Preserved: component internal state
// ✅ 保留：自定义 Hook 状态 / Preserved: custom hook state
// ❌ 重置：模块级变量 / Reset: module-level variables
// ❌ 重置：非组件导出 / Reset: non-component exports
```

### 13.3 HMR API 与自定义处理 / HMR API & Custom Handling

```typescript
// 模块内使用 HMR API / Using HMR API within modules
if (import.meta.hot) {
  // 接受自身更新 / Accept self update
  import.meta.hot.accept((newModule) => {
    if (newModule) {
      console.log('模块已更新 / Module updated');
    }
  });

  // 接受依赖更新 / Accept dependency update
  import.meta.hot.accept('./config.ts', (newConfig) => {
    applyConfig(newConfig.default);
  });

  // 清理副作用 / Cleanup side effects
  import.meta.hot.dispose(() => {
    clearInterval(pollingTimer);
    websocket.close();
  });

  // 在更新间传递数据 / Pass data between updates
  import.meta.hot.data.wsConnection = existingConnection;
}

// 本项目中的 HMR 体验 / HMR experience in this project:
// - 修改组件 → 即时更新，保留当前请求状态 / Instant update, preserve request state
// - 修改 CSS → 无刷新样式替换 / No-reload style replacement
// - 修改 API 客户端 → 全页刷新（非组件模块）/ Full reload (non-component)
```

## 14. 插件开发详解 / Plugin Development Details

### 14.1 Vite 插件架构 / Vite Plugin Architecture

```typescript
// Vite 插件基于 Rollup 插件接口扩展 / Vite plugins extend Rollup plugin interface
import type { Plugin } from 'vite';

function myPlugin(): Plugin {
  return {
    name: 'my-plugin',           // 插件名称（必须）/ Plugin name (required)
    enforce: 'pre',              // 执行顺序 / Execution order: pre|post
    apply: 'serve',              // 应用模式 / Apply mode: serve|build

    // === Vite 特有钩子 / Vite-specific hooks ===
    config(config, { command }) {
      // 修改配置 / Modify config
      return { resolve: { alias: { '@': '/src' } } };
    },
    configResolved(resolvedConfig) {
      // 配置确定后 / After config resolved
    },
    configureServer(server) {
      // 自定义服务器行为 / Customize server behavior
      server.middlewares.use((req, res, next) => { next(); });
    },
    transformIndexHtml(html) {
      // 修改 index.html / Modify index.html
      return html.replace('</head>', '<meta name="custom" /></head>');
    },
    handleHotUpdate({ file, server }) {
      // 自定义 HMR 处理 / Custom HMR handling
      if (file.endsWith('.custom')) {
        server.ws.send({ type: 'full-reload' });
        return [];  // 阻止默认 HMR / Prevent default HMR
      }
    },

    // === Rollup 兼容钩子 / Rollup-compatible hooks ===
    resolveId(source) { return null; },   // 解析模块 ID / Resolve module ID
    load(id) { return null; },            // 加载模块内容 / Load module content
    transform(code, id) { return null; }, // 转换代码 / Transform code
  };
}
```

### 14.2 本项目使用的插件 / Plugins Used in This Project

| 插件 / Plugin | 作用 / Purpose | 关键功能 / Key Feature |
|---|---|---|
| `@vitejs/plugin-react` | React 支持 / React support | Fast Refresh + JSX 转换 / JSX transform |
| `tailwindcss` (PostCSS) | CSS 工具类 / CSS utilities | JIT 编译 / JIT compilation |
| `autoprefixer` (PostCSS) | 浏览器前缀 / Browser prefixes | Browserslist 集成 / Integration |

### 14.3 插件执行顺序 / Plugin Execution Order

```text
插件执行管线 / Plugin execution pipeline:

1. alias 解析 / Alias resolution
2. enforce: 'pre' 插件 / Pre plugins
3. Vite 核心插件 / Vite core plugins
4. 普通插件（无 enforce）/ Normal plugins (no enforce)
5. Vite 构建插件 / Vite build plugins
6. enforce: 'post' 插件 / Post plugins
7. Vite 压缩插件 / Vite minify plugins

本项目插件顺序 / This project's plugin order:
  react() → tailwindcss (via PostCSS) → autoprefixer (via PostCSS)
```

## 15. 多页面应用与 SSR / Multi-page App & SSR

### 15.1 多页面应用配置 / Multi-page App Configuration

```typescript
// Vite 支持多页面应用（本项目未使用）/ Multi-page support (not used)
import { resolve } from 'path';

export default defineConfig({
  build: {
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'index.html'),
        admin: resolve(__dirname, 'admin/index.html'),
        docs: resolve(__dirname, 'docs/index.html'),
      },
    },
  },
});

// 本项目为单页面应用 / This project is a single-page app:
// - 仅一个 index.html 入口 / Only one index.html entry
// - 无路由，通过状态切换视图 / No routing, state-based view switching
// - 无需多页面配置 / No multi-page config needed
```

### 15.2 SSR 支持概述 / SSR Support Overview

```typescript
// Vite SSR 架构（本项目未使用）/ Vite SSR architecture (not used)

// server.js — SSR 服务器 / SSR server
import express from 'express';
import { createServer as createViteServer } from 'vite';

async function createServer() {
  const app = express();
  const vite = await createViteServer({
    server: { middlewareMode: true },
    appType: 'custom',
  });
  app.use(vite.middlewares);

  app.use('*', async (req, res) => {
    const template = await vite.transformIndexHtml(
      req.originalUrl, htmlTemplate
    );
    const { render } = await vite.ssrLoadModule('/src/entry-server.tsx');
    const appHtml = await render(req.originalUrl);
    res.status(200).set({ 'Content-Type': 'text/html' })
       .end(template.replace('<!--app-html-->', appHtml));
  });
}
```

### 15.3 本项目不使用 SSR 的原因 / Why This Project Doesn't Use SSR

| 因素 / Factor | 分析 / Analysis | 结论 / Conclusion |
|---|---|---|
| SEO 需求 / SEO need | 本地工具，无搜索引擎索引 / Local tool, no indexing | 无需 SSR |
| 首屏性能 / First paint | 内网访问，延迟极低 / Intranet, very low latency | CSR 足够 |
| 服务器负载 / Server load | 无公共服务器 / No public server | 无需 SSR |
| 复杂度 / Complexity | SSR 增加大量复杂度 / SSR adds significant complexity | 不值得 |
| 交互密集 / Interaction-heavy | 工具型 UI，重交互 / Tool UI, heavy interaction | CSR 更合适 |

## 16. Vite 与 esbuild 协作 / Vite & esbuild Collaboration

### 16.1 esbuild 在 Vite 中的角色 / esbuild's Role in Vite

```text
┌────────────────────────────────────────────────────────────────┐
│  Vite 中 esbuild 的使用场景 / esbuild Usage in Vite            │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  开发模式 / Dev mode:                                          │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ 1. 依赖预构建 / Dependency pre-bundling                  │  │
│  │    - node_modules 包 → 单文件 ESM                      │  │
│  │    - CommonJS → ESM 转换                               │  │
│  │    - 速度：比 Rollup 快 10-100x                        │  │
│  │                                                        │  │
│  │ 2. TypeScript/JSX 转换 / TS/JSX transform              │  │
│  │    - .ts/.tsx → .js (剥离类型 / Strip types)            │  │
│  │    - 每个模块按需转换 / Per-module on-demand            │  │
│  │    - 不做类型检查 / No type checking                   │  │
│  │                                                        │  │
│  │ 3. CSS 压缩 (可选) / CSS minify (optional)              │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                                │
│  生产模式 / Prod mode:                                         │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ 1. TS/JSX 转换 / TS/JSX transform                      │  │
│  │    - 同开发模式 / Same as dev mode                     │  │
│  │                                                        │  │
│  │ 2. 代码压缩 (minify) / Code minification                │  │
│  │    - build.minify: 'esbuild' (默认 / default)          │  │
│  │    - 比 terser 快 20-40x                               │  │
│  │    - 压缩率略低 / Slightly less compression            │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                                │
│  注意：打包(bundle)仍用 Rollup，不用 esbuild                │
│  Note: bundling still uses Rollup, not esbuild                │
└────────────────────────────────────────────────────────────────┘
```

### 16.2 esbuild 配置选项 / esbuild Configuration Options

```typescript
// vite.config.ts - esbuild 相关配置
// esbuild-related configuration
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],

  // esbuild 转换选项 / esbuild transform options
  esbuild: {
    // 保留类名（调试用）/ Keep class names (for debugging)
    keepNames: true,
    // 目标浏览器 / Target browsers
    target: 'es2020',
    // JSX 配置 (通常由 plugin-react 管理)
    // JSX config (usually managed by plugin-react)
    jsx: 'automatic',
  },

  build: {
    // 压缩器选择 / Minifier choice
    minify: 'esbuild',  // 默认 / Default
    // minify: 'terser',  // 替代（更慢但压缩率更高）/ Alt (slower, better ratio)

    // esbuild 压缩选项 / esbuild minify options
    // (仅在 minify: 'esbuild' 时生效 / Only when minify: 'esbuild')
  },

  // 依赖预构建配置 / Dep pre-bundling config
  optimizeDeps: {
    // 强制预构建 / Force pre-bundle
    include: ['react', 'react-dom'],
    // 排除预构建 / Exclude from pre-bundle
    exclude: [],
    esbuildOptions: {
      // 传递给 esbuild 的额外选项 / Extra options for esbuild
      target: 'es2020',
    },
  },
});
```

### 16.3 esbuild vs Rollup 分工 / esbuild vs Rollup Division

| 任务 / Task | 工具 / Tool | 原因 / Reason |
|---|---|---|
| TS/JSX 转换 / Transform | esbuild | 极快（Go 实现）/ Extremely fast (Go) |
| 依赖预构建 / Pre-bundle | esbuild | 开发启动速度 / Dev startup speed |
| 代码压缩 / Minify | esbuild | 速度优先 / Speed priority |
| 生产打包 / Prod bundle | Rollup | 更好的 tree-shaking / Better tree-shaking |
| 代码分割 / Code splitting | Rollup | 更成熟的策略 / More mature strategy |
| 插件生态 / Plugin ecosystem | Rollup | 丰富的插件 / Rich plugins |

### 16.4 本项目 esbuild 实践 / This Project's esbuild Practice

| 配置 / Config | 值 / Value | 说明 / Notes |
|---|---|---|
| minify | esbuild (默认) | 速度优先 / Speed priority |
| target | es2020 | 现代浏览器 / Modern browsers |
| jsx | automatic | React 17+ JSX transform |
| keepNames | false (默认) | 无需保留类名 / No need to keep names |

## 17. 开发服务器代理 / Dev Server Proxy

### 17.1 代理配置详解 / Proxy Configuration Details

```typescript
// vite.config.ts - 开发服务器代理配置
// Dev server proxy configuration
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],

  server: {
    port: 5173,
    // API 代理配置 / API proxy configuration
    proxy: {
      // 所有 /api 请求代理到后端 / All /api requests proxied to backend
      '/api': {
        target: 'http://127.0.0.1:8080',  // Go 后端 / Go backend
        changeOrigin: true,                // 修改 Origin header
        // rewrite: (path) => path.replace(/^\/api/, ''),  // 可选重写
      },
      // 可选：直接代理到 Python Agent
      // Optional: proxy directly to Python Agent
      '/agent': {
        target: 'http://127.0.0.1:8079',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/agent/, ''),
      },
    },
  },
});
```

### 17.2 代理工作原理 / Proxy Working Principle

```text
┌────────────────────────────────────────────────────────────────┐
│  Vite Dev Server 代理流程 / Vite Dev Server Proxy Flow         │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  浏览器 / Browser                                              │
│    │  fetch('/api/mask', { method: 'POST', body: ... })        │
│    ▼                                                           │
│  Vite Dev Server (:5173)                                       │
│    │                                                           │
│    ├── 路径匹配 /api? / Path matches /api?                     │
│    │   ├── 是 / Yes → 代理转发 / Proxy forward                 │
│    │   │   └── http-proxy → http://127.0.0.1:8080/api/mask    │
│    │   └── 否 / No → 返回前端资源 / Serve frontend assets     │
│    │                                                           │
│    ▼                                                           │
│  Go Backend (:8080)                                            │
│    │  处理请求 / Process request                               │
│    ▼                                                           │
│  响应返回浏览器 / Response back to browser                     │
│                                                                │
│  优势 / Advantages:                                            │
│  - 无 CORS 问题（同源）/ No CORS issues (same origin)          │
│  - 前端代码无需知道后端地址 / Frontend doesn't know backend URL │
│  - 生产环境由后端服务静态文件 / Prod: backend serves static     │
└────────────────────────────────────────────────────────────────┘
```

### 17.3 本项目代理实践 / This Project's Proxy Practice

| 环境 / Environment | 前端访问 / Frontend Access | 后端 / Backend | 说明 / Notes |
|---|---|---|---|
| 开发 / Dev | localhost:5173/api/* | Go :8080 | Vite proxy 转发 / Vite proxy forward |
| 生产 / Prod | 同源 / Same origin | Go 服务静态 / Go serves static | 无代理 / No proxy |
| Python 模式 / Python mode | localhost:5173/api/* | Python :8080 | 同样代理 / Same proxy |

## 18. 构建产物分析 / Build Output Analysis

### 18.1 构建输出结构 / Build Output Structure

```text
本项目构建输出 / This project's build output:

console/web/dist/
├── index.html              ← 入口 HTML / Entry HTML (~0.5KB)
├── assets/
│   ├── index-[hash].js     ← 主 JS bundle / Main JS (~180KB gzip)
│   └── index-[hash].css    ← 主 CSS bundle / Main CSS (~15KB gzip)
└── (无其他资源 / No other assets)

分析 / Analysis:
- 单 JS 文件（无代码分割）/ Single JS file (no code splitting)
- 单 CSS 文件（Tailwind 输出）/ Single CSS file (Tailwind output)
- 无图片/字体资源 / No image/font assets
- 总体积小（工具型应用）/ Small total (tool app)
```

### 18.2 产物分析工具 / Output Analysis Tools

```bash
# 1. Vite 内置分析 / Vite built-in analysis
pnpm build
# 输出 / Output:
# dist/index.html          0.46 kB │ gzip:  0.30 kB
# dist/assets/index-abc.css  14.23 kB │ gzip:  3.85 kB
# dist/assets/index-xyz.js  182.45 kB │ gzip: 58.12 kB

# 2. 可视化分析 / Visual analysis
npx vite-bundle-visualizer
# 生成 treemap 图 / Generate treemap chart
# 显示每个包占比 / Show each package proportion

# 3. rollup-plugin-visualizer (更详细)
# vite.config.ts:
# import { visualizer } from 'rollup-plugin-visualizer';
# plugins: [visualizer({ open: true, gzipSize: true })]
```

### 18.3 体积优化策略 / Size Optimization Strategies

| 策略 / Strategy | 效果 / Effect | 本项目 / This Project |
|---|---|---|
| Tree-shaking | 移除未用代码 / Remove unused code | ✅ Rollup 自动 / Auto |
| Tailwind purge | 移除未用 CSS / Remove unused CSS | ✅ content 配置 / content config |
| esbuild minify | 压缩代码 / Compress code | ✅ 默认 / Default |
| 代码分割 / Code split | 分块加载 / Chunk loading | ❌ 不需要 / Not needed |
| 动态导入 / Dynamic import | 懒加载 / Lazy load | ❌ 不需要 / Not needed |
| CDN 外部化 / CDN external | 减少 bundle / Reduce bundle | ❌ 本地工具 / Local tool |

### 18.4 本项目构建决策 / This Project's Build Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 单 bundle 输出 / Single bundle | 依赖少，无需分割 / Few deps, no split needed |
| 无 sourcemap (prod) | 内部工具，无需调试 / Internal tool, no debug needed |
| gzip 报告 / gzip report | 真实传输大小参考 / Real transfer size reference |
| 无 PWA/预加载 / No PWA/preload | 本地工具，无离线需求 / Local tool, no offline need |

---

## 19. Vite 插件开发实践 / Vite Plugin Development Practice

### 19.1 插件结构与钩子 / Plugin Structure & Hooks

Vite 插件基于 Rollup 插件接口，并扩展了 Vite 特有的钩子：

```typescript
// ===== Vite 插件基本结构 / Vite Plugin Basic Structure =====
import type { Plugin, ResolvedConfig } from 'vite';

interface MyPluginOptions {
  prefix?: string;
  debug?: boolean;
}

// 插件工厂函数 / Plugin factory function
export function myPlugin(options: MyPluginOptions = {}): Plugin {
  const { prefix = '[MyPlugin]', debug = false } = options;
  let config: ResolvedConfig;

  return {
    // 插件名称（必须）/ Plugin name (required)
    name: 'vite-plugin-my-plugin',

    // 应用时机 / When to apply
    apply: 'serve',  // 'serve' | 'build' | undefined (both)

    // 强制执行顺序 / Enforce execution order
    enforce: 'pre',  // 'pre' | 'post' | undefined (normal)

    // === Vite 特有钩子 / Vite-specific hooks ===

    // 配置解析前 / Before config resolved
    config(userConfig, env) {
      if (debug) console.log(`${prefix} config:`, env.command);
      // 返回部分配置进行合并 / Return partial config to merge
      return {
        define: {
          __MY_PLUGIN_ENABLED__: JSON.stringify(true),
        },
      };
    },

    // 配置解析后 / After config resolved
    configResolved(resolvedConfig) {
      config = resolvedConfig;
      if (debug) console.log(`${prefix} root:`, config.root);
    },

    // 开发服务器启动 / Dev server startup
    configureServer(server) {
      // 添加自定义中间件 / Add custom middleware
      server.middlewares.use((req, res, next) => {
        if (req.url === '/my-plugin-health') {
          res.end('OK');
          return;
        }
        next();
      });

      // 监听文件变化 / Watch file changes
      server.watcher.on('change', (file) => {
        if (file.endsWith('.custom')) {
          server.ws.send({ type: 'full-reload' });
        }
      });
    },

    // === Rollup 兼容钩子 / Rollup-compatible hooks ===

    // 转换代码 / Transform code
    transform(code, id) {
      if (!id.endsWith('.ts')) return null;

      // 简单的代码注入 / Simple code injection
      if (code.includes('__INJECT_VERSION__')) {
        return code.replace(
          '__INJECT_VERSION__',
          JSON.stringify(config.env?.VITE_APP_VERSION || '0.0.0')
        );
      }
      return null;
    },

    // 构建开始 / Build start
    buildStart() {
      if (debug) console.log(`${prefix} build started`);
    },

    // 构建结束 / Build end
    buildEnd() {
      if (debug) console.log(`${prefix} build ended`);
    },
  };
}
```

### 19.2 实用插件示例 / Practical Plugin Examples

```typescript
// ===== 示例 1: API Mock 插件 / Example 1: API Mock Plugin =====
export function apiMockPlugin(mocks: Record<string, any>): Plugin {
  return {
    name: 'vite-plugin-api-mock',
    apply: 'serve',  // 仅开发模式 / Dev mode only

    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const mockData = mocks[req.url || ''];
        if (mockData && req.method === 'GET') {
          res.setHeader('Content-Type', 'application/json');
          res.end(JSON.stringify(mockData));
          return;
        }
        next();
      });
    },
  };
}

// 使用 / Usage:
// vite.config.ts
export default defineConfig({
  plugins: [
    apiMockPlugin({
      '/api/endpoints': [{ id: 'mask', name: 'Masking' }],
      '/api/health': { status: 'ok' },
    }),
  ],
});


// ===== 示例 2: 构建时间统计 / Example 2: Build Time Stats =====
export function buildTimePlugin(): Plugin {
  let startTime: number;

  return {
    name: 'vite-plugin-build-time',
    apply: 'build',

    buildStart() {
      startTime = performance.now();
    },

    closeBundle() {
      const duration = ((performance.now() - startTime) / 1000).toFixed(2);
      console.log(`\n✨ Build completed in ${duration}s\n`);
    },
  };
}
```

### 19.3 插件执行顺序 / Plugin Execution Order

```
┌─────────────────────────────────────────────────────────────────┐
│         Vite 插件执行顺序 / Plugin Execution Order              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Alias 解析 / Alias resolution                               │
│  2. enforce: 'pre' 插件 / pre plugins                          │
│     └─ vite:react-babel (before)                               │
│  3. Vite 核心插件 / Vite core plugins                          │
│     └─ vite:resolve, vite:html                                 │
│  4. 普通插件 / Normal plugins                                  │
│     └─ 用户插件（无 enforce）/ User plugins (no enforce)       │
│  5. Vite 构建插件 / Vite build plugins                         │
│     └─ vite:esbuild, vite:rollup                               │
│  6. enforce: 'post' 插件 / post plugins                        │
│     └─ vite:import-analysis                                    │
│  7. Vite 后置插件 / Vite post plugins                          │
│     └─ vite:hmr, vite:client-inject                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 20. 多环境配置管理 / Multi-environment Configuration Management

### 20.1 环境变量系统 / Environment Variable System

```bash
# ===== 环境文件 / Environment Files =====

# .env                 ← 所有环境 / All environments
VITE_APP_TITLE=Privacy Console
VITE_API_TIMEOUT=30000

# .env.development     ← 开发环境 / Development
VITE_API_BASE_URL=http://localhost:8079
VITE_ENABLE_MOCK=true
VITE_LOG_LEVEL=debug

# .env.staging         ← 预发布 / Staging
VITE_API_BASE_URL=https://staging-api.internal:8079
VITE_ENABLE_MOCK=false
VITE_LOG_LEVEL=info

# .env.production      ← 生产环境 / Production
VITE_API_BASE_URL=https://api.internal:8079
VITE_ENABLE_MOCK=false
VITE_LOG_LEVEL=warn

# .env.local           ← 本地覆盖（不提交）/ Local override (not committed)
VITE_API_BASE_URL=http://192.168.1.100:8079
```

### 20.2 类型安全的环境变量 / Type-safe Environment Variables

```typescript
// ===== src/vite-env.d.ts =====
/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_APP_TITLE: string;
  readonly VITE_API_BASE_URL: string;
  readonly VITE_API_TIMEOUT: string;
  readonly VITE_ENABLE_MOCK: string;
  readonly VITE_LOG_LEVEL: 'debug' | 'info' | 'warn' | 'error';
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

// ===== src/config/env.ts =====
// 集中管理环境变量 / Centralized env management
export const env = {
  appTitle: import.meta.env.VITE_APP_TITLE,
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL,
  apiTimeout: parseInt(import.meta.env.VITE_API_TIMEOUT, 10),
  enableMock: import.meta.env.VITE_ENABLE_MOCK === 'true',
  logLevel: import.meta.env.VITE_LOG_LEVEL,
  isDev: import.meta.env.DEV,
  isProd: import.meta.env.PROD,
  mode: import.meta.env.MODE,
} as const;

// 使用 / Usage:
// import { env } from '@/config/env';
// fetch(`${env.apiBaseUrl}/api/mask`)
```

### 20.3 条件配置 / Conditional Configuration

```typescript
// ===== vite.config.ts 多环境配置 / Multi-env Config =====
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ command, mode }) => {
  // 加载环境变量 / Load env variables
  const env = loadEnv(mode, process.cwd(), '');

  return {
    plugins: [react()],

    // 根据环境调整配置 / Adjust config by environment
    server: {
      port: mode === 'development' ? 5173 : 4173,
      proxy: {
        '/api': {
          target: env.VITE_API_BASE_URL || 'http://localhost:8079',
          changeOrigin: true,
        },
      },
    },

    build: {
      // 生产环境优化 / Production optimizations
      minify: mode === 'production' ? 'esbuild' : false,
      sourcemap: mode !== 'production',
      rollupOptions: {
        output: {
          // 生产环境添加 hash / Add hash in production
          entryFileNames: mode === 'production'
            ? 'assets/[name].[hash].js'
            : 'assets/[name].js',
        },
      },
    },

    // 定义全局常量 / Define global constants
    define: {
      __APP_VERSION__: JSON.stringify(process.env.npm_package_version),
      __BUILD_TIME__: JSON.stringify(new Date().toISOString()),
    },
  };
});
```

---

## 21. 从 Webpack 迁移到 Vite / Migrating from Webpack to Vite

### 21.1 迁移动机与收益 / Migration Motivation & Benefits

| 指标 / Metric | Webpack | Vite | 改善 / Improvement |
|---|---|---|---|
| 冷启动 / Cold start | 30-60s | <1s | 30-60x 快 / faster |
| HMR 更新 / HMR update | 2-5s | <50ms | 40-100x 快 / faster |
| 生产构建 / Prod build | 60-120s | 10-20s | 5-6x 快 / faster |
| 配置复杂度 / Config complexity | 高 / High | 低 / Low | 显著降低 / Significant |
| 开箱即用 / Out-of-box | 需配置 / Needs config | TS/JSX/CSS | 零配置 / Zero config |

### 21.2 迁移步骤 / Migration Steps

```bash
# ===== 迁移步骤 / Migration Steps =====

# 1. 安装 Vite / Install Vite
pnpm remove webpack webpack-cli webpack-dev-server
pnpm remove babel-loader @babel/core @babel/preset-*
pnpm remove css-loader style-loader postcss-loader
pnpm add -D vite @vitejs/plugin-react

# 2. 创建 Vite 配置 / Create Vite config
# vite.config.ts (见上文 / see above)

# 3. 移动 index.html / Move index.html
# Webpack: public/index.html + HtmlWebpackPlugin
# Vite:    根目录 index.html + <script type="module" src="/src/main.tsx">
mv public/index.html ./index.html

# 4. 更新入口文件 / Update entry file
# Webpack: module.exports = { entry: './src/index.tsx' }
# Vite:    index.html 中直接引用 / Reference directly in index.html

# 5. 替换环境变量 / Replace env variables
# Webpack: process.env.REACT_APP_API_URL
# Vite:    import.meta.env.VITE_API_URL

# 6. 替换 require / Replace require
# Webpack: const config = require('./config.json')
# Vite:    import config from './config.json'

# 7. 处理静态资源 / Handle static assets
# Webpack: import logo from './logo.png' (需要 file-loader)
# Vite:    import logo from './logo.png' (原生支持 / native)

# 8. 更新脚本 / Update scripts
# package.json:
#   "dev": "vite",
#   "build": "vite build",
#   "preview": "vite preview"
```

### 21.3 常见迁移问题 / Common Migration Issues

| 问题 / Issue | Webpack 方式 / Webpack Way | Vite 解决 / Vite Solution |
|---|---|---|
| 环境变量 / Env vars | `process.env.REACT_APP_*` | `import.meta.env.VITE_*` |
| JSON 导入 / JSON import | `require('./data.json')` | `import data from './data.json'` |
| 全局变量 / Global vars | `ProvidePlugin` | `define` 选项 / `define` option |
| 别名 / Aliases | `resolve.alias` | `resolve.alias` (相同 / same) |
| CSS Modules | `css-loader?modules` | 原生支持 / Native support |
| 动态导入 / Dynamic import | `import(/* webpackChunkName */)` | `import()` (原生 / native) |
| 图片优化 / Image optimization | `image-webpack-loader` | `vite-plugin-image-optimizer` |
| Bundle 分析 / Bundle analysis | `webpack-bundle-analyzer` | `rollup-plugin-visualizer` |

### 21.4 本项目无需迁移 / This Project Needs No Migration

本项目从一开始就使用 Vite，无需迁移：

| 因素 / Factor | 状态 / Status | 说明 / Notes |
|---|---|---|
| 项目创建时间 / Created | 2024 | Vite 已成熟 / Vite mature |
| 技术栈 / Stack | React + TS | Vite 最佳支持 / Best support |
| 团队经验 / Team exp | 熟悉 Vite / Familiar | 无学习成本 / No learning cost |
| 构建需求 / Build needs | 简单 / Simple | 无复杂 loader / No complex loaders |

## 22. SSR 与同构渲染 / SSR & Isomorphic Rendering

### 22.1 Vite SSR 架构 / Vite SSR Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Vite SSR 架构                                          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  开发模式 / Dev Mode:                                  │
│  Browser ──▶ Vite Dev Server ──▶ SSR 渲染 ──▶ HTML    │
│              (HMR + Transform)   (Node.js)              │
│                                                         │
│  生产模式 / Prod Mode:                                 │
│  Browser ──▶ Node Server ──▶ 预渲染 HTML + 水合       │
│              (express)       (hydrate)                  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 22.2 基本 SSR 服务器 / Basic SSR Server

```typescript
// server.ts — Vite SSR 服务器
// server.ts — Vite SSR server
import express from 'express'
import { createServer as createViteServer } from 'vite'

async function createServer() {
  const app = express()
  
  // 创建 Vite 服务器（中间件模式）
  // Create Vite server (middleware mode)
  const vite = await createViteServer({
    server: { middlewareMode: true },
    appType: 'custom',  // 不使用 Vite 内置 HTML 处理
  })
  
  app.use(vite.middlewares)
  
  app.use('*', async (req, res) => {
    const url = req.originalUrl
    
    try {
      // 1. 读取并转换 HTML 模板
      // 1. Read and transform HTML template
      let template = await vite.transformIndexHtml(
        url,
        '<div id="app"><!--ssr-outlet--></div>'
      )
      
      // 2. 加载 SSR 入口模块
      // 2. Load SSR entry module
      const { render } = await vite.ssrLoadModule('/src/entry-server.tsx')
      
      // 3. 渲染应用 HTML
      // 3. Render app HTML
      const appHtml = await render(url)
      
      // 4. 注入并返回
      // 4. Inject and return
      const html = template.replace('<!--ssr-outlet-->', appHtml)
      res.status(200).set({ 'Content-Type': 'text/html' }).end(html)
    } catch (e) {
      vite.ssrFixStacktrace(e as Error)
      res.status(500).end((e as Error).message)
    }
  })
  
  app.listen(3000)
}

createServer()
```

### 22.3 SSR vs CSR vs SSG 对比 / SSR vs CSR vs SSG Comparison

| 特性 / Feature | CSR (SPA) | SSR | SSG |
|---|---|---|---|
| 首屏速度 / FCP | 慢 / Slow | 快 / Fast | 最快 / Fastest |
| SEO 友好 / SEO | ✘ | ✅ | ✅ |
| 服务器负载 / Server load | 低 / Low | 高 / High | 无 / None |
| 动态内容 / Dynamic | ✅ | ✅ | ✘ |
| 复杂度 / Complexity | 低 / Low | 高 / High | 中 / Medium |
| 本项目选择 / Project choice | ✅ | ✘ | ✘ |

## 23. 构建分析与优化 / Build Analysis & Optimization

### 23.1 Bundle 分析工具 / Bundle Analysis Tools

```typescript
// vite.config.ts — 集成 bundle 分析
// vite.config.ts — Integrate bundle analysis
import { defineConfig } from 'vite'
import { visualizer } from 'rollup-plugin-visualizer'

export default defineConfig({
  plugins: [
    // 生成 bundle 可视化报告
    // Generate bundle visualization report
    visualizer({
      filename: 'dist/stats.html',
      open: true,           // 自动打开 / Auto open
      gzipSize: true,       // 显示 gzip 大小 / Show gzip size
      brotliSize: true,     // 显示 brotli 大小 / Show brotli size
    }),
  ],
})
```

```bash
# 构建并分析
# Build and analyze
pnpm build
# 生成 dist/stats.html — 可视化 treemap

# 查看各 chunk 大小
# View chunk sizes
ls -la dist/assets/
# index-abc123.js    142KB  (gzip: 45KB)
# vendor-def456.js    89KB  (gzip: 28KB)
# index-ghi789.css    12KB  (gzip: 3KB)
```

### 23.2 代码分割策略 / Code Splitting Strategy

```typescript
// vite.config.ts — 手动分割配置
// vite.config.ts — Manual split configuration
export default defineConfig({
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          // React 核心单独分包
          // React core in separate chunk
          'react-vendor': ['react', 'react-dom'],
          // UI 库单独分包
          // UI library in separate chunk
          'ui-vendor': ['@headlessui/react', '@heroicons/react'],
        },
      },
    },
    // chunk 大小警告阈值
    // Chunk size warning threshold
    chunkSizeWarningLimit: 500,  // KB
  },
})
```

### 23.3 Tree Shaking 优化 / Tree Shaking Optimization

```typescript
// ✔ 命名导入（可 tree-shake）
// ✔ Named imports (tree-shakeable)
import { debounce, throttle } from 'lodash-es'

// ✘ 默认导入（无法 tree-shake）
// ✘ Default import (cannot tree-shake)
// import _ from 'lodash'  // 导入整个库 / Imports entire library

// ✔ 按需导入图标
// ✔ Import icons on demand
import { ShieldIcon, LockIcon } from '@heroicons/react/24/solid'

// ✘ 导入所有图标
// ✘ Import all icons
// import * as Icons from '@heroicons/react/24/solid'
```

### 23.4 构建优化检查清单 / Build Optimization Checklist

| 优化项 / Optimization | 效果 / Effect | 配置 / Config |
|---|---|---|
| 代码分割 / Code split | 首屏加载 -40% | manualChunks |
| Tree shaking | 移除死代码 / Remove dead code | ESM imports |
| CSS 提取 / CSS extract | 并行加载 / Parallel load | 默认开启 / On by default |
| 压缩 / Minify | 体积 -60% | esbuild (default) |
| 图片优化 / Image opt | 体积 -50% | vite-plugin-image-optimizer |
| 预加载 / Preload | LCP 改善 / Better LCP | modulepreload |

## 24. 模块联邦与微前端 / Module Federation & Micro-frontends

### 24.1 模块联邦概念 / Module Federation Concept

```
┌─────────────────────────────────────────────────────────┐
│  模块联邦架构 / Module Federation Architecture          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌──────────────┐     ┌──────────────┐              │
│  │  Host App    │     │  Remote A    │              │
│  │  (主应用)     │────▶│  (脱敏模块)   │              │
│  │              │     └──────────────┘              │
│  │              │     ┌──────────────┐              │
│  │              │────▶│  Remote B    │              │
│  │              │     │  (DP 模块)    │              │
│  └──────────────┘     └──────────────┘              │
│                                                         │
│  • 运行时加载远程模块 / Runtime load remote modules   │
│  • 共享依赖去重 / Shared deps dedup                    │
│  • 独立部署 / Independent deployment                   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 24.2 Vite 模块联邦插件 / Vite Module Federation Plugin

```typescript
// vite.config.ts — 主应用 (Host)
// vite.config.ts — Host app
import { federation } from '@module-federation/vite'

export default defineConfig({
  plugins: [
    federation({
      name: 'host_app',
      remotes: {
        masking_module: 'http://localhost:3001/assets/remoteEntry.js',
        dp_module: 'http://localhost:3002/assets/remoteEntry.js',
      },
      shared: ['react', 'react-dom'],  // 共享依赖 / Shared deps
    }),
  ],
})

// vite.config.ts — 远程模块 (Remote)
// vite.config.ts — Remote module
export default defineConfig({
  plugins: [
    federation({
      name: 'masking_module',
      filename: 'remoteEntry.js',
      exposes: {
        './MaskPage': './src/features/masking/MaskPage.tsx',
      },
      shared: ['react', 'react-dom'],
    }),
  ],
})
```

### 24.3 微前端 vs 单体对比 / Micro-frontend vs Monolith

| 特性 / Feature | 单体 SPA | 模块联邦 | iframe |
|---|---|---|---|
| 开发体验 / DX | ✅ 简单 | 中 / Medium | ✘ 差 |
| 独立部署 / Deploy | ✘ 整体 | ✅ 独立 | ✅ 独立 |
| 性能 / Performance | ✅ 快 | 中 / Medium | ✘ 慢 |
| 样式隔离 / Style isolation | ✘ | 中 / Medium | ✅ 完全 |
| 通信 / Communication | ✅ 直接 | 中 / Medium | ✘ postMessage |
| 适用规模 / Scale | 小-中 / S-M | 大 / Large | 超大 / XL |
| 本项目选择 / Project | ✅ | ✘ | ✘ |
