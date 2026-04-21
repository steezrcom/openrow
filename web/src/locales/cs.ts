import type { en } from './en'

export const cs: Partial<typeof en> = {
  'nav.home': 'Domů',
  'nav.timesheet': 'Výkaz práce',
  'nav.settings': 'Nastavení',
  'nav.dashboards': 'Nástěnky',
  'nav.entities': 'Entity',
  'nav.newEntity': 'Nová entita',
  'nav.newDashboard': 'Nová nástěnka',
  'nav.logout': 'Odhlásit se',

  'timer.start': 'Spustit stopky',
  'timer.running': 'Stopky běží',
  'timer.startTimer': 'Spustit stopky',

  'timesheet.title': 'Týden',
  'timesheet.logged': 'h zapsáno tento týden',
  'timesheet.thisWeek': 'Tento týden',
  'timesheet.project': 'Projekt',
  'timesheet.totals': 'Celkem',
  'timesheet.empty': 'Tento týden nic nenasbíráno. Spusť stopky nebo přidej záznam ručně.',
  'timesheet.hoursUnit': 'h',

  'day.mon': 'Po',
  'day.tue': 'Út',
  'day.wed': 'St',
  'day.thu': 'Čt',
  'day.fri': 'Pá',
  'day.sat': 'So',
  'day.sun': 'Ne',

  'settings.title': 'Nastavení',
  'settings.preferences': 'Předvolby',
  'settings.llm': 'LLM',
  'settings.language': 'Jazyk',
  'settings.language.hint': 'Použije se pro rozhraní aplikace. Názvy entit zůstanou dle konfigurace workspace.',
  'settings.theme': 'Vzhled',
  'settings.theme.light': 'Světlý',
  'settings.theme.dark': 'Tmavý',
  'settings.theme.system': 'Dle systému',

  'common.save': 'Uložit',
  'common.cancel': 'Zrušit',
  'common.delete': 'Smazat',
  'common.create': 'Vytvořit',
  'common.loading': 'Načítám…',
}
