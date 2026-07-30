/**
 * 响应查看器：展示请求结果。
 *
 * 三种状态：空状态（未发送）/ 错误状态 / 成功状态；
 * 成功时对 JSON 做语法高亮，并提供复制与下载按钮。
 */
import { useState, type ReactNode } from 'react';
import type { ProxyResponse } from '@/types/api';
import { Icon } from '@/components/icons';
import { useI18n } from '@/i18n';

interface ResponsePanelProps {
  response: ProxyResponse | null;
  error: string | null;
  duration: number | null;
  /** 当前请求路径，用于下载文件命名 */
  path?: string;
}

/**
 * 轻量 JSON 语法高亮：将序列化后的 JSON 按 token 着色。
 * 不引入第三方库，满足控制台展示需求。
 */
function highlightJson(json: string): ReactNode[] {
  // 防御性安全限制：当 JSON 串长度超过 100k 字符时停止正则高亮，改用纯文本渲染，防止 ReDoS
  if (json.length > 100_000) {
    return [<span key={0}>{json}</span>];
  }
  const tokenRegex =
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g;
  const nodes: ReactNode[] = [];
  let last = 0;
  let match: RegExpExecArray | null;
  let key = 0;

  while ((match = tokenRegex.exec(json)) !== null) {
    if (match.index > last) {
      nodes.push(<span key={key++}>{json.slice(last, match.index)}</span>);
    }
    const token = match[0];
    let cls = 'text-orange-600'; // number
    if (/^"/.test(token)) {
      cls = /:$/.test(token) ? 'text-sky-700' : 'text-emerald-700'; // key : string
    } else if (/true|false/.test(token)) {
      cls = 'text-amber-600';
    } else if (/null/.test(token)) {
      cls = 'text-rose-500';
    }
    nodes.push(
      <span key={key++} className={cls}>
        {token}
      </span>,
    );
    last = match.index + token.length;
  }
  if (last < json.length) {
    nodes.push(<span key={key++}>{json.slice(last)}</span>);
  }
  return nodes;
}

/**
 * 截断响应中超长的 base64 / data URI 字符串，避免图片编码内容填满屏幕。
 *
 * 规则：
 *   - 以 data:image/ 开头的 data URI → 替换为 "[image data, ~N KB]"
 *   - 严格的纯 base64（无空格且符合 A-Za-z0-9+/=，长度 > 200）→ 替换为 "[base64 data, ~N KB]"
 *   - 其他超长字符串（> 500）→ 截断前 80 字符 + "…(N chars)"
 *   - 限制最大递归深度（depth <= 20），防止深度嵌套导致栈溢出 Crash
 */
function truncateLongStrings(obj: unknown, depth = 0): unknown {
  if (depth > 20) return obj;
  if (typeof obj === 'string') {
    // data URI 图片
    const dataUriMatch = obj.match(/^data:image\/[a-zA-Z]+;base64,/);
    if (dataUriMatch) {
      const rawLen = obj.length - dataUriMatch[0].length;
      const kb = Math.max(1, Math.round((rawLen * 3) / 4 / 1024));
      return `[image data, ~${kb} KB]`;
    }
    // 纯 base64（不含空格，连续字符长度 > 200 且符合标准 Base64 字符集）
    if (obj.length > 200 && !/\s/.test(obj) && /^[A-Za-z0-9+/=]+$/.test(obj.slice(0, 128))) {
      const kb = Math.max(1, Math.round((obj.length * 3) / 4 / 1024));
      return `[base64 data, ~${kb} KB]`;
    }
    // 其他超长字符串
    if (obj.length > 500) {
      return obj.slice(0, 80) + `…(${obj.length} chars)`;
    }
    return obj;
  }
  if (Array.isArray(obj)) {
    return obj.map((item) => truncateLongStrings(item, depth + 1));
  }
  if (obj !== null && typeof obj === 'object') {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(obj)) {
      out[k] = truncateLongStrings(v, depth + 1);
    }
    return out;
  }
  return obj;
}

/** 复制到剪贴板按钮，复制成功后短暂显示对勾。 */
function CopyButton({ text }: { text: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore clipboard unavailable */
    }
  };
  return (
    <button
      onClick={handleCopy}
      className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
      title={t('response.copy')}
    >
      <Icon name={copied ? 'check' : 'copy'} className="h-3.5 w-3.5" />
      {copied ? t('response.copied') : t('response.copy')}
    </button>
  );
}

/** 下载响应 JSON 为文件。 */
function DownloadButton({ text, path }: { text: string; path: string }) {
  const { t } = useI18n();
  const handleDownload = () => {
    const blob = new Blob([text], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const safeName = path.replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_+|_+$/g, '') || 'response';
    a.href = url;
    a.download = `${safeName}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <button
      onClick={handleDownload}
      className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700"
      title={t('response.download')}
    >
      <Icon name="download" className="h-3.5 w-3.5" />
      {t('response.download')}
    </button>
  );
}

export default function ResponsePanel({ response, error, duration, path = 'response' }: ResponsePanelProps) {
  const { t } = useI18n();
  // Empty state
  if (!response && !error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-gray-300">
        <Icon name="zap" className="h-10 w-10" strokeWidth={1.5} />
        <p className="text-sm text-gray-400">{t('response.empty')}</p>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex h-full flex-col">
        <div className="flex items-center justify-between border-b border-red-100 px-4 py-2.5">
          <span className="inline-flex items-center gap-2 text-sm font-semibold text-red-600">
            <Icon name="alert" className="h-4 w-4" />
            {t('response.failed')}
          </span>
          {duration !== null && (
            <span className="text-xs text-gray-400">{duration.toFixed(1)} ms</span>
          )}
        </div>
        <pre className="flex-1 overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-xs leading-relaxed text-red-600">
          {error}
        </pre>
      </div>
    );
  }

  // 成功状态：对响应 data 中的超长 base64 / data URI 字符串做截断处理，
  // 避免图片编码内容擑满屏幕，仅展示分级结果等有效信息。
  const jsonText = JSON.stringify(truncateLongStrings(response?.data), null, 2);
  // 后端身份标识：via 为处理请求的控制台后端，protocol 为其与 agent 的通信协议。
  // 切换 Python REST / Go gRPC 后，该徽章随之变化，可直观验证切换生效。
  const via = response?.via;
  const protocol = response?.protocol;
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-gray-100 px-4 py-2.5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-semibold text-emerald-700">
            <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
            {response?.status ?? 200}
          </span>
          {via && (
            <span
              className="inline-flex items-center rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-semibold text-indigo-700"
              title="处理本请求的控制台后端"
            >
              {via}
            </span>
          )}
          {protocol && (
            <span
              className="inline-flex items-center rounded-full bg-sky-50 px-2 py-0.5 text-xs font-semibold text-sky-700"
              title="后端与 agent 的通信协议"
            >
              {protocol}
            </span>
          )}
          <span className="text-xs text-gray-400">
            {response?.duration_ms !== undefined
              ? `${response.duration_ms.toFixed(2)} ms`
              : duration !== null
                ? `${duration.toFixed(1)} ms`
                : ''}
          </span>
        </div>
        <div className="flex items-center">
          <CopyButton text={jsonText} />
          <DownloadButton text={jsonText} path={path} />
        </div>
      </div>
      <pre className="flex-1 overflow-auto whitespace-pre-wrap break-words bg-gray-50/50 p-4 font-mono text-xs leading-relaxed text-gray-700">
        {highlightJson(jsonText)}
      </pre>
    </div>
  );
}
