import { Link } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n'

export function SettingsTabs({ active }: { active: 'preferences' | 'llm' }) {
  const t = useT()
  const tabs: { id: 'preferences' | 'llm'; label: string; to: '/app/settings/preferences' | '/app/settings/llm' }[] = [
    { id: 'preferences', label: t('settings.preferences'), to: '/app/settings/preferences' },
    { id: 'llm', label: t('settings.llm'), to: '/app/settings/llm' },
  ]
  return (
    <div className="mb-6 flex items-center gap-1 border-b border-border">
      {tabs.map((tab) => (
        <Link
          key={tab.id}
          to={tab.to}
          className={cn(
            '-mb-px border-b-2 px-3 py-2 text-sm transition-colors',
            active === tab.id
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground'
          )}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  )
}
