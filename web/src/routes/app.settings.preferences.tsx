import { createFileRoute, Link } from '@tanstack/react-router'
import { ChevronRight, Monitor, Moon, Sun } from 'lucide-react'
import { Card } from '@/components/ui'
import { SettingsTabs } from '@/components/SettingsTabs'
import { useTheme, type Theme } from '@/lib/theme'
import { useLocaleStore, useT, LOCALES, type Locale } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/preferences')({
  component: PreferencesPage,
})

function PreferencesPage() {
  const t = useT()
  const theme = useTheme((s) => s.theme)
  const setTheme = useTheme((s) => s.setTheme)
  const locale = useLocaleStore((s) => s.locale)
  const setLocale = useLocaleStore((s) => s.setLocale)

  const themes: { id: Theme; label: string; icon: React.ReactNode }[] = [
    { id: 'light', label: t('settings.theme.light'), icon: <Sun className="h-4 w-4" /> },
    { id: 'dark', label: t('settings.theme.dark'), icon: <Moon className="h-4 w-4" /> },
    { id: 'system', label: t('settings.theme.system'), icon: <Monitor className="h-4 w-4" /> },
  ]

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {t('settings.title')}
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {t('settings.preferences')}
        </p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">{t('settings.preferences')}</h1>
      </header>

      <SettingsTabs active="preferences" />

      <Card className="p-6 space-y-8">
        <section className="space-y-3">
          <h2 className="text-sm font-medium">{t('settings.theme')}</h2>
          <div className="grid grid-cols-3 gap-2">
            {themes.map((opt) => (
              <button
                key={opt.id}
                type="button"
                onClick={() => setTheme(opt.id)}
                className={cn(
                  'flex items-center gap-2 rounded-md border border-border bg-card p-3 text-sm transition-colors hover:bg-accent',
                  theme === opt.id && 'border-primary ring-2 ring-primary/30'
                )}
              >
                {opt.icon}
                {opt.label}
              </button>
            ))}
          </div>
        </section>

        <section className="space-y-3">
          <h2 className="text-sm font-medium">{t('settings.language')}</h2>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {LOCALES.map((l) => (
              <button
                key={l.id}
                type="button"
                onClick={() => setLocale(l.id as Locale)}
                className={cn(
                  'rounded-md border border-border bg-card p-3 text-sm transition-colors hover:bg-accent',
                  locale === l.id && 'border-primary ring-2 ring-primary/30'
                )}
              >
                {l.label}
              </button>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">{t('settings.language.hint')}</p>
        </section>
      </Card>
    </div>
  )
}
