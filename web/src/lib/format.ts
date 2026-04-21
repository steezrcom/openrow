import type { DataType, Field, RefOption } from '@/lib/api'

const dtFormatter = new Intl.DateTimeFormat('en-CA', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})

const relative = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

export function formatCell(
  value: unknown,
  field: Field | undefined,
  refLookup?: (entityName: string, id: string) => string | null
): string {
  if (value === null || value === undefined) return ''
  const type = field?.data_type as DataType | undefined

  if (type === 'boolean') return value ? '✓' : ''
  if (type === 'reference' && field?.reference_entity && typeof value === 'string' && refLookup) {
    return refLookup(field.reference_entity, value) ?? value
  }
  if (type === 'date' && typeof value === 'string') return value
  if (type === 'timestamptz' && typeof value === 'string') {
    const d = new Date(value)
    if (!isNaN(d.getTime())) return dtFormatter.format(d)
  }
  if ((type === 'numeric' || type === 'integer' || type === 'bigint') && (typeof value === 'number' || typeof value === 'string')) {
    return String(value)
  }
  if (type === 'jsonb') {
    try {
      return typeof value === 'string' ? value : JSON.stringify(value)
    } catch {
      return String(value)
    }
  }
  if (typeof value === 'string') return value
  if (value instanceof Date) return dtFormatter.format(value)
  return String(value)
}

export function formatTimestampRelative(value: unknown): string {
  if (typeof value !== 'string') return ''
  const d = new Date(value)
  if (isNaN(d.getTime())) return ''
  const diffSec = Math.round((d.getTime() - Date.now()) / 1000)
  const abs = Math.abs(diffSec)
  if (abs < 60) return relative.format(diffSec, 'second')
  if (abs < 3600) return relative.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return relative.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 30 * 86400) return relative.format(Math.round(diffSec / 86400), 'day')
  return dtFormatter.format(d)
}

export function buildRefLookup(
  refOptions: Record<string, RefOption[]>
): (entityName: string, id: string) => string | null {
  const byEntity: Record<string, Map<string, string>> = {}
  for (const [entity, opts] of Object.entries(refOptions)) {
    const m = new Map<string, string>()
    for (const o of opts) m.set(o.ID, o.Label)
    byEntity[entity] = m
  }
  return (entity, id) => byEntity[entity]?.get(id) ?? null
}
