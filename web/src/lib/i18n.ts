import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { en, type TranslationKey } from '@/locales/en'
import { cs } from '@/locales/cs'
import { de } from '@/locales/de'
import { es } from '@/locales/es'

export type Locale = 'en' | 'cs' | 'de' | 'es'

export const LOCALES: { id: Locale; label: string }[] = [
  { id: 'en', label: 'English' },
  { id: 'cs', label: 'Čeština' },
  { id: 'de', label: 'Deutsch' },
  { id: 'es', label: 'Español' },
]

const DICTS: Record<Locale, Partial<typeof en>> = { en, cs, de, es }

interface LocaleState {
  locale: Locale
  setLocale: (l: Locale) => void
}

export const useLocaleStore = create<LocaleState>()(
  persist(
    (set) => ({
      locale: detectBrowserLocale(),
      setLocale: (locale) => set({ locale }),
    }),
    { name: 'openrow.locale', version: 1 }
  )
)

export function useT() {
  const locale = useLocaleStore((s) => s.locale)
  return (key: TranslationKey) => DICTS[locale][key] ?? en[key]
}

function detectBrowserLocale(): Locale {
  const tag = (navigator.language || 'en').slice(0, 2).toLowerCase()
  if (tag === 'cs' || tag === 'de' || tag === 'es') return tag
  return 'en'
}
