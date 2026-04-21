export const en = {
  'nav.home': 'Home',
  'nav.timesheet': 'Timesheet',
  'nav.settings': 'Settings',
  'nav.dashboards': 'Dashboards',
  'nav.entities': 'Entities',
  'nav.newEntity': 'New entity',
  'nav.newDashboard': 'New dashboard',
  'nav.logout': 'Log out',

  'timer.start': 'Track time',
  'timer.running': 'Timer running',
  'timer.startTimer': 'Start timer',

  'timesheet.title': 'Week of',
  'timesheet.logged': 'logged this week',
  'timesheet.thisWeek': 'This week',
  'timesheet.project': 'Project',
  'timesheet.totals': 'Totals',
  'timesheet.empty': 'No time logged this week. Start the timer or add entries by hand.',
  'timesheet.hoursUnit': 'h',

  'day.mon': 'Mon',
  'day.tue': 'Tue',
  'day.wed': 'Wed',
  'day.thu': 'Thu',
  'day.fri': 'Fri',
  'day.sat': 'Sat',
  'day.sun': 'Sun',

  'settings.title': 'Settings',
  'settings.preferences': 'Preferences',
  'settings.llm': 'LLM',
  'settings.language': 'Language',
  'settings.language.hint': 'Applies to the app UI. Entity names keep their tenant-configured labels.',
  'settings.theme': 'Theme',
  'settings.theme.light': 'Light',
  'settings.theme.dark': 'Dark',
  'settings.theme.system': 'System',

  'common.save': 'Save',
  'common.cancel': 'Cancel',
  'common.delete': 'Delete',
  'common.create': 'Create',
  'common.loading': 'Loading…',
}

export type TranslationKey = keyof typeof en
