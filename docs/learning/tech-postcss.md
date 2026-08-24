# PostCSS 技术栈说明 / PostCSS Technology Stack

## 1. 技术简介 / Introduction

PostCSS 是一个用 JavaScript 插件转换 CSS 的工具平台，本身不做任何转换，所有功能由插件提供。
PostCSS is a tool platform for transforming CSS with JavaScript plugins. It does nothing by itself; all functionality comes from plugins.

核心特性 / Core Features：
- **插件化架构（Plugin Architecture）**：每个功能独立为插件，按需组合。
- **AST 级别操作（AST-level）**：将 CSS 解析为抽象语法树，插件可精确操作任意节点。
- **Source Map 支持**：转换后仍保持源码映射，便于调试。
- **与构建工具集成**：Vite、Webpack、Rollup 等均内置 PostCSS 管道。
- **高性能（High Performance）**：基于 Rust 实现的 Lightning CSS 可选替代，但 PostCSS 生态更成熟。

本项目使用的插件 / Plugins Used：
- **tailwindcss**：将 Tailwind 工具类编译为实际 CSS / Compiles Tailwind utilities to actual CSS
- **autoprefixer**：自动添加浏览器厂商前缀 / Auto-adds browser vendor prefixes

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 配置文件 / Configuration File

文件 / File：`console/web/postcss.config.js`

```javascript
/**
 * PostCSS 配置文件 / PostCSS configuration file
 *
 * Vite 在构建时自动加载此配置，无需手动调用。
 * Vite automatically loads this config during build, no manual invocation needed.
 */
export default {
  plugins: {
    tailwindcss: {},   // Tailwind CSS 编译插件 / Tailwind CSS compilation plugin
    autoprefixer: {},  // 浏览器前缀自动补全 / Browser prefix auto-completion
  },
}
```

### 2.2 构建管道中的位置 / Position in Build Pipeline

```
源文件 / Source Files                    构建产物 / Build Artifacts
─────────────────                        ────────────────────────
index.css (@tailwind 指令)               
    │                                    
    ▼                                    
[Vite CSS Pipeline]                      
    │                                    
    ├─▶ PostCSS                          
    │     ├─▶ tailwindcss 插件           
    │     │     扫描 src/**/*.{tsx,ts}   
    │     │     提取 class="..." 类名    
    │     │     生成对应 CSS 规则        
    │     │                              
    │     └─▶ autoprefixer 插件          
    │           根据 browserslist        
    │           添加 -webkit-/-moz- 前缀 
    │                                    
    ▼                                    
dist/assets/index-[hash].css             
(压缩 + 树摇后的最终 CSS)                
```

### 2.3 Tailwind 插件工作原理 / Tailwind Plugin Working Principle

```css
/* 输入：index.css 中的 Tailwind 指令 / Input: Tailwind directives in index.css */
@tailwind base;       /* 注入 Preflight 重置样式 / Inject Preflight reset styles */
@tailwind components; /* 注入 @layer components 自定义组件 / Inject custom components */
@tailwind utilities;  /* 注入按需生成的工具类 / Inject on-demand generated utilities */

/* 输出：编译后的实际 CSS / Output: compiled actual CSS */
*, ::before, ::after { box-sizing: border-box; border-width: 0; ... }
.flex { display: flex; }
.h-screen { height: 100vh; }
.bg-gray-50 { --tw-bg-opacity: 1; background-color: rgb(249 250 251 / var(--tw-bg-opacity)); }
/* ... 仅包含项目中实际使用的类 / Only includes classes actually used in project */
```

### 2.4 Autoprefixer 插件作用 / Autoprefixer Plugin Purpose

```css
/* 输入 / Input */
.backdrop-blur { backdrop-filter: blur(8px); }

/* 输出（根据 browserslist 目标浏览器）/ Output (based on browserslist targets) */
.backdrop-blur {
  -webkit-backdrop-filter: blur(8px);  /* Safari 兼容 / Safari compat */
  backdrop-filter: blur(8px);
}
```

### 2.5 与 Vite 的集成方式 / Integration with Vite

Vite 内置 PostCSS 支持，自动按以下优先级查找配置：
Vite has built-in PostCSS support, auto-discovers config by priority:

1. `postcss.config.js` / `postcss.config.mjs`（本项目使用）/ This project uses
2. `package.json` 中的 `postcss` 字段 / `postcss` field in package.json
3. `vite.config.ts` 中的 `css.postcss` 字段 / `css.postcss` in vite.config.ts

无需在 `vite.config.ts` 中额外配置，Vite 自动发现并应用 `postcss.config.js`。
No extra config needed in `vite.config.ts`; Vite auto-discovers and applies `postcss.config.js`.

### 2.6 PostCSS AST 解析机制 / PostCSS AST Parsing Mechanism

PostCSS 将 CSS 解析为抽象语法树（AST），插件通过操作 AST 节点实现转换：

```text
CSS 源码 / CSS Source
    │
    ▼  PostCSS Parser
┌─────────────────────────────────────────────────┐
│  Root                                           │
│  ├── AtRule (@tailwind base)                    │
│  ├── AtRule (@tailwind components)              │
│  ├── AtRule (@tailwind utilities)               │
│  └── Rule (.custom-class)                       │
│       ├── Selector: ".custom-class"             │
│       └── Declaration: color: #333              │
│            ├── prop: "color"                    │
│            └── value: "#333"                    │
└─────────────────────────────────────────────────┘
    │
    ▼  插件转换 / Plugin Transform
┌─────────────────────────────────────────────────┐
│  修改/添加/删除 AST 节点                       │
│  Modify/Add/Remove AST nodes                    │
└─────────────────────────────────────────────────┘
    │
    ▼  PostCSS Stringifier
最终 CSS 输出 / Final CSS Output
```

**AST 节点类型 / AST Node Types**：

| 节点类型 / Node Type | 对应 CSS / CSS Equivalent | 示例 / Example |
|---|---|---|
| `Root` | 整个文件 / Entire file | 根节点 |
| `AtRule` | `@` 规则 / @ rules | `@tailwind`, `@media`, `@keyframes` |
| `Rule` | 选择器 + 声明块 / Selector + declarations | `.flex { display: flex }` |
| `Declaration` | 属性: 值 / Property: value | `color: #333` |
| `Comment` | 注释 / Comment | `/* ... */` |

### 2.7 插件 API 与执行顺序 / Plugin API & Execution Order

```javascript
// PostCSS 插件结构（简化）/ PostCSS plugin structure (simplified)
const myPlugin = () => ({
  postcssPlugin: 'my-plugin',
  // 访问器钩子（按 AST 节点类型）/ Visitor hooks (by AST node type)
  AtRule(atRule) {
    // 处理 @tailwind 指令 / Handle @tailwind directives
    if (atRule.name === 'tailwind') { /* ... */ }
  },
  Declaration(decl) {
    // 处理每个属性声明 / Process each declaration
    if (decl.prop === 'backdrop-filter') {
      decl.cloneBefore({ prop: '-webkit-backdrop-filter' });
    }
  },
  OnceExit(root) {
    // 所有节点处理完毕后 / After all nodes processed
  },
});

// 执行顺序：按 plugins 数组顺序串行执行
// Execution order: serial by plugins array order
plugins: {
  tailwindcss: {},   // 1️⃣ 先执行：生成实际 CSS / First: generate actual CSS
  autoprefixer: {},  // 2️⃣ 后执行：添加前缀 / Second: add prefixes
}
```

**顺序重要性 / Order Importance**：

```text
✗ 错误顺序 / Wrong order:
  autoprefixer → tailwindcss
  （autoprefixer 处理的是 @tailwind 指令，而非生成的 CSS）

✓ 正确顺序 / Correct order:
  tailwindcss → autoprefixer
  （tailwindcss 先生成实际 CSS，autoprefixer 再添加前缀）
```

### 2.8 性能特征 / Performance Characteristics

| 阶段 / Phase | 耗时占比 / Time % | 说明 / Description |
|---|---|---|
| CSS 解析 / Parsing | ~5% | 将源码解析为 AST |
| Tailwind 扫描 / Scanning | ~60% | 扫描所有源文件提取类名 |
| CSS 生成 / Generation | ~25% | 根据类名生成对应规则 |
| Autoprefixer | ~5% | 添加浏览器前缀 |
| 压缩 / Minification | ~5% | Vite 内置 esbuild 压缩 |

**优化策略 / Optimization Strategies**：

```javascript
// tailwind.config.js 中的 content 配置直接影响扫描性能
// content config in tailwind.config.js directly affects scan performance
content: [
  "./index.html",               // 仅扫描必要文件 / Only scan necessary files
  "./src/**/*.{js,ts,jsx,tsx}", // 避免扫描 node_modules / Avoid scanning node_modules
]
// ✗ 避免: "./**/*"  // 这会扫描所有文件，极度降低性能
// ✗ Avoid: "./**/*"  // This scans all files, extremely slow
```

### 2.9 调试与问题排查 / Debugging & Troubleshooting

| 问题 / Issue | 原因 / Cause | 解决 / Solution |
|---|---|---|
| 样式不生效 / Styles not applied | content 未包含目标文件 / content misses target file | 检查 tailwind.config.js content 路径 |
| 前缀缺失 / Missing prefixes | browserslist 配置过新 / browserslist too modern | 检查 package.json browserslist 字段 |
| 构建缓慢 / Slow build | content 范围过大 / content scope too broad | 缩小扫描范围，排除 node_modules |
| 样式被覆盖 / Styles overridden | CSS 优先级冲突 / CSS specificity conflict | 使用 `!` 前缀或调整类名顺序 |

**查看生成的 CSS / Inspect generated CSS**：

```bash
# 开发模式：在浏览器 DevTools 中查看生成的样式
# Dev mode: inspect generated styles in browser DevTools
# 生产构建：查看 dist/assets/index-*.css
# Production: inspect dist/assets/index-*.css
cat console/web/dist/assets/index-*.css | head -50
```

### 2.10 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| PostCSS + Tailwind 而非 CSS-in-JS | 构建时生成，零运行时开销 / Build-time generation, zero runtime cost |
| autoprefixer 而非手写前缀 | 自动化、跟随 browserslist 更新 / Automated, follows browserslist updates |
| ESM 配置格式（export default）| 与 Vite 的 ESM 生态一致 / Consistent with Vite's ESM ecosystem |
| 不添加 cssnano 插件 | Vite 生产构建已内置 CSS 压缩 / Vite production build has built-in CSS minification |
| 插件顺序 tailwind → autoprefixer | 先生成再添加前缀，确保前缀覆盖所有生成的规则 / Generate first then prefix |

## 3. Source Map 与调试支持 / Source Map & Debugging Support

### 3.1 Source Map 工作原理 / Source Map Working Principle

PostCSS 在转换 CSS 时自动生成 Source Map，使浏览器 DevTools 能定位到原始源码位置：

```text
┌─────────────────────────────────────────────────────────────┐
│  源文件 / Source File                                        │
│  src/index.css (含 @tailwind 指令)                          │
└──────────────────────────┬──────────────────────────────────┘
                           │  PostCSS 转换 + Source Map 生成
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  输出 / Output                                               │
│  dist/assets/index-[hash].css                                │
│  dist/assets/index-[hash].css.map  ← Source Map 文件        │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼  浏览器 DevTools
┌─────────────────────────────────────────────────────────────┐
│  开发者看到 / Developer sees:                                │
│  - 原始文件名 (index.css)                                   │
│  - 原始行号 (line 3: @tailwind utilities)                   │
│  - 而非压缩后的单行 CSS                                    │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Vite 中的 Source Map 配置 / Source Map Config in Vite

```typescript
// vite.config.ts 中控制 CSS Source Map / Control CSS Source Map in vite.config.ts
export default defineConfig({
  css: {
    devSourcemap: true,  // 开发模式启用 CSS Source Map / Enable CSS Source Map in dev
  },
  build: {
    sourcemap: false,    // 生产模式关闭（减小产物体积）/ Disabled in prod (reduce size)
  },
})
```

**Source Map 格式说明 / Source Map Format**：

| 字段 / Field | 含义 / Meaning |
|---|---|
| `version` | Source Map 规范版本（固定为 3）/ Spec version (always 3) |
| `sources` | 原始文件路径数组 / Original file path array |
| `mappings` | VLQ 编码的位置映射 / VLQ-encoded position mappings |
| `names` | 原始标识符名 / Original identifier names |
| `file` | 生成文件名 / Generated file name |

### 3.3 开发模式调试流程 / Dev Mode Debugging Flow

```text
1. 修改 src/index.css 或组件 className
   Modify src/index.css or component className
        │
2. Vite HMR 触发 PostCSS 重新编译
   Vite HMR triggers PostCSS recompilation
        │
3. 生成新 CSS + 内联 Source Map（base64 data URI）
   Generate new CSS + inline Source Map (base64 data URI)
        │
4. 浏览器 DevTools → Sources 面板 → 定位原始文件
   Browser DevTools → Sources panel → locate original file
        │
5. 修改即时生效，无需手动刷新
   Changes take effect instantly, no manual refresh
```

## 4. Browserslist 集成详解 / Browserslist Integration Details

### 4.1 Browserslist 配置方式 / Browserslist Configuration

Browserslist 决定 autoprefixer 添加哪些浏览器前缀：

```text
配置优先级（从高到低）/ Config priority (high to low):
  1. browserslist 配置文件 (.browserslistrc)
  2. package.json 中的 "browserslist" 字段
  3. 默认值："> 0.5%, last 2 versions, Firefox ESR, not dead"
```

```json
// package.json 中配置示例 / Example in package.json
{
  "browserslist": [
    "> 1%",              // 全球使用率 > 1% 的浏览器 / Browsers with > 1% global usage
    "last 2 versions",   // 每个浏览器最近 2 个版本 / Last 2 versions of each browser
    "not dead",          // 排除已停止维护的浏览器 / Exclude dead browsers
    "not IE 11"          // 明确排除 IE 11 / Explicitly exclude IE 11
  ]
}
```

### 4.2 前缀生成规则 / Prefix Generation Rules

autoprefixer 根据 Browserslist 目标浏览器查询 [caniuse.com](https://caniuse.com) 数据库：

```text
┌─────────────────────────────────────────────────────────────┐
│  Browserslist 查询 / Browserslist query                      │
│  "> 1%, last 2 versions, not dead"                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  caniuse-lite 数据库查询 / caniuse-lite database query       │
│  - backdrop-filter: Safari 需要 -webkit- (至 v18)           │
│  - user-select: 所有浏览器需要前缀                          │
│  - grid: IE 需要 -ms- (若目标含 IE)                        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  输出 CSS / Output CSS                                       │
│  仅添加目标浏览器实际需要的前缀                            │
│  Only add prefixes actually needed by target browsers        │
└─────────────────────────────────────────────────────────────┘
```

### 4.3 常见前缀场景 / Common Prefix Scenarios

| CSS 属性 / CSS Property | 前缀 / Prefix | 需要浏览器 / Browsers Needing |
|---|---|---|
| `backdrop-filter` | `-webkit-` | Safari (all versions) |
| `user-select` | `-webkit-`, `-moz-`, `-ms-` | Chrome, Firefox, IE/Edge |
| `appearance` | `-webkit-`, `-moz-` | Chrome, Firefox |
| `mask` | `-webkit-` | Chrome, Safari |
| `hyphens` | `-webkit-`, `-ms-` | Chrome, IE/Edge |
| `text-size-adjust` | `-webkit-`, `-moz-`, `-ms-` | Mobile browsers |

### 4.4 查看当前目标浏览器 / Check Current Target Browsers

```bash
# 查看项目实际匹配的浏览器列表 / View actual matched browser list
cd console/web && npx browserslist

# 输出示例 / Example output:
# and_chr 125
# chrome 125
# edge 125
# firefox 126
# safari 17.4
# ...
```

## 5. CSS 嵌套与现代特性 / CSS Nesting & Modern Features

### 5.1 原生 CSS 嵌套（PostCSS 支持）/ Native CSS Nesting

PostCSS 可配合 `postcss-nesting` 插件支持 CSS 嵌套语法（本项目使用 Tailwind 无需此插件）：

```css
/* 嵌套语法（需 postcss-nesting 插件）/ Nesting syntax (requires postcss-nesting) */
.card {
  padding: 1rem;
  & .card-title {      /* 子选择器 / Child selector */
    font-weight: bold;
  }
  &:hover {            /* 伪类嵌套 / Pseudo-class nesting */
    box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
  }
  @media (min-width: 768px) {  /* 媒体查询嵌套 / Media query nesting */
    padding: 2rem;
  }
}

/* 编译输出 / Compiled output */
.card { padding: 1rem; }
.card .card-title { font-weight: bold; }
.card:hover { box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1); }
@media (min-width: 768px) { .card { padding: 2rem; } }
```

### 5.2 本项目为何不需要 CSS 嵌套 / Why This Project Doesn't Need CSS Nesting

| 原因 / Reason | 说明 / Description |
|---|---|
| Tailwind 工具类替代 / Tailwind utilities replace | 样式直接写在 className 中，无需嵌套 / Styles in className, no nesting needed |
| 无自定义 CSS 文件 / No custom CSS files | 仅 index.css 含 @tailwind 指令 / Only index.css with @tailwind directives |
| 减少插件依赖 / Reduce plugin deps | 更少的 PostCSS 插件 = 更快的构建 / Fewer plugins = faster build |

### 5.3 CSS 自定义属性（变量）/ CSS Custom Properties (Variables)

Tailwind 生成的 CSS 大量使用 CSS 变量实现主题化：

```css
/* Tailwind 生成的 CSS 变量 / Tailwind generated CSS variables */
:root {
  --tw-bg-opacity: 1;
  --tw-text-opacity: 1;
  --tw-shadow: 0 0 #0000;
  --tw-ring-offset-width: 0px;
  --tw-ring-color: rgb(59 130 246 / 0.5);
}

/* 工具类引用变量 / Utilities reference variables */
.bg-indigo-600 {
  background-color: rgb(79 70 229 / var(--tw-bg-opacity));
}
.shadow-md {
  box-shadow: var(--tw-ring-offset-shadow, 0 0 #0000),
              var(--tw-ring-shadow, 0 0 #0000),
              var(--tw-shadow);
}
```

## 6. PostCSS 插件生态 / PostCSS Plugin Ecosystem

### 6.1 常用插件分类 / Common Plugin Categories

| 类别 / Category | 插件 / Plugin | 功能 / Function | 本项目 / This Project |
|---|---|---|---|
| 框架 / Framework | tailwindcss | 工具类 CSS 生成 / Utility CSS generation | ✅ 使用 |
| 兼容性 / Compat | autoprefixer | 浏览器前缀 / Browser prefixes | ✅ 使用 |
| 压缩 / Minify | cssnano | CSS 压缩优化 / CSS minification | ❌ Vite 内置 |
| 嵌套 / Nesting | postcss-nesting | CSS 嵌套语法 / CSS nesting syntax | ❌ 不需要 |
| 导入 / Import | postcss-import | @import 内联 / @import inlining | ❌ 不需要 |
| 前缀变量 / Preset | postcss-preset-env | 未来 CSS 语法降级 / Future CSS syntax downgrade | ❌ 不需要 |
| 排序 / Sorting | postcss-sorting | 属性排序 / Property sorting | ❌ 不需要 |
| 单位转换 / Units | postcss-pxtorem | px → rem 转换 / px → rem conversion | ❌ 不需要 |

### 6.2 插件选择决策树 / Plugin Selection Decision Tree

```text
需要 PostCSS 插件吗？/ Need a PostCSS plugin?
    │
    ├── 使用 Tailwind CSS？/ Using Tailwind CSS?
    │   └── 是 → 添加 tailwindcss 插件 / Yes → add tailwindcss plugin
    │
    ├── 需要支持旧浏览器？/ Need to support old browsers?
    │   └── 是 → 添加 autoprefixer / Yes → add autoprefixer
    │
    ├── 使用 Vite 构建？/ Using Vite build?
    │   ├── 需要压缩？→ 不需要，Vite 内置 esbuild 压缩 / No, Vite built-in
    │   └── 需要 @import？→ 不需要，Vite 内置处理 / No, Vite handles
    │
    └── 使用 CSS-in-JS？/ Using CSS-in-JS?
        └── 是 → 通常不需要 PostCSS / Yes → usually no PostCSS needed
```

### 6.3 插件开发规范 / Plugin Development Standards

```javascript
// PostCSS 8.x 插件标准结构 / PostCSS 8.x plugin standard structure
/** @type {import('postcss').PluginCreator} */
const pluginCreator = (opts = {}) => {
  // 1. 插件选项处理 / Plugin options handling
  const options = { prefix: 'my-', ...opts };

  return {
    // 2. 必须声明 postcssPlugin 名称 / Must declare postcssPlugin name
    postcssPlugin: 'postcss-my-plugin',

    // 3. 访问器钩子 / Visitor hooks
    Declaration(decl, { result }) {
      // 处理每个声明 / Process each declaration
    },

    // 4. 一次性钩子 / Once hooks
    Once(root, { result }) {
      // 整个文件处理一次 / Process entire file once
    },

    // 5. 退出钩子 / Exit hooks
    OnceExit(root) {
      // 所有处理完成后 / After all processing done
    },
  };
};

// 6. 必须声明为 PostCSS 插件 / Must declare as PostCSS plugin
pluginCreator.postcss = true;
module.exports = pluginCreator;
```

## 7. 与 Lightning CSS 对比 / Comparison with Lightning CSS

### 7.1 技术对比 / Technical Comparison

| 维度 / Dimension | PostCSS | Lightning CSS |
|---|---|---|
| 实现语言 / Implementation | JavaScript | Rust |
| 性能 / Performance | 基准 / Baseline | 快 10-100x / 10-100x faster |
| 插件生态 / Plugin ecosystem | 极丰富（200+ 插件）/ Very rich | 有限（内置功能为主）/ Limited |
| Tailwind 支持 / Tailwind support | ✅ 官方集成 / Official | ⚠️ 实验性 / Experimental |
| Source Map | ✅ 完善 / Complete | ✅ 支持 / Supported |
| CSS 嵌套 / CSS nesting | 需插件 / Needs plugin | ✅ 内置 / Built-in |
| 压缩 / Minification | 需 cssnano | ✅ 内置 / Built-in |
| 浏览器目标 / Browser targets | 通过 browserslist | 内置 targets 配置 |
| 成熟度 / Maturity | 非常成熟 / Very mature | 较新 / Newer |

### 7.2 本项目选择 PostCSS 的原因 / Why This Project Chooses PostCSS

| 原因 / Reason | 说明 / Description |
|---|---|
| Tailwind 官方集成 / Tailwind official integration | Tailwind CSS 通过 PostCSS 插件工作 / Tailwind works as PostCSS plugin |
| Vite 默认支持 / Vite default support | Vite 内置 PostCSS 管道，零配置 / Vite built-in PostCSS pipeline, zero config |
| 生态成熟 / Mature ecosystem | autoprefixer 等插件久经考验 / Battle-tested plugins like autoprefixer |
| 构建性能已足够 / Build performance sufficient | 本项目 CSS 量小，PostCSS 速度不是瓶颈 / Small CSS volume, speed not bottleneck |

## 8. 常见问题与排查 / Common Issues & Troubleshooting

### 8.1 完整排查清单 / Complete Troubleshooting Checklist

```text
样式不生效排查流程 / Style not working troubleshooting flow:

1. 检查 tailwind.config.js content 是否包含目标文件
   Check if tailwind.config.js content includes target file
        │
2. 检查类名拼写是否正确（区分大小写）
   Check class name spelling (case-sensitive)
        │
3. 检查是否被其他样式覆盖（DevTools → Computed 面板）
   Check if overridden by other styles (DevTools → Computed panel)
        │
4. 检查 @tailwind 指令是否存在于入口 CSS
   Check if @tailwind directives exist in entry CSS
        │
5. 检查 PostCSS 配置是否被正确加载（Vite 日志）
   Check if PostCSS config is loaded correctly (Vite logs)
        │
6. 清除缓存重试：rm -rf node_modules/.vite && pnpm dev
   Clear cache and retry: rm -rf node_modules/.vite && pnpm dev
```

### 8.2 构建产物分析 / Build Output Analysis

```bash
# 查看生成的 CSS 文件大小 / Check generated CSS file size
ls -lh console/web/dist/assets/*.css

# 统计生成的 CSS 规则数 / Count generated CSS rules
grep -c '{' console/web/dist/assets/index-*.css

# 查看是否包含特定类 / Check if specific class is included
grep '.bg-indigo-600' console/web/dist/assets/index-*.css
```

### 8.3 性能基准 / Performance Benchmarks

| 操作 / Operation | 本项目耗时 / This Project Time | 说明 / Description |
|---|---|---|
| 冷启动首次编译 / Cold start first compile | ~800ms | 含 Tailwind 全量扫描 / Includes full Tailwind scan |
| HMR 增量编译 / HMR incremental compile | ~50ms | 仅重新扫描变更文件 / Only rescan changed files |
| 生产构建 / Production build | ~1.2s | 含压缩 + 树摇 / Includes minify + tree-shake |
| autoprefixer 处理 / autoprefixer processing | ~10ms | 前缀添加极快 / Prefix addition very fast |

## 9. PostCSS AST 处理机制 / PostCSS AST Processing Mechanism

### 9.1 AST 节点类型 / AST Node Types

PostCSS 将 CSS 解析为抽象语法树（AST），包含以下节点类型：
PostCSS parses CSS into an Abstract Syntax Tree (AST) with these node types:

```text
Root
└── AtRule (@media, @keyframes, @layer)
    └── Rule (选择器 + 声明块 / Selector + declaration block)
        └── Declaration (属性: 值 / property: value)
            └── Comment (/* 注释 / comment */)
```

| 节点类型 / Node Type | 对应 CSS / CSS Equivalent | 关键属性 / Key Properties |
|---|---|---|
| `Root` | 整个文件 / Entire file | `nodes`, `source` |
| `AtRule` | `@media`, `@import` | `name`, `params`, `nodes` |
| `Rule` | `.class { }`, `#id { }` | `selector`, `nodes` |
| `Declaration` | `color: red` | `prop`, `value`, `important` |
| `Comment` | `/* ... */` | `text` |

### 9.2 AST 遍历与修改 / AST Traversal & Modification

```javascript
// PostCSS 插件内部 AST 操作示例 / AST manipulation example inside PostCSS plugin
module.exports = () => ({
  postcssPlugin: 'example-plugin',
  Once(root) {
    // 遍历所有规则 / Walk all rules
    root.walkRules((rule) => {
      // 遍历规则内的声明 / Walk declarations within rule
      rule.walkDecls((decl) => {
        // 修改值 / Modify value
        if (decl.prop === 'color' && decl.value === 'red') {
          decl.value = 'var(--color-danger)';
        }
      });
    });

    // 遍历所有 at-rules / Walk all at-rules
    root.walkAtRules('media', (atRule) => {
      console.log(`Media query: ${atRule.params}`);
    });
  },
});
module.exports.postcss = true;
```

### 9.3 Source Map 关联 / Source Map Association

每个 AST 节点携带 `source` 属性，记录原始位置：
Every AST node carries a `source` property recording original position:

```javascript
// 节点位置信息 / Node position info
decl.source.start.line   // 原始行号 / Original line number
decl.source.start.column // 原始列号 / Original column number
decl.source.input.file   // 源文件路径 / Source file path

// 错误报告时自动关联源位置 / Errors auto-link to source position
throw decl.error('Unsupported value', { plugin: 'my-plugin' });
// 输出 / Output: "index.css:12:5: Unsupported value"
```

## 10. 自定义插件开发 / Custom Plugin Development

### 10.1 插件模板 / Plugin Template

```javascript
// postcss-plugin-privacy-vars.js
// 将硬编码颜色替换为 CSS 变量 / Replace hardcoded colors with CSS variables
const COLOR_MAP = {
  '#4f46e5': 'var(--color-primary)',
  '#dc2626': 'var(--color-danger)',
  '#16a34a': 'var(--color-success)',
};

module.exports = (opts = {}) => {
  const map = { ...COLOR_MAP, ...opts.overrides };
  return {
    postcssPlugin: 'postcss-privacy-vars',
    Declaration(decl) {
      const lower = decl.value.toLowerCase();
      if (map[lower]) {
        decl.value = map[lower];
      }
    },
  };
};
module.exports.postcss = true;
```

### 10.2 插件生命周期钩子 / Plugin Lifecycle Hooks

| 钩子 / Hook | 触发时机 / Trigger | 典型用途 / Typical Use |
|---|---|---|
| `Once(root)` | 整个文件解析后 / After full file parse | 全局转换 / Global transforms |
| `Rule(rule)` | 每个规则 / Each rule | 选择器修改 / Selector modification |
| `Declaration(decl)` | 每个声明 / Each declaration | 值替换 / Value replacement |
| `AtRule(atRule)` | 每个 at-rule | @media 处理 / @media processing |
| `OnceExit(root)` | 所有插件完成后 / After all plugins done | 注入额外内容 / Inject extra content |

### 10.3 插件测试 / Plugin Testing

```javascript
// 使用 postcss 内置测试工具 / Using postcss built-in test utilities
const postcss = require('postcss');
const plugin = require('./postcss-plugin-privacy-vars');

async function run(input, expected, opts) {
  const result = await postcss([plugin(opts)]).process(input, { from: undefined });
  expect(result.css).toEqual(expected);
  expect(result.warnings()).toHaveLength(0);
}

test('replaces hex colors with variables', async () => {
  await run(
    'a { color: #4f46e5; }',
    'a { color: var(--color-primary); }'
  );
});

test('ignores unmapped colors', async () => {
  await run(
    'a { color: #333333; }',
    'a { color: #333333; }'  // 不变 / Unchanged
  );
});
```

## 11. 错误处理与日志 / Error Handling & Logging

### 11.1 PostCSS 错误类型 / PostCSS Error Types

| 错误类型 / Error Type | 触发场景 / Trigger | 处理方式 / Handling |
|---|---|---|
| `CssSyntaxError` | CSS 解析失败 / CSS parse failure | 检查源文件语法 / Check source syntax |
| `Plugin Error` | 插件抛出异常 / Plugin throws | 检查插件逻辑 / Check plugin logic |
| `Warning` | 插件发出警告 / Plugin warns | 不阻断构建 / Doesn't block build |

### 11.2 警告机制 / Warning Mechanism

```javascript
// 插件中发出警告（不阻断构建）/ Emit warning in plugin (non-blocking)
Declaration(decl, { result }) {
  if (decl.prop === 'color' && decl.value === 'red') {
    decl.warn(result, 'Avoid using raw "red", use var(--color-danger)', {
      word: 'red',  // 高亮位置 / Highlight position
    });
  }
}

// 构建时查看警告 / View warnings during build
// Vite 会自动在终端输出 PostCSS warnings
// Vite automatically outputs PostCSS warnings in terminal
```

### 11.3 调试技巧 / Debugging Tips

```bash
# 查看 PostCSS 处理链详情 / View PostCSS processing chain details
DEBUG=postcss* npx vite build

# 单独运行 PostCSS 检查输出 / Run PostCSS standalone to inspect output
npx postcss src/index.css --use tailwindcss --use autoprefixer -o /dev/null --verbose

# 检查生成的中间产物 / Inspect generated intermediate output
npx postcss src/index.css --use tailwindcss --no-map | head -50
```

## 12. 与构建工具集成模式 / Build Tool Integration Patterns

### 12.1 Vite 集成架构 / Vite Integration Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  Vite Dev Server / Build                                     │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  CSS 处理管线 / CSS Processing Pipeline                      │
│                                                              │
│  1. 解析 @tailwind 指令 / Parse @tailwind directives         │
│  2. Tailwind 扫描 + 生成 / Tailwind scan + generate          │
│  3. PostCSS 插件链 / PostCSS plugin chain                    │
│     └─ autoprefixer                                          │
│  4. Dev: 注入 <style> / Dev: inject <style>                  │
│     Prod: 输出 .css 文件 / Prod: output .css file            │
└─────────────────────────────────────────────────────────────┘
```

### 12.2 配置优先级 / Configuration Priority

| 配置源 / Config Source | 优先级 / Priority | 说明 / Description |
|---|---|---|
| `vite.config.ts` css.postcss | 最高 / Highest | 内联 PostCSS 配置 / Inline PostCSS config |
| `postcss.config.js` | 中 / Medium | 独立配置文件 / Standalone config file |
| `package.json` postcss 字段 | 最低 / Lowest | 简化配置 / Simplified config |

**本项目使用 `postcss.config.js`**：独立配置文件更清晰，与 Vite 解耦。
This project uses `postcss.config.js`: standalone config is clearer, decoupled from Vite.

### 12.3 不同构建工具对比 / Build Tool Comparison

| 构建工具 / Build Tool | PostCSS 集成方式 / Integration | 配置位置 / Config Location |
|---|---|---|
| Vite | 内置支持 / Built-in | `postcss.config.js` 或 vite.config |
| webpack | `postcss-loader` | webpack.config + postcss.config |
| Rollup | `@rollup/plugin-postcss` | rollup.config.js |
| esbuild | `esbuild-postcss` 插件 / Plugin | esbuild 配置 / esbuild config |
| Parcel | 内置支持 / Built-in | `postcss.config.js` |

### 12.4 本项目集成设计决策 / Integration Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 使用独立 postcss.config.js | 与 Vite 解耦，便于切换构建工具 / Decoupled from Vite, easy to switch |
| 仅配置 tailwindcss + autoprefixer | 最小化插件链，减少构建时间 / Minimal plugin chain, reduce build time |
| 不使用 cssnano | Vite 内置 esbuild 压缩更快 / Vite's built-in esbuild minify is faster |
| 不使用 postcss-import | Vite 原生处理 @import / Vite handles @import natively |

## 13. CSS 变量与主题系统 / CSS Variables & Theming System

### 13.1 CSS 自定义属性基础 / Custom Properties Basics

CSS 自定义属性（CSS Variables）是现代主题系统的核心。PostCSS 在 Tailwind CSS 管道中负责将主题配置编译为 CSS 变量：
CSS Custom Properties are the core of modern theming systems. PostCSS compiles theme config into CSS variables in the Tailwind pipeline:

```css
/* Tailwind 编译输出 / Tailwind compiled output */
:root {
  --color-primary-50: #eff6ff;
  --color-primary-100: #dbeafe;
  --color-primary-500: #3b82f6;
  --color-primary-600: #2563eb;
  --color-primary-700: #1d4ed8;
  --spacing-1: 0.25rem;
  --spacing-2: 0.5rem;
  --spacing-4: 1rem;
  --font-size-sm: 0.875rem;
  --font-size-base: 1rem;
  --border-radius-md: 0.375rem;
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1);
}

/* 暗色主题覆盖 / Dark theme override */
.dark {
  --color-primary-500: #60a5fa;
  --color-primary-600: #3b82f6;
  --color-primary-700: #2563eb;
}
```

### 13.2 PostCSS 处理变量的时机 / When PostCSS Processes Variables

```text
┌────────────────────────────────────────────────────────────────┐
│ Tailwind Config (tailwind.config.js)                           │
│   theme.extend.colors.primary = { 50: '#eff6ff', ... }        │
└──────────────────────┬─────────────────────────────────────────┘
                       ▼
┌────────────────────────────────────────────────────────────────┐
│ PostCSS Plugin: tailwindcss                                    │
│   1. 读取配置 / Read config                                     │
│   2. 扫描源文件 class / Scan source classes                      │
│   3. 生成 CSS 变量 + 工具类 / Generate variables + utilities      │
└──────────────────────┬─────────────────────────────────────────┘
                       ▼
┌────────────────────────────────────────────────────────────────┐
│ PostCSS Plugin: autoprefixer                                   │
│   - 添加浏览器前缀（如需）/ Add vendor prefixes (if needed)        │
│   - CSS 变量通常无需前缀 / Variables rarely need prefixes          │
└──────────────────────┬─────────────────────────────────────────┘
                       ▼
┌────────────────────────────────────────────────────────────────┐
│ Vite CSS 压缩 / Vite CSS minification                          │
│   - 移除空白 / Remove whitespace                                │
│   - 合并重复声明 / Merge duplicate declarations                  │
└────────────────────────────────────────────────────────────────┘
```

### 13.3 动态主题切换模式 / Dynamic Theme Switching Patterns

```css
/* 方案一：class 切换 / Approach 1: class toggle */
html.dark {
  --bg-primary: #1a1a2e;
  --text-primary: #e2e8f0;
  --border-color: #374151;
}

/* 方案二：data 属性 / Approach 2: data attribute */
[data-theme="ocean"] {
  --bg-primary: #0f172a;
  --text-primary: #f1f5f9;
  --accent: #06b6d4;
}

/* 方案三：prefers-color-scheme 自动 / Approach 3: auto via media query */
@media (prefers-color-scheme: dark) {
  :root:not(.light) {
    --bg-primary: #1a1a2e;
    --text-primary: #e2e8f0;
  }
}
```

### 13.4 本项目主题实践 / This Project's Theming Practice

| 方面 / Aspect | 实现 / Implementation | 说明 / Notes |
|---|---|---|
| 颜色系统 / Colors | Tailwind 默认调色板 / Default palette | 无需自定义 CSS 变量 / No custom vars needed |
| 暗色模式 / Dark mode | `darkMode: 'class'` | 通过 `dark:` 前缀切换 / Toggle via `dark:` prefix |
| 间距 / Spacing | Tailwind 默认 scale | 4px 基准单位 / 4px base unit |
| 圆角 / Border radius | `rounded-md`/`rounded-lg` | 统一 UI 风格 / Consistent UI style |
| 阴影 / Shadows | `shadow-sm`/`shadow-md` | 层级深度表达 / Depth expression |

### 13.5 PostCSS 变量降级策略 / Variable Fallback Strategy

对于不支持 CSS 变量的旧浏览器，PostCSS 插件可提供降级：
For older browsers without CSS variable support, PostCSS plugins provide fallbacks:

```javascript
// postcss.config.js - 可选降级插件 / Optional fallback plugin
module.exports = {
  plugins: [
    require('tailwindcss'),
    require('autoprefixer'),
    // 仅在需要支持 IE11 时启用 / Only enable for IE11 support
    // require('postcss-custom-properties')({ preserve: true }),
  ],
};

// postcss-custom-properties 输出示例 / Output example
// 输入 / Input:
//   .card { color: var(--text-primary); }
// 输出（preserve: true）/ Output:
//   .card { color: #1a202c; color: var(--text-primary); }
```

**本项目不需要降级** / This project doesn't need fallbacks：
- 目标浏览器均支持 CSS 变量 / Target browsers all support CSS variables
- 本地开发工具，无需兼容旧浏览器 / Local dev tool, no legacy browser need
- Tailwind v3 默认不生成变量降级 / Tailwind v3 doesn't generate fallbacks by default

## 14. PostCSS 性能优化 / PostCSS Performance Optimization

### 14.1 处理管线性能分析 / Pipeline Performance Profiling

```javascript
// 使用 postcss-reporter 分析插件耗时 / Profile plugin timing
const postcss = require('postcss');
const reporter = require('postcss-reporter');

// 开发环境启用性能报告 / Enable perf report in dev
module.exports = {
  plugins: [
    require('tailwindcss'),
    require('autoprefixer'),
    // 开发调试用 / For dev debugging
    ...(process.env.POSTCSS_PROFILE ? [reporter({ clearReportedMessages: true })] : []),
  ],
};
```

### 14.2 缓存策略 / Caching Strategy

```text
┌─────────────────────────────────────────────────────────────┐
│ Vite 开发服务器缓存层 / Vite Dev Server Cache Layers          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Layer 1: 文件系统缓存 / File system cache                    │
│  ┌──────────────────────────────────────────────┐           │
│  │ node_modules/.vite/deps/                     │           │
│  │ - 预构建依赖缓存 / Pre-bundled dep cache       │           │
│  │ - hash 校验 / Hash verification               │           │
│  └──────────────────────────────────────────────┘           │
│                                                             │
│  Layer 2: 内存转换缓存 / In-memory transform cache            │
│  ┌──────────────────────────────────────────────┐           │
│  │ Map<filePath, { code, map, timestamp }>       │           │
│  │ - 文件未变则跳过 PostCSS / Skip if unchanged    │           │
│  │ - HMR 仅处理变更文件 / HMR processes only diff  │           │
│  └──────────────────────────────────────────────┘           │
│                                                             │
│  Layer 3: Tailwind JIT 缓存 / Tailwind JIT cache             │
│  ┌──────────────────────────────────────────────┐           │
│  │ - 已见 class 集合 / Seen class set             │           │
│  │ - 增量生成 / Incremental generation            │           │
│  │ - 内容 hash 比对 / Content hash comparison     │           │
│  └──────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────┘
```

### 14.3 构建时间优化 / Build Time Optimization

| 优化手段 / Optimization | 效果 / Effect | 适用场景 / Scenario |
|---|---|---|
| 减少插件数量 / Fewer plugins | 减少 AST 遍历次数 / Fewer AST traversals | 始终 / Always |
| Tailwind `content` 精确配置 / Precise content | 减少扫描文件 / Fewer scanned files | 大型项目 / Large projects |
| 生产环境用 esbuild 压缩 / esbuild minify | 比 cssnano 快 10-100x / 10-100x faster | 生产构建 / Production |
| 避免 `@import` 链 / Avoid import chains | 减少文件 I/O / Reduce file I/O | 多文件 CSS / Multi-file CSS |
| 使用 Lightning CSS（可选）/ Lightning CSS | Rust 实现更快 / Rust impl faster | 性能敏感 / Perf-critical |

### 14.4 本项目构建性能数据 / This Project's Build Performance

```text
构建阶段耗时分析（典型值）/ Build stage timing (typical):

  阶段 / Stage              耗时 / Duration    占比 / Share
  ─────────────────────────────────────────────────────────
  文件扫描 / File scan       ~50ms            8%
  Tailwind JIT 生成 / Gen    ~200ms           33%
  Autoprefixer / Prefix      ~30ms            5%
  esbuild 压缩 / Minify      ~80ms            13%
  模块打包 / Bundle          ~150ms           25%
  文件写入 / Write            ~90ms            15%
  ─────────────────────────────────────────────────────────
  总计 / Total               ~600ms           100%
```

### 14.5 大规模项目的 PostCSS 优化 / PostCSS Optimization for Large Projects

```javascript
// 大型项目 tailwind.config.js 优化示例 / Large project optimization
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    // 精确指定，避免扫描 node_modules / Precise paths, avoid node_modules
    './src/**/*.{ts,tsx}',
    './index.html',
    // 不要使用 './**/*' 这样的宽泛模式 / Don't use broad patterns
  ],
  // 禁用不需要的核心插件 / Disable unused core plugins
  corePlugins: {
    container: false,     // 不使用容器 / Not using container
    scrollSnapType: false, // 不使用滚动吸附 / Not using scroll snap
  },
  // 安全列表仅在必要时使用 / Safelist only when necessary
  safelist: [],
};
```

## 15. CSS-in-JS 方案对比 / CSS-in-JS Comparison

### 15.1 主流方案架构对比 / Architecture Comparison

```text
┌─────────────────────────────────────────────────────────────────────┐
│                    CSS 方案光谱 / CSS Solution Spectrum               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  编译时 / Compile-time          运行时 / Runtime                      │
│  ◄──────────────────────────────────────────────────────────►       │
│                                                                     │
│  Tailwind CSS    CSS Modules    Styled-       Emotion    Vanilla    │
│  (PostCSS)       (PostCSS)      Components    (zero-run) Extract   │
│                                                                     │
│  零运行时开销     低开销         高运行时       可选静态    编译时提取   │
│  Zero runtime    Low overhead   High runtime  Optional    Extract    │
│  cost                                         static     at build   │
└─────────────────────────────────────────────────────────────────────┘
```

### 15.2 详细对比表 / Detailed Comparison Table

| 维度 / Dimension | Tailwind + PostCSS | Styled-Components | Emotion | CSS Modules |
|---|---|---|---|---|
| 运行时开销 / Runtime cost | 零 / Zero | ~12KB + 样式计算 / +calc | ~8KB（零运行时可选）/ ~8KB | 零 / Zero |
| 包体积影响 / Bundle impact | CSS 独立文件 / Separate CSS | JS 增大 / JS grows | JS 增大 / JS grows | CSS 独立 / Separate |
| 动态样式 / Dynamic styles | 有限（class 切换）/ Limited | 完全动态 / Fully dynamic | 完全动态 / Fully dynamic | 有限 / Limited |
| SSR 支持 / SSR support | 天然支持 / Native | 需配置 / Needs config | 需配置 / Needs config | 天然支持 / Native |
| 类型安全 / Type safety | 无（字符串）/ None (string) | 有 / Yes | 有 / Yes | 有（插件）/ Yes (plugin) |
| 学习曲线 / Learning curve | 低 / Low | 中 / Medium | 中 / Medium | 低 / Low |
| 工具链复杂度 / Toolchain | PostCSS + Tailwind | Babel 插件 / Babel plugin | Babel 插件 / Babel plugin | PostCSS / webpack |
| 调试体验 / Debugging | 类名可读 / Readable classes | 生成类名 / Generated names | 生成类名 / Generated names | 哈希类名 / Hashed names |

### 15.3 本项目选择 Tailwind + PostCSS 的原因 / Why This Project Chose Tailwind + PostCSS

| 原因 / Reason | 详细说明 / Details |
|---|---|
| 零运行时 / Zero runtime | 代理工具 UI 追求最小开销 / Proxy tool UI wants minimal overhead |
| 无需 Babel / No Babel needed | Vite + esbuild 已够快 / Vite + esbuild already fast enough |
| 一致性 / Consistency | 预定义 scale 避免样式碎片 / Predefined scale avoids style fragmentation |
| 快速原型 / Rapid prototyping | 无需切换文件即可样式化 / Style without file switching |
| 生产体积 / Production size | PurgeCSS 移除未用样式 / PurgeCSS removes unused styles |
| 团队熟悉度 / Team familiarity | 广泛使用的方案 / Widely adopted solution |

### 15.4 何时应考虑 CSS-in-JS / When to Consider CSS-in-JS

```text
选择决策树 / Decision tree:

  需要高度动态样式？/ Need highly dynamic styles?
  ├── 是 / Yes → 需要 SSR？/ Need SSR?
  │   ├── 是 / Yes → Emotion (zero-runtime) 或 Vanilla Extract
  │   └── 否 / No  → Styled-Components 或 Emotion
  └── 否 / No → 组件库还是应用？/ Library or App?
      ├── 组件库 / Library → CSS Modules（无运行时依赖）/ No runtime dep
      └── 应用 / App → Tailwind CSS + PostCSS ✓（本项目 / This project）
```

### 15.5 迁移路径 / Migration Path

如果未来需要迁移到 CSS-in-JS，PostCSS 管道可以渐进式共存：
If future migration to CSS-in-JS is needed, PostCSS pipeline can coexist incrementally:

```javascript
// 共存配置示例 / Coexistence config example
// vite.config.ts
export default defineConfig({
  css: {
    // PostCSS 仍处理全局样式和 Tailwind / PostCSS still handles global + Tailwind
    postcss: './postcss.config.js',
    // CSS Modules 用于特定组件 / CSS Modules for specific components
    modules: {
      localsConvention: 'camelCase',
      generateScopedName: '[name]__[local]--[hash:base64:5]',
    },
  },
});
```

| 迁移阶段 / Migration Phase | 策略 / Strategy | 风险 / Risk |
|---|---|---|
| 阶段 1 / Phase 1 | 新组件用 CSS Modules / New components use CSS Modules | 低 / Low |
| 阶段 2 / Phase 2 | 复杂交互组件引入 Emotion / Complex components add Emotion | 中 / Medium |
| 阶段 3 / Phase 3 | 全局样式保留 Tailwind / Keep Tailwind for global | 低 / Low |
| 阶段 4 / Phase 4 | 评估是否完全迁移 / Evaluate full migration | 高 / High |

---

## 16. 容器查询与现代布局 / Container Queries & Modern Layout

### 16.1 容器查询概念与语法 / Container Query Concepts & Syntax

容器查询（Container Queries）允许组件根据其父容器的尺寸而非视口来调整样式，是组件化 CSS 的重大突破：

```css
/* ===== 容器查询基础语法 / Container Query Basic Syntax ===== */

/* 1. 定义容器 / Define container */
.card-wrapper {
  /* 将元素声明为尺寸容器 / Declare element as size container */
  container-type: inline-size;
  container-name: card;  /* 可选命名 / Optional name */
}

/* 简写形式 / Shorthand */
.sidebar {
  container: sidebar / inline-size;
}

/* 2. 使用容器查询 / Use container query */
@container card (min-width: 400px) {
  .card {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 1rem;
  }
}

@container card (max-width: 399px) {
  .card {
    display: block;
  }
  .card__image {
    width: 100%;
    margin-bottom: 0.5rem;
  }
}

/* 3. 容器查询单位 / Container query units */
.card__title {
  /* 相对于容器宽度的字体大小 / Font size relative to container */
  font-size: clamp(1rem, 4cqw, 2rem);
}

.card__padding {
  /* cqw = 容器宽度 1%, cqh = 容器高度 1% */
  padding: 2cqw 4cqw;
}

/* 4. 容器查询逻辑组合 / Container query logical combinations */
@container sidebar (min-width: 300px) and (max-width: 600px) {
  .widget {
    flex-direction: column;
  }
}

/* 5. 样式容器（不建立布局包含）/ Style container (no layout containment) */
.theme-region {
  container-type: normal;  /* 仅用于命名，不影响布局 / Name only */
}
```

### 16.2 PostCSS 容器查询插件 / PostCSS Container Query Plugins

```javascript
// ===== postcss.config.js 配置 / Configuration =====
module.exports = {
  plugins: {
    // 容器查询 polyfill（旧浏览器）/ Container query polyfill (old browsers)
    '@csstools/postcss-container-query-units': {
      // 将 cqw/cqh 转换为回退值 / Convert cqw/cqh to fallbacks
      preserve: true,  // 保留原始声明 / Preserve original declarations
    },

    // 其他现代 CSS 特性 / Other modern CSS features
    'postcss-preset-env': {
      stage: 2,
      features: {
        'container-queries': true,
        'cascade-layers': true,
        'nesting-rules': true,
      },
    },
  },
};
```

### 16.3 容器查询 vs 媒体查询 / Container Queries vs Media Queries

| 特性 / Feature | 媒体查询 / Media Query | 容器查询 / Container Query | 适用场景 / Use Case |
|---|---|---|---|
| 参照物 / Reference | 视口 / Viewport | 父容器 / Parent container | 组件复用 / Component reuse |
| 组件独立性 / Independence | 依赖页面位置 / Depends on page | 自适应容器 / Self-adaptive | 设计系统 / Design system |
| 侧边栏组件 / Sidebar widget | 需要特殊处理 / Needs special | 自动适配 / Auto adapts | ✅ 容器查询 / CQ |
| 全局布局 / Global layout | 自然选择 / Natural choice | 不适用 / N/A | ✅ 媒体查询 / MQ |
| 浏览器支持 / Browser support | 100% | 90%+ (2023+) | 渐进增强 / Progressive |

---

## 17. CSS 兼容性处理与 Polyfill 策略 / CSS Compatibility & Polyfill Strategy

### 17.1 Autoprefixer 深入 / Autoprefixer Deep Dive

Autoprefixer 是 PostCSS 生态中最常用的插件，自动添加浏览器前缀：

```css
/* ===== 输入 CSS / Input CSS ===== */
.gradient-box {
  background: linear-gradient(to right, #ff0000, #0000ff);
  display: grid;
  place-items: center;
  user-select: none;
  backdrop-filter: blur(10px);
}

/* ===== 输出 CSS（Autoprefixer 处理后）/ Output (after Autoprefixer) ===== */
.gradient-box {
  background: -webkit-linear-gradient(left, #ff0000, #0000ff);  /* Safari 6.1+ */
  background: linear-gradient(to right, #ff0000, #0000ff);
  display: -ms-grid;  /* IE 10-11 */
  display: grid;
  place-items: center;
  -webkit-user-select: none;  /* Safari */
  -moz-user-select: none;     /* Firefox */
  -ms-user-select: none;      /* IE/Edge */
  user-select: none;
  -webkit-backdrop-filter: blur(10px);  /* Safari */
  backdrop-filter: blur(10px);
}
```

### 17.2 Browserslist 配置策略 / Browserslist Configuration Strategy

```ini
# ===== .browserslistrc 配置示例 / Configuration Examples =====

# 本项目配置（现代浏览器）/ This project (modern browsers)
# 位于 package.json 或 .browserslistrc
last 2 Chrome versions
last 2 Firefox versions
last 2 Safari versions
last 2 Edge versions
not dead

# 企业内网项目（更宽松）/ Enterprise intranet (more relaxed)
# > 0.5%
# last 3 versions
# not dead
# not IE 11

# 需要支持旧浏览器 / Need legacy support
# > 0.1%
# last 5 versions
# Firefox ESR
# not dead
```

### 17.3 渐进增强与回退模式 / Progressive Enhancement & Fallback Patterns

```css
/* ===== 渐进增强示例 / Progressive Enhancement Examples ===== */

/* 1. 逻辑属性回退 / Logical properties fallback */
.element {
  /* 回退（旧浏览器）/ Fallback (old browsers) */
  margin-left: 1rem;
  margin-right: 1rem;
  /* 现代（逻辑属性）/ Modern (logical properties) */
  margin-inline: 1rem;
}

/* 2. :has() 选择器回退 / :has() selector fallback */
/* 基础样式（所有浏览器）/ Base styles (all browsers) */
.form-group input {
  border: 1px solid #ccc;
}

/* 增强样式（支持 :has 的浏览器）/ Enhanced (browsers with :has) */
.form-group:has(input:invalid) {
  border-color: #ef4444;
  background: #fef2f2;
}

/* 3. subgrid 回退 / subgrid fallback */
.card-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
}

.card {
  display: grid;
  gap: 0.5rem;
  /* 回退: 独立网格 / Fallback: independent grid */
  grid-template-rows: auto 1fr auto;
}

/* 增强: 子网格对齐 / Enhanced: subgrid alignment */
@supports (grid-template-rows: subgrid) {
  .card {
    grid-template-rows: subgrid;
    grid-row: span 3;
  }
}

/* 4. 颜色函数回退 / Color function fallback */
.button {
  /* 回退 / Fallback */
  background-color: rgba(59, 130, 246, 0.8);
  /* 现代 / Modern */
  background-color: oklch(0.7 0.15 250 / 0.8);
}
```

---

## 18. PostCSS 与 Sass/Less 迁移 / PostCSS & Sass/Less Migration

### 18.1 Sass vs PostCSS 架构对比 / Sass vs PostCSS Architecture Comparison

```
┌─────────────────────────────────────────────────────────────────┐
│         Sass vs PostCSS 架构对比 / Architecture Comparison       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Sass 管道 / Sass Pipeline:                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  .scss  ──▶  Sass Compiler  ──▶  CSS  ──▶  Autoprefixer│   │
│  │              (dart-sass)                              │   │
│  │  - 变量、混合、继承 / Variables, mixins, inheritance  │   │
│  │  - 编译时计算 / Compile-time computation              │   │
│  │  - 单一语言 / Single language                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  PostCSS 管道 / PostCSS Pipeline:                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  .css  ──▶  [Plugin 1] ──▶ [Plugin 2] ──▶ ... ──▶ CSS │   │
│  │              │              │                          │   │
│  │              ▼              ▼                          │   │
│  │         postcss-import  postcss-nesting               │   │
│  │         tailwindcss     autoprefixer                  │   │
│  │         cssnano         postcss-preset-env            │   │
│  │  - 纯 CSS + 插件 / Pure CSS + plugins                │   │
│  │  - 可组合管道 / Composable pipeline                   │   │
│  │  - 渐进采用 / Progressive adoption                   │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 18.2 功能对照与迁移映射 / Feature Mapping & Migration

| Sass 功能 / Sass Feature | PostCSS 替代 / PostCSS Alternative | 插件 / Plugin | 迁移难度 / Difficulty |
|---|---|---|---|
| `$variable` | CSS 自定义属性 / CSS custom props | 原生 / Native | 低 / Low |
| `@mixin` / `@include` | `@apply` 或工具类 / @apply or utilities | tailwindcss | 中 / Medium |
| `@extend` | `composes` (CSS Modules) | postcss-modules | 中 / Medium |
| `@if` / `@each` | JS 插件逻辑 / JS plugin logic | 自定义 / Custom | 高 / High |
| `@function` | postcss-calc / JS | postcss-calc | 中 / Medium |
| 嵌套 / Nesting | postcss-nesting | postcss-nesting | 低 / Low |
| `@import` | postcss-import | postcss-import | 低 / Low |
| 数学运算 / Math | postcss-calc | postcss-calc | 低 / Low |

### 18.3 迁移实战步骤 / Migration Steps in Practice

```bash
# ===== 从 Sass 迁移到 PostCSS / Migrate from Sass to PostCSS =====

# 1. 安装依赖 / Install dependencies
pnpm remove sass sass-loader
pnpm add -D postcss postcss-import postcss-nesting postcss-preset-env autoprefixer cssnano

# 2. 创建 PostCSS 配置 / Create PostCSS config
# postcss.config.js (见上文 / see above)

# 3. 批量重命名文件 / Batch rename files
# find src -name "*.scss" -exec sh -c 'mv "$1" "${1%.scss}.css"' _ {} \;

# 4. 转换变量 / Convert variables
# Sass: $primary-color: #3b82f6;
# CSS:  :root { --primary-color: #3b82f6; }

# 5. 转换混合 / Convert mixins
# Sass: @mixin flex-center { display: flex; align-items: center; justify-content: center; }
# CSS:  使用 Tailwind 工具类 / Use Tailwind utilities: flex items-center justify-center

# 6. 验证构建 / Verify build
pnpm build
```

### 18.4 本项目技术选型理由 / This Project's Technology Rationale

| 考量因素 / Factor | 决策 / Decision | 理由 / Reason |
|---|---|---|
| 团队熟悉度 / Team familiarity | PostCSS + Tailwind | 无需学习 Sass 语法 / No Sass syntax to learn |
| 构建速度 / Build speed | PostCSS 更快 / Faster | 无编译步骤 / No compile step |
| 生态兼容 / Ecosystem | Vite 原生支持 / Native | 零配置 / Zero config |
| 运行时 CSS / Runtime CSS | CSS 变量 / CSS vars | 主题切换无需重编译 / Theme switch no recompile |
| 包体积 / Bundle size | 更小 / Smaller | 无 Sass runtime / No Sass runtime |
| 未来兼容 / Future-proof | 标准 CSS / Standard CSS | 浏览器原生支持 / Native browser support |

## 19. CSS Houdini 与未来 API / CSS Houdini & Future APIs

### 19.1 Houdini 概述 / Houdini Overview

CSS Houdini 是一组底层 API，允许开发者扩展 CSS 引擎：

```
┌─────────────────────────────────────────────────────────┐
│  CSS Houdini API 家族                                    │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────┐ │
│  │ CSS Properties│  │  Paint API    │  │ Layout API  │ │
│  │ & Values API  │  │  (Worklet)    │  │ (Worklet)   │ │
│  │ 自定义属性    │  │  自定义绘制    │  │  自定义布局  │ │
│  └───────────────┘  └───────────────┘  └─────────────┘ │
│                                                         │
│  ┌───────────────┐  ┌───────────────┐  ┌─────────────┐ │
│  │ Animation API │  │  Parser API   │  │ Font Metrics│ │
│  │ 动画控制      │  │  解析扩展      │  │  字体度量    │ │
│  └───────────────┘  └───────────────┘  └─────────────┘ │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 19.2 CSS Properties & Values API / Properties & Values API

```css
/* 注册自定义属性（带类型检查） */
/* Register custom properties (with type checking) */
@property --brand-color {
  syntax: '<color>';
  inherits: false;
  initial-value: #3b82f6;
}

@property --spacing-unit {
  syntax: '<length>';
  inherits: true;
  initial-value: 8px;
}

@property --gradient-angle {
  syntax: '<angle>';
  inherits: false;
  initial-value: 0deg;
}

/* 使用：现在可以动画过渡自定义属性 */
/* Usage: now can animate custom properties */
.card {
  --brand-color: #3b82f6;
  background: var(--brand-color);
  transition: --brand-color 0.3s ease;  /* ✔ 可动画 / Animatable */
}

.card:hover {
  --brand-color: #1d4ed8;  /* 平滑过渡 / Smooth transition */
}

/* 渐变角度动画 */
/* Gradient angle animation */
.gradient-bg {
  --gradient-angle: 0deg;
  background: linear-gradient(var(--gradient-angle), #667eea, #764ba2);
  animation: rotate-gradient 3s linear infinite;
}

@keyframes rotate-gradient {
  to { --gradient-angle: 360deg; }
}
```

### 19.3 Paint Worklet 自定义绘制 / Paint Worklet Custom Drawing

```javascript
// paint-worklet.js — 自定义绘制工作器
// paint-worklet.js — Custom paint worklet
class PrivacyShieldPainter {
  static get inputProperties() {
    return ['--shield-color', '--shield-size', '--shield-opacity']
  }
  
  paint(ctx, size, properties) {
    const color = properties.get('--shield-color').toString()
    const shieldSize = parseInt(properties.get('--shield-size')) || 24
    const opacity = parseFloat(properties.get('--shield-opacity')) || 0.1
    
    ctx.globalAlpha = opacity
    ctx.fillStyle = color
    
    // 绘制盾牌图案（平铺）
    // Draw shield pattern (tiled)
    for (let x = 0; x < size.width; x += shieldSize * 2) {
      for (let y = 0; y < size.height; y += shieldSize * 2) {
        this.drawShield(ctx, x, y, shieldSize)
      }
    }
  }
  
  drawShield(ctx, x, y, size) {
    ctx.beginPath()
    ctx.moveTo(x + size / 2, y)
    ctx.lineTo(x + size, y + size * 0.3)
    ctx.lineTo(x + size, y + size * 0.7)
    ctx.lineTo(x + size / 2, y + size)
    ctx.lineTo(x, y + size * 0.7)
    ctx.lineTo(x, y + size * 0.3)
    ctx.closePath()
    ctx.fill()
  }
}

registerPaint('privacy-shield', PrivacyShieldPainter)
```

```css
/* 使用 Paint Worklet */
/* Use Paint Worklet */
.privacy-banner {
  --shield-color: #3b82f6;
  --shield-size: 32;
  --shield-opacity: 0.05;
  background-image: paint(privacy-shield);
}
```

### 19.4 Houdini 浏览器支持 / Houdini Browser Support

| API | Chrome | Firefox | Safari | 状态 / Status |
|---|---|---|---|---|
| Properties & Values | ✅ 85+ | ✅ 128+ | ✅ 16.4+ | 可用 / Usable |
| Paint Worklet | ✅ 65+ | ✘ | ✘ | 仅 Chromium |
| Layout Worklet | ✅ 实验性 | ✘ | ✘ | 实验 / Experimental |
| Animation Worklet | ✘ | ✘ | ✘ | 未实现 / Not impl |

## 20. 设计系统集成 / Design System Integration

### 20.1 设计令牌架构 / Design Token Architecture

```css
/* tokens/base.css — 原始令牌（不直接使用） */
/* tokens/base.css — Primitive tokens (not used directly) */
:root {
  /* 颜色原始值 / Color primitives */
  --color-blue-50: #eff6ff;
  --color-blue-100: #dbeafe;
  --color-blue-500: #3b82f6;
  --color-blue-600: #2563eb;
  --color-blue-700: #1d4ed8;
  
  --color-gray-50: #f9fafb;
  --color-gray-100: #f3f4f6;
  --color-gray-900: #111827;
  
  /* 间距原始值 / Spacing primitives */
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-6: 24px;
  --space-8: 32px;
}

/* tokens/semantic.css — 语义令牌（组件使用） */
/* tokens/semantic.css — Semantic tokens (used by components) */
:root {
  /* 颜色语义 / Color semantics */
  --color-primary: var(--color-blue-600);
  --color-primary-hover: var(--color-blue-700);
  --color-surface: #ffffff;
  --color-surface-elevated: var(--color-gray-50);
  --color-text-primary: var(--color-gray-900);
  --color-text-secondary: var(--color-gray-500);
  --color-border: var(--color-gray-200);
  
  /* 间距语义 / Spacing semantics */
  --spacing-component: var(--space-4);
  --spacing-section: var(--space-8);
  --spacing-inline: var(--space-2);
}

/* tokens/dark.css — 暗色主题覆盖 */
/* tokens/dark.css — Dark theme override */
.dark {
  --color-primary: var(--color-blue-500);
  --color-surface: #1f2937;
  --color-surface-elevated: #374151;
  --color-text-primary: #f9fafb;
  --color-text-secondary: #9ca3af;
  --color-border: #4b5563;
}
```

### 20.2 PostCSS 令牌处理管线 / PostCSS Token Processing Pipeline

```javascript
// postcss.config.js — 设计系统管线
// postcss.config.js — Design system pipeline
module.exports = {
  plugins: [
    // 1. 令牌注入（将 JSON 令牌转为 CSS 变量）
    // 1. Token injection (JSON tokens → CSS variables)
    require('postcss-design-tokens')({
      tokens: './design-tokens/tokens.json',
      prefix: 'ds',
    }),
    
    // 2. Tailwind CSS
    require('tailwindcss'),
    
    // 3. 自定义属性回退
    // 3. Custom property fallbacks
    require('postcss-custom-properties')({
      preserve: true,  // 保留变量 + 生成回退
    }),
    
    // 4. 自动前缀
    // 4. Auto prefix
    require('autoprefixer'),
    
    // 5. 压缩（生产）
    // 5. Minify (production)
    ...(process.env.NODE_ENV === 'production'
      ? [require('cssnano')({ preset: 'default' })]
      : []),
  ],
}
```

### 20.3 组件库样式约定 / Component Library Style Conventions

```css
/* 组件样式约定：使用语义令牌而非原始值 */
/* Component style convention: use semantic tokens not primitives */

/* ✔ 正确：使用语义令牌 */
/* ✔ Correct: use semantic tokens */
.ds-card {
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  padding: var(--spacing-component);
  border-radius: var(--radius-md);
}

/* ✘ 错误：硬编码值 */
/* ✘ Wrong: hardcoded values */
.bad-card {
  background: #ffffff;
  border: 1px solid #e5e7eb;
  padding: 16px;
}
```

## 21. 构建管线与工程化 / Build Pipeline & Engineering

### 21.1 完整构建流程 / Complete Build Flow

```
源代码 / Source              PostCSS 管线 / Pipeline           输出 / Output
─────────────────────────────────────────────────────────────────────────────

┌─────────────┐     ┌────────────────────────────────┐     ┌────────────┐
│ index.css   │     │  1. postcss-import             │     │            │
│ (入口)       │────▶│  2. tailwindcss               │────▶│  dist/     │
└─────────────┘     │  3. postcss-nesting           │     │  ├─ app.css│
                    │  4. postcss-custom-properties  │     │  ├─ app.css│
┌─────────────┐     │  5. autoprefixer              │     │  │   .map  │
│ tokens/     │────▶│  6. cssnano (prod)            │     │  └─ tokens/│
│ (令牌)       │     └────────────────────────────────┘     └────────────┘
└─────────────┘
```

### 21.2 多环境构建配置 / Multi-environment Build Config

```javascript
// postcss.config.js — 环境感知配置
// postcss.config.js — Environment-aware config
const isDev = process.env.NODE_ENV === 'development'
const isProd = process.env.NODE_ENV === 'production'

module.exports = {
  plugins: [
    require('tailwindcss'),
    require('autoprefixer'),
    
    // 开发环境：Source Map + 详细错误
    // Development: Source Map + detailed errors
    ...(isDev ? [
      require('postcss-reporter')({ clearReportedMessages: true }),
    ] : []),
    
    // 生产环境：压缩 + 优化
    // Production: minify + optimize
    ...(isProd ? [
      require('cssnano')({
        preset: ['advanced', {
          discardComments: { removeAll: true },
          reduceIdents: true,
          mergeIdents: true,
        }],
      }),
      // 移除未使用的 CSS（配合 PurgeCSS）
      // Remove unused CSS (with PurgeCSS)
      require('@fullhuman/postcss-purgecss')({
        content: ['./src/**/*.{tsx,ts,html}'],
        safelist: [/^dark:/, /^animate-/],
      }),
    ] : []),
  ],
  
  // Source Map 配置
  // Source Map configuration
  map: isDev ? { inline: true } : false,
}
```

### 21.3 构建性能监控 / Build Performance Monitoring

```javascript
// 构建时间分析插件
// Build time analysis plugin
const buildAnalyzer = {
  postcssPlugin: 'build-analyzer',
  OnceExit(root, { result }) {
    const stats = {
      rules: 0,
      declarations: 0,
      customProperties: 0,
    }
    
    root.walkRules(() => stats.rules++)
    root.walkDecls((decl) => {
      stats.declarations++
      if (decl.prop.startsWith('--')) {
        stats.customProperties++
      }
    })
    
    console.log(`[PostCSS Stats] Rules: ${stats.rules}, Decls: ${stats.declarations}, Vars: ${stats.customProperties}`)
  },
}
buildAnalyzer.postcss = true
```

### 21.4 构建优化检查清单 / Build Optimization Checklist

| 优化项 / Optimization | 效果 / Effect | 配置 / Config |
|---|---|---|
| PurgeCSS 移除未用 | CSS -80% | content 配置 |
| cssnano 压缩 | CSS -30% | advanced preset |
| Source Map 分离 | 调试方便 / Easy debug | map: { inline: false } |
| 缓存 / Cache | 重建 -70% | postcss-cache |
| 并行处理 / Parallel | 多文件加速 / Multi-file | thread-loader |
