/**
 * 侧边栏组件：接口导航树 / Sidebar Component: Endpoint Navigation Tree
 *
 * 功能：按分类分组展示全部接口，支持搜索过滤与分组折叠/展开；
 * 顶部提供"接口总览"、"批量测试"、"文件处理"、"负载均衡"、"动态分类分级"五个快捷入口。
 *
 * Function: Displays all endpoints grouped by category, supports search filtering
 * and group collapse/expand; top provides five quick entries: "Overview", "Batch Test",
 * "File Test", "Load Balancer", "Dynamic Classification".
 *
 * 详细逻辑 / Detailed Logic：
 *   1. 接收 samples 数组，按 category 分组（useMemo 缓存）；
 *   2. 搜索框输入时实时过滤（匹配 label / path / category）；
 *   3. 默认全部折叠，选中项变化时自动展开所属分组；
 *   4. 搜索时强制展开所有命中分组，便于查看结果。
 */

/** 引入 React Hooks：副作用 / 记忆化 / 状态 / Import React Hooks: side effect / memoization / state */
import { useEffect, useMemo, useState } from 'react';
/** 引入端点示例类型 / Import endpoint sample type */
import type { EndpointSample } from '@/types/api';
/** 引入分类元数据与排序工具 / Import category metadata and ordering utility */
import { categoryMeta, orderCategories } from '@/lib/categories';
/** 引入内联 SVG 图标组件 / Import inline SVG icon component */
import { Icon } from '@/components/icons';
/** 引入国际化 Hook / Import i18n Hook */
import { useI18n } from '@/i18n';

/**
 * Sidebar 组件属性接口 / Sidebar Component Props Interface
 *
 * 由 App 组件传入，控制侧边栏的全部行为与高亮状态。
 * Passed from App component, controls all sidebar behaviors and highlight states.
 */
interface SidebarProps {
  /** 全部端点示例数据 / All endpoint sample data */
  samples: EndpointSample[];
  /** 当前选中的端点（null 表示未选中）/ Currently selected endpoint (null means none) */
  selected: EndpointSample | null;
  /** 选择端点的回调 / Endpoint selection callback */
  onSelect: (sample: EndpointSample) => void;
  /** 返回总览页 / Return to overview page */
  onHome?: () => void;
  /** 进入批量测试 / Enter batch test */
  onBatch?: () => void;
  /** 当前是否处于批量测试视图 / Whether currently in batch test view */
  batchActive?: boolean;
  /** 进入文件处理 / Enter file processing */
  onFileTest?: () => void;
  /** 当前是否处于文件处理视图 / Whether currently in file test view */
  fileTestActive?: boolean;
  /** 进入负载均衡测试 / Enter load balancer test */
  onLbTest?: () => void;
  /** 当前是否处于负载均衡测试视图 / Whether currently in LB test view */
  lbTestActive?: boolean;
  /** 进入动态分类分级 / Enter dynamic classification */
  onDynClassify?: () => void;
  /** 当前是否处于动态分类分级视图 / Whether currently in dynamic classification view */
  dynClassifyActive?: boolean;
  /** 进入运维诊断 / Enter ops diagnostics */
  onOps?: () => void;
  /** 当前是否处于运维诊断视图 / Whether currently in ops diagnostics view */
  opsActive?: boolean;
}


/**
 * HTTP 方法徽章配色 / HTTP Method Badge Color Scheme
 *
 * 根据方法返回对应的 Tailwind CSS 类名：
 * Returns corresponding Tailwind CSS class names based on method:
 *   - GET → 绿色系 / Green scheme
 *   - POST → 蓝色系 / Blue scheme
 *   - 其他 → 灰色系 / Gray scheme
 *
 * @param method - HTTP 方法字符串 / HTTP method string
 * @returns Tailwind CSS 类名 / Tailwind CSS class names
 */
function methodBadge(method: string): string {
  switch (method.toUpperCase()) {
    case 'GET':
      return 'bg-emerald-50 text-emerald-600';   // 绿色：安全读取 / Green: safe read
    case 'POST':
      return 'bg-sky-50 text-sky-600';           // 蓝色：数据提交 / Blue: data submission
    default:
      return 'bg-gray-100 text-gray-500';        // 灰色：其他方法 / Gray: other methods
  }
}

/**
 * 按 category 分组端点示例 / Group endpoint samples by category
 *
 * 遍历 samples 数组，以 category 为键构建 Map。
 * Iterates samples array, builds Map with category as key.
 *
 * @param samples - 全部端点示例 / All endpoint samples
 * @returns 分类名 → 端点数组的 Map / Map of category name → endpoints array
 */
function groupSamples(samples: EndpointSample[]): Map<string, EndpointSample[]> {
  const grouped = new Map<string, EndpointSample[]>(); // 初始化分组 Map / Initialize grouping Map
  for (const s of samples) {
    // 获取或初始化该分类的数组 / Get or initialize array for this category
    const list = grouped.get(s.category) || [];
    list.push(s);                    // 加入当前端点 / Add current endpoint
    grouped.set(s.category, list);   // 更新 Map / Update Map
  }
  return grouped;
}

/**
 * 侧边栏主组件 / Sidebar Main Component
 *
 * 详细逻辑 / Detailed Logic：
 *   1. 维护搜索关键词 query 与展开分组集合 expanded 两个本地状态；
 *   2. useMemo 缓存分组与排序结果，避免每次渲染重复计算；
 *   3. 搜索时实时过滤（匹配 label / path / category），强制展开命中分组；
 *   4. 选中项变化时自动展开所属分组，保证选中接口始终可见；
 *   5. 顶部渲染五个快捷入口按钮（总览/批量/文件/负载均衡/动态分类）；
 *   6. 下方渲染可折叠的分类分组列表，每组内列出该分类的全部接口。
 */
export default function Sidebar({
  samples,
  selected,
  onSelect,
  onHome,
  onBatch,
  batchActive = false,
  onFileTest,
  fileTestActive = false,
  onLbTest,
  lbTestActive = false,
  onDynClassify,
  dynClassifyActive = false,
  onOps,
  opsActive = false,
}: SidebarProps) {
  const { t } = useI18n(); // 获取翻译函数 / Get translation function

  /** 搜索关键词状态 / Search query state */
  const [query, setQuery] = useState('');
  /** 展开的分组集合（默认全部折叠，避免首页侧边栏过长）/ Expanded groups set (default all collapsed to avoid overly long sidebar) */
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  // 按 category 分组（useMemo 缓存，仅 samples 变化时重算）/ Group by category (useMemo cached, recompute only when samples change)
  const grouped = useMemo(() => groupSamples(samples), [samples]);
  // 对分类名排序（保证固定顺序）/ Sort category names (ensure fixed order)
  const categories = useMemo(() => orderCategories([...grouped.keys()]), [grouped]);

  /**
   * 搜索过滤逻辑 / Search Filtering Logic
   *
   * 将输入转小写后匹配 label / path / category 三个字段，
   * 仅保留有命中项的分类。空输入时返回全部分组。
   * Converts input to lowercase and matches label / path / category fields,
   * keeps only categories with hits. Returns all groups when input is empty.
   */
  const q = query.trim().toLowerCase(); // 去除首尾空白并转小写 / Trim and lowercase
  const filtered = useMemo(() => {
    if (!q) return grouped; // 无搜索词时返回全部 / Return all when no search term
    const map = new Map<string, EndpointSample[]>(); // 过滤结果 Map / Filtered result Map
    for (const [cat, list] of grouped) {
      // 对每个分类下的端点做三字段匹配 / Match three fields for each endpoint in category
      const hits = list.filter(
        (s) =>
          s.label.toLowerCase().includes(q) ||   // 匹配接口名称 / Match endpoint label
          s.path.toLowerCase().includes(q) ||    // 匹配路径 / Match path
          cat.toLowerCase().includes(q),         // 匹配分类名 / Match category name
      );
      if (hits.length > 0) map.set(cat, hits); // 仅保留有命中的分类 / Keep only categories with hits
    }
    return map;
  }, [grouped, q]);

  /**
   * 选中项变化时自动展开所属分组 / Auto-expand group when selected item changes
   *
   * 包括从总览卡片进入的场景，保证选中接口在侧边栏中始终可见。
   * Including entering from overview cards, ensures selected endpoint is always visible in sidebar.
   */
  useEffect(() => {
    if (selected) {
      // 若所属分组尚未展开，则添加到 expanded 集合 / If group not yet expanded, add to expanded set
      setExpanded((prev) =>
        prev.has(selected.category) ? prev : new Set(prev).add(selected.category),
      );
    }
  }, [selected]);

  /**
   * 切换分组折叠/展开状态 / Toggle group collapse/expand state
   *
   * @param cat - 分类名 / Category name
   */
  const toggle = (cat: string) => {
    setExpanded((prev) => {
      const next = new Set(prev); // 复制集合避免直接修改 state / Copy set to avoid mutating state directly
      if (next.has(cat)) next.delete(cat); // 已展开则折叠 / Collapse if expanded
      else next.add(cat);                  // 已折叠则展开 / Expand if collapsed
      return next;
    });
  };

  // 过滤后仍有数据的分类（用于渲染）/ Categories with data after filtering (for rendering)
  const visibleCategories = categories.filter((c) => filtered.has(c));

  return (
    /* 侧边栏容器：固定宽度 288px(w-72)、不缩小、纵向弹性布局、右侧边框分隔 */
    /* Sidebar container: fixed width 288px(w-72), no shrink, vertical flex layout, right border separator */
    <aside className="flex w-72 shrink-0 flex-col border-r border-gray-200 bg-white">
      {/* ====== 搜索框区域 / Search Box Area ====== */}
      {/* 底部边框分隔，内边距 12px / Bottom border separator, padding 12px */}
      <div className="border-b border-gray-100 p-3">
        {/* relative 定位容器，用于放置搜索图标 / Relative positioning container for search icon */}
        <label className="relative block">
          {/* 搜索图标：绝对定位于输入框左侧，pointer-events-none 避免拦截点击 */}
          {/* Search icon: absolutely positioned at input left, pointer-events-none to avoid intercepting clicks */}
          <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400">
            <Icon name="search" className="h-3.5 w-3.5" />
          </span>
          {/* 搜索输入框：受控组件，输入时实时更新 query 状态触发过滤 */}
          {/* Search input: controlled component, updates query state on input to trigger filtering */}
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('sidebar.search_placeholder')}
            className="w-full rounded-lg border border-gray-200 bg-gray-50 py-1.5 pl-8 pr-3 text-sm text-gray-700 placeholder-gray-400 transition-colors focus:border-indigo-400 focus:bg-white focus:outline-none focus:ring-2 focus:ring-indigo-100"
          />
        </label>
      </div>

      {/* ====== 分组导航列表（可滚动区域）/ Grouped Navigation List (Scrollable Area) ====== */}
      {/* flex-1 占满剩余高度，overflow-y-auto 内容溢出时纵向滚动 */}
      {/* flex-1 fills remaining height, overflow-y-auto enables vertical scroll on overflow */}
      <nav className="flex-1 overflow-y-auto px-2 py-2">
        {/* --- 快捷入口：接口总览 / Quick Entry: API Overview --- */}
        {/* 未选中端点且非批量模式时高亮（靛蓝底色）/ Highlighted when no endpoint selected and not in batch mode (indigo bg) */}
        <button
          onClick={onHome}
          className={[
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            /* 条件高亮：当前无选中且非批量视图 → 靛蓝活跃态 / Conditional highlight: no selection & not batch → indigo active */
            !selected && !batchActive
              ? 'bg-indigo-50 font-medium text-indigo-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          {/* 图标容器：灰色圆角方块 / Icon container: gray rounded square */}
          <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 text-gray-500">
            <Icon name="inbox" className="h-3 w-3" />
          </span>
          {t('sidebar.overview')}
        </button>
        {/* --- 快捷入口：批量测试 / Quick Entry: Batch Test --- */}
        {/* batchActive 为 true 时高亮 / Highlighted when batchActive is true */}
        <button
          onClick={onBatch}
          className={[
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            batchActive
              ? 'bg-indigo-50 font-medium text-indigo-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 text-gray-500">
            <Icon name="play" className="h-3 w-3" />
          </span>
          {t('sidebar.batch_test')}
        </button>
        {/* --- 快捷入口：文件处理 / Quick Entry: File Processing --- */}
        {/* fileTestActive 为 true 时高亮 / Highlighted when fileTestActive is true */}
        <button
          onClick={onFileTest}
          className={[
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            fileTestActive
              ? 'bg-indigo-50 font-medium text-indigo-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 text-gray-500">
            <Icon name="upload" className="h-3 w-3" />
          </span>
          {t('sidebar.file_test')}
        </button>
        {/* --- 快捷入口：负载均衡测试 / Quick Entry: Load Balancer Test --- */}
        {/* lbTestActive 为 true 时高亮 / Highlighted when lbTestActive is true */}
        <button
          onClick={onLbTest}
          className={[
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            lbTestActive
              ? 'bg-indigo-50 font-medium text-indigo-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 text-gray-500">
            <Icon name="scale" className="h-3 w-3" />
          </span>
          {t('sidebar.lb_test')}
        </button>
        {/* --- 快捷入口：动态分类分级 / Quick Entry: Dynamic Classification --- */}
        {/* dynClassifyActive 为 true 时以紫色系高亮（区别于其他靛蓝入口）/ Purple highlight when active (distinguished from indigo entries) */}
        <button
          onClick={onDynClassify}
          className={[
            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            dynClassifyActive
              ? 'bg-purple-50 font-medium text-purple-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          {/* 紫色图标容器，突出 AI 分类功能 / Purple icon container, highlights AI classification feature */}
          <span className="flex h-5 w-5 items-center justify-center rounded bg-purple-100 text-purple-600">
            <Icon name="sparkles" className="h-3 w-3" />
          </span>
          {t('sidebar.dyn_classify')}
        </button>
        {/* --- 快捷入口：运维诊断 / Quick Entry: Ops Diagnostics --- */}
        {/* opsActive 为 true 时以青色系高亮 / Teal highlight when opsActive is true */}
        {/* mb-2 与下方分组列表保持间距 / mb-2 keeps spacing from group list below */}
        <button
          onClick={onOps}
          className={[
            'mb-2 flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
            opsActive
              ? 'bg-teal-50 font-medium text-teal-700'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
          ].join(' ')}
        >
          {/* 青色图标容器，突出运维排障功能 / Teal icon container, highlights ops troubleshooting feature */}
          <span className="flex h-5 w-5 items-center justify-center rounded bg-teal-100 text-teal-600">
            <Icon name="activity" className="h-3 w-3" />
          </span>
          {t('sidebar.ops')}
        </button>

        {/* ====== 搜索无结果提示 / No Search Results Hint ====== */}
        {/* 过滤后无可见分类时显示空态提示 / Shows empty state when no visible categories after filtering */}
        {visibleCategories.length === 0 && (
          <div className="px-3 py-8 text-center text-sm text-gray-400">
            {t('sidebar.no_match')}
          </div>
        )}
        {/* ====== 分类分组列表渲染 / Category Group List Rendering ====== */}
        {/* 遍历过滤后的可见分类，每组渲染为可折叠区块 / Iterates filtered visible categories, each rendered as collapsible block */}
        {visibleCategories.map((category) => {
          const meta = categoryMeta(category); // 获取分类元数据（图标/配色）/ Get category metadata (icon/color)
          const list = filtered.get(category)!; // 该分类下的端点列表 / Endpoints list under this category
          // 搜索时强制展开命中分组（isCollapsed=false）；否则尊重用户手动折叠状态
          // Force expand hit groups during search (isCollapsed=false); otherwise respect user's manual collapse state
          const isCollapsed = q ? false : !expanded.has(category);
          return (
            /* 单个分类容器，底部间距 10px / Single category container, bottom margin 10px */
            <div key={category} className="mb-2.5">
              {/* 分组标题按钮：点击切换折叠/展开 / Group header button: click to toggle collapse/expand */}
              <button
                onClick={() => toggle(category)}
                className="flex w-full items-center gap-2 rounded-md border border-gray-300 bg-gray-100 px-2 py-1.5 text-left transition-colors hover:border-gray-400 hover:bg-gray-200"
              >
                {/* 折叠/展开箭头图标：折叠时右箭头，展开时下箭头 / Collapse/expand arrow: right when collapsed, down when expanded */}
                <span className="text-gray-500">
                  <Icon
                    name={isCollapsed ? 'chevron-right' : 'chevron-down'}
                    className="h-3.5 w-3.5"
                  />
                </span>
                {/* 分类图标徽章（使用 categoryMeta 中的配色）/ Category icon badge (uses categoryMeta color scheme) */}
                <span className={`flex h-5 w-5 items-center justify-center rounded ${meta.chip}`}>
                  <Icon name={meta.icon} className="h-3 w-3" />
                </span>
                {/* 分类名称（大写、截断溢出）/ Category name (uppercase, truncate overflow) */}
                <span className="flex-1 truncate text-xs font-semibold uppercase tracking-wide text-gray-600">
                  {category}
                </span>
                {/* 该分类下接口数量徽章 / Endpoint count badge for this category */}
                <span className="rounded-full border border-gray-200 bg-white px-1.5 py-0.5 text-[10px] font-medium text-gray-500">
                  {list.length}
                </span>
              </button>

              {/* 展开时渲染该分类下的端点列表 / Render endpoint list under this category when expanded */}
              {!isCollapsed && (
                /* 端点列表容器：左侧缩进、纵向间距 2px / Endpoint list container: left indent, vertical gap 2px */
                <ul className="mt-1 space-y-0.5 pl-3">
                  {list.map((sample) => {
                    /* 判断当前端点是否为选中态（路径+方法完全匹配）/ Determine if endpoint is active (path+method exact match) */
                    const isActive =
                      selected?.path === sample.path && selected?.method === sample.method;
                    return (
                      /* 单个端点列表项，key 由 method+path 唯一标识 / Single endpoint list item, key uniquely identified by method+path */
                      <li key={`${sample.method}-${sample.path}`}>
                        {/* 端点按钮：点击触发 onSelect 回调切换主区域视图 / Endpoint button: click triggers onSelect callback to switch main area view */}
                        <button
                          onClick={() => onSelect(sample)}
                          className={[
                            'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
                            /* 选中态：靛蓝底色+粗体 / Active state: indigo bg + bold */
                            isActive
                              ? 'bg-indigo-50 font-medium text-indigo-700'
                              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900',
                          ].join(' ')}
                        >
                          {/* HTTP 方法徽章（GET=绿/POST=蓝）/ HTTP method badge (GET=green/POST=blue) */}
                          <span
                            className={`w-10 shrink-0 rounded px-1 py-0.5 text-center text-[10px] font-bold ${methodBadge(sample.method)}`}
                          >
                            {sample.method}
                          </span>
                          {/* 接口名称（溢出截断）/ Endpoint label (truncate on overflow) */}
                          <span className="flex-1 truncate">{sample.label}</span>
                          {/* 仅 REST 后端支持的接口显示 REST 标记 / Endpoints only supported by REST backend show REST tag */}
                          {sample.backend === 'rest' && (
                            <span
                              className="shrink-0 rounded bg-amber-50 px-1 py-0.5 text-[9px] font-semibold uppercase text-amber-600"
                              title="仅 Python REST 后端支持"
                            >
                              REST
                            </span>
                          )}
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          );
        })}
      </nav>
    </aside>
  );
}
