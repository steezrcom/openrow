import type { Entity, Field } from '@/lib/api'
import { formatCell } from '@/lib/format'

type Row = Record<string, unknown>

// CardsView config shape: { title_field?: string, summary_fields?: string[] }.
// Defaults come from the entity schema when the config is empty.
interface CardsConfig {
  title_field?: string
  summary_fields?: string[]
}

export function CardsView({
  entity,
  rows,
  refLookup,
  config,
  onOpen,
}: {
  entity: Entity
  rows: Row[]
  refLookup: (entityName: string, id: string) => string | null
  config: Record<string, unknown>
  onOpen: (row: Row) => void
}) {
  const cfg = config as CardsConfig
  const titleField = pickTitleField(entity, cfg.title_field)
  const summaryFields = pickSummaryFields(entity, cfg.summary_fields, titleField)

  return (
    <div className="grid grid-cols-1 gap-3 p-6 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {rows.map((row) => (
        <button
          key={String(row.id)}
          onClick={() => onOpen(row)}
          className="group flex flex-col gap-1.5 rounded-md border border-border bg-card p-4 text-left transition-colors hover:bg-accent hover:border-primary/40"
        >
          <div className="truncate text-sm font-medium">
            {titleField ? formatCell(row[titleField.name], titleField, refLookup) || '—' : String(row.id).slice(0, 8)}
          </div>
          <dl className="mt-1 space-y-0.5 text-xs">
            {summaryFields.map((f) => (
              <div key={f.name} className="flex items-baseline gap-2">
                <dt className="shrink-0 text-muted-foreground">{f.display_name}</dt>
                <dd className="min-w-0 flex-1 truncate">
                  {formatCell(row[f.name], f, refLookup) || <span className="text-muted-foreground/60">—</span>}
                </dd>
              </div>
            ))}
          </dl>
        </button>
      ))}
      {rows.length === 0 && (
        <div className="col-span-full py-10 text-center text-sm text-muted-foreground">
          No records.
        </div>
      )}
    </div>
  )
}

function pickTitleField(entity: Entity, preferred?: string): Field | null {
  if (preferred) {
    const f = entity.fields.find((f) => f.name === preferred)
    if (f) return f
  }
  // Favour the conventional "name" / "title" / "label", else the first text field.
  for (const candidate of ['name', 'title', 'label']) {
    const f = entity.fields.find((f) => f.name === candidate)
    if (f) return f
  }
  return entity.fields.find((f) => f.data_type === 'text') ?? entity.fields[0] ?? null
}

function pickSummaryFields(entity: Entity, configured: string[] | undefined, title: Field | null): Field[] {
  if (configured && configured.length > 0) {
    const byName = new Map(entity.fields.map((f) => [f.name, f] as const))
    return configured.map((n) => byName.get(n)).filter((f): f is Field => Boolean(f))
  }
  // Default: up to 4 non-title fields, prefer anything non-jsonb.
  return entity.fields
    .filter((f) => f.name !== title?.name && f.data_type !== 'jsonb')
    .slice(0, 4)
}
