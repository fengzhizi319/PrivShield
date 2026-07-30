/**
 * 顶部导航栏：品牌标识 + Agent 健康状态灯 + 语言切换 + 后端切换器。
 *
 * 点击品牌区可返回总览页；HealthPill 实时反映 agent 连通性。
 */
import type { ConsoleHealth } from '@/types/api';
import type { BackendOption } from '@/components/BackendSelector';
import BackendSelector from '@/components/BackendSelector';
import { Icon } from '@/components/icons';
import { useI18n } from '@/i18n';

interface HeaderProps {
  backend: BackendOption;
  onBackendChange: (option: BackendOption) => void;
  health: ConsoleHealth | null;
  loading: boolean;
  /** 点击 logo 返回总览页 */
  onHome?: () => void;
}

/** 健康状态徽章：绿色圆点表示正常，红色表示不可达。
 *
 * 同时展示后端与 agent 的通信协议（REST / gRPC），
 * 切换 Python REST / Go gRPC 后该标识随之变化，可直观验证切换生效。 */
function HealthPill({ health, loading }: { health: ConsoleHealth | null; loading: boolean }) {
  const { t } = useI18n();
  if (loading && !health) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs font-medium text-gray-500">
        <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-gray-400" />
        {t('header.detecting')}
      </span>
    );
  }
  if (!health) return null;

  const ok = !health.error;
  return (
    <span
      className={[
        'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium',
        ok ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700',
      ].join(' ')}
      title={`${health.agent_url}${health.protocol ? ` · ${health.protocol}` : ''}`}
    >
      <span className={['h-1.5 w-1.5 rounded-full', ok ? 'bg-emerald-500' : 'bg-red-500'].join(' ')} />
      {ok ? t('header.agent_ok') : t('header.agent_down')}
      {health.protocol && (
        <span className="ml-0.5 rounded bg-white/60 px-1 py-px text-[10px] font-semibold">
          {health.protocol}
        </span>
      )}
    </span>
  );
}

/** Language toggle button: switches between zh and en. */
function LangSwitch() {
  const { lang, setLang } = useI18n();
  return (
    <button
      onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')}
      className="inline-flex items-center gap-1 rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:border-gray-300 hover:bg-gray-100"
      title={lang === 'zh' ? 'Switch to English' : '切换为中文'}
    >
      <Icon name="globe" className="h-3.5 w-3.5" />
      {lang === 'zh' ? 'EN' : '中'}
    </button>
  );
}

export default function Header({ backend, onBackendChange, health, loading, onHome }: HeaderProps) {
  const { t } = useI18n();
  return (
    <header className="flex h-14 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-4">
      <button
        onClick={onHome}
        className="group flex items-center gap-3 rounded-lg px-1 py-1 text-left transition-colors hover:bg-gray-50"
        title={t('header.back_home')}
      >
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600 text-white shadow-sm transition-colors group-hover:bg-indigo-700">
          <Icon name="shield" className="h-5 w-5" />
        </div>
        <div className="leading-tight">
          <div className="text-sm font-semibold text-gray-900">Privacy Test Console</div>
          <div className="text-[11px] text-gray-400">privacy-local-agent</div>
        </div>
      </button>

      <div className="flex items-center gap-3">
        <HealthPill health={health} loading={loading} />
        <LangSwitch />
        <BackendSelector value={backend} onChange={onBackendChange} />
      </div>
    </header>
  );
}
