# Tailwind CSS 技术栈说明 / Tailwind CSS Technology Stack

## 1. 技术简介 / Introduction

Tailwind CSS 是一个实用优先（Utility-first）的 CSS 框架，通过组合原子化类名快速构建现代 UI。
Tailwind CSS is a utility-first CSS framework for rapidly building modern UIs by composing atomic class names.

核心特性 / Core Features：
- **实用优先（Utility-first）**：提供大量小粒度工具类（如 `flex`、`px-4`、`text-sm`），无需编写自定义 CSS。
- **响应式设计（Responsive Design）**：通过前缀（`sm:`、`md:`、`lg:`）轻松实现断点适配。
- **状态变体（State Variants）**：`hover:`、`focus:`、`disabled:` 等前缀处理交互状态。
- **JIT 编译（Just-In-Time）**：按需生成 CSS，最终产物极小（通常 < 10KB gzip）。
- **高度可定制（Customizable）**：通过配置文件扩展颜色、间距、字体等设计令牌。

本项目使用版本 / Version Used：`tailwindcss ^3.4.17` + `postcss ^8.5.1` + `autoprefixer ^10.4.20`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 Tailwind 配置 / Tailwind Configuration

文件 / File：`console/web/tailwind.config.js`

```javascript
/** @type {import('tailwindcss').Config} */
export default {
  // 内容扫描路径：Tailwind 扫描这些文件中的类名，按需生成 CSS
  // Content scan paths: Tailwind scans these files for class names, generates CSS on demand
  content: [
    "./index.html",                    // HTML 入口 / HTML entry
    "./src/**/*.{js,ts,jsx,tsx}",      // 所有源代码 / All source code
  ],
  theme: {
    extend: {},  // 未扩展默认主题，使用 Tailwind 内置设计令牌 / No theme extension, uses built-in tokens
  },
  plugins: [],   // 未使用额外插件 / No additional plugins
}
```

### 2.2 PostCSS 配置 / PostCSS Configuration

文件 / File：`console/web/postcss.config.js`

```javascript
// PostCSS 插件链：Tailwind 编译 → 自动添加浏览器前缀
// PostCSS plugin chain: Tailwind compile → auto-add browser prefixes
export default {
  plugins: {
    tailwindcss: {},     // Tailwind CSS 编译器 / Tailwind CSS compiler
    autoprefixer: {},    // 自动添加 -webkit-/-moz- 等前缀 / Auto vendor prefixes
  },
}
```

### 2.3 CSS 入口 / CSS Entry

文件 / File：`console/web/src/index.css`

```css
/* Tailwind 三层指令：基础重置 → 组件层 → 工具类层 */
/* Tailwind three-layer directives: base reset → components → utilities */
@tailwind base;       /* 重置浏览器默认样式 / Reset browser defaults */
@tailwind components; /* 组件类（本项目未使用）/ Component classes (unused) */
@tailwind utilities;  /* 工具类（核心）/ Utility classes (core) */
```

### 2.4 组件中的使用模式 / Usage Patterns in Components

```tsx
// 布局：Flexbox 三栏结构 / Layout: Flexbox 3-column structure
<div className="flex h-screen flex-col bg-gray-50">

// 状态灯：条件类名拼接 / Status pill: conditional class joining
<span className={[
  'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium',
  ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700',
].join(' ')} />

// 响应式：小屏隐藏分类标签 / Responsive: hide category label on small screens
<span className="hidden sm:inline-flex ...">

// 交互动画：悬停上浮 + 阴影 / Interaction: hover lift + shadow
className="transition-all hover:-translate-y-0.5 hover:shadow-md"

// 加载动画：旋转圆环 / Loading animation: spinning ring
<span className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-indigo-500" />
```

### 2.5 设计令牌使用 / Design Token Usage

| 用途 / Purpose | 类名示例 / Class Examples |
|---|---|
| 主色调（Indigo）| `bg-indigo-600`、`text-indigo-700`、`ring-indigo-100` |
| 成功状态（Emerald）| `bg-emerald-50`、`text-emerald-600` |
| 错误状态（Red）| `bg-red-50`、`text-red-700` |
| 圆角 | `rounded-md`（6px）、`rounded-lg`（8px）、`rounded-xl`（12px）|
| 间距 | `px-4`（16px）、`py-2`（8px）、`gap-3`（12px）|
| 字号 | `text-xs`（12px）、`text-sm`（14px）、`text-2xl`（24px）|

### 2.6 响应式设计与断点 / Responsive Design & Breakpoints

Tailwind 采用移动优先（mobile-first）的断点策略：
Tailwind uses mobile-first breakpoint strategy:

```tsx
// 断点前缀（无前缀 = 所有尺寸，sm: = ≥640px，md: = ≥768px，lg: = ≥1024px）
// Breakpoint prefixes (no prefix = all sizes, sm: = ≥640px, md: = ≥768px, lg: = ≥1024px)

// 小屏隐藏分类标签，大屏显示 / Hide category label on small, show on large
<span className="hidden sm:inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs">
  {sample.category}
</span>

// 侧边栏宽度响应式 / Sidebar width responsive
<aside className="w-56 md:w-64 lg:w-72 flex-shrink-0 overflow-y-auto border-r bg-white">

// 主区域网格布局响应式 / Main area grid layout responsive
<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
```

### 2.7 状态变体与交互 / State Variants & Interaction

```tsx
// 悬停效果：上浮 + 阴影 + 颜色变化 / Hover: lift + shadow + color change
className="transition-all hover:-translate-y-0.5 hover:shadow-md hover:border-indigo-200"

// 焦点状态：输入框聚焦时显示环 / Focus: ring on input focus
className="focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"

// 禁用状态：按钮不可点击时降低透明度 / Disabled: reduce opacity when not clickable
className="disabled:opacity-50 disabled:cursor-not-allowed"

// 条件类名拼接（成功/失败状态）/ Conditional class joining (success/failure)
className={[
  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
  ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700',
].join(' ')}
```

### 2.8 动画与过渡 / Animation & Transition

```tsx
// 加载旋转动画 / Loading spin animation
<span className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-indigo-500" />

// 平滑过渡（颜色、阴影、位移）/ Smooth transition (color, shadow, transform)
className="transition-colors duration-150"
className="transition-all duration-200 ease-in-out"

// 淡入效果（结合条件渲染）/ Fade-in (combined with conditional rendering)
className="animate-in fade-in duration-300"
```

### 2.9 为何选择 Tailwind / Why Tailwind

| 优势 / Advantage | 说明 / Description |
|---|---|
| 零 CSS 文件维护 / Zero CSS file maintenance | 所有样式内联在 JSX，无命名困难与死代码 / All styles inline in JSX |
| 一致性 / Consistency | 固定设计令牌，避免“魔法数字” / Fixed design tokens, no magic numbers |
| 性能 / Performance | JIT 按需生成，最终 CSS < 10KB gzip / On-demand generation |
| 开发速度 / Dev speed | 无需在 JSX 与 CSS 文件间切换 / No context switching |
| 类型安全 / Type safety | 配合 ESLint 插件检测无效类名 / ESLint plugin detects invalid classes |
| 无运行时开销 / No runtime cost | 构建时生成，不像 CSS-in-JS 需运行时计算 / Build-time, unlike CSS-in-JS |

### 2.10 JIT 引擎工作原理 / JIT Engine Working Principle

Tailwind CSS v3 默认启用 JIT（Just-In-Time）引擎：

```text
┌─────────────────────────────────────────────────────────────┐
│  1. 扫描源文件 / Scan source files                          │
│     content: ["./src/**/*.{ts,tsx}"]                        │
│     提取所有 class="..." 中的类名                        │
│     Extract all class names from class="..."                │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. 解析类名 / Parse class names                            │
│     "flex" → { display: flex }                              │
│     "px-4" → { padding-left: 1rem; padding-right: 1rem }    │
│     "hover:bg-blue-500" → :hover 变体 + 背景色             │
│     "sm:hidden" → @media (min-width: 640px) 媒体查询       │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 生成 CSS / Generate CSS                                  │
│     仅为实际使用的类名生成规则                            │
│     Only generate rules for actually used class names        │
│     未使用的类不会出现在产物中                            │
│     Unused classes won't appear in output                    │
└─────────────────────────────────────────────────────────────┘
```

**JIT vs 传统 AOT 对比 / JIT vs Traditional AOT**：

| 特性 / Feature | JIT (v3) | AOT (v2) |
|---|---|---|
| 生成时机 / Generation time | 构建时按需 / On-demand at build | 预生成所有组合 / Pre-generate all combos |
| 产物大小 / Output size | ~5-10KB gzip | ~2MB+ (full build) |
| 任意值 / Arbitrary values | ✅ `w-[137px]` | ❌ 不支持 / Not supported |
| 构建速度 / Build speed | 快（仅生成用到的）/ Fast | 慢（生成全部）/ Slow |

### 2.11 CSS 层叠与优先级 / CSS Cascade & Specificity

Tailwind 使用 `@layer` 指令控制层叠顺序：

```css
/* Tailwind 生成的 CSS 层叠顺序 / Tailwind's CSS cascade order */
@layer base {       /* 1️⃣ 最低优先级：浏览器重置 / Lowest: browser reset */
  *, ::before, ::after { box-sizing: border-box; }
}
@layer components { /* 2️⃣ 中优先级：自定义组件 / Medium: custom components */
  .btn-primary { @apply bg-indigo-600 text-white px-4 py-2 rounded; }
}
@layer utilities {  /* 3️⃣ 最高优先级：工具类 / Highest: utilities */
  .flex { display: flex; }
  .px-4 { padding-left: 1rem; padding-right: 1rem; }
}
```

**优先级规则 / Specificity Rules**：

```text
层叠顺序（低 → 高）/ Cascade order (low → high):
  base < components < utilities < 内联样式 (style="")

同一层内：后定义的规则覆盖先定义的
Within same layer: later rules override earlier ones

变体提升优先级 / Variants increase specificity:
  .hover\:bg-blue-500:hover  >  .bg-red-500
  （:hover 伪类提升优先级 / :hover pseudo-class increases specificity）
```

### 2.12 暗色模式支持 / Dark Mode Support

Tailwind 内置暗色模式支持（本项目未启用，但可轻松添加）：

```javascript
// tailwind.config.js 中启用暗色模式 / Enable dark mode in tailwind.config.js
export default {
  darkMode: 'class',  // 基于 .dark 类切换 / Toggle based on .dark class
  // darkMode: 'media',  // 跟随系统偏好 / Follow system preference
}
```

```tsx
// 使用 dark: 前缀 / Use dark: prefix
<div className="bg-white dark:bg-gray-900 text-gray-900 dark:text-white">
  {/* 浅色模式：白底黑字 / Light: white bg, black text */}
  {/* 深色模式：黑底白字 / Dark: dark bg, white text */}
</div>
```

### 2.13 性能分析与产物大小 / Performance Analysis & Output Size

**本项目构建产物分析 / Build output analysis**：

```text
构建前 / Before build:
  - Tailwind 完整框架：~3.5MB (所有工具类)
  - Tailwind full framework: ~3.5MB (all utilities)

构建后 / After build:
  - 实际生成 CSS：~8-15KB gzip
  - Actual generated CSS: ~8-15KB gzip
  - 仅包含项目实际使用的 ~200 个类
  - Only ~200 classes actually used in project
  - 压缩比：>99%
```

**影响产物大小的因素 / Factors affecting output size**：

| 因素 / Factor | 影响 / Impact |
|---|---|
| 使用的类名数量 / Number of classes used | 直接决定 / Directly determines |
| 响应式断点 / Responsive breakpoints | 每个断点复制一套规则 / Each breakpoint duplicates rules |
| 状态变体 / State variants | hover/focus 等增加规则数 / hover/focus increase rule count |
| 颜色调色板 / Color palette | 使用多种颜色增加规则 / More colors = more rules |

### 2.14 与其他 CSS 方案对比 / Comparison with Other CSS Solutions

| 方案 / Solution | 运行时开销 / Runtime | 产物大小 / Output | 开发体验 / DX | 类型安全 / Type-safe |
|---|---|---|---|---|
| **Tailwind CSS** | 零 / Zero | 极小 / Tiny | 极佳 / Excellent | 中 / Medium |
| CSS Modules | 零 / Zero | 小 / Small | 良好 / Good | 高 / High |
| styled-components | 有 / Yes | 中 / Medium | 良好 / Good | 高 / High |
| CSS-in-JS (Emotion) | 有 / Yes | 中 / Medium | 良好 / Good | 高 / High |
| 原生 CSS | 零 / Zero | 大（死代码）/ Large | 一般 / Fair | 无 / None |

## 3. @apply 指令详解 / @apply Directive Details

### 3.1 @apply 工作原理 / @apply Working Principle

`@apply` 允许在自定义 CSS 中复用 Tailwind 工具类：

```css
/* 在 @layer components 中使用 @apply / Use @apply in @layer components */
@layer components {
  /* 按钮基础样式 / Button base styles */
  .btn {
    @apply inline-flex items-center justify-center rounded-md px-4 py-2
           text-sm font-medium transition-colors focus:outline-none
           focus:ring-2 focus:ring-offset-2;
  }

  /* 主按钮变体 / Primary button variant */
  .btn-primary {
    @apply bg-indigo-600 text-white hover:bg-indigo-700
           focus:ring-indigo-500 disabled:opacity-50;
  }

  /* 卡片容器 / Card container */
  .card {
    @apply rounded-xl border border-gray-200 bg-white p-6
           shadow-sm transition-shadow hover:shadow-md;
  }
}
```

**编译结果 / Compiled Result**：

```css
/* @apply 被展开为实际 CSS 属性 / @apply expanded to actual CSS properties */
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  padding-left: 1rem;
  padding-right: 1rem;
  padding-top: 0.5rem;
  padding-bottom: 0.5rem;
  font-size: 0.875rem;
  font-weight: 500;
  transition-property: color, background-color, border-color;
  /* ... 更多属性 / more properties */
}
```

### 3.2 本项目为何不使用 @apply / Why This Project Doesn't Use @apply

| 原因 / Reason | 说明 / Description |
|---|---|
| 工具类已足够表达 / Utilities expressive enough | 直接在 JSX 中组合类名更直观 / Composing in JSX more intuitive |
| 避免 CSS 文件维护 / Avoid CSS file maintenance | @apply 需要维护额外的 CSS 文件 / Requires extra CSS files |
| 条件样式更灵活 / Conditional styles more flexible | JSX 中可用三元表达式动态切换 / Ternary expressions in JSX |
| 官方推荐 / Official recommendation | Tailwind 团队建议优先使用工具类 / Prefer utilities over @apply |

### 3.3 @apply 适用场景 / @apply Use Cases

```text
何时使用 @apply / When to use @apply:
  ✅ 第三方库需要类名（无法用 JSX）/ Third-party lib needs class name
  ✅ 多次复用的复杂组件样式 / Complex component styles reused many times
  ✅ CSS 文件中的媒体查询嵌套 / Media query nesting in CSS files
  ❌ 一次性样式（直接用工具类）/ One-off styles (use utilities directly)
  ❌ 动态条件样式（用 JSX 表达式）/ Dynamic conditional (use JSX expressions)
```

## 4. 设计令牌定制 / Design Token Customization

### 4.1 主题扩展机制 / Theme Extension Mechanism

```javascript
// tailwind.config.js 完整定制示例 / Full customization example
/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    // extend: 保留默认 + 新增 / Keep defaults + add new
    extend: {
      colors: {
        // 新增品牌色 / Add brand colors
        brand: {
          50: '#eef2ff',
          100: '#e0e7ff',
          500: '#6366f1',
          600: '#4f46e5',  // 主色 / Primary
          700: '#4338ca',
        },
      },
      spacing: {
        // 新增间距 / Add spacing
        '18': '4.5rem',
        '88': '22rem',
      },
      borderRadius: {
        '4xl': '2rem',
      },
      animation: {
        // 自定义动画 / Custom animation
        'fade-in': 'fadeIn 0.3s ease-in-out',
        'slide-up': 'slideUp 0.2s ease-out',
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { transform: 'translateY(10px)', opacity: '0' },
          '100%': { transform: 'translateY(0)', opacity: '1' },
        },
      },
    },
    // 直接覆盖（谨慎使用）/ Direct override (use carefully)
    // screens: {
    //   'sm': '640px',
    //   'md': '768px',
    //   'lg': '1024px',
    //   'xl': '1280px',
    // },
  },
  plugins: [],
}
```

### 4.2 默认设计令牌体系 / Default Design Token System

| 类别 / Category | 令牌示例 / Token Examples | 值 / Values |
|---|---|---|
| 颜色 / Colors | `gray-50` ~ `gray-950` | 11 级灰度 / 11 gray shades |
| 间距 / Spacing | `px-1` ~ `px-96` | 0.25rem ~ 24rem |
| 字号 / Font size | `text-xs` ~ `text-9xl` | 12px ~ 128px |
| 圆角 / Border radius | `rounded-sm` ~ `rounded-3xl` | 2px ~ 24px |
| 阴影 / Shadow | `shadow-sm` ~ `shadow-2xl` | 6 级 / 6 levels |
| 断点 / Breakpoints | `sm` ~ `2xl` | 640px ~ 1536px |
| 字重 / Font weight | `font-thin` ~ `font-black` | 100 ~ 900 |
| 过渡 / Transition | `duration-75` ~ `duration-1000` | 75ms ~ 1000ms |

### 4.3 任意值（Arbitrary Values）/ Arbitrary Values

```tsx
// JIT 引擎支持任意值（无需配置）/ JIT engine supports arbitrary values (no config)
<div className="w-[137px]" />           // 任意宽度 / Arbitrary width
<div className="bg-[#1a1a2e]" />        // 任意颜色 / Arbitrary color
<div className="grid-cols-[1fr_2fr]" /> // 任意网格 / Arbitrary grid
<div className="text-[13px]" />         // 任意字号 / Arbitrary font size
<div className="mt-[calc(100vh-64px)]" /> // 任意计算 / Arbitrary calc

// 本项目使用示例 / Usage in this project:
// 无（优先使用标准令牌保持一致性）/ None (prefer standard tokens for consistency)
```

## 5. 可访问性（A11y）支持 / Accessibility (A11y) Support

### 5.1 焦点管理 / Focus Management

```tsx
// 可见焦点指示器（键盘导航）/ Visible focus indicator (keyboard navigation)
<button className="
  rounded-md px-4 py-2
  focus:outline-none              /* 移除默认轮廓 / Remove default outline */
  focus:ring-2                    /* 2px 环 / 2px ring */
  focus:ring-indigo-500           /* 环颜色 / Ring color */
  focus:ring-offset-2             /* 环偏移 / Ring offset */
">
  提交 / Submit
</button>

// 仅键盘用户可见的焦点样式 / Focus styles visible only to keyboard users
<a className="
  focus-visible:ring-2            /* 仅键盘导航时显示 / Only show on keyboard nav */
  focus-visible:ring-indigo-500
">
  链接 / Link
</a>
```

### 5.2 屏幕阅读器支持 / Screen Reader Support

```tsx
// 仅屏幕阅读器可见的文本 / Screen reader only text
<span className="sr-only">加载状态：成功 / Loading status: success</span>

// 跳过导航链接 / Skip navigation link
<a href="#main-content" className="sr-only focus:not-sr-only">
  跳到主内容 / Skip to main content
</a>

// ARIA 属性配合 / ARIA attributes配合
<button
  aria-label="关闭对话框 / Close dialog"
  aria-expanded={isOpen}
  className="rounded-full p-1 hover:bg-gray-100"
>
  <XIcon className="h-5 w-5" />
</button>
```

### 5.3 颜色对比度 / Color Contrast

| 组合 / Combination | 对比度 / Contrast Ratio | WCAG 等级 / WCAG Level |
|---|---|---|
| `text-gray-900` on `bg-white` | 17.4:1 | AAA ✅ |
| `text-gray-600` on `bg-white` | 5.7:1 | AA ✅ |
| `text-white` on `bg-indigo-600` | 4.6:1 | AA ✅ |
| `text-emerald-700` on `bg-emerald-50` | 5.2:1 | AA ✅ |
| `text-red-700` on `bg-red-50` | 5.4:1 | AA ✅ |

## 6. 组件模式与最佳实践 / Component Patterns & Best Practices

### 6.1 条件类名组织 / Conditional Class Organization

```tsx
// 模式 1：数组 + join（本项目使用）/ Pattern 1: Array + join (this project)
<span className={[
  'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
  ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700',
].join(' ')} />

// 模式 2：模板字符串 / Pattern 2: Template string
<div className={`card ${isActive ? 'ring-2 ring-indigo-500' : ''}`} />

// 模式 3：clsx/cn 工具（大型项目推荐）/ Pattern 3: clsx/cn utility (large projects)
import { cn } from '@/lib/utils';  // classnames + tailwind-merge
<div className={cn('card', isActive && 'ring-2 ring-indigo-500')} />
```

### 6.2 响应式组件模式 / Responsive Component Patterns

```tsx
// 本项目中的响应式布局 / Responsive layout in this project
function App() {
  return (
    <div className="flex h-screen flex-col bg-gray-50">
      {/* 固定头部 / Fixed header */}
      <Header />

      <div className="flex flex-1 overflow-hidden">
        {/* 侧边栏：小屏隐藏，大屏显示 / Sidebar: hidden on small, visible on large */}
        <aside className="hidden md:flex w-64 flex-col border-r bg-white">
          <Sidebar />
        </aside>

        {/* 主内容区 / Main content area */}
        <main className="flex-1 overflow-y-auto p-6">
          {renderView()}
        </main>
      </div>
    </div>
  );
}
```

### 6.3 加载状态与骨架屏 / Loading States & Skeleton

```tsx
// 加载旋转器 / Loading spinner
function Spinner() {
  return (
    <span className="
      h-8 w-8 animate-spin rounded-full
      border-2 border-gray-200 border-t-indigo-500
    " />
  );
}

// 骨架屏（加载占位）/ Skeleton (loading placeholder)
function CardSkeleton() {
  return (
    <div className="animate-pulse rounded-xl border p-6">
      <div className="h-4 w-3/4 rounded bg-gray-200" />      {/* 标题 / Title */}
      <div className="mt-3 h-3 w-full rounded bg-gray-100" /> {/* 内容 / Content */}
      <div className="mt-2 h-3 w-2/3 rounded bg-gray-100" />  {/* 内容 / Content */}
    </div>
  );
}
```

## 7. 构建优化与树摇 / Build Optimization & Tree-shaking

### 7.1 内容扫描优化 / Content Scanning Optimization

```javascript
// tailwind.config.js 中的 content 配置直接影响构建性能
// content config directly affects build performance
content: [
  "./index.html",                    // ✅ 必要 / Necessary
  "./src/**/*.{js,ts,jsx,tsx}",      // ✅ 精确匹配 / Precise match
  // "./node_modules/flowbite/**/*.js"  // ✅ 第三方组件库 / Third-party lib
  // "./**/*"                           // ❌ 太宽泛，极慢 / Too broad, very slow
]
```

### 7.2 生产构建流程 / Production Build Flow

```text
┌─────────────────────────────────────────────────────────────┐
│  1. 扫描阶段 / Scan Phase                                    │
│     - 遍历 content 中所有文件 / Traverse all content files  │
│     - 提取 class="..." 中的类名 / Extract class names       │
│     - 解析变体（hover:, sm: 等）/ Parse variants           │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. 生成阶段 / Generation Phase                              │
│     - 为每个类名生成 CSS 规则 / Generate CSS per class      │
│     - 处理变体（媒体查询/伪类）/ Handle variants           │
│     - 注入 Preflight 重置 / Inject Preflight reset          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 优化阶段 / Optimization Phase                            │
│     - 移除未使用的 CSS（树摇）/ Remove unused CSS           │
│     - autoprefixer 添加前缀 / Add prefixes                  │
│     - esbuild 压缩 / Minify with esbuild                    │
│     - 输出独立 CSS 文件 / Output standalone CSS file        │
└─────────────────────────────────────────────────────────────┘
```

### 7.3 产物分析 / Output Analysis

```bash
# 查看生成的 CSS 大小 / Check generated CSS size
ls -lh console/web/dist/assets/*.css

# 统计实际使用的类数 / Count actual classes used
grep -o '\.[a-z-]*' console/web/dist/assets/index-*.css | sort -u | wc -l

# 查看是否包含特定工具类 / Check if specific utility is included
grep '.flex' console/web/dist/assets/index-*.css | head -5
```

## 8. 响应式与暗色模式设计 / Responsive & Dark Mode Design

### 8.1 移动优先断点系统 / Mobile-first Breakpoint System

Tailwind 采用 `min-width` 移动优先策略，断点从小到大生效：

| 前缀 / Prefix | 断点 / Breakpoint | 生效范围 / Active Range | 典型设备 / Typical Device |
|---|---|---|---|
| （无）/ None | 0px | 所有尺寸 / All sizes | 手机 / Mobile |
| `sm:` | 640px | ≥640px | 大屏手机 / Large phones |
| `md:` | 768px | ≥768px | 平板 / Tablets |
| `lg:` | 1024px | ≥1024px | 笔记本 / Laptops |
| `xl:` | 1280px | ≥1280px | 桌面 / Desktops |
| `2xl:` | 1536px | ≥1536px | 大屏 / Large screens |

### 8.2 本项目响应式实践 / Responsive Practice in This Project

```tsx
// 控制台布局：小屏堆叠，大屏并排 / Console layout: stacked on small, side-by-side on large
<div className="flex flex-col lg:flex-row gap-4">
  {/* 侧边栏：小屏全宽，大屏固定宽度 / Sidebar: full-width small, fixed large */}
  <aside className="w-full lg:w-64 shrink-0">
    <EndpointList />
  </aside>
  {/* 主内容区：自动填充剩余空间 / Main content: fill remaining space */}
  <main className="flex-1 min-w-0">
    <ResponseViewer />
  </main>
</div>

// 表格响应式：小屏隐藏次要列 / Table responsive: hide secondary columns on small
<table className="w-full text-sm">
  <thead>
    <tr>
      <th className="px-3 py-2">端点 / Endpoint</th>
      <th className="px-3 py-2 hidden md:table-cell">协议 / Protocol</th>
      <th className="px-3 py-2 hidden lg:table-cell">耗时 / Duration</th>
    </tr>
  </thead>
</table>
```

### 8.3 暗色模式实现 / Dark Mode Implementation

```tsx
// tailwind.config.js 中的暗色模式策略 / Dark mode strategy in tailwind.config.js
// darkMode: 'class'  → 由 <html class="dark"> 控制 / Controlled by <html class="dark">
// darkMode: 'media'  → 跟随系统偏好 / Follow system preference

// 组件中的暗色适配 / Dark adaptation in components
<div className="
  bg-white dark:bg-gray-900
  text-gray-900 dark:text-gray-100
  border-gray-200 dark:border-gray-700
">
  {/* 内容 / Content */}
</div>
```

**本项目未启用暗色模式的原因 / Why dark mode is not enabled**：
- 控制台为内部测试工具，使用场景固定 / Console is internal test tool, fixed usage scenario
- 减少 CSS 产物体积（每个 dark: 变体都生成额外规则）/ Reduce CSS output (each dark: variant generates extra rules)
- 如未来需要，仅需在 config 中设置 `darkMode: 'class'` / If needed, just set `darkMode: 'class'` in config

## 9. 动画与过渡工具类 / Animation & Transition Utilities

### 9.1 过渡系统 / Transition System

```tsx
// 按钮悬停过渡 / Button hover transition
<button className="
  bg-indigo-600 hover:bg-indigo-700
  transition-colors duration-200 ease-in-out
">
  发送请求 / Send Request
</button>

// 展开/折叠动画 / Expand/collapse animation
<div className="
  overflow-hidden
  transition-all duration-300 ease-in-out
  max-h-0 opacity-0          /* 折叠态 / Collapsed */
  group-open:max-h-96 group-open:opacity-100  /* 展开态 / Expanded */
">
  {content}
</div>
```

### 9.2 过渡属性分类 / Transition Property Categories

| 工具类 / Utility | 过渡属性 / Transition Properties | 适用场景 / Use Case |
|---|---|---|
| `transition-all` | 所有属性 / All properties | 通用（性能较差）/ General (poor perf) |
| `transition-colors` | color, background, border | 悬停变色 / Hover color change |
| `transition-opacity` | opacity | 淡入淡出 / Fade in/out |
| `transition-transform` | transform | 缩放/位移 / Scale/translate |
| `transition-shadow` | box-shadow | 阴影变化 / Shadow change |

### 9.3 内置动画 / Built-in Animations

```tsx
// 加载指示器 / Loading indicator
<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600" />

// 脉冲提示 / Pulse notification
<span className="animate-pulse bg-red-500 rounded-full h-3 w-3" />

// 淡入 / Fade in (bounce)
<div className="animate-bounce">↓</div>
```

**本项目动画使用场景 / Animation usage in this project**：
- 请求加载状态：`animate-spin` / Request loading: `animate-spin`
- 响应到达提示：`transition-opacity` / Response arrival: `transition-opacity`
- 按钮交互反馈：`transition-colors` / Button interaction: `transition-colors`

## 10. 插件开发机制 / Plugin Development Mechanism

### 10.1 插件 API / Plugin API

```javascript
// tailwind.config.js
const plugin = require('tailwindcss/plugin');

module.exports = {
  plugins: [
    // 添加自定义工具类 / Add custom utilities
    plugin(function({ addUtilities, addComponents, theme }) {
      // 工具类：像 flex, p-4 一样的原子类 / Utilities: atomic classes like flex, p-4
      addUtilities({
        '.text-balance': { textWrap: 'balance' },
        '.scrollbar-hide': {
          '-ms-overflow-style': 'none',
          'scrollbar-width': 'none',
          '&::-webkit-scrollbar': { display: 'none' },
        },
      });

      // 组件类：像 .btn, .card 一样的复合类 / Components: compound classes like .btn
      addComponents({
        '.btn-primary': {
          padding: `${theme('spacing.2')} ${theme('spacing.4')}`,
          borderRadius: theme('borderRadius.md'),
          backgroundColor: theme('colors.indigo.600'),
          color: 'white',
          '&:hover': { backgroundColor: theme('colors.indigo.700') },
        },
      });
    }),
  ],
};
```

### 10.2 插件类型对比 / Plugin Type Comparison

| 类型 / Type | API | 生成时机 / Generation | 示例 / Example |
|---|---|---|---|
| 工具类 / Utilities | `addUtilities()` | 按需（使用时）/ On demand | `.text-balance` |
| 组件 / Components | `addComponents()` | 按需 / On demand | `.btn-primary` |
| 基础 / Base | `addBase()` | 始终包含 / Always included | CSS reset |
| 变体 / Variants | `addVariant()` | 按需 / On demand | `group-hover:` |

### 10.3 本项目未使用自定义插件的原因 / Why No Custom Plugins

| 原因 / Reason | 说明 / Description |
|---|---|
| 原子类已足够 / Atomic classes suffice | 控制台 UI 简单，无需复合组件 / Console UI is simple |
| 避免维护成本 / Avoid maintenance cost | 插件需随 Tailwind 版本更新 / Plugins need version updates |
| 内联样式更灵活 / Inline styles more flexible | 测试控制台频繁变更 / Test console changes frequently |

## 11. CSS 层叠层与优先级 / CSS Cascade Layers & Specificity

### 11.1 Tailwind 的层叠顺序 / Tailwind's Cascade Order

```css
/* Tailwind v3.4+ 使用 @layer 管理优先级 / Uses @layer for priority */
@layer base {
  /* Preflight 重置（最低优先级）/ Preflight reset (lowest priority) */
  *, ::before, ::after { box-sizing: border-box; border-width: 0; }
}

@layer components {
  /* 组件类（中优先级）/ Component classes (medium priority) */
  .btn-primary { ... }
}

@layer utilities {
  /* 工具类（最高优先级）/ Utilities (highest priority) */
  .flex { display: flex; }
  .p-4 { padding: 1rem; }
}
```

### 11.2 优先级冲突解决 / Specificity Conflict Resolution

| 场景 / Scenario | 解决方式 / Resolution | 示例 / Example |
|---|---|---|
| 工具类 vs Preflight | 工具类自动胜出 / Utilities auto-win | `border` 覆盖 reset |
| 工具类 vs 组件类 | 工具类优先 / Utilities take priority | `.p-4` 覆盖 `.card` padding |
| 同类名冲突 / Same class conflict | 后声明者胜出 / Later declaration wins | 配置顺序决定 / Config order decides |
| 需要 `!important` | 添加 `!` 前缀 / Add `!` prefix | `!text-red-500` |

### 11.3 与第三方 CSS 共存 / Coexistence with Third-party CSS

```css
/* index.css 中的层叠管理 / Cascade management in index.css */
@tailwind base;      /* → @layer base */
@tailwind components; /* → @layer components */
@tailwind utilities;  /* → @layer utilities */

/* 第三方库样式（无层，优先级最高）/ Third-party styles (no layer, highest priority) */
@import 'some-lib/styles.css';

/* 注意：无层 CSS 始终覆盖有层 CSS / Note: unlayered CSS always overrides layered CSS */
/* 如需降低第三方优先级，可包裹在层中 / To lower third-party priority, wrap in layer */
@layer third-party {
  @import 'some-lib/styles.css';
}
```

## 12. JIT 引擎与按需生成 / JIT Engine & On-demand Generation

### 12.1 JIT 编译原理 / JIT Compilation Principles

Tailwind CSS v3+ 默认启用 JIT（Just-In-Time）引擎，彻底改变了 CSS 生成方式：

```text
传统模式 (v2) / Legacy mode (v2):
┌───────────────────────────────────────────────────┐
│  预生成所有工具类 / Pre-generate ALL utilities    │
│  - 生成 ~4MB CSS / Generates ~4MB CSS            │
│  - 构建时间 30s+ / Build time 30s+               │
│  - 需要 PurgeCSS 清理 / Requires PurgeCSS        │
└───────────────────────────────────────────────────┘

JIT 模式 (v3+) / JIT mode (v3+):
┌───────────────────────────────────────────────────┐
│  扫描源文件，按需生成 / Scan sources, generate    │
│  - 仅生成使用的类 / Only used classes generated  │
│  - 构建时间 < 1s / Build time < 1s               │
│  - 无需 PurgeCSS / No PurgeCSS needed            │
│  - 支持任意值 / Supports arbitrary values        │
└───────────────────────────────────────────────────┘
```

### 12.2 内容扫描配置 / Content Scanning Configuration

```javascript
// tailwind.config.js — 本项目配置 / This project's config
module.exports = {
  content: [
    "./index.html",              // HTML 入口 / HTML entry
    "./src/**/*.{ts,tsx}",       // React 组件 / React components
  ],
  // JIT 扫描规则 / JIT scanning rules:
  // - 提取所有 class="" 中的类名 / Extract all class names in class=""
  // - 提取 className={...} 中的字符串 / Extract strings in className={}
  // - 提取模板字面量中的类 / Extract classes in template literals
  // - 忽略 node_modules / Ignore node_modules
}
```

### 12.3 任意值与动态类 / Arbitrary Values & Dynamic Classes

```tsx
// JIT 支持的任意值语法 / Arbitrary value syntax supported by JIT
<div className="w-[calc(100%-2rem)]" />     // 任意宽度 / Arbitrary width
<div className="text-[#1a1a2e]" />          // 任意颜色 / Arbitrary color
<div className="grid-cols-[1fr_2fr_1fr]" /> // 任意网格 / Arbitrary grid
<div className="max-h-[80vh]" />            // 任意最大高 / Arbitrary max-h

// ❗ 动态类名陷阱 / Dynamic class name pitfall:
// ❌ 错误 / Wrong: JIT 无法检测拼接的类 / JIT can't detect concatenated classes
const color = "red";
<div className={`text-${color}-500`} />  // 不会生成 / Won't generate

// ✅ 正确 / Correct: 使用完整类名 / Use complete class names
const colorClass = "text-red-500";
<div className={colorClass} />           // 正常生成 / Generates correctly

// ✅ 或使用映射 / Or use mapping
const colors = { red: "text-red-500", blue: "text-blue-500" };
<div className={colors[color]} />
```

## 13. 状态变体与交互 / State Variants & Interaction

### 13.1 变体系统全景 / Variant System Overview

```text
变体类型 / Variant types:
┌─────────────────────────────────────────────────────────┐
│  伪类变体 / Pseudo-class variants                       │
│  hover: focus: active: disabled: visited: first: last:  │
├─────────────────────────────────────────────────────────┤
│  伪元素变体 / Pseudo-element variants                   │
│  before: after: placeholder: selection: file:           │
├─────────────────────────────────────────────────────────┤
│  状态变体 / State variants                              │
│  group-hover: peer-focus: aria-expanded: data-[state]:  │
├─────────────────────────────────────────────────────────┤
│  媒体变体 / Media variants                              │
│  dark: motion-reduce: print: contrast-more:             │
├─────────────────────────────────────────────────────────┤
│  响应式变体 / Responsive variants                       │
│  sm: md: lg: xl: 2xl: (min-width 断点 / breakpoints)    │
└─────────────────────────────────────────────────────────┘
```

### 13.2 本项目的状态变体实践 / This Project's State Variant Practice

```tsx
// 按钮状态 / Button states
<button className={cn(
  "px-4 py-2 rounded-lg font-medium transition-all duration-200",
  "bg-blue-600 text-white",
  "hover:bg-blue-700 hover:shadow-lg",       // 悬停 / Hover
  "active:scale-[0.98]",                     // 按下 / Active
  "focus:outline-none focus:ring-2 focus:ring-blue-500/50",  // 焦点 / Focus
  "disabled:opacity-50 disabled:cursor-not-allowed",         // 禁用 / Disabled
)}>
  发送请求 / Send Request
</button>

// 输入框状态 / Input states
<input className={cn(
  "w-full px-3 py-2 border rounded-md",
  "border-gray-300 dark:border-gray-600",
  "focus:border-blue-500 focus:ring-1 focus:ring-blue-500",  // 焦点 / Focus
  "placeholder:text-gray-400",              // 占位符 / Placeholder
  "invalid:border-red-500 invalid:ring-red-500",  // 验证失败 / Invalid
)} />

// 组状态 (group) / Group state
<div className="group cursor-pointer">
  <span className="text-gray-700 group-hover:text-blue-600">端点 / Endpoint</span>
  <svg className="opacity-0 group-hover:opacity-100 transition-opacity">
    {/* 悬停时显示图标 / Show icon on hover */}
  </svg>
</div>
```

### 13.3 变体堆叠与优先级 / Variant Stacking & Priority

```tsx
// 变体可以堆叠 / Variants can be stacked
<div className="dark:hover:bg-gray-700" />
// 等价 CSS / Equivalent CSS:
// @media (prefers-color-scheme: dark) {
//   .dark\:hover\:bg-gray-700:hover { background-color: ... }
// }

// 响应式 + 状态组合 / Responsive + state combo
<button className="w-full sm:w-auto hover:bg-blue-700 lg:hover:bg-blue-800" />

// 优先级规则 / Priority rules:
// 1. 后定义的变体优先级更高 / Later variants have higher priority
// 2. 响应式变体 > 基础类 / Responsive > base
// 3. 状态变体 > 基础类 / State > base
// 4. 具体性相同时，源码顺序决定 / Same specificity → source order
```

## 14. 与 React 组件集成 / Integration with React Components

### 14.1 条件类名模式 / Conditional Class Patterns

```tsx
// 本项目使用 clsx/tailwind-merge 管理条件类 / This project uses clsx/tailwind-merge
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// 工具函数：合并 + 去重 / Utility: merge + deduplicate
function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// 组件中使用 / Usage in components
interface ButtonProps {
  variant?: "primary" | "secondary" | "danger";
  size?: "sm" | "md" | "lg";
  isLoading?: boolean;
  className?: string;  // 允许外部覆盖 / Allow external override
}

function Button({ variant = "primary", size = "md", isLoading, className }: ButtonProps) {
  return (
    <button className={cn(
      // 基础样式 / Base styles
      "inline-flex items-center justify-center rounded-md font-medium",
      "transition-colors focus:outline-none focus:ring-2",
      // 变体映射 / Variant mapping
      {
        primary: "bg-blue-600 text-white hover:bg-blue-700",
        secondary: "bg-gray-200 text-gray-800 hover:bg-gray-300",
        danger: "bg-red-600 text-white hover:bg-red-700",
      }[variant],
      // 尺寸映射 / Size mapping
      {
        sm: "px-2 py-1 text-sm",
        md: "px-4 py-2 text-base",
        lg: "px-6 py-3 text-lg",
      }[size],
      // 条件状态 / Conditional state
      isLoading && "opacity-70 cursor-wait",
      // 外部覆盖（twMerge 解决冲突）/ External override (twMerge resolves conflicts)
      className,
    )}>
      {isLoading && <Spinner className="mr-2 h-4 w-4 animate-spin" />}
      {children}
    </button>
  );
}
```

### 14.2 组件样式封装策略 / Component Style Encapsulation Strategy

| 策略 / Strategy | 实现 / Implementation | 优点 / Pros | 缺点 / Cons |
|---|---|---|---|
| 内联工具类 / Inline utilities | className="..." | 零抽象成本 / Zero abstraction | 类名较长 / Long class names |
| cn() 工具函数 / cn() utility | twMerge(clsx(...)) | 冲突解决 / Conflict resolution | 需额外依赖 / Extra dependency |
| @apply 抽取 / @apply extraction | CSS 文件 / CSS file | 复用性 / Reusability | 失去可见性 / Lost visibility |
| 组件抽象 / Component abstraction | React 组件 / React component | 类型安全 / Type-safe | 过度抽象风险 / Over-abstraction |

### 14.3 本项目组件样式架构 / This Project's Component Style Architecture

```text
样式分层 / Style layers:
┌─────────────────────────────────────────────────────────┐
│  Layer 1: Tailwind 工具类 / Tailwind utilities          │
│  - 布局、间距、颜色、字体 / Layout, spacing, color, font │
│  - 直接在 JSX 中使用 / Used directly in JSX             │
├─────────────────────────────────────────────────────────┤
│  Layer 2: cn() 条件组合 / cn() conditional composition  │
│  - 变体、状态、响应式 / Variants, states, responsive    │
│  - 动态类名管理 / Dynamic class management              │
├─────────────────────────────────────────────────────────┤
│  Layer 3: 组件封装 / Component encapsulation            │
│  - Button, Input, Card 等 / Reusable UI components      │
│  - 接受 className prop / Accept className prop          │
├─────────────────────────────────────────────────────────┤
│  Layer 4: 全局样式 / Global styles                      │
│  - index.css 中的 @layer base / @layer base in CSS      │
│  - 滚动条、字体等 / Scrollbar, fonts, etc.              │
└─────────────────────────────────────────────────────────┘
```

---

## 15. 多主题设计系统 / Multi-theme Design System

### 15.1 CSS 变量驱动的主题架构 / CSS Variable-driven Theme Architecture

使用 CSS 自定义属性作为主题令牌，可以在运行时动态切换主题：

```css
/* ===== 主题令牌定义 / Theme Token Definitions ===== */
/* 文件: src/styles/themes.css */

/* 默认主题（亮色）/ Default theme (light) */
:root {
  /* 主色调 / Primary colors */
  --color-primary: 59 130 246;       /* blue-500 */
  --color-primary-hover: 37 99 235;  /* blue-600 */
  --color-primary-light: 219 234 254; /* blue-100 */

  /* 背景色 / Background colors */
  --color-bg-base: 255 255 255;
  --color-bg-surface: 249 250 251;   /* gray-50 */
  --color-bg-elevated: 255 255 255;

  /* 文本色 / Text colors */
  --color-text-primary: 17 24 39;    /* gray-900 */
  --color-text-secondary: 107 114 128; /* gray-500 */
  --color-text-muted: 156 163 175;   /* gray-400 */

  /* 边框 / Borders */
  --color-border: 229 231 235;       /* gray-200 */
  --color-border-focus: 59 130 246;  /* blue-500 */

  /* 语义色 / Semantic colors */
  --color-success: 34 197 94;        /* green-500 */
  --color-warning: 234 179 8;        /* yellow-500 */
  --color-error: 239 68 68;          /* red-500 */

  /* 间距系统 / Spacing system */
  --space-xs: 0.25rem;
  --space-sm: 0.5rem;
  --space-md: 1rem;
  --space-lg: 1.5rem;
  --space-xl: 2rem;

  /* 圆角 / Border radius */
  --radius-sm: 0.25rem;
  --radius-md: 0.375rem;
  --radius-lg: 0.5rem;
}

/* 暗色主题 / Dark theme */
[data-theme='dark'] {
  --color-primary: 96 165 250;       /* blue-400 */
  --color-primary-hover: 147 197 253; /* blue-300 */
  --color-primary-light: 30 58 138;  /* blue-900 */

  --color-bg-base: 17 24 39;         /* gray-900 */
  --color-bg-surface: 31 41 55;      /* gray-800 */
  --color-bg-elevated: 55 65 81;     /* gray-700 */

  --color-text-primary: 243 244 246; /* gray-100 */
  --color-text-secondary: 156 163 175; /* gray-400 */
  --color-text-muted: 107 114 128;   /* gray-500 */

  --color-border: 55 65 81;          /* gray-700 */
  --color-border-focus: 96 165 250;  /* blue-400 */
}
```

### 15.2 Tailwind 配置集成 / Tailwind Config Integration

```javascript
// ===== tailwind.config.js 中引用 CSS 变量 / Reference CSS vars in config =====
/** @type {import('tailwindcss').Config} */
module.exports = {
  theme: {
    extend: {
      colors: {
        // 使用 RGB 变量以支持透明度 / Use RGB vars for opacity support
        primary: {
          DEFAULT: 'rgb(var(--color-primary) / <alpha-value>)',
          hover: 'rgb(var(--color-primary-hover) / <alpha-value>)',
          light: 'rgb(var(--color-primary-light) / <alpha-value>)',
        },
        surface: {
          base: 'rgb(var(--color-bg-base) / <alpha-value>)',
          DEFAULT: 'rgb(var(--color-bg-surface) / <alpha-value>)',
          elevated: 'rgb(var(--color-bg-elevated) / <alpha-value>)',
        },
        content: {
          primary: 'rgb(var(--color-text-primary) / <alpha-value>)',
          secondary: 'rgb(var(--color-text-secondary) / <alpha-value>)',
          muted: 'rgb(var(--color-text-muted) / <alpha-value>)',
        },
      },
      borderRadius: {
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
      },
    },
  },
};
```

### 15.3 主题切换实现 / Theme Switching Implementation

```tsx
// ===== React 主题切换 Hook / React Theme Switch Hook =====
import { useState, useEffect, useCallback } from 'react';

type Theme = 'light' | 'dark' | 'system';

function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    return (localStorage.getItem('theme') as Theme) || 'system';
  });

  const applyTheme = useCallback((t: Theme) => {
    const root = document.documentElement;
    const isDark = t === 'dark' ||
      (t === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);

    root.setAttribute('data-theme', isDark ? 'dark' : 'light');
  }, []);

  useEffect(() => {
    applyTheme(theme);
    localStorage.setItem('theme', theme);

    // 监听系统主题变化 / Listen for system theme changes
    if (theme === 'system') {
      const mq = window.matchMedia('(prefers-color-scheme: dark)');
      const handler = () => applyTheme('system');
      mq.addEventListener('change', handler);
      return () => mq.removeEventListener('change', handler);
    }
  }, [theme, applyTheme]);

  return { theme, setTheme };
}

// 组件中使用 / Use in component
function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  return (
    <div className="flex gap-1 p-1 bg-surface rounded-lg">
      {(['light', 'dark', 'system'] as const).map((t) => (
        <button
          key={t}
          onClick={() => setTheme(t)}
          className={
            theme === t
              ? 'px-3 py-1 rounded-md bg-primary text-white text-sm'
              : 'px-3 py-1 rounded-md text-content-secondary hover:text-content-primary text-sm'
          }
        >
          {t === 'light' ? '☀️' : t === 'dark' ? '🌙' : '💻'}
        </button>
      ))}
    </div>
  );
}
```

---

## 16. 布局体系与响应式策略 / Layout System & Responsive Strategy

### 16.1 Flexbox 布局模式 / Flexbox Layout Patterns

Tailwind 的 Flexbox 工具类覆盖了所有常见布局场景：

```tsx
// ===== 常见 Flex 布局模式 / Common Flex Layout Patterns =====

// 1. 经典圣杯布局 / Classic Holy Grail Layout
function AppLayout() {
  return (
    <div className="flex flex-col h-screen">
      {/* 固定头部 / Fixed header */}
      <header className="flex items-center justify-between px-6 h-14 border-b border-gray-200 shrink-0">
        <h1 className="text-lg font-semibold">Privacy Console</h1>
        <nav className="flex gap-4">
          <a href="#" className="text-sm text-gray-600 hover:text-gray-900">Docs</a>
          <a href="#" className="text-sm text-gray-600 hover:text-gray-900">Settings</a>
        </nav>
      </header>

      {/* 主体区域 / Main area */}
      <div className="flex flex-1 overflow-hidden">
        {/* 侧边栏 / Sidebar */}
        <aside className="w-64 border-r border-gray-200 overflow-y-auto shrink-0">
          <SidebarNav />
        </aside>

        {/* 内容区 / Content */}
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

// 2. 卡片网格 / Card Grid
function EndpointGrid() {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {endpoints.map(ep => (
        <div key={ep.id} className="flex flex-col p-4 border rounded-lg hover:shadow-md transition-shadow">
          <span className="text-xs font-mono px-2 py-0.5 bg-blue-100 text-blue-700 rounded self-start">
            {ep.method}
          </span>
          <h3 className="mt-2 font-medium text-gray-900">{ep.name}</h3>
          <p className="mt-1 text-sm text-gray-500 flex-1">{ep.description}</p>
          <button className="mt-3 text-sm text-blue-600 hover:text-blue-800 self-end">
            Try it →
          </button>
        </div>
      ))}
    </div>
  );
}

// 3. 工具栏布局 / Toolbar Layout
function RequestToolbar() {
  return (
    <div className="flex items-center gap-2">
      {/* 左侧固定 / Left fixed */}
      <select className="w-24 px-2 py-1.5 border rounded text-sm font-mono">
        <option>GET</option>
        <option>POST</option>
      </select>

      {/* 中间弹性 / Middle flexible */}
      <input
        className="flex-1 min-w-0 px-3 py-1.5 border rounded text-sm font-mono"
        placeholder="/api/mask"
      />

      {/* 右侧固定 / Right fixed */}
      <button className="px-4 py-1.5 bg-blue-600 text-white rounded text-sm hover:bg-blue-700 shrink-0">
        Send
      </button>
    </div>
  );
}
```

### 16.2 Grid 布局进阶 / Advanced Grid Layout

```tsx
// ===== Grid 布局模式 / Grid Layout Patterns =====

// 响应式仪表盘 / Responsive Dashboard
function Dashboard() {
  return (
    <div className="grid grid-cols-12 gap-4 auto-rows-min">
      {/* 概览卡片 - 占 3 列 / Overview cards - span 3 cols */}
      <div className="col-span-12 sm:col-span-6 lg:col-span-3 p-4 bg-white rounded-lg border">
        <StatCard title="Total Requests" value="1,234" />
      </div>
      <div className="col-span-12 sm:col-span-6 lg:col-span-3 p-4 bg-white rounded-lg border">
        <StatCard title="Avg Latency" value="42ms" />
      </div>
      <div className="col-span-12 sm:col-span-6 lg:col-span-3 p-4 bg-white rounded-lg border">
        <StatCard title="Error Rate" value="0.1%" />
      </div>
      <div className="col-span-12 sm:col-span-6 lg:col-span-3 p-4 bg-white rounded-lg border">
        <StatCard title="Uptime" value="99.9%" />
      </div>

      {/* 图表区 - 占 8 列 / Chart area - span 8 cols */}
      <div className="col-span-12 lg:col-span-8 p-4 bg-white rounded-lg border min-h-[300px]">
        <RequestChart />
      </div>

      {/* 侧边信息 - 占 4 列 / Side info - span 4 cols */}
      <div className="col-span-12 lg:col-span-4 p-4 bg-white rounded-lg border">
        <RecentActivity />
      </div>
    </div>
  );
}

// 自适应网格（无需媒体查询）/ Auto-fit grid (no media queries)
function AutoGrid({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid gap-4 grid-cols-[repeat(auto-fill,minmax(280px,1fr))]">
      {children}
    </div>
  );
}
```

### 16.3 响应式断点策略 / Responsive Breakpoint Strategy

| 断点 / Breakpoint | 前缀 / Prefix | 最小宽度 / Min-width | 典型设备 / Typical Device | 本项目布局 / Project Layout |
|---|---|---|---|---|
| 默认 / Default | 无 / none | 0px | 手机 / Mobile | 单列堆叠 / Single col stack |
| sm | `sm:` | 640px | 大手机 / Large phone | 双列卡片 / 2-col cards |
| md | `md:` | 768px | 平板 / Tablet | 侧边栏可折叠 / Collapsible sidebar |
| lg | `lg:` | 1024px | 笔记本 / Laptop | 完整三栏 / Full 3-column |
| xl | `xl:` | 1280px | 桌面 / Desktop | 宽屏优化 / Widescreen optimized |
| 2xl | `2xl:` | 1536px | 大屏 / Large display | 最大宽度限制 / Max-width cap |

---

## 17. Tailwind CSS v4 迁移与新特性 / Tailwind v4 Migration & New Features

### 17.1 v4 架构变化 / v4 Architecture Changes

Tailwind CSS v4 带来了底层引擎的重大重构：

```
┌─────────────────────────────────────────────────────────────────┐
│           Tailwind v3 vs v4 架构对比 / Architecture Comparison   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Tailwind v3:                                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  tailwind.config.js (JS)  ──▶  PostCSS Plugin  ──▶  CSS │   │
│  │  - JavaScript 配置 / JS config                          │   │
│  │  - PostCSS 管道 / PostCSS pipeline                      │   │
│  │  - 内容扫描 (JIT) / Content scanning (JIT)              │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  Tailwind v4:                                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  CSS-first 配置  ──▶  Oxide Engine (Rust)  ──▶  CSS  │   │
│  │  - @theme 指令 / @theme directive                       │   │
│  │  - 原生 CSS 层叠层 / Native CSS cascade layers          │   │
│  │  - 自动内容检测 / Automatic content detection           │   │
│  │  - 5x 构建速度提升 / 5x build speed improvement         │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 17.2 CSS-first 配置方式 / CSS-first Configuration

```css
/* ===== Tailwind v4: CSS-first 配置 / CSS-first Config ===== */
/* 文件: src/index.css */

@import "tailwindcss";

/* 使用 @theme 定义设计令牌 / Define design tokens with @theme */
@theme {
  /* 自定义颜色 / Custom colors */
  --color-primary: #3b82f6;
  --color-primary-dark: #1d4ed8;
  --color-surface: #f9fafb;

  /* 自定义字体 / Custom fonts */
  --font-sans: 'Inter', system-ui, sans-serif;
  --font-mono: 'JetBrains Mono', monospace;

  /* 自定义间距 / Custom spacing */
  --spacing-18: 4.5rem;

  /* 自定义断点 / Custom breakpoints */
  --breakpoint-3xl: 1920px;

  /* 自定义动画 / Custom animations */
  --animate-fade-in: fade-in 0.3s ease-out;

  @keyframes fade-in {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }
}

/* 自定义工具类 / Custom utilities */
@utility scrollbar-thin {
  scrollbar-width: thin;
  scrollbar-color: var(--color-gray-300) transparent;
}

/* 自定义变体 / Custom variants */
@variant hocus (&:hover, &:focus);
```

### 17.3 迁移步骤与注意事项 / Migration Steps & Considerations

```bash
# ===== 从 v3 迁移到 v4 / Migrate from v3 to v4 =====

# 1. 升级依赖 / Upgrade dependencies
pnpm remove tailwindcss postcss autoprefixer
pnpm add tailwindcss@latest @tailwindcss/vite

# 2. 更新 Vite 配置 / Update Vite config
# vite.config.ts:
#   import tailwindcss from '@tailwindcss/vite'
#   plugins: [react(), tailwindcss()]

# 3. 移除 PostCSS 配置 / Remove PostCSS config
# 删除 postcss.config.js（不再需要）/ Delete postcss.config.js (no longer needed)

# 4. 运行自动迁移工具 / Run automatic migration tool
npx @tailwindcss/upgrade
```

### 17.4 破坏性变更对照 / Breaking Changes Reference

| v3 写法 / v3 Syntax | v4 写法 / v4 Syntax | 说明 / Notes |
|---|---|---|
| `tailwind.config.js` | `@theme` in CSS | JS 配置仍支持但非推荐 / JS still supported |
| `@tailwind base/components/utilities` | `@import "tailwindcss"` | 单行导入 / Single import |
| `postcss.config.js` + autoprefixer | `@tailwindcss/vite` plugin | 内置处理 / Built-in |
| `content: ['./src/**/*']` | 自动检测 / Auto-detect | 无需手动配置 / No manual config |
| `ring` (default 3px) | `ring` (default 1px) | 默认宽度变化 / Default width changed |
| `shadow-sm` | `shadow-xs` | 命名调整 / Naming adjusted |
| `rounded-sm` | `rounded-xs` | 命名调整 / Naming adjusted |
| `outline-none` | `outline-hidden` | 语义更清晰 / Clearer semantics |

### 17.5 本项目迁移评估 / This Project's Migration Assessment

| 因素 / Factor | 当前状态 / Current State | 迁移影响 / Migration Impact |
|---|---|---|
| 配置复杂度 / Config complexity | 简单 tailwind.config.js | 低影响 / Low impact |
| PostCSS 插件 / PostCSS plugins | 仅 tailwindcss + autoprefixer | 可完全移除 / Can remove entirely |
| 自定义组件 / Custom components | 少量 @apply | 需调整 / Needs adjustment |
| 构建工具 / Build tool | Vite 5.x | 完美支持 / Perfect support |
| 内容扫描 / Content scan | 手动配置 | 自动检测更简单 / Auto simpler |
| 风险 / Risk | 低 / Low | 建议小版本升级时迁移 / Migrate on minor |

## 18. 动画系统详解 / Animation System Details

### 18.1 内置动画工具类 / Built-in Animation Utilities

```css
/* Tailwind 内置动画 */
/* Tailwind built-in animations */

/* 旋转加载 / Spin loading */
.animate-spin { animation: spin 1s linear infinite; }

/* 脉冲 / Pulse */
.animate-pulse { animation: pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite; }

/* 弹跳 / Bounce */
.animate-bounce { animation: bounce 1s infinite; }

/* 淡入 / Ping */
.animate-ping { animation: ping 1s cubic-bezier(0, 0, 0.2, 1) infinite; }
```

```tsx
// 使用示例：加载状态
// Usage: Loading states
function LoadingSpinner() {
  return (
    <div className="flex items-center gap-3">
      <div className="h-5 w-5 animate-spin rounded-full border-2 border-blue-500 border-t-transparent" />
      <span className="text-sm text-gray-500">处理中... / Processing...</span>
    </div>
  )
}

// 骨架屏
// Skeleton screen
function SkeletonCard() {
  return (
    <div className="animate-pulse space-y-3 rounded-lg border p-4">
      <div className="h-4 w-3/4 rounded bg-gray-200" />
      <div className="h-3 w-full rounded bg-gray-200" />
      <div className="h-3 w-5/6 rounded bg-gray-200" />
    </div>
  )
}

// 通知徽章动画
// Notification badge animation
function NotificationBadge({ count }: { count: number }) {
  return (
    <span className="relative">
      <BellIcon className="h-6 w-6" />
      {count > 0 && (
        <span className="absolute -right-1 -top-1 flex h-4 w-4">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
          <span className="relative inline-flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
            {count}
          </span>
        </span>
      )}
    </span>
  )
}
```

### 18.2 自定义动画 / Custom Animations

```javascript
// tailwind.config.js 自定义动画
// tailwind.config.js custom animations
module.exports = {
  theme: {
    extend: {
      keyframes: {
        // 淡入上移
        // Fade in up
        'fade-in-up': {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        // 滑入
        // Slide in
        'slide-in-right': {
          '0%': { transform: 'translateX(100%)', opacity: '0' },
          '100%': { transform: 'translateX(0)', opacity: '1' },
        },
        // 摇晃（错误提示）
        // Shake (error hint)
        'shake': {
          '0%, 100%': { transform: 'translateX(0)' },
          '10%, 30%, 50%, 70%, 90%': { transform: 'translateX(-4px)' },
          '20%, 40%, 60%, 80%': { transform: 'translateX(4px)' },
        },
        // 进度条
        // Progress bar
        'progress': {
          '0%': { width: '0%' },
          '100%': { width: '100%' },
        },
      },
      animation: {
        'fade-in-up': 'fade-in-up 0.3s ease-out',
        'slide-in-right': 'slide-in-right 0.3s ease-out',
        'shake': 'shake 0.5s ease-in-out',
        'progress': 'progress 2s ease-in-out',
      },
    },
  },
}
```

### 18.3 过渡与性能 / Transitions & Performance

```tsx
// 高性能过渡：仅动画 transform 和 opacity
// High-performance transitions: only animate transform and opacity

// ✔ 高性能（GPU 加速）
// ✔ High performance (GPU accelerated)
<div className="transition-transform duration-200 hover:scale-105" />
<div className="transition-opacity duration-300 hover:opacity-80" />

// ✘ 低性能（触发重排）
// ✘ Low performance (triggers reflow)
// <div className="transition-all duration-200 hover:w-64" />
// <div className="transition-all hover:top-0" />

// 指定过渡属性（避免 transition-all）
// Specify transition properties (avoid transition-all)
<button className="
  transition-[background-color,box-shadow]
  duration-150
  ease-in-out
  hover:bg-blue-600
  hover:shadow-lg
">
  提交 / Submit
</button>
```

### 18.4 动画性能对比 / Animation Performance Comparison

| 属性 / Property | 性能 / Performance | GPU 加速 / GPU | 建议 / Recommendation |
|---|---|---|---|
| transform | ⭐⭐⭐ | ✅ | 首选 / Preferred |
| opacity | ⭐⭐⭐ | ✅ | 首选 / Preferred |
| background-color | ⭐⭐ | 部分 / Partial | 可用 / OK |
| box-shadow | ⭐⭐ | 部分 / Partial | 可用 / OK |
| width/height | ⭐ | ✘ | 避免 / Avoid |
| top/left/margin | ⭐ | ✘ | 避免 / Avoid |

## 19. 暗色模式实现详解 / Dark Mode Implementation Details

### 19.1 暗色模式策略 / Dark Mode Strategy

```javascript
// tailwind.config.js
module.exports = {
  // 'class' 策略：通过 HTML class 控制
  // 'class' strategy: controlled via HTML class
  darkMode: 'class',
  
  // 'media' 策略：跟随系统设置
  // 'media' strategy: follows system setting
  // darkMode: 'media',
}
```

```tsx
// 暗色模式切换 Hook
// Dark mode toggle Hook
import { useState, useEffect } from 'react'

type Theme = 'light' | 'dark' | 'system'

function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    return (localStorage.getItem('theme') as Theme) || 'system'
  })
  
  useEffect(() => {
    const root = document.documentElement
    
    if (theme === 'system') {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)')
      root.classList.toggle('dark', prefersDark.matches)
      
      const listener = (e: MediaQueryListEvent) => {
        root.classList.toggle('dark', e.matches)
      }
      prefersDark.addEventListener('change', listener)
      return () => prefersDark.removeEventListener('change', listener)
    }
    
    root.classList.toggle('dark', theme === 'dark')
  }, [theme])
  
  useEffect(() => {
    localStorage.setItem('theme', theme)
  }, [theme])
  
  return { theme, setTheme }
}

// 切换按钮组件
// Toggle button component
function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  
  return (
    <button
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
      className="
        rounded-lg p-2
        text-gray-500
        transition-colors
        hover:bg-gray-100
        dark:text-gray-400
        dark:hover:bg-gray-800
      "
    >
      {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
    </button>
  )
}
```

### 19.2 暗色模式设计令牌 / Dark Mode Design Tokens

```tsx
// 语义化颜色组件
// Semantic color components
function Card({ children }: { children: React.ReactNode }) {
  return (
    <div className="
      rounded-lg border p-6
      border-gray-200 dark:border-gray-700
      bg-white dark:bg-gray-800
      shadow-sm dark:shadow-gray-900/20
    ">
      {children}
    </div>
  )
}

// 文本颜色层次
// Text color hierarchy
function Typography() {
  return (
    <div>
      <h1 className="text-gray-900 dark:text-white">标题 / Title</h1>
      <p className="text-gray-700 dark:text-gray-300">正文 / Body</p>
      <span className="text-gray-500 dark:text-gray-400">辅助 / Secondary</span>
      <span className="text-gray-400 dark:text-gray-500">禁用 / Disabled</span>
    </div>
  )
}

// 状态颜色
// Status colors
const statusStyles = {
  success: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  warning: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400',
  error: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  info: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
}
```

### 19.3 暗色模式检查清单 / Dark Mode Checklist

| 检查项 / Check Item | 说明 / Description | 状态 / Status |
|---|---|---|
| 背景色反转 / Background inversion | white → gray-800/900 | ✅ |
| 文本对比度 / Text contrast | WCAG AA 4.5:1 | ✅ |
| 边框可见 / Border visible | gray-200 → gray-700 | ✅ |
| 阴影调整 / Shadow adjust | 深色阴影更微妙 / Darker subtler | ✅ |
| 图片/图标 / Images/icons | 降低亮度 / Reduce brightness | ✅ |
| 图表颜色 / Chart colors | 暗色背景适配 / Dark bg adapted | ✅ |

## 20. 组件库构建实践 / Component Library Building Practice

### 20.1 可复用组件设计 / Reusable Component Design

```tsx
// src/components/ui/Button.tsx
import { forwardRef, ButtonHTMLAttributes } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

// 使用 CVA 管理变体
// Use CVA to manage variants
const buttonVariants = cva(
  // 基础样式
  // Base styles
  'inline-flex items-center justify-center rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        primary: 'bg-blue-600 text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600',
        secondary: 'bg-gray-100 text-gray-900 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-100 dark:hover:bg-gray-700',
        outline: 'border border-gray-300 bg-transparent hover:bg-gray-50 dark:border-gray-600 dark:hover:bg-gray-800',
        ghost: 'bg-transparent hover:bg-gray-100 dark:hover:bg-gray-800',
        danger: 'bg-red-600 text-white hover:bg-red-700 dark:bg-red-500',
      },
      size: {
        sm: 'h-8 px-3 text-xs',
        md: 'h-10 px-4 text-sm',
        lg: 'h-12 px-6 text-base',
        icon: 'h-10 w-10',
      },
    },
    defaultVariants: {
      variant: 'primary',
      size: 'md',
    },
  }
)

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={buttonVariants({ variant, size, className })}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = 'Button'

// 使用
// Usage
// <Button variant="primary" size="lg">提交 / Submit</Button>
// <Button variant="outline" size="sm">取消 / Cancel</Button>
// <Button variant="danger">删除 / Delete</Button>
```

### 20.2 复合组件模式 / Compound Component Pattern

```tsx
// src/components/ui/Tabs.tsx
import { createContext, useContext, useState, ReactNode } from 'react'

interface TabsContextValue {
  activeTab: string
  setActiveTab: (id: string) => void
}

const TabsContext = createContext<TabsContextValue | null>(null)

// 主容器
// Main container
export function Tabs({ children, defaultValue }: {
  children: ReactNode
  defaultValue: string
}) {
  const [activeTab, setActiveTab] = useState(defaultValue)
  
  return (
    <TabsContext.Provider value={{ activeTab, setActiveTab }}>
      <div className="w-full">{children}</div>
    </TabsContext.Provider>
  )
}

// 标签列表
// Tab list
export function TabList({ children }: { children: ReactNode }) {
  return (
    <div className="flex border-b border-gray-200 dark:border-gray-700">
      {children}
    </div>
  )
}

// 单个标签
// Single tab
export function Tab({ id, children }: { id: string; children: ReactNode }) {
  const ctx = useContext(TabsContext)!
  const isActive = ctx.activeTab === id
  
  return (
    <button
      onClick={() => ctx.setActiveTab(id)}
      className={`
        px-4 py-2 text-sm font-medium transition-colors
        border-b-2 -mb-px
        ${isActive
          ? 'border-blue-500 text-blue-600 dark:text-blue-400'
          : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
        }
      `}
    >
      {children}
    </button>
  )
}

// 内容面板
// Content panel
export function TabPanel({ id, children }: { id: string; children: ReactNode }) {
  const ctx = useContext(TabsContext)!
  if (ctx.activeTab !== id) return null
  
  return <div className="py-4 animate-fade-in-up">{children}</div>
}

// 使用
// Usage
// <Tabs defaultValue="mask">
//   <TabList>
//     <Tab id="mask">数据脱敏 / Masking</Tab>
//     <Tab id="dp">差分隐私 / DP</Tab>
//   </TabList>
//   <TabPanel id="mask">...</TabPanel>
//   <TabPanel id="dp">...</TabPanel>
// </Tabs>
```

### 20.3 组件库工具函数 / Component Library Utilities

```typescript
// src/lib/utils.ts
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

// 合并 Tailwind 类名（解决冲突）
// Merge Tailwind classes (resolve conflicts)
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// 使用示例
// Usage
// cn('px-2 py-1', 'px-4')  → 'py-1 px-4'  (后者覆盖 / latter wins)
// cn('text-red-500', isActive && 'text-blue-500')  → 条件合并 / Conditional merge
```

```json
// package.json 依赖
// package.json dependencies
{
  "dependencies": {
    "class-variance-authority": "^0.7.0",
    "clsx": "^2.1.0",
    "tailwind-merge": "^2.2.0"
  }
}
```

### 20.4 组件库架构总结 / Component Library Architecture Summary

```
src/components/ui/
├── Button.tsx          # CVA 变体 / CVA variants
├── Input.tsx           # 表单输入 / Form input
├── Select.tsx          # 下拉选择 / Dropdown select
├── Card.tsx            # 卡片容器 / Card container
├── Tabs.tsx            # 复合组件 / Compound component
├── Modal.tsx           # 模态框 / Modal dialog
├── Toast.tsx           # 通知提示 / Toast notification
├── DataTable.tsx       # 数据表格 / Data table
└── index.ts            # 统一导出 / Unified export

src/lib/
└── utils.ts            # cn() 工具 / cn() utility
```

| 工具 / Tool | 用途 / Purpose | 优势 / Advantage |
|---|---|---|
| CVA | 变体管理 / Variant management | 类型安全 + 可组合 / Type-safe + composable |
| clsx | 条件类名 / Conditional classes | 简洁条件逻辑 / Clean conditions |
| tailwind-merge | 冲突解决 / Conflict resolution | 智能覆盖 / Smart override |
| forwardRef | ref 转发 / Ref forwarding | 兼容原生元素 / Native compatible |
