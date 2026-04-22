import { Link } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n'

type TabID = 'preferences' | 'llm' | 'connectors'
type TabPath = '/app/settings/preferences' | '/app/settings/llm' | '/app/settings/connectors'

const TAB_LABEL_KEY: Record<TabID, 'settings.preferences' | 'settings.llm' | 'settings.connectors'> = {
  preferences: 'settings.preferences',
  llm: 'settings.llm',
  connectors: 'settings.connectors',
}

const TABS: { id: TabID; to: TabPath }[] = [
  { id: 'preferences', to: '/app/settings/preferences' },
  { id: 'connectors', to: '/app/settings/connectors' },
  { id: 'llm', to: '/app/settings/llm' },
]

export function SettingsShell({
  active,
  hint,
  children,
}: {
  active: TabID
  hint?: string
  children: React.ReactNode
}) {
  const t = useT()
  const title = t(TAB_LABEL_KEY[active])
  return (
    <div className="mx-auto max-w-4xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {t('settings.title')}
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {title}
        </p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">{title}</h1>
        {/* min-h reserves one line so tabs don't jump when some tabs omit a hint. */}
        <p className="mt-1 min-h-5 text-sm text-muted-foreground">{hint}</p>
      </header>

      <div className="mb-6 flex items-center gap-1 border-b border-border">
        {TABS.map((tab) => (
          <Link
            key={tab.id}
            to={tab.to}
            className={cn(
              '-mb-px border-b-2 px-3 py-2 text-sm transition-colors',
              active === tab.id
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t(TAB_LABEL_KEY[tab.id])}
          </Link>
        ))}
      </div>

      {children}
    </div>
  )
}
