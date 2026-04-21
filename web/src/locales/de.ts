import type { en } from './en'

export const de: Partial<typeof en> = {
  'nav.home': 'Start',
  'nav.timesheet': 'Arbeitszeit',
  'nav.settings': 'Einstellungen',
  'nav.dashboards': 'Dashboards',
  'nav.entities': 'Entitäten',
  'nav.newEntity': 'Neue Entität',
  'nav.newDashboard': 'Neues Dashboard',
  'nav.logout': 'Abmelden',

  'timer.start': 'Zeit erfassen',
  'timer.running': 'Timer läuft',
  'timer.startTimer': 'Timer starten',

  'timesheet.title': 'Woche ab',
  'timesheet.logged': 'Std. diese Woche erfasst',
  'timesheet.thisWeek': 'Diese Woche',
  'timesheet.project': 'Projekt',
  'timesheet.totals': 'Gesamt',
  'timesheet.empty': 'Diese Woche noch keine Zeit erfasst. Starte den Timer oder füge Einträge manuell hinzu.',
  'timesheet.hoursUnit': 'Std',

  'day.mon': 'Mo',
  'day.tue': 'Di',
  'day.wed': 'Mi',
  'day.thu': 'Do',
  'day.fri': 'Fr',
  'day.sat': 'Sa',
  'day.sun': 'So',

  'settings.title': 'Einstellungen',
  'settings.preferences': 'Präferenzen',
  'settings.llm': 'LLM',
  'settings.language': 'Sprache',
  'settings.language.hint': 'Gilt für die Benutzeroberfläche. Entity-Namen behalten ihre Workspace-Beschriftung.',
  'settings.theme': 'Erscheinungsbild',
  'settings.theme.light': 'Hell',
  'settings.theme.dark': 'Dunkel',
  'settings.theme.system': 'System',

  'common.save': 'Speichern',
  'common.cancel': 'Abbrechen',
  'common.delete': 'Löschen',
  'common.create': 'Erstellen',
  'common.loading': 'Lädt…',
}
