# ESLint 技术栈说明 / ESLint Technology Stack

## 1. 技术简介 / Introduction

ESLint 是 JavaScript/TypeScript 生态中最主流的静态代码检查工具，用于发现和修复代码质量问题与风格不一致。
ESLint is the most mainstream static code analysis tool in the JavaScript/TypeScript ecosystem, used to find and fix code quality issues and style inconsistencies.

核心特性 / Core Features：
- **可插拔规则（Pluggable Rules）**：每条规则独立，可按项目需求开启/关闭/自定义严重级别。
- **Flat Config（ESLint 9+）**：全新的 `eslint.config.js` 配置格式，取代旧版 `.eslintrc`，更简洁直观。
- **TypeScript 深度集成**：通过 `typescript-eslint` 支持类型感知规则（如禁止 `any`、检测未使用变量）。
- **自动修复（Auto-fix）**：`--fix` 参数可自动修复格式类问题（缩进、分号、引号等）。
- **插件生态（Plugin Ecosystem）**：React Hooks、React Refresh、Import 等数百个社区插件。
- **CI 集成（CI Integration）**：退出码非零即失败，天然适配 GitHub Actions / GitLab CI。

本项目使用版本 / Versions Used：
- `eslint ^9.20.0`
- `typescript-eslint ^8.24.0`
- `eslint-plugin-react-hooks ^5.1.0`
- `eslint-plugin-react-refresh ^0.4.19`

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 配置文件（Flat Config 格式）/ Configuration File (Flat Config Format)

文件 / File：`console/web/eslint.config.js`

```javascript
import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  // 1. 忽略构建产物目录 / Ignore build output directory
  { ignores: ['dist'] },

  // 2. 主配置块 / Main configuration block
  {
    // 继承推荐规则集（JS 核心 + TypeScript 推荐）
    // Extend recommended rule sets (JS core + TypeScript recommended)
    extends: [js.configs.recommended, ...tseslint.configs.recommended],

    // 仅对 TypeScript 文件生效 / Only applies to TypeScript files
    files: ['**/*.{ts,tsx}'],

    // 语言选项 / Language options
    languageOptions: {
      ecmaVersion: 2020,        // 目标 ES 版本 / Target ES version
      globals: globals.browser, // 浏览器全局变量 (window, document 等)
    },

    // 插件注册 / Plugin registration
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },

    // 自定义规则 / Custom rules
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
    },
  },
);
```

### 2.2 规则详解 / Rule Details

| 规则 / Rule | 级别 / Level | 作用 / Purpose |
|---|---|---|
| `react-hooks/rules-of-hooks` | error | 禁止在条件/循环/嵌套函数中调用 Hooks / No Hooks in conditions/loops/nested functions |
| `react-hooks/exhaustive-deps` | warn | 确保 useEffect/useCallback/useMemo 依赖数组完整 / Ensure complete dependency arrays |
| `react-refresh/only-export-components` | warn | 组件文件仅导出组件（保证 HMR 可热更新）/ Component files export only components (ensures HMR works) |
| `@typescript-eslint/no-explicit-any` | warn | 警告显式 `any` 类型（鼓励使用具体类型）/ Warn on explicit `any` (encourage specific types) |
| `@typescript-eslint/no-unused-vars` | warn | 警告未使用的变量（`_` 前缀参数除外）/ Warn on unused vars (except `_`-prefixed params) |

### 2.3 React Hooks 规则的重要性 / Importance of React Hooks Rules

```tsx
// ❌ 错误：条件调用 Hook（违反 rules-of-hooks）
// ❌ Wrong: conditional Hook call (violates rules-of-hooks)
if (condition) {
  const [val, setVal] = useState(0);  // ESLint ERROR
}

// ❌ 错误：依赖数组缺少 count（违反 exhaustive-deps）
// ❌ Wrong: missing count in dependency array (violates exhaustive-deps)
useEffect(() => {
  console.log(count);  // ESLint WARNING: 'count' is missing
}, []);

// ✅ 正确：完整依赖数组
// ✅ Correct: complete dependency array
useEffect(() => {
  console.log(count);
}, [count]);
```

### 2.4 React Refresh 规则 / React Refresh Rule

```tsx
// ❌ 警告：混合导出组件和工具函数（HMR 无法局部刷新）
// ❌ Warning: mixed export of component and utility (HMR can't partial refresh)
export function MyComponent() { ... }
export const helper = () => { ... };  // ESLint WARNING

// ✅ 正确：仅导出组件，工具函数放到独立文件
// ✅ Correct: export only components, utilities in separate files
export function MyComponent() { ... }

// ✅ 正确：allowConstantExport 允许常量导出
// ✅ Correct: allowConstantExport permits constant exports
export const MAX_RETRIES = 3;  // OK (constant)
```

### 2.5 运行方式 / Running ESLint

```bash
# 检查所有文件 / Check all files
cd console/web && pnpm lint

# 自动修复可修复的问题 / Auto-fix fixable issues
npx eslint . --fix

# 仅检查特定文件 / Check specific files only
npx eslint src/App.tsx src/components/
```

### 2.6 与编辑器的集成 / Editor Integration

推荐 VS Code 配置（`.vscode/settings.json`）：
Recommended VS Code settings:

```json
{
  "eslint.validate": ["typescript", "typescriptreact"],
  "editor.codeActionsOnSave": {
    "source.fixAll.eslint": "explicit"
  }
}
```

### 2.7 关键设计决策 / Key Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| Flat Config（非 .eslintrc）| ESLint 9 官方推荐，更简洁、可组合 / ESLint 9 official, cleaner and composable |
| `no-explicit-any` 设为 warn | 代理场景中部分 API 响应确实为 any，不阻断开发 / Some API responses are genuinely any, don't block dev |
| `no-unused-vars` 忽略 `_` 前缀 | 解构时常用 `_` 标记有意忽略的变量 / Common pattern to mark intentionally ignored vars |
| `allowConstantExport: true` | 允许导出常量（如配置值），不强制拆分文件 / Allow exporting constants without file splitting |
| 仅检查 ts/tsx 文件 | JS 配置文件（postcss.config.js 等）无需严格检查 / JS config files don't need strict checking |

### 2.8 Flat Config vs 传统 .eslintrc / Flat Config vs Legacy .eslintrc

| 维度 / Dimension | Flat Config (eslint.config.js) | 传统 .eslintrc |
|---|---|---|
| 配置格式 / Format | ES Module 导出数组 / ES Module exports array | JSON/YAML/JS 对象 |
| 继承机制 / Inheritance | `extends` 数组（显式）| `extends` 字符串（隐式解析）|
| 插件注册 / Plugin registration | 显式 `plugins` 对象 | 字符串前缀（`plugin:`）|
| 全局变量 / Globals | `languageOptions.globals` | `env` + `globals` |
| 文件匹配 / File matching | `files` / `ignores` 数组 | `overrides` + `.eslintignore` |
| 状态 / Status | ESLint 9+ 默认 / Default in ESLint 9+ | 已废弃 / Deprecated |

**Flat Config 配置结构 / Flat Config structure**：

```javascript
// eslint.config.js 是一个数组，每个元素是一个配置对象
// eslint.config.js is an array, each element is a config object
export default tseslint.config(
  { ignores: ['dist'] },           // 1. 全局忽略 / Global ignores
  {
    files: ['**/*.{ts,tsx}'],      // 2. 文件匹配 / File matching
    extends: [...],                // 3. 继承规则集 / Extend rule sets
    plugins: {...},                // 4. 注册插件 / Register plugins
    languageOptions: {...},        // 5. 语言选项 / Language options
    rules: {...},                  // 6. 自定义规则 / Custom rules
  },
);
```

### 2.9 规则严重级别 / Rule Severity Levels

| 级别 / Level | 值 / Value | 效果 / Effect |
|---|---|---|
| `"off"` / `0` | 关闭 / Off | 完全不检查 / No checking |
| `"warn"` / `1` | 警告 / Warning | 显示警告，不影响退出码 / Shows warning, exit code 0 |
| `"error"` / `2` | 错误 / Error | 显示错误，退出码非零 / Shows error, exit code 1 |

**本项目的级别策略 / Project's severity strategy**：

```text
┌─────────────────────────────────────────────────────────────┐
│  error 级别：必须修复（阻断 CI）/ Must fix (blocks CI)        │
│  - react-hooks/rules-of-hooks                               │
│  - @typescript-eslint/no-unused-vars (部分)                 │
├─────────────────────────────────────────────────────────────┤
│  warn 级别：建议修复（不阻断）/ Should fix (non-blocking)     │
│  - react-hooks/exhaustive-deps                              │
│  - @typescript-eslint/no-explicit-any                       │
│  - react-refresh/only-export-components                     │
└─────────────────────────────────────────────────────────────┘
```

### 2.10 CI 集成 / CI Integration

```yaml
# .github/workflows/ci.yml 中的 ESLint 检查
# ESLint check in .github/workflows/ci.yml
- name: Lint frontend
  run: |
    cd console/web
    corepack pnpm install
    corepack pnpm lint  # 退出码非零则 CI 失败 / Non-zero exit fails CI
```

**本地 Git Hook 集成（可选）/ Local Git Hook integration (optional)**：

```bash
# 使用 lint-staged 在提交前检查暂存文件
# Use lint-staged to check staged files before commit
npx lint-staged
# 配置 / Config: { "*.{ts,tsx}": "eslint --fix" }
```

### 2.11 常见问题与解决 / Common Issues & Solutions

| 问题 / Issue | 原因 / Cause | 解决 / Solution |
|---|---|---|
| `React Hook useEffect has a missing dependency` | 依赖数组不完整 / Incomplete deps array | 添加依赖或用 useCallback 包装 / Add dep or wrap with useCallback |
| `Unexpected any. Specify a different type` | 使用了 any 类型 / Used any type | 定义具体接口 / Define concrete interface |
| `Fast refresh only works when a file only exports components` | 混合导出 / Mixed exports | 拆分组件和工具函数 / Split components and utils |
| `'_' is defined but never used` | 未使用的变量 / Unused variable | 使用 `_` 前缀或移除 / Use `_` prefix or remove |

### 2.12 类型感知规则 / Type-aware Rules

typescript-eslint 提供两类规则：

```text
┌─────────────────────────────────────────────────────────────┐
│  普通规则（本项目使用）/ Normal Rules (used in this project)   │
│  - 仅需 AST，无需类型信息 / Only needs AST, no type info    │
│  - 速度快，无额外配置 / Fast, no extra config               │
│  - 例: no-explicit-any, no-unused-vars                     │
├─────────────────────────────────────────────────────────────┤
│  类型感知规则（本项目未启用）/ Type-aware Rules (not enabled)  │
│  - 需要完整类型信息 / Requires full type information        │
│  - 需配置 parserOptions.project / Needs parserOptions.project│
│  - 速度较慢（~2-5x）但更精确 / Slower but more precise     │
│  - 例: no-floating-promises, no-misused-promises           │
└─────────────────────────────────────────────────────────────┘
```

**如需启用类型感知规则 / To enable type-aware rules**：

```javascript
// 在 eslint.config.js 中添加 / Add to eslint.config.js:
languageOptions: {
  parserOptions: {
    project: './tsconfig.json',  // 指向 TS 配置 / Point to TS config
    tsconfigRootDir: __dirname,
  },
},
rules: {
  '@typescript-eslint/no-floating-promises': 'error',  // 禁止未处理的 Promise
  '@typescript-eslint/no-misused-promises': 'error',   // 禁止 Promise 误用
  '@typescript-eslint/await-thenable': 'error',        // 禁止 await 非 Promise
},
```

### 2.13 ESLint 执行流程 / ESLint Execution Pipeline

```text
pnpm lint 执行流程 / pnpm lint execution flow:

1. 读取 eslint.config.js（Flat Config）
   Read eslint.config.js (Flat Config)
       │
       ▼
2. 确定目标文件（files: ['**/*.{ts,tsx}']）
   Determine target files
       │
       ▼
3. 对每个文件 / For each file:
   ├─ 解析为 AST（@typescript-eslint/parser）
   │  Parse to AST
   ├─ 遍历 AST 节点 / Traverse AST nodes
   ├─ 对每个节点执行匹配的规则 / Run matching rules on each node
   └─ 收集报告（warning/error）/ Collect reports
       │
       ▼
4. 输出结果 / Output results
   ├─ 有 error → 退出码 1（CI 失败）/ Exit code 1
   └─ 仅 warning → 退出码 0（CI 通过）/ Exit code 0
```

### 2.14 性能优化 / Performance Optimization

| 策略 / Strategy | 说明 / Description |
|---|---|
| 仅检查 ts/tsx | 跳过 .js 配置文件（postcss.config.js 等）/ Skip .js config files |
| 忽略 dist/ | 不检查构建产物 / Don't check build output |
| 普通规则（非类型感知）| 无需构建类型程序，速度快 2-5x / No type program needed |
| `skipLibCheck: true` | 不检查 node_modules 中的 .d.ts / Skip node_modules .d.ts |
| ESLint 缓存 | `--cache` 标志跳过未修改文件 / Skip unchanged files |

```bash
# 启用缓存加速重复检查 / Enable cache for faster repeat checks
npx eslint . --cache --cache-location node_modules/.eslintcache
```

### 2.15 本项目规则覆盖范围 / Project Rule Coverage

```text
┌─────────────────────────────────────────────────────────────┐
│  @eslint/js recommended (~60 条规则)                         │
│  - no-undef, no-dupe-keys, no-unreachable...                │
├─────────────────────────────────────────────────────────────┤
│  typescript-eslint recommended (~15 条额外规则)              │
│  - no-explicit-any, no-unused-vars, no-empty-interface...   │
├─────────────────────────────────────────────────────────────┤
│  react-hooks recommended (2 条规则)                          │
│  - rules-of-hooks (error)                                   │
│  - exhaustive-deps (warn)                                   │
├─────────────────────────────────────────────────────────────┤
│  react-refresh (1 条规则)                                    │
│  - only-export-components (warn)                            │
├─────────────────────────────────────────────────────────────┤
│  自定义覆盖 / Custom overrides (2 条)                       │
│  - no-explicit-any: error → warn                            │
│  - no-unused-vars: 添加 argsIgnorePattern: '^_'             │
└─────────────────────────────────────────────────────────────┘
```

### 2.16 关键设计决策补充 / Additional Design Decisions

| 决策 / Decision | 原因 / Reason |
|---|---|
| 未启用类型感知规则 | 项目规模小，普通规则已足够，避免编译开销 / Small project, normal rules suffice |
| 未使用 Prettier | Tailwind 类名排序由插件处理，无需额外格式化 / Tailwind plugin handles sorting |
| 未使用 eslint-import-resolver | Vite alias 简单，无复杂导入解析需求 / Simple Vite alias, no complex resolution |
| warn 而非 error 为主 | POC 阶段优先开发速度，不阻断构建 / POC phase prioritizes dev speed |

## 3. 类型感知规则详解 / Type-aware Rules Details

### 3.1 类型感知 vs 普通规则 / Type-aware vs Normal Rules

```text
普通规则 / Normal rules:
  仅分析 AST 结构 / Only analyze AST structure
  例：no-unused-vars 检查变量是否被引用
  Example: no-unused-vars checks if variable is referenced
  速度：快 / Speed: fast

类型感知规则 / Type-aware rules:
  需要 TypeScript 编译器信息 / Requires TypeScript compiler info
  例：no-floating-promises 检查 Promise 是否被 await
  Example: no-floating-promises checks if Promise is awaited
  速度：慢 2-5x / Speed: 2-5x slower
```

### 3.2 启用类型感知规则 / Enabling Type-aware Rules

```javascript
// 本项目未启用，但以下是启用方式 / Not enabled in this project, but here's how
import tseslint from 'typescript-eslint';

export default tseslint.config(
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      ...tseslint.configs.recommendedTypeChecked,  // 类型感知推荐集
    ],
    languageOptions: {
      parserOptions: {
        project: './tsconfig.json',  // ❗ 必须指定 tsconfig / Must specify tsconfig
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
);
```

### 3.3 类型感知规则示例 / Type-aware Rule Examples

| 规则 / Rule | 作用 / Purpose | 本项目 / This Project |
|---|---|---|
| `no-floating-promises` | 禁止未处理的 Promise / Forbid unhandled Promises | ❌ 未启用 |
| `no-misused-promises` | 禁止 Promise 用作条件 / Forbid Promise as condition | ❌ 未启用 |
| `await-thenable` | 仅 await 可等待对象 / Only await thenables | ❌ 未启用 |
| `no-unnecessary-type-assertion` | 禁止多余的类型断言 / Forbid redundant assertions | ❌ 未启用 |
| `restrict-template-expressions` | 模板字符串类型限制 / Template string type restriction | ❌ 未启用 |

## 4. ESLint 执行管线 / ESLint Execution Pipeline

### 4.1 完整执行流程 / Complete Execution Flow

```text
┌─────────────────────────────────────────────────────────────┐
│  1. 配置加载 / Config Loading                                │
│     读取 eslint.config.js / Read eslint.config.js            │
│     解析 extends、plugins、rules / Parse extends, plugins    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  2. 文件发现 / File Discovery                                │
│     根据 files/ignores 匹配目标文件                        │
│     Match target files by files/ignores                      │
│     本项目: **/*.{ts,tsx} 排除 dist/                        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  3. 解析 / Parsing                                           │
│     @typescript-eslint/parser 将源码解析为 AST              │
│     Parse source to AST with @typescript-eslint/parser       │
│     (可选) 创建 TypeScript Program 用于类型感知            │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  4. 规则遍历 / Rule Traversal                                │
│     每条规则注册访问器（如 CallExpression, Identifier）   │
│     Each rule registers visitors (e.g. CallExpression)       │
│     深度优先遍历 AST，触发规则回调                        │
│     Depth-first AST traversal, trigger rule callbacks        │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  5. 报告收集 / Report Collection                             │
│     收集所有规则报告的问题 / Collect all reported issues    │
│     按严重级别分类 (error/warn) / Classify by severity      │
│     应用 --fix 自动修复 / Apply --fix auto-fixes            │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│  6. 输出 / Output                                            │
│     格式化报告 (stylish/json/junit) / Format report         │
│     设置退出码 / Set exit code                               │
│     error 存在 → exit 1 / error exists → exit 1             │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 AST 访问器模式 / AST Visitor Pattern

```javascript
// ESLint 规则内部结构（简化）/ ESLint rule internal structure (simplified)
module.exports = {
  meta: {
    type: 'problem',
    messages: { noAny: 'Avoid using any type' },
  },
  create(context) {
    return {
      // 访问器：遇到 TSTypeAnnotation 节点时触发
      // Visitor: triggered when encountering TSTypeAnnotation node
      TSTypeAnnotation(node) {
        if (node.typeAnnotation.type === 'TSAnyKeyword') {
          context.report({ node, messageId: 'noAny' });
        }
      },
    };
  },
};
```

## 5. 性能优化 / Performance Optimization

### 5.1 影响性能的因素 / Performance Factors

| 因素 / Factor | 影响 / Impact | 优化 / Optimization |
|---|---|---|
| 文件数量 / File count | 线性增长 / Linear growth | 精确 files 匹配 / Precise files match |
| 类型感知规则 / Type-aware rules | 慢 2-5x / 2-5x slower | 仅必要时启用 / Enable only when needed |
| 插件数量 / Plugin count | 每个插件增加遍历 / Each plugin adds traversal | 精简插件 / Minimize plugins |
| 缓存 / Cache | 首次慢，后续快 / First slow, then fast | `--cache` 标志 / --cache flag |

### 5.2 缓存机制 / Cache Mechanism

```bash
# 启用缓存（仅重新检查变更文件）/ Enable cache (only recheck changed files)
npx eslint . --cache --cache-location=node_modules/.cache/eslint/

# 缓存失效条件 / Cache invalidation conditions:
# - 文件内容变更 / File content changed
# - ESLint 配置变更 / ESLint config changed
# - ESLint 版本升级 / ESLint version upgraded
```

### 5.3 本项目性能基准 / This Project Performance

| 操作 / Operation | 耗时 / Time | 说明 / Notes |
|---|---|---|
| 首次全量检查 / First full check | ~2s | 无缓存 / No cache |
| 缓存后检查 / Cached check | ~0.3s | 仅检查变更文件 / Only changed files |
| 单文件检查 / Single file | ~100ms | 包含解析时间 / Including parse time |
| --fix 自动修复 / --fix auto-fix | ~2.5s | 含重新检查 / Including recheck |

## 6. CI/CD 集成 / CI/CD Integration

### 6.1 GitHub Actions 配置 / GitHub Actions Config

```yaml
# .github/workflows/ci.yml 中的 ESLint 步骤
# ESLint step in CI workflow
- name: Lint
  run: |
    cd console/web
    pnpm install --frozen-lockfile
    pnpm lint
  # pnpm lint = eslint .
  # 退出码非零即失败 CI / Non-zero exit code fails CI
```

### 6.2 预提交钩子 / Pre-commit Hook

```yaml
# .pre-commit-config.yaml 中的 ESLint 钩子
# ESLint hook in pre-commit config
repos:
  - repo: local
    hooks:
      - id: eslint
        name: ESLint
        entry: bash -c 'cd console/web && npx eslint --fix'
        language: system
        files: 'console/web/src/.*\.(ts|tsx)$'
        pass_filenames: true
```

### 6.3 lint-staged 集成 / lint-staged Integration

```jsonc
// package.json 中的 lint-staged 配置
// lint-staged config in package.json
{
  "lint-staged": {
    "src/**/*.{ts,tsx}": [
      "eslint --fix",       // 自动修复 / Auto-fix
      "eslint --max-warnings=0"  // 零警告策略 / Zero warnings policy
    ]
  }
}
```

## 7. 自定义规则开发 / Custom Rule Development

### 7.1 规则结构 / Rule Structure

ESLint 规则是一个导出 `create()` 方法的对象，返回 AST 节点访问器：
An ESLint rule is an object exporting a `create()` method that returns AST node visitors:

```typescript
// eslint-rules/no-hardcoded-api-url.ts
import { ESLintUtils, TSESTree } from '@typescript-eslint/utils';

const createRule = ESLintUtils.RuleCreator(
  (name) => `https://example.com/rules/${name}`
);

export const noHardcodedApiUrl = createRule({
  name: 'no-hardcoded-api-url',
  meta: {
    type: 'suggestion',
    docs: { description: '禁止硬编码 API URL / Forbid hardcoded API URLs' },
    fixable: 'code',
    messages: {
      noHardcode: '使用配置常量替代硬编码 URL / Use config constant instead of hardcoded URL',
    },
    schema: [],
  },
  defaultOptions: [],
  create(context) {
    return {
      // 访问字符串字面量节点 / Visit string literal nodes
      Literal(node: TSESTree.Literal) {
        if (typeof node.value === 'string' && node.value.includes('http://localhost')) {
          context.report({
            node,
            messageId: 'noHardcode',
            fix: (fixer) => fixer.replaceText(node, 'API_BASE_URL'),
          });
        }
      },
    };
  },
});
```

### 7.2 规则测试 / Rule Testing

```typescript
// eslint-rules/tests/no-hardcoded-api-url.test.ts
import { RuleTester } from '@typescript-eslint/rule-tester';
import { noHardcodedApiUrl } from '../no-hardcoded-api-url';

const ruleTester = new RuleTester({
  parser: '@typescript-eslint/parser',
});

ruleTester.run('no-hardcoded-api-url', noHardcodedApiUrl, {
  valid: [
    'const url = API_BASE_URL;',         // ✅ 使用常量 / Use constant
    'const url = config.apiUrl;',        // ✅ 配置对象 / Config object
  ],
  invalid: [
    {
      code: 'const url = "http://localhost:8080";',
      errors: [{ messageId: 'noHardcode' }],
      output: 'const url = API_BASE_URL;',  // 自动修复结果 / Auto-fix result
    },
  ],
});
```

### 7.3 规则类型分类 / Rule Type Classification

| 类型 / Type | 作用 / Purpose | 示例 / Example |
|---|---|---|
| `problem` | 捕获代码错误 / Catch code errors | `no-unused-vars`, `no-undef` |
| `suggestion` | 改进代码质量 / Improve code quality | `prefer-const`, `no-hardcoded-api-url` |
| `layout` | 格式化（空格/缩进）/ Formatting | `indent`, `semi`, `quotes` |

## 8. Flat Config 配置系统详解 / Flat Config System Details

### 8.1 Flat Config vs Legacy Config

ESLint 9+ 默认使用 Flat Config（`eslint.config.js`）替代旧版 `.eslintrc.*`：

| 特性 / Feature | Flat Config | Legacy (.eslintrc) |
|---|---|---|
| 配置文件 / Config file | `eslint.config.js` | `.eslintrc.json` / `.eslintrc.js` |
| 继承机制 / Inheritance | 数组顺序合并 / Array order merge | `extends` 链式 / Chain |
| 插件引用 / Plugin reference | 直接导入对象 / Direct import object | 字符串名称 / String name |
| 环境配置 / Env config | `languageOptions.globals` | `env` 字段 / `env` field |
| 级联查找 / Cascading lookup | 不级联（单文件）/ No cascade (single file) | 目录级联 / Directory cascade |
| 状态 / Status | ✅ 推荐 / Recommended | ⚠️ 已废弃 / Deprecated |

### 8.2 本项目 Flat Config 结构 / This Project's Flat Config Structure

```typescript
// console/web/eslint.config.js
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default tseslint.config(
  // 层 1：全局忽略 / Layer 1: Global ignores
  { ignores: ['dist'] },

  // 层 2：基础规则 / Layer 2: Base rules
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
);
```

### 8.3 配置合并顺序 / Config Merge Order

```text
数组索引越大，优先级越高 / Higher array index = higher priority

[0] ignores         → 全局排除（独立生效）/ Global excludes (standalone)
[1] js.recommended  → 基础 JS 规则 / Base JS rules
[2] tseslint        → TypeScript 规则覆盖 / TypeScript rules override
[3] react-hooks     → React Hooks 规则追加 / React Hooks rules append
[4] react-refresh   → HMR 规则追加 / HMR rules append

冲突时后者覆盖前者 / Later entries override earlier on conflict
```

## 9. IDE 集成与自动修复 / IDE Integration & Auto-fix

### 9.1 VS Code 配置 / VS Code Configuration

```jsonc
// .vscode/settings.json
{
  // 保存时自动修复 / Auto-fix on save
  "editor.codeActionsOnSave": {
    "source.fixAll.eslint": "explicit"
  },
  // ESLint 验证的文件类型 / File types ESLint validates
  "eslint.validate": ["javascript", "typescript", "typescriptreact"],
  // 使用 Flat Config / Use Flat Config
  "eslint.useFlatConfig": true,
  // 工作目录（monorepo 场景）/ Working directory (monorepo)
  "eslint.workingDirectories": ["console/web"]
}
```

### 9.2 修复能力分级 / Fix Capability Levels

| 级别 / Level | 命令 / Command | 作用 / Effect |
|---|---|---|
| 安全修复 / Safe fix | `eslint --fix` | 仅应用不改变语义的修复 / Only semantics-preserving fixes |
| 建议修复 / Suggestion fix | IDE 手动接受 / IDE manual accept | 可能改变行为 / May change behavior |
| 格式化 / Formatting | Prettier + ESLint | 空格/缩进/换行 / Spacing/indent/newlines |

### 9.3 与 Prettier 协作 / Collaboration with Prettier

```text
┌─────────────────────────────────────────────────────────────┐
│  代码保存 / Code Save                                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        ▼                                     ▼
┌───────────────────────┐         ┌───────────────────────┐
│  Prettier              │         │  ESLint --fix          │
│  格式化（布局）       │         │  代码质量修复          │
│  Formatting (layout)   │         │  Code quality fixes    │
│  - 缩进/引号/分号    │         │  - 未使用变量          │
│  - 换行/空格          │         │  - 类型错误            │
└───────────────────────┘         └───────────────────────┘
        │                                     │
        └──────────────────┬──────────────────┘
                           ▼
              最终代码 / Final Code
```

**职责分离原则 / Separation of Concerns**：
- Prettier 负责“代码长什么样” / Prettier handles "how code looks"
- ESLint 负责“代码写得对不对” / ESLint handles "is code correct"
- 使用 `eslint-config-prettier` 禁用 ESLint 中的格式规则 / Use `eslint-config-prettier` to disable ESLint formatting rules

## 10. 插件架构与扩展 / Plugin Architecture & Extension

### 10.1 插件结构 / Plugin Structure

```typescript
// 一个 ESLint 插件的标准结构 / Standard ESLint plugin structure
export default {
  meta: {
    name: 'eslint-plugin-privacy',
    version: '1.0.0',
  },
  rules: {
    'no-raw-pii-logging': noRawPiiLogging,   // 自定义规则 / Custom rules
    'require-masking': requireMasking,
  },
  configs: {
    recommended: {
      plugins: ['privacy'],
      rules: {
        'privacy/no-raw-pii-logging': 'error',
        'privacy/require-masking': 'warn',
      },
    },
  },
};
```

### 10.2 本项目使用的插件 / Plugins Used in This Project

| 插件 / Plugin | 作用 / Purpose | 核心规则 / Core Rules |
|---|---|---|
| `typescript-eslint` | TS 类型感知检查 / TS type-aware checks | `no-explicit-any`, `no-unused-vars` |
| `eslint-plugin-react-hooks` | Hooks 规则强制 / Hooks rule enforcement | `rules-of-hooks`, `exhaustive-deps` |
| `eslint-plugin-react-refresh` | HMR 兼容性 / HMR compatibility | `only-export-components` |

### 10.3 规则严重级别 / Rule Severity Levels

| 级别 / Level | 值 / Value | 效果 / Effect |
|---|---|---|
| 关闭 / Off | `"off"` 或 `0` | 完全不检查 / No check at all |
| 警告 / Warning | `"warn"` 或 `1` | 显示但不失败 / Show but don't fail |
| 错误 / Error | `"error"` 或 `2` | 显示并导致非零退出码 / Show and cause non-zero exit |

**本项目的策略 / This project's strategy**：
- 生产代码质量规则 → `error`（必须修复）/ Production quality → `error` (must fix)
- 开发体验规则 → `warn`（提示但不阻塞）/ DX rules → `warn` (hint but don't block)
- Prettier 管辖的格式规则 → `off`（避免冲突）/ Prettier-owned formatting → `off` (avoid conflict)

## 11. 从旧版迁移指南 / Migration Guide from Legacy

### 11.1 .eslintrc → Flat Config 迁移 / .eslintrc → Flat Config Migration

```text
迁移步骤 / Migration steps:

1. 创建 eslint.config.js / Create eslint.config.js
   └─ 使用 ESM 语法 / Use ESM syntax

2. 转换 extends / Convert extends
   └─ "extends": ["eslint:recommended"]
   └─ → import js from '@eslint/js'; js.configs.recommended

3. 转换 plugins / Convert plugins
   └─ "plugins": ["@typescript-eslint"]
   └─ → import tseslint from 'typescript-eslint';

4. 转换 env / Convert env
   └─ "env": { "browser": true, "es2021": true }
   └─ → languageOptions: { globals: { ...globals.browser } }

5. 删除旧文件 / Remove old files
   └─ 删除 .eslintrc.* 和 .eslintignore / Delete .eslintrc.* and .eslintignore
```

### 11.2 常见迁移问题 / Common Migration Issues

| 问题 / Issue | 原因 / Cause | 解决 / Solution |
|---|---|---|
| `__dirname` 未定义 / undefined | ESM 无 __dirname / No __dirname in ESM | 使用 `import.meta.url` |
| 插件找不到 / Plugin not found | 需直接导入 / Need direct import | `import plugin from '...'` |
| globals 未定义 / globals undefined | 需导入 globals 包 / Need import globals | `import globals from 'globals'` |
| parser 配置失败 / Parser config fail | 语法变更 / Syntax change | `languageOptions.parser` |
| ignore 不生效 / ignore not working | 需独立对象 / Need standalone object | `{ ignores: [...] }` 单独一项 |

### 11.3 本项目迁移状态 / This Project's Migration Status

```text
本项目已完成 Flat Config 迁移 / This project has completed Flat Config migration:

✅ 使用 eslint.config.js / Using eslint.config.js
✅ 使用 typescript-eslint 统一包 / Using typescript-eslint unified package
✅ 无 .eslintrc.* 文件 / No .eslintrc.* files
✅ 无 .eslintignore（用 ignores 字段）/ No .eslintignore (uses ignores field)
✅ ESLint 9+ 兼容 / ESLint 9+ compatible
```

## 12. 规则分类体系 / Rule Classification System

### 12.1 规则来源分类 / Rule Source Classification

```text
本项目规则来源 / This project's rule sources:

┌─────────────────────────────────────────────────────────┐
│  @eslint/js (recommended)                                │
│  - no-unused-vars, no-undef, no-const-assign            │
│  - 基础 JavaScript 质量 / Basic JavaScript quality      │
├─────────────────────────────────────────────────────────┤
│  typescript-eslint (recommended)                         │
│  - no-explicit-any, no-unused-vars (TS 版)             │
│  - TypeScript 特有检查 / TypeScript-specific checks     │
├─────────────────────────────────────────────────────────┤
│  eslint-plugin-react-hooks                               │
│  - rules-of-hooks: Hook 调用规则 / Hook call rules      │
│  - exhaustive-deps: 依赖数组完整 / Deps array complete  │
├─────────────────────────────────────────────────────────┤
│  eslint-plugin-react-refresh                             │
│  - only-export-components: HMR 兼容 / HMR compatible   │
└─────────────────────────────────────────────────────────┘
```

### 12.2 规则功能分类 / Rule Functional Classification

| 分类 / Category | 规则示例 / Rule Examples | 目的 / Purpose |
|---|---|---|
| 正确性 / Correctness | `no-undef`, `no-const-assign` | 防止运行时错误 / Prevent runtime errors |
| 类型安全 / Type safety | `no-explicit-any`, `no-unsafe-*` | 充分利用 TS / Leverage TS fully |
| React 规范 / React conventions | `rules-of-hooks`, `exhaustive-deps` | 防止 Hook 误用 / Prevent Hook misuse |
| 可维护性 / Maintainability | `no-unused-vars`, `prefer-const` | 代码整洁 / Code cleanliness |
| HMR 兼容 / HMR compat | `only-export-components` | 开发体验 / Developer experience |

### 12.3 规则配置最佳实践 / Rule Configuration Best Practices

```typescript
// 推荐的规则配置策略 / Recommended rule configuration strategy
export default tseslint.config(
  // 1. 从 recommended 开始 / Start from recommended
  { extends: [js.configs.recommended, ...tseslint.configs.recommended] },

  // 2. 根据项目调整 / Adjust for project
  {
    rules: {
      // 升级：本项目严格禁止 any / Escalate: strictly forbid any
      '@typescript-eslint/no-explicit-any': 'error',

      // 降级：允许常量导出 / Downgrade: allow constant exports
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],

      // 关闭：与 Prettier 冲突 / Off: conflicts with Prettier
      '@typescript-eslint/indent': 'off',
      '@typescript-eslint/quotes': 'off',
    },
  },

  // 3. 测试文件特殊规则 / Special rules for test files
  {
    files: ['**/*.test.{ts,tsx}'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',  // 测试中允许 any / Allow any in tests
    },
  },
);
```

## 13. Monorepo 与多项目 Lint / Monorepo & Multi-project Lint

### 13.1 Monorepo Lint 架构 / Monorepo Lint Architecture

```text
多项目 Lint 配置策略 / Multi-project lint config strategy:

PrivShield/
├── console/
│   └── web/                  ← 前端 ESLint / Frontend ESLint
│       ├── eslint.config.js  ← 独立配置 / Independent config
│       ├── package.json      ← 独立依赖 / Independent deps
│       └── src/
├── PrivShield/      ← Python（无 ESLint）/ Python (no ESLint)
└── console/backend-go/       ← Go（无 ESLint）/ Go (no ESLint)

本项目结构特点 / This project's structure:
- 仅 console/web 使用 ESLint / Only console/web uses ESLint
- 无需 monorepo lint 工具 / No monorepo lint tool needed
- 各技术栈独立工具链 / Independent toolchain per tech stack
```

### 13.2 多包 Lint 方案对比 / Multi-package Lint Solution Comparison

| 方案 / Solution | 适用场景 / Use Case | 优点 / Pros | 缺点 / Cons |
|---|---|---|---|
| 独立配置 / Independent config | 少量包 / Few packages | 简单灵活 / Simple & flexible | 重复配置 / Duplicated config |
| 共享配置包 / Shared config pkg | 多包统一 / Multi-pkg uniform | DRY、一致 / DRY, consistent | 额外包 / Extra package |
| nx lint / turbo lint | 大型 monorepo | 缓存、并行 / Cache, parallel | 重量级 / Heavyweight |
| 根级配置 / Root config | 同质项目 / Homogeneous | 最简单 / Simplest | 不灵活 / Inflexible |

### 13.3 本项目 Lint 工作流 / This Project's Lint Workflow

```bash
# 前端 Lint / Frontend Lint
cd console/web
pnpm lint              # 检查所有文件 / Check all files
pnpm lint:fix          # 自动修复 / Auto-fix

# CI 中的 Lint 步骤 / Lint step in CI
# .github/workflows/ci.yml
- name: Lint frontend
  working-directory: console/web
  run: |
    corepack pnpm install --frozen-lockfile
    corepack pnpm lint
    corepack pnpm typecheck

# Python 质量检查（非 ESLint）/ Python quality (not ESLint)
- name: Lint backend
  run: |
    pip install ruff
    ruff check PrivShield/

# Go 质量检查（非 ESLint）/ Go quality (not ESLint)
- name: Lint Go backend
  working-directory: console/backend-go
  run: |
    go vet ./...
    staticcheck ./...
```

## 14. AST 操作与修复器 / AST Operations & Fixers

### 14.1 ESLint AST 基础 / ESLint AST Basics

```text
┌────────────────────────────────────────────────────────────────┐
│  ESLint AST 处理流程 / ESLint AST Processing Flow              │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  源代码 / Source Code                                          │
│    const x: number = 42;                                       │
│       │                                                        │
│       ▼  (解析 / Parse)                                        │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ESTree AST (typescript-eslint/parser)                │  │
│  │  Program                                              │  │
│  │   └── VariableDeclaration                             │  │
│  │       └── VariableDeclarator                          │  │
│  │           ├── id: Identifier (name: "x")              │  │
│  │           │   └── typeAnnotation: TSNumberKeyword     │  │
│  │           └── init: Literal (value: 42)               │  │
│  └──────────────────────────────────────────────────────┘  │
│       │                                                        │
│       ▼  (遍历 / Traverse)                                     │
│  规则访问器 / Rule Visitors:                                    │
│    VariableDeclarator(node) { ... }                            │
│    Identifier(node) { ... }                                    │
│       │                                                        │
│       ▼  (报告 / Report)                                       │
│  context.report({ node, message, fix })                        │
└────────────────────────────────────────────────────────────────┘
```

### 14.2 修复器（Fixer）实现 / Fixer Implementation

```javascript
// 自定义规则示例：强制使用 const 替代 let（当变量未重新赋值时）
// Custom rule: enforce const over let (when not reassigned)
module.exports = {
  meta: {
    type: 'suggestion',
    fixable: 'code',  // 声明可修复 / Declare fixable
    schema: [],
  },
  create(context) {
    return {
      VariableDeclaration(node) {
        if (node.kind !== 'let') return;

        const sourceCode = context.sourceCode;
        const scope = sourceCode.getScope(node);

        for (const decl of node.declarations) {
          if (decl.id.type !== 'Identifier') continue;

          // 检查是否被重新赋值 / Check if reassigned
          const variable = scope.variables.find(v => v.name === decl.id.name);
          const isReassigned = variable?.references.some(
            ref => ref.isWrite() && ref.identifier !== decl.id
          );

          if (!isReassigned) {
            context.report({
              node,
              message: 'Use "const" instead of "let" for never-reassigned variables.',
              // 自动修复：将 let 替换为 const
              // Auto-fix: replace let with const
              fix(fixer) {
                const letToken = sourceCode.getFirstToken(node);
                return fixer.replaceText(letToken, 'const');
              },
            });
          }
        }
      },
    };
  },
};
```

### 14.3 常用 AST 操作 / Common AST Operations

| 操作 / Operation | API | 用途 / Purpose |
|---|---|---|
| 替换文本 / Replace text | `fixer.replaceText(node, text)` | 替换节点内容 / Replace node content |
| 插入前 / Insert before | `fixer.insertTextBefore(node, text)` | 添加导入 / Add imports |
| 插入后 / Insert after | `fixer.insertTextAfter(node, text)` | 添加分号 / Add semicolons |
| 删除 / Remove | `fixer.remove(node)` | 删除节点 / Remove node |
| 替换范围 / Replace range | `fixer.replaceTextRange(range, text)` | 精确替换 / Precise replace |
| 组合修复 / Composite fix | `fixer.replaceText(...)` + `fixer.insertText...` | 多步修复 / Multi-step fix |

### 14.4 本项目 AST 实践 / This Project's AST Practice

| 场景 / Scenario | 规则 / Rule | 修复 / Fix |
|---|---|---|
| 未使用变量 / Unused vars | `@typescript-eslint/no-unused-vars` | ❌ 无自动修复 / No auto-fix |
| 显式 any / Explicit any | `@typescript-eslint/no-explicit-any` | ❌ 需手动 / Manual |
| Hook 规则 / Hook rules | `react-hooks/rules-of-hooks` | ❌ 无修复 / No fix |
| 导入排序 / Import order | `import/order` (可选) | ✅ 自动排序 / Auto-sort |
| 分号 / Semicolons | Prettier 管理 / Prettier managed | ✅ 自动 / Auto |

## 15. 规则测试 / Rule Testing

### 15.1 ESLint RuleTester / ESLint RuleTester

```javascript
// tests/rules/no-console-in-prod.test.js
const { RuleTester } = require('eslint');
const rule = require('../../rules/no-console-in-prod');

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2022,
    sourceType: 'module',
  },
});

ruleTester.run('no-console-in-prod', rule, {
  // 合法代码（不触发规则）/ Valid code (doesn't trigger rule)
  valid: [
    'const x = 42;',
    'logger.info("hello");',
    'console.error("critical");',  // 允许 error / Allow error
  ],

  // 非法代码（触发规则）/ Invalid code (triggers rule)
  invalid: [
    {
      code: 'console.log("debug");',
      errors: [{ messageId: 'noConsole' }],
      output: '',  // 期望修复后输出 / Expected output after fix
    },
    {
      code: 'console.warn("warning");',
      errors: [{ messageId: 'noConsole' }],
    },
  ],
});
```

### 15.2 TypeScript 规则测试 / TypeScript Rule Testing

```javascript
// 类型感知规则需要特殊配置 / Type-aware rules need special config
const { RuleTester } = require('@typescript-eslint/rule-tester');
const rule = require('../../rules/no-unsafe-api-response');

const ruleTester = new RuleTester({
  parser: '@typescript-eslint/parser',
  parserOptions: {
    project: './tsconfig.test.json',  // 类型信息 / Type info
    tsconfigRootDir: __dirname,
  },
});

ruleTester.run('no-unsafe-api-response', rule, {
  valid: [
    `
      interface ApiResponse { data: string; }
      const resp: ApiResponse = { data: 'ok' };
      console.log(resp.data);
    `,
  ],
  invalid: [
    {
      code: `
        const resp: any = fetchData();
        console.log(resp.data);
      `,
      errors: [{ messageId: 'unsafeAccess' }],
    },
  ],
});
```

### 15.3 本项目测试实践 / This Project's Testing Practice

| 方面 / Aspect | 状态 / Status | 说明 / Notes |
|---|---|---|
| 自定义规则测试 / Custom rule tests | N/A | 无自定义规则 / No custom rules |
| 配置验证 / Config validation | ✅ CI lint | `pnpm lint` 通过即可 / Pass is enough |
| 规则冲突检测 / Conflict detection | ✅ 手动 / Manual | 新增规则后全量 lint / Full lint after new rules |
| 回归测试 / Regression test | ✅ CI | 每次 PR 运行 lint / Lint on every PR |

## 16. 性能分析与 Profiling / Performance Analysis & Profiling

### 16.1 ESLint 性能分析工具 / ESLint Performance Profiling Tools

```bash
# 1. 内置计时 / Built-in timing
TIMING=1 npx eslint src/ --ext .ts,.tsx
# 输出每条规则耗时 / Output per-rule timing:
# Rule                    | Time (ms) | Relative
# ------------------------|-----------|--------
# @typescript-eslint/no-  |   245.3   |  32.1%
#   unused-vars           |           |
# react-hooks/exhaustive- |   180.2   |  23.6%
#   deps                  |           |
# @typescript-eslint/no-  |    95.1   |  12.4%
#   explicit-any          |           |

# 2. 详细性能分析 / Detailed profiling
TIMING=all npx eslint src/ --ext .ts,.tsx
# 显示每个文件每条规则 / Show per-file per-rule

# 3. 缓存加速 / Cache acceleration
npx eslint src/ --cache --cache-location .eslintcache
# 仅检查变更文件 / Only check changed files
```

### 16.2 性能优化策略 / Performance Optimization Strategies

```text
┌────────────────────────────────────────────────────────────────┐
│  ESLint 性能优化层次 / ESLint Performance Optimization Layers  │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  Layer 1: 减少解析范围 / Reduce parse scope                    │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ - .eslintignore 排除无关文件 / Exclude irrelevant       │  │
│  │ - 指定 --ext 而非全文件 / Specify --ext not all files  │  │
│  │ - 使用 overrides 缩小范围 / Use overrides to narrow    │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                                │
│  Layer 2: 减少规则数 / Reduce rule count                       │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ - 禁用不需要的规则 / Disable unneeded rules             │  │
│  │ - 类型感知规则最贵，按需启用 / Type-aware most expensive│  │
│  │ - Prettier 管理的规则关闭 / Turn off Prettier-owned     │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                                │
│  Layer 3: 缓存与并行 / Cache & parallel                       │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ - --cache 跳过未变文件 / Skip unchanged files           │  │
│  │ - lint-staged 仅检查暂存 / Only check staged           │  │
│  │ - CI 中分片并行 / Shard parallel in CI                 │  │
│  └────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### 16.3 本项目性能数据 / This Project's Performance Data

```text
本项目 lint 性能基准 / This project lint performance baseline:

  文件数 / Files:        ~35 (.ts/.tsx)
  规则数 / Rules:        ~45 active
  无缓存 / No cache:     ~2.8s
  有缓存 / With cache:   ~0.4s (仅变更 / only changed)
  类型感知 / Type-aware:  ~1.2s (占比 43%)

  最耗时规则 / Most expensive rules:
  1. @typescript-eslint/no-unused-vars    ~350ms
  2. react-hooks/exhaustive-deps          ~280ms
  3. @typescript-eslint/no-explicit-any   ~150ms
```

### 16.4 本项目性能实践 / This Project's Performance Practice

| 措施 / Measure | 状态 / Status | 效果 / Effect |
|---|---|---|
| --cache 标志 / --cache flag | ✅ CI 使用 / Used in CI | 减少 80% 时间 / 80% time reduction |
| lint-staged (pre-commit) | ✅ 配置 / Configured | 仅检查暂存文件 / Only staged files |
| .eslintignore | ✅ 配置 / Configured | 排除 dist/node_modules |
| 类型感知规则按需 / Type-aware on demand | ✅ 实践 / Practiced | 仅必要规则 / Only necessary rules |
| 并行分片 / Parallel sharding | ❌ 不需要 / Not needed | 文件少 / Few files |

---

## 17. ESLint 与 Prettier 协作 / ESLint & Prettier Collaboration

### 17.1 职责分离原则 / Separation of Concerns Principle

ESLint 和 Prettier 有明确的职责分工，理解这一点是正确配置的关键：

```
┌─────────────────────────────────────────────────────────────────┐
│       ESLint vs Prettier 职责 / Responsibilities                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ESLint (代码质量 / Code Quality):                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  - 未使用变量 / Unused variables                      │   │
│  │  - 未定义引用 / Undefined references                  │   │
│  │  - 类型错误 / Type errors                            │   │
│  │  - 最佳实践 / Best practices                         │   │
│  │  - 潜在 Bug / Potential bugs                         │   │
│  │  - 复杂度检查 / Complexity checks                    │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  Prettier (代码格式 / Code Formatting):                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  - 缩进 / Indentation                               │   │
│  │  - 引号 / Quotes                                    │   │
│  │  - 分号 / Semicolons                                │   │
│  │  - 行宽 / Line width                                │   │
│  │  - 空格 / Spacing                                   │   │
│  │  - 换行 / Line breaks                               │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  重叠区域（冲突）/ Overlap (Conflict):                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  - 尾随逗号 / Trailing commas                        │   │
│  │  - 箭头函数括号 / Arrow function parens              │   │
│  │  → 解决: 让 Prettier 管理格式 / Let Prettier handle  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 17.2 配置集成方式 / Configuration Integration

```javascript
// ===== eslint.config.js (Flat Config) =====
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import prettier from 'eslint-plugin-prettier/recommended';

export default tseslint.config(
  js.configs.recommended,
  ...tseslint.configs.recommended,

  // Prettier 集成（必须放最后）/ Prettier integration (must be last)
  prettier,

  // 自定义规则 / Custom rules
  {
    rules: {
      // 关闭与 Prettier 冲突的规则 / Disable rules conflicting with Prettier
      // (eslint-plugin-prettier/recommended 已自动处理 / auto-handled)

      // 代码质量规则（Prettier 不管）/ Quality rules (Prettier doesn't care)
      '@typescript-eslint/no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
      }],
      '@typescript-eslint/explicit-function-return-type': 'warn',
      'no-console': ['warn', { allow: ['warn', 'error'] }],
    },
  }
);
```

```json
// ===== .prettierrc =====
{
  "semi": true,
  "singleQuote": true,
  "tabWidth": 2,
  "trailingComma": "es5",
  "printWidth": 100,
  "bracketSpacing": true,
  "arrowParens": "always",
  "endOfLine": "lf"
}
```

### 17.3 工作流集成 / Workflow Integration

```json
// ===== package.json scripts =====
{
  "scripts": {
    // 检查（不修改）/ Check (no modify)
    "lint": "eslint . --report-unused-disable-directives",
    "format:check": "prettier --check .",

    // 修复 / Fix
    "lint:fix": "eslint . --fix",
    "format": "prettier --write .",

    // 组合 / Combined
    "check": "pnpm lint && pnpm format:check",
    "fix": "pnpm lint:fix && pnpm format"
  }
}
```

```yaml
# ===== lint-staged 配置 / lint-staged config =====
# .lintstagedrc.json
{
  "*.{ts,tsx}": [
    "eslint --fix",
    "prettier --write"
  ],
  "*.{json,md,css}": [
    "prettier --write"
  ]
}
```

---

## 18. 自定义规则开发实践 / Custom Rule Development Practice

### 18.1 规则结构详解 / Rule Structure Details

```javascript
// ===== 自定义 ESLint 规则 / Custom ESLint Rule =====
// rules/no-hardcoded-api-url.js

/** @type {import('eslint').Rule.RuleModule} */
module.exports = {
  meta: {
    type: 'problem',  // 'problem' | 'suggestion' | 'layout'
    docs: {
      description: '禁止硬编码 API URL / Disallow hardcoded API URLs',
      category: 'Best Practices',
      recommended: true,
    },
    fixable: 'code',  // 可自动修复 / Auto-fixable
    schema: [
      {
        type: 'object',
        properties: {
          allowedPatterns: {
            type: 'array',
            items: { type: 'string' },
          },
        },
        additionalProperties: false,
      },
    ],
    messages: {
      noHardcodedUrl: '禁止硬编码 API URL，请使用环境变量 / Use env var instead of hardcoded URL',
      useEnvVar: '替换为 import.meta.env.VITE_API_BASE_URL / Replace with env var',
    },
  },

  create(context) {
    const options = context.options[0] || {};
    const allowedPatterns = options.allowedPatterns || ['localhost', '127.0.0.1'];

    return {
      // 检查字符串字面量 / Check string literals
      Literal(node) {
        if (typeof node.value !== 'string') return;

        const value = node.value;

        // 检查是否是 URL / Check if it's a URL
        if (!value.match(/^https?:\/\//)) return;

        // 检查是否允许 / Check if allowed
        const isAllowed = allowedPatterns.some(p => value.includes(p));
        if (isAllowed) return;

        context.report({
          node,
          messageId: 'noHardcodedUrl',
          fix(fixer) {
            return fixer.replaceText(
              node,
              '`${import.meta.env.VITE_API_BASE_URL}`'
            );
          },
        });
      },

      // 检查模板字符串 / Check template literals
      TemplateLiteral(node) {
        const fullText = node.quasis.map(q => q.value.raw).join('');
        if (fullText.match(/^https?:\/\//) &&
            !allowedPatterns.some(p => fullText.includes(p))) {
          context.report({
            node,
            messageId: 'noHardcodedUrl',
          });
        }
      },
    };
  },
};
```

### 18.2 规则测试 / Rule Testing

```javascript
// ===== tests/rules/no-hardcoded-api-url.test.js =====
const { RuleTester } = require('eslint');
const rule = require('../../rules/no-hardcoded-api-url');

const ruleTester = new RuleTester({
  languageOptions: {
    ecmaVersion: 2022,
    sourceType: 'module',
  },
});

ruleTester.run('no-hardcoded-api-url', rule, {
  valid: [
    // 允许的 URL / Allowed URLs
    'const url = "http://localhost:8079";',
    'const url = "http://127.0.0.1:8079";',
    // 环境变量 / Env variables
    'const url = import.meta.env.VITE_API_BASE_URL;',
    // 非 URL 字符串 / Non-URL strings
    'const name = "hello world";',
  ],

  invalid: [
    {
      code: 'const url = "https://api.example.com/v1";',
      errors: [{ messageId: 'noHardcodedUrl' }],
      output: 'const url = `${import.meta.env.VITE_API_BASE_URL}`;',
    },
    {
      code: 'fetch("https://prod-api.internal/mask");',
      errors: [{ messageId: 'noHardcodedUrl' }],
    },
  ],
});
```

### 18.3 本地规则集成 / Local Rule Integration

```javascript
// ===== eslint.config.js 中使用本地规则 / Use local rules in config =====
import noHardcodedApiUrl from './rules/no-hardcoded-api-url.js';

export default [
  // ... 其他配置 / Other configs
  {
    plugins: {
      'local': {
        rules: {
          'no-hardcoded-api-url': noHardcodedApiUrl,
        },
      },
    },
    rules: {
      'local/no-hardcoded-api-url': ['error', {
        allowedPatterns: ['localhost', '127.0.0.1', 'example.com'],
      }],
    },
  },
];
```

---

## 19. 多语言 Lint 策略 / Multi-language Lint Strategy

### 19.1 项目多语言架构 / Project Multi-language Architecture

本项目包含三种语言，每种需要不同的 lint 工具：

```
┌─────────────────────────────────────────────────────────────────┐
│         多语言 Lint 架构 / Multi-language Lint Architecture     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  TypeScript/JavaScript (console/web/):                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  ESLint + typescript-eslint + Prettier              │   │
│  │  - 代码质量 + 格式 / Quality + formatting           │   │
│  │  - 类型感知规则 / Type-aware rules                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  Python (console/backend/):                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Ruff (替代 flake8 + black + isort)                  │   │
│  │  - 超快 linter + formatter / Ultra-fast              │   │
│  │  - 单一工具 / Single tool                            │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
│  Go (console/backend-go/):                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  go vet + staticcheck + gofmt                       │   │
│  │  - 官方工具链 / Official toolchain                   │   │
│  │  - 静态分析 / Static analysis                       │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 19.2 Python Lint 配置 / Python Lint Configuration

```toml
# ===== pyproject.toml (Ruff 配置) =====
[tool.ruff]
# 目标 Python 版本 / Target Python version
target-version = "py310"
line-length = 100

[tool.ruff.lint]
# 启用的规则 / Enabled rules
select = [
    "E",   # pycodestyle errors
    "W",   # pycodestyle warnings
    "F",   # pyflakes
    "I",   # isort
    "N",   # pep8-naming
    "UP",  # pyupgrade
    "B",   # flake8-bugbear
    "C4",  # flake8-comprehensions
    "SIM", # flake8-simplify
]
ignore = [
    "E501",  # 行长度由 formatter 处理 / Line length handled by formatter
]

[tool.ruff.lint.isort]
known-first-party = ["app"]

[tool.ruff.format]
quote-style = "double"
indent-style = "space"
```

```bash
# Python lint 命令 / Python lint commands
ruff check console/backend/          # 检查 / Check
ruff check console/backend/ --fix    # 修复 / Fix
ruff format console/backend/         # 格式化 / Format
```

### 19.3 Go Lint 配置 / Go Lint Configuration

```bash
# ===== Go lint 命令 / Go lint commands =====

# 官方 vet 工具 / Official vet tool
go vet ./...

# 静态分析（更严格）/ Static analysis (stricter)
staticcheck ./...

# 格式化 / Formatting
gofmt -w .

# 导入排序 / Import sorting
goimports -w .

# 综合检查脚本 / Comprehensive check script
#!/bin/bash
set -e
echo "Running go vet..."
go vet ./...
echo "Running staticcheck..."
staticcheck ./...
echo "Checking formatting..."
test -z "$(gofmt -l .)" || (echo "Please run gofmt -w ." && exit 1)
echo "All checks passed!"
```

### 19.4 统一 CI 检查 / Unified CI Check

```yaml
# ===== .github/workflows/lint.yml =====
name: Lint All Languages

on: [push, pull_request]

jobs:
  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: pnpm }
      - run: pnpm install --frozen-lockfile
        working-directory: console/web
      - run: pnpm lint
        working-directory: console/web

  python:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: '3.11' }
      - run: pip install ruff
      - run: ruff check console/backend/
      - run: ruff format --check console/backend/

  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go vet ./...
        working-directory: console/backend-go
      - run: go install honnef.co/go/tools/cmd/staticcheck@latest
      - run: staticcheck ./...
        working-directory: console/backend-go
```

### 19.5 Lint 工具对比总结 / Lint Tool Comparison Summary

| 语言 / Language | 工具 / Tool | 速度 / Speed | 功能 / Features | 本项目 / Project |
|---|---|---|---|---|
| TypeScript | ESLint | 中 / Medium | 最全面 / Most comprehensive | ✅ |
| TypeScript | Biome | 快 / Fast | 新兴 / Emerging | 未使用 / Not used |
| Python | Ruff | 极快 / Very fast | 全面 / Comprehensive | ✅ |
| Python | flake8+black | 慢 / Slow | 经典 / Classic | 已替换 / Replaced |
| Go | go vet | 快 / Fast | 官方 / Official | ✅ |
| Go | staticcheck | 中 / Medium | 深入 / Deep | ✅ |
| Go | golangci-lint | 中 / Medium | 聚合 / Aggregator | 可选 / Optional |

## 20. 代码质量度量与趋势分析 / Code Quality Metrics & Trend Analysis

代码质量度量是将代码健康状态量化的方法。通过持续跟踪关键指标，团队可以发现质量退化趋势并及时干预。ESLint 作为静态分析工具，是质量度量体系的核心数据源。

Code quality metrics quantify code health status. By continuously tracking key indicators, teams can detect quality degradation trends and intervene timely. ESLint, as a static analysis tool, is the core data source for quality metric systems.

### 20.1 质量指标采集 / Quality Metrics Collection

```typescript
// scripts/collect-quality-metrics.ts
// 代码质量指标采集器 / Code quality metrics collector
import { execSync } from 'child_process'
import { writeFileSync, mkdirSync, existsSync, readFileSync } from 'fs'
import { resolve } from 'path'

interface QualitySnapshot {
  timestamp: string
  commit: string
  branch: string
  metrics: {
    // ESLint 指标 / ESLint metrics
    totalErrors: number
    totalWarnings: number
    errorsByRule: Record<string, number>
    warningsByRule: Record<string, number>
    filesWithIssues: number
    totalFiles: number
    
    // 复杂度指标 / Complexity metrics
    avgCyclomaticComplexity: number
    maxComplexity: number
    functionsOverThreshold: number
    
    // 规模指标 / Size metrics
    totalLines: number
    avgFileLength: number
    maxFileLength: number
    
    // 派生指标 / Derived metrics
    errorDensity: number      // 每千行错误数 / Errors per 1000 lines
    warningDensity: number    // 每千行警告数 / Warnings per 1000 lines
    cleanFileRatio: number    // 无问题文件比例 / Clean file ratio
  }
}

class QualityMetricsCollector {
  collectEslintMetrics(): Partial<QualitySnapshot['metrics']> {
    // 运行 ESLint 并获取 JSON 输出 / Run ESLint with JSON output
    const output = execSync(
      'npx eslint . --format json --max-warnings=999999',
      { encoding: 'utf-8', cwd: resolve(__dirname, '../console/web') }
    )
    
    const results = JSON.parse(output)
    
    const errorsByRule: Record<string, number> = {}
    const warningsByRule: Record<string, number> = {}
    let totalErrors = 0
    let totalWarnings = 0
    let filesWithIssues = 0
    
    for (const file of results) {
      if (file.messages.length > 0) filesWithIssues++
      
      for (const msg of file.messages) {
        const rule = msg.ruleId || 'unknown'
        if (msg.severity === 2) {
          totalErrors++
          errorsByRule[rule] = (errorsByRule[rule] || 0) + 1
        } else {
          totalWarnings++
          warningsByRule[rule] = (warningsByRule[rule] || 0) + 1
        }
      }
    }
    
    return {
      totalErrors,
      totalWarnings,
      errorsByRule,
      warningsByRule,
      filesWithIssues,
      totalFiles: results.length,
    }
  }

  collectComplexityMetrics(): Partial<QualitySnapshot['metrics']> {
    // 使用 eslint-plugin-complexity 或 cr / Use complexity plugin or cr
    const output = execSync(
      'npx cr --format json src/',
      { encoding: 'utf-8', cwd: resolve(__dirname, '../console/web') }
    )
    
    const data = JSON.parse(output)
    const complexities = data.reports.map((r: any) => r.complexity.cyclomatic)
    
    return {
      avgCyclomaticComplexity: complexities.reduce((a: number, b: number) => a + b, 0) / complexities.length,
      maxComplexity: Math.max(...complexities),
      functionsOverThreshold: complexities.filter((c: number) => c > 10).length,
    }
  }

  generateSnapshot(): QualitySnapshot {
    const eslint = this.collectEslintMetrics()
    const complexity = this.collectComplexityMetrics()
    const totalLines = parseInt(execSync('find src -name "*.ts" -o -name "*.tsx" | xargs wc -l | tail -1', { encoding: 'utf-8' }))
    
    return {
      timestamp: new Date().toISOString(),
      commit: execSync('git rev-parse --short HEAD', { encoding: 'utf-8' }).trim(),
      branch: execSync('git branch --show-current', { encoding: 'utf-8' }).trim(),
      metrics: {
        ...eslint,
        ...complexity,
        totalLines,
        avgFileLength: totalLines / (eslint.totalFiles || 1),
        maxFileLength: 0,
        errorDensity: (eslint.totalErrors || 0) / (totalLines / 1000),
        warningDensity: (eslint.totalWarnings || 0) / (totalLines / 1000),
        cleanFileRatio: 1 - (eslint.filesWithIssues || 0) / (eslint.totalFiles || 1),
      } as QualitySnapshot['metrics'],
    }
  }
}
```

### 20.2 质量趋势报告 / Quality Trend Reporting

```typescript
// scripts/quality-trend-report.ts
// 质量趋势分析与报告 / Quality trend analysis and reporting
import { readFileSync, readdirSync } from 'fs'
import { resolve, join } from 'path'

interface TrendPoint {
  date: string
  errorDensity: number
  warningDensity: number
  cleanFileRatio: number
  avgComplexity: number
}

class QualityTrendAnalyzer {
  private history: TrendPoint[] = []

  loadHistory(snapshotDir: string) {
    const files = readdirSync(snapshotDir)
      .filter(f => f.endsWith('.json'))
      .sort()
    
    this.history = files.map(f => {
      const snap = JSON.parse(readFileSync(join(snapshotDir, f), 'utf-8'))
      return {
        date: snap.timestamp.split('T')[0],
        errorDensity: snap.metrics.errorDensity,
        warningDensity: snap.metrics.warningDensity,
        cleanFileRatio: snap.metrics.cleanFileRatio,
        avgComplexity: snap.metrics.avgCyclomaticComplexity,
      }
    })
  }

  detectDegradation(windowSize: number = 7): string[] {
    const alerts: string[] = []
    
    if (this.history.length < windowSize * 2) return alerts
    
    const recent = this.history.slice(-windowSize)
    const previous = this.history.slice(-windowSize * 2, -windowSize)
    
    // 错误密度趋势 / Error density trend
    const recentAvgErr = recent.reduce((s, p) => s + p.errorDensity, 0) / windowSize
    const prevAvgErr = previous.reduce((s, p) => s + p.errorDensity, 0) / windowSize
    
    if (recentAvgErr > prevAvgErr * 1.2) {
      alerts.push(
        `⚠️ 错误密度上升 / Error density rising: ` +
        `${prevAvgErr.toFixed(2)} → ${recentAvgErr.toFixed(2)} (+${((recentAvgErr/prevAvgErr - 1)*100).toFixed(0)}%)`
      )
    }
    
    // 清洁文件比例趋势 / Clean file ratio trend
    const recentClean = recent.reduce((s, p) => s + p.cleanFileRatio, 0) / windowSize
    const prevClean = previous.reduce((s, p) => s + p.cleanFileRatio, 0) / windowSize
    
    if (recentClean < prevClean - 0.05) {
      alerts.push(
        `⚠️ 清洁文件比例下降 / Clean file ratio declining: ` +
        `${(prevClean*100).toFixed(0)}% → ${(recentClean*100).toFixed(0)}%`
      )
    }
    
    // 复杂度趋势 / Complexity trend
    const recentCx = recent.reduce((s, p) => s + p.avgComplexity, 0) / windowSize
    const prevCx = previous.reduce((s, p) => s + p.avgComplexity, 0) / windowSize
    
    if (recentCx > prevCx * 1.15) {
      alerts.push(
        `⚠️ 平均复杂度上升 / Avg complexity rising: ` +
        `${prevCx.toFixed(1)} → ${recentCx.toFixed(1)}`
      )
    }
    
    return alerts
  }

  generateMarkdownReport(): string {
    const latest = this.history[this.history.length - 1]
    const alerts = this.detectDegradation()
    
    let report = `# 📊 代码质量周报 / Code Quality Weekly Report\n\n`
    report += `**日期 / Date**: ${latest.date}\n\n`
    report += `## 关键指标 / Key Metrics\n\n`
    report += `| 指标 / Metric | 当前 / Current | 状态 / Status |\n`
    report += `|---|---|---|\n`
    report += `| 错误密度 / Error density | ${latest.errorDensity.toFixed(2)}/kLOC | ${latest.errorDensity < 1 ? '✅' : '⚠️'} |\n`
    report += `| 清洁文件 / Clean files | ${(latest.cleanFileRatio*100).toFixed(0)}% | ${latest.cleanFileRatio > 0.8 ? '✅' : '⚠️'} |\n`
    report += `| 平均复杂度 / Avg complexity | ${latest.avgComplexity.toFixed(1)} | ${latest.avgComplexity < 8 ? '✅' : '⚠️'} |\n`
    
    if (alerts.length > 0) {
      report += `\n## ⚠️ 告警 / Alerts\n\n`
      alerts.forEach(a => report += `- ${a}\n`)
    }
    
    return report
  }
}
```

### 20.3 质量门与 CI 集成 / Quality Gates & CI Integration

```yaml
# .github/workflows/quality-gate.yml
name: Quality Gate

on:
  pull_request:
    branches: [main]

jobs:
  quality-gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # 完整历史用于趋势 / Full history for trends
      - uses: pnpm/action-setup@v4
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: pnpm }
      - run: pnpm install --frozen-lockfile
        working-directory: console/web

      - name: 采集指标 / Collect metrics
        run: |
          cd console/web
          npx ts-node ../../scripts/collect-quality-metrics.ts

      - name: 质量门检查 / Quality gate check
        run: |
          cd console/web
          # 硬性门：不允许新增 error / Hard gate: no new errors
          npx eslint . --max-warnings=999999 --format json | \
            jq '[.[] | .errorCount] | add' | \
            xargs -I{} test {} -eq 0 || exit 1

      - name: 趋势分析 / Trend analysis
        run: |
          npx ts-node scripts/quality-trend-report.ts > quality-report.md

      - name: 发布报告 / Publish report
        uses: actions/upload-artifact@v4
        with:
          name: quality-report
          path: quality-report.md
```

### 20.4 质量指标参考基准 / Quality Metrics Reference Benchmarks

| 指标 / Metric | 优秀 / Excellent | 良好 / Good | 警告 / Warning | 危险 / Critical |
|---|---|---|---|---|
| 错误密度 / Error density | 0/kLOC | <0.5/kLOC | 0.5-2/kLOC | >2/kLOC |
| 警告密度 / Warning density | <1/kLOC | 1-3/kLOC | 3-8/kLOC | >8/kLOC |
| 清洁文件比 / Clean file ratio | >95% | 80-95% | 60-80% | <60% |
| 平均复杂度 / Avg complexity | <5 | 5-8 | 8-12 | >12 |
| 最大复杂度 / Max complexity | <15 | 15-25 | 25-40 | >40 |
| 平均文件长度 / Avg file length | <150行 | 150-300 | 300-500 | >500 |
| Lint 修复时间 / Fix time | <1天 | 1-3天 | 3-7天 | >7天 |

## 21. 安全 Lint 与漏洞检测 / Security Lint & Vulnerability Detection

安全 Lint 是静态应用安全测试（SAST）的轻量形式。它能在编码阶段发现常见安全漏洞，如 XSS、注入攻击、不安全的加密实践等，比渗透测试早数个开发周期。

Security linting is a lightweight form of Static Application Security Testing (SAST). It catches common vulnerabilities like XSS, injection attacks, and insecure crypto practices at coding time—weeks before penetration testing would find them.

### 21.1 安全规则配置 / Security Rules Configuration

```typescript
// eslint.security.config.ts
// 安全专用 ESLint 配置 / Security-focused ESLint configuration
import tseslint from 'typescript-eslint'
import security from 'eslint-plugin-security'
import noUnsafe from 'eslint-plugin-no-unsanitized'

export default tseslint.config(
  // === 基础安全规则 / Base security rules ===
  security.configs.recommended,
  
  // === XSS 防护 / XSS protection ===
  noUnsafe.configs.recommended,
  
  {
    rules: {
      // --- 注入防护 / Injection protection ---
      'security/detect-object-injection': 'error',     // 对象注入 / Object injection
      'security/detect-non-literal-regexp': 'error',   // ReDoS / ReDoS
      'security/detect-non-literal-fs-filename': 'warn', // 路径遍历 / Path traversal
      'security/detect-child-process': 'error',        // 命令注入 / Command injection
      'security/detect-eval-with-expression': 'error', // 代码注入 / Code injection
      'security/detect-no-csrf-before-method-override': 'error',
      'security/detect-buffer-noassert': 'error',      // 缓冲区溢出 / Buffer overflow
      'security/detect-disable-mustache-escape': 'error',
      'security/detect-possible-timing-attacks': 'warn', // 时序攻击 / Timing attack
      
      // --- 不安全实践 / Insecure practices ---
      'no-eval': 'error',                    // 禁止 eval / No eval
      'no-implied-eval': 'error',            // 禁止隐式 eval / No implied eval
      'no-new-func': 'error',                // 禁止 Function 构造 / No Function constructor
      'no-script-url': 'error',              // 禁止 javascript: URL / No javascript: URL
      'no-unsanitized/property': 'error',    // 禁止未消毒属性 / No unsanitized property
      'no-unsanitized/method': 'error',      // 禁止未消毒方法 / No unsanitized method
    },
  },
  
  // === TypeScript 安全规则 / TypeScript security rules ===
  {
    files: ['**/*.ts', '**/*.tsx'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'warn',
      '@typescript-eslint/no-unsafe-call': 'warn',
      '@typescript-eslint/no-unsafe-member-access': 'warn',
      '@typescript-eslint/no-unsafe-return': 'warn',
      '@typescript-eslint/no-unsafe-argument': 'warn',
    },
  }
)
```

### 21.2 自定义安全规则 / Custom Security Rules

```typescript
// eslint-rules/no-hardcoded-secrets.ts
// 自定义规则：禁止硬编码密钥 / Custom rule: no hardcoded secrets
import type { Rule } from 'eslint'
import type { Node } from 'estree'

const SECRET_PATTERNS = [
  /(?:api[_-]?key|apikey)\s*[:=]\s*['"][^'"]{8,}['"]/i,
  /(?:secret|password|passwd|pwd)\s*[:=]\s*['"][^'"]{6,}['"]/i,
  /(?:token|auth_token|access_token)\s*[:=]\s*['"][^'"]{16,}['"]/i,
  /-----BEGIN (?:RSA |EC )?PRIVATE KEY-----/,
  /(?:AKIA|ASIA)[A-Z0-9]{16}/,  // AWS Access Key
]

const noHardcodedSecrets: Rule.RuleModule = {
  meta: {
    type: 'problem',
    docs: {
      description: '禁止硬编码密钥和凭证 / Disallow hardcoded secrets and credentials',
      category: 'Security',
      recommended: true,
    },
    messages: {
      hardcodedSecret: '检测到硬编码密钥，请使用环境变量 / Hardcoded secret detected, use env vars',
      suspiciousPattern: '可疑的密钥模式 / Suspicious secret pattern: {{pattern}}',
    },
    schema: [],
  },

  create(context) {
    return {
      // 检查字符串字面量 / Check string literals
      Literal(node: Node & { value: unknown }) {
        if (typeof node.value !== 'string') return
        if (node.value.length < 8) return
        
        for (const pattern of SECRET_PATTERNS) {
          if (pattern.test(node.value)) {
            context.report({
              node,
              messageId: 'suspiciousPattern',
              data: { pattern: pattern.source.slice(0, 30) },
            })
            return
          }
        }
      },
      
      // 检查变量赋值 / Check variable assignments
      VariableDeclarator(node: any) {
        if (!node.id?.name || !node.init) return
        
        const name = node.id.name.toLowerCase()
        const isSecretName = /(?:key|secret|password|token|credential)/.test(name)
        
        if (isSecretName && node.init.type === 'Literal' && typeof node.init.value === 'string') {
          if (node.init.value.length > 6) {
            context.report({ node: node.init, messageId: 'hardcodedSecret' })
          }
        }
      },
    }
  },
}

export default noHardcodedSecrets
```

### 21.3 隐私相关 Lint 规则 / Privacy-related Lint Rules

```typescript
// eslint-rules/privacy-aware-lint.ts
// 隐私感知 Lint 规则 / Privacy-aware lint rules
// 本项目特有：确保敏感数据处理符合规范
// Project-specific: ensure sensitive data handling follows standards
import type { Rule } from 'eslint'

const privacyRules: Record<string, Rule.RuleModule> = {
  // 规则 1: 日志中禁止输出敏感字段 / Rule 1: No sensitive fields in logs
  'no-sensitive-logging': {
    meta: {
      type: 'problem',
      docs: { description: '禁止在日志中输出敏感数据 / No sensitive data in logs' },
      messages: {
        sensitiveLog: '日志中包含敏感字段 "{{field}}"，请脱敏后输出 / Sensitive field in log',
      },
    },
    create(context) {
      const sensitiveFields = ['password', 'idCard', 'phone', 'email', 'ssn', 'creditCard']
      
      return {
        CallExpression(node: any) {
          // 检测 console.log / logger.info 等 / Detect console.log / logger.info
          const callee = node.callee
          const isLogCall = 
            (callee.object?.name === 'console' || callee.object?.name === 'logger') &&
            ['log', 'info', 'warn', 'error', 'debug'].includes(callee.property?.name)
          
          if (!isLogCall) return
          
          // 检查参数中是否包含敏感字段 / Check args for sensitive fields
          for (const arg of node.arguments) {
            if (arg.type === 'TemplateLiteral') {
              for (const expr of arg.expressions) {
                if (expr.type === 'MemberExpression' && expr.property?.name) {
                  if (sensitiveFields.includes(expr.property.name)) {
                    context.report({
                      node: arg,
                      messageId: 'sensitiveLog',
                      data: { field: expr.property.name },
                    })
                  }
                }
              }
            }
          }
        },
      }
    },
  },

  // 规则 2: 敏感数据必须经过脱敏处理 / Rule 2: Sensitive data must be masked
  'require-masking-before-response': {
    meta: {
      type: 'suggestion',
      docs: { description: '响应前必须脱敏 / Must mask before response' },
      messages: {
        unmasked: '敏感字段 "{{field}}" 未经脱敏直接返回 / Sensitive field returned unmasked',
      },
    },
    create(context) {
      return {
        // 检测 res.json() 中的敏感字段 / Detect sensitive fields in res.json()
        CallExpression(node: any) {
          if (node.callee?.property?.name !== 'json') return
          // 简化检测逻辑 / Simplified detection logic
        },
      }
    },
  },
}

export default privacyRules
```

### 21.4 安全 Lint 工具链对比 / Security Lint Toolchain Comparison

| 工具 / Tool | 检测范围 / Detection Scope | 语言 / Language | 误报率 / False Positive | 本项目 / Project |
|---|---|---|---|---|
| eslint-plugin-security | 通用安全模式 / General patterns | JS/TS | 中 / Medium | ✅ |
| eslint-plugin-no-unsanitized | XSS / XSS | JS/TS | 低 / Low | ✅ |
| Semgrep | 全面 SAST / Comprehensive | 多语言 / Multi | 低 / Low | 可选 / Optional |
| CodeQL | 深度数据流 / Deep dataflow | 多语言 / Multi | 极低 / Very low | 未使用 / Not used |
| Snyk Code | 商业 SAST / Commercial | 多语言 / Multi | 低 / Low | 未使用 / Not used |
| 自定义规则 / Custom rules | 业务特定 / Business-specific | JS/TS | 极低 / Very low | ✅ |

## 22. 团队规范治理与演进 / Team Standard Governance & Evolution

团队规范治理是将编码标准从“个人习惯”提升为“组织能力”的过程。它包括规则的制定、沟通、执行、度量、迭代全生命周期管理。

Team standard governance elevates coding standards from "individual habits" to "organizational capability." It covers the full lifecycle of rule creation, communication, enforcement, measurement, and iteration.

### 22.1 规则分级体系 / Rule Tiering System

```typescript
// eslint.config.tiers.ts
// 规则分级配置 / Rule tiering configuration
//
// 分级原则 / Tiering principles:
// - P0 (error): 必须修复，阻塞 CI / Must fix, blocks CI
// - P1 (warn): 应该修复，不阻塞但跟踪 / Should fix, tracked
// - P2 (off→warn): 新规则引入期 / New rule introduction
// - P3 (off): 参考规则，不强制 / Reference only

import tseslint from 'typescript-eslint'

// === P0: 强制规则（阻塞合并）/ Mandatory rules (block merge) ===
const P0_RULES = {
  // 正确性 / Correctness
  'no-undef': 'error',
  'no-unused-vars': 'off',  // 由 TS 处理 / Handled by TS
  '@typescript-eslint/no-unused-vars': 'error',
  'no-const-assign': 'error',
  'no-dupe-keys': 'error',
  'no-unreachable': 'error',
  'eqeqeq': ['error', 'always'],
  
  // 安全 / Security
  'no-eval': 'error',
  'no-implied-eval': 'error',
  'security/detect-object-injection': 'error',
  
  // 隐私 / Privacy (project-specific)
  'privacy/no-sensitive-logging': 'error',
}

// === P1: 推荐规则（跟踪但不阻塞）/ Recommended (tracked, non-blocking) ===
const P1_RULES = {
  'complexity': ['warn', { max: 15 }],
  'max-depth': ['warn', { max: 4 }],
  'max-params': ['warn', { max: 5 }],
  '@typescript-eslint/explicit-function-return-type': 'warn',
  '@typescript-eslint/no-explicit-any': 'warn',
  'prefer-const': 'warn',
  'no-console': ['warn', { allow: ['warn', 'error'] }],
}

// === P2: 引入期规则（新规则缓冲期）/ Introduction period ===
const P2_RULES = {
  // 新规则先 warn 2 个 sprint，然后升级为 error
  // New rules start as warn for 2 sprints, then upgrade to error
  '@typescript-eslint/strict-boolean-expressions': 'warn',
  'functional/no-let': 'warn',
}

export default tseslint.config(
  { rules: P0_RULES },
  { rules: P1_RULES },
  { rules: P2_RULES },
)
```

### 22.2 规则变更流程 / Rule Change Process

```markdown
<!-- docs/eslint-rule-change-process.md -->
<!-- ESLint 规则变更流程 / ESLint Rule Change Process -->

## 规则变更流程 / Rule Change Process

### 1. 提案阶段 / Proposal Phase
- 任何团队成员可提出规则变更 RFC
- Any team member can propose a rule change RFC
- 必须包含：问题描述、规则配置、影响分析、迁移方案
- Must include: problem, config, impact analysis, migration plan

### 2. 评审阶段 / Review Phase
- 至少 2 人 approve / At least 2 approvals
- 影响分析：受影响文件数、修复工作量
- Impact: affected file count, fix effort
- 试运行：在分支上启用 1 周 / Trial: enable on branch for 1 week

### 3. 引入阶段 / Introduction Phase
- 新规则先设为 `warn` / New rules start as `warn`
- 缓冲期：2 个 sprint / Buffer: 2 sprints
- 期间修复所有 warning / Fix all warnings during buffer

### 4. 强制阶段 / Enforcement Phase
- 升级为 `error` / Upgrade to `error`
- CI 阻塞 / CI blocking
- 不可降级 / No downgrade

### 5. 度量与回顾 / Metrics & Retrospective
- 每月回顾规则有效性 / Monthly rule effectiveness review
- 误报率 > 10% 的规则需调整 / Rules with >10% FP rate need adjustment
- 季度清理无用规则 / Quarterly cleanup of unused rules
```

### 22.3 技术债务管理 / Technical Debt Management

```typescript
// scripts/lint-debt-tracker.ts
// Lint 技术债务跟踪器 / Lint technical debt tracker
import { execSync } from 'child_process'

interface DebtItem {
  rule: string
  count: number
  severity: 'error' | 'warning'
  firstSeen: string
  trend: 'increasing' | 'stable' | 'decreasing'
  estimatedFixHours: number
}

class LintDebtTracker {
  private baseline: Record<string, number> = {}
  private current: Record<string, number> = {}

  // 建立基线（只减不增原则）/ Establish baseline (ratchet principle)
  establishBaseline() {
    const output = execSync('npx eslint . --format json', { encoding: 'utf-8' })
    const results = JSON.parse(output)
    
    for (const file of results) {
      for (const msg of file.messages) {
        const rule = msg.ruleId || 'unknown'
        this.baseline[rule] = (this.baseline[rule] || 0) + 1
      }
    }
    
    // 保存基线 / Save baseline
    // writeFileSync('.eslint-baseline.json', JSON.stringify(this.baseline, null, 2))
  }

  // 检查是否有新增债务 / Check for new debt
  checkForRegression(): { rule: string; delta: number }[] {
    const regressions: { rule: string; delta: number }[] = []
    
    for (const [rule, count] of Object.entries(this.current)) {
      const base = this.baseline[rule] || 0
      if (count > base) {
        regressions.push({ rule, delta: count - base })
      }
    }
    
    return regressions
  }

  // 生成债务报告 / Generate debt report
  generateReport(): string {
    const totalDebt = Object.values(this.current).reduce((a, b) => a + b, 0)
    const regressions = this.checkForRegression()
    
    let report = `# Lint 技术债务报告 / Lint Debt Report\n\n`
    report += `总债务 / Total debt: **${totalDebt}** 条 / items\n\n`
    
    if (regressions.length > 0) {
      report += `## ❌ 新增债务 / New Regressions\n\n`
      regressions.forEach(r => {
        report += `- \`${r.rule}\`: +${r.delta} 条 / items\n`
      })
    } else {
      report += `## ✅ 无新增债务 / No New Regressions\n`
    }
    
    return report
  }
}

// CI 中使用：只减不增原则 / In CI: ratchet principle
// 新代码不允许引入新的 lint 问题 / New code must not introduce new lint issues
// 存量问题逐步清理 / Existing issues cleaned up gradually
```

### 22.4 规范治理成熟度模型 / Governance Maturity Model

| 级别 / Level | 名称 / Name | 特征 / Characteristics | 执行方式 / Enforcement |
|---|---|---|---|
| L1 | 无规范 / No standards | 各自为政 / Each dev's own style | 无 / None |
| L2 | 基础规范 / Basic | 有配置文件，手动运行 / Config exists, manual run | 手动 / Manual |
| L3 | CI 集成 / CI integrated | 提交自动检查 / Auto-check on commit | CI 阻塞 / CI block |
| L4 | 分级治理 / Tiered | P0/P1/P2 分级，有缓冲期 / Tiered with buffer | 分级阻塞 / Tiered block |
| L5 | 度量驱动 / Metrics-driven | 趋势跟踪、债务管理 / Trend tracking, debt mgmt | 数据驱动 / Data-driven |
| L6 | 自动化演进 / Auto-evolving | 规则自动提议、ML 辅助 / Auto-proposal, ML-assisted | 智能 / Intelligent |
