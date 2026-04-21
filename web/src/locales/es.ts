import type { en } from './en'

export const es: Partial<typeof en> = {
  'nav.home': 'Inicio',
  'nav.timesheet': 'Horas',
  'nav.settings': 'Ajustes',
  'nav.dashboards': 'Paneles',
  'nav.entities': 'Entidades',
  'nav.newEntity': 'Nueva entidad',
  'nav.newDashboard': 'Nuevo panel',
  'nav.logout': 'Cerrar sesión',

  'timer.start': 'Registrar tiempo',
  'timer.running': 'Cronómetro en curso',
  'timer.startTimer': 'Iniciar cronómetro',

  'timesheet.title': 'Semana del',
  'timesheet.logged': 'h registradas esta semana',
  'timesheet.thisWeek': 'Esta semana',
  'timesheet.project': 'Proyecto',
  'timesheet.totals': 'Total',
  'timesheet.empty': 'Sin tiempo registrado esta semana. Inicia el cronómetro o añade entradas a mano.',
  'timesheet.hoursUnit': 'h',

  'day.mon': 'Lu',
  'day.tue': 'Ma',
  'day.wed': 'Mi',
  'day.thu': 'Ju',
  'day.fri': 'Vi',
  'day.sat': 'Sá',
  'day.sun': 'Do',

  'settings.title': 'Ajustes',
  'settings.preferences': 'Preferencias',
  'settings.llm': 'LLM',
  'settings.language': 'Idioma',
  'settings.language.hint': 'Se aplica a la interfaz. Los nombres de entidades conservan su etiqueta del espacio de trabajo.',
  'settings.theme': 'Tema',
  'settings.theme.light': 'Claro',
  'settings.theme.dark': 'Oscuro',
  'settings.theme.system': 'Sistema',

  'common.save': 'Guardar',
  'common.cancel': 'Cancelar',
  'common.delete': 'Eliminar',
  'common.create': 'Crear',
  'common.loading': 'Cargando…',
}
