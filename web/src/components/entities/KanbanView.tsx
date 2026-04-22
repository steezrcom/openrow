import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  DndContext,
  PointerSensor,
  useDroppable,
  useDraggable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { api, type Entity, type Field } from '@/lib/api'
import { formatCell } from '@/lib/format'
import { cn } from '@/lib/utils'

type Row = Record<string, unknown>

interface KanbanConfig {
  group_by?: string
  title_field?: string
  summary_fields?: string[]
}

export function KanbanView({
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
  const cfg = config as KanbanConfig
  const groupField = entity.fields.find((f) => f.name === cfg.group_by) ?? null
  const titleField = pickTitleField(entity, cfg.title_field)
  const summaryFields = pickSummaryFields(entity, cfg.summary_fields, titleField, groupField)
  const qc = useQueryClient()

  const move = useMutation({
    mutationFn: async ({ id, value }: { id: string; value: string | null }) => {
      if (!groupField) return
      const values: Record<string, string> = {
        [groupField.name]: value ?? '',
      }
      await api.updateRow(entity.name, id, values)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rows', entity.name] })
    },
  })

  // Local-optimistic group assignment so drops feel instant.
  const [override, setOverride] = useState<Record<string, string | null>>({})
  function currentGroup(row: Row): string | null {
    const id = String(row.id)
    if (id in override) return override[id]
    if (!groupField) return null
    const v = row[groupField.name]
    return v == null || v === '' ? null : String(v)
  }

  // For reference group fields, labels come from refLookup; for text we
  // use the raw value. Columns are the union of distinct values currently
  // present in rows + any explicitly-declared columns (config.columns —
  // not wired yet, can come in a follow-up).
  const groupOptions = useMemo(() => {
    if (!groupField) return [] as string[]
    const s = new Set<string>()
    for (const r of rows) {
      const v = currentGroup(r)
      if (v != null) s.add(v)
    }
    return Array.from(s)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, groupField, override])

  const columns: { key: string; label: string }[] = useMemo(() => {
    const cols: { key: string; label: string }[] = [{ key: '__none__', label: '(no value)' }]
    for (const v of groupOptions) {
      cols.push({ key: v, label: columnLabel(groupField, v, refLookup) })
    }
    return cols
  }, [groupOptions, groupField, refLookup])

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  if (!groupField) {
    return (
      <div className="p-8 text-sm text-muted-foreground">
        Kanban needs a <code>group_by</code> field configured. Edit the view and pick one.
      </div>
    )
  }

  function onDragEnd(e: DragEndEvent) {
    const rowId = String(e.active.id)
    const destKey = e.over ? String(e.over.id) : ''
    if (!destKey || destKey === '') return
    const nextValue = destKey === '__none__' ? null : destKey
    setOverride((o) => ({ ...o, [rowId]: nextValue }))
    move.mutate({ id: rowId, value: nextValue })
  }

  return (
    <DndContext sensors={sensors} onDragEnd={onDragEnd}>
      <div className="flex h-full gap-3 overflow-x-auto p-6">
        {columns.map((col) => (
          <Column
            key={col.key}
            colKey={col.key}
            label={col.label}
            rows={rows.filter((r) => (currentGroup(r) ?? '__none__') === col.key)}
            titleField={titleField}
            summaryFields={summaryFields}
            refLookup={refLookup}
            onOpen={onOpen}
          />
        ))}
      </div>
    </DndContext>
  )
}

function Column({
  colKey,
  label,
  rows,
  titleField,
  summaryFields,
  refLookup,
  onOpen,
}: {
  colKey: string
  label: string
  rows: Row[]
  titleField: Field | null
  summaryFields: Field[]
  refLookup: (entityName: string, id: string) => string | null
  onOpen: (row: Row) => void
}) {
  const { isOver, setNodeRef } = useDroppable({ id: colKey })
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'flex w-72 shrink-0 flex-col rounded-md border border-border bg-muted/10 transition-colors',
        isOver && 'border-primary/50 bg-primary/5'
      )}
    >
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="truncate text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </span>
        <span className="text-[10px] text-muted-foreground/70">{rows.length}</span>
      </div>
      <div className="flex-1 space-y-2 overflow-y-auto p-2">
        {rows.map((row) => (
          <DraggableCard
            key={String(row.id)}
            row={row}
            titleField={titleField}
            summaryFields={summaryFields}
            refLookup={refLookup}
            onOpen={onOpen}
          />
        ))}
      </div>
    </div>
  )
}

function DraggableCard({
  row,
  titleField,
  summaryFields,
  refLookup,
  onOpen,
}: {
  row: Row
  titleField: Field | null
  summaryFields: Field[]
  refLookup: (entityName: string, id: string) => string | null
  onOpen: (row: Row) => void
}) {
  const id = String(row.id)
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id })
  const style = transform
    ? { transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` }
    : undefined
  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      onClick={(e) => {
        // dnd-kit fires click through despite drag activation constraint —
        // suppress clicks that follow a drag by checking movement on the
        // event's type. For simplicity, open on click when not dragging.
        if (!isDragging) onOpen(row)
        e.stopPropagation()
      }}
      className={cn(
        'cursor-grab rounded-md border border-border bg-card p-3 text-left shadow-sm',
        isDragging && 'opacity-60'
      )}
    >
      <div className="truncate text-sm font-medium">
        {titleField ? formatCell(row[titleField.name], titleField, refLookup) || '—' : id.slice(0, 8)}
      </div>
      <dl className="mt-1.5 space-y-0.5 text-xs">
        {summaryFields.map((f) => (
          <div key={f.name} className="flex items-baseline gap-2">
            <dt className="shrink-0 text-muted-foreground">{f.display_name}</dt>
            <dd className="min-w-0 flex-1 truncate">
              {formatCell(row[f.name], f, refLookup) || (
                <span className="text-muted-foreground/60">—</span>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function pickTitleField(entity: Entity, preferred?: string): Field | null {
  if (preferred) {
    const f = entity.fields.find((f) => f.name === preferred)
    if (f) return f
  }
  for (const candidate of ['name', 'title', 'label']) {
    const f = entity.fields.find((f) => f.name === candidate)
    if (f) return f
  }
  return entity.fields.find((f) => f.data_type === 'text') ?? entity.fields[0] ?? null
}

function pickSummaryFields(
  entity: Entity,
  configured: string[] | undefined,
  title: Field | null,
  group: Field | null
): Field[] {
  if (configured && configured.length > 0) {
    const byName = new Map(entity.fields.map((f) => [f.name, f] as const))
    return configured.map((n) => byName.get(n)).filter((f): f is Field => Boolean(f))
  }
  return entity.fields
    .filter((f) => f.name !== title?.name && f.name !== group?.name && f.data_type !== 'jsonb')
    .slice(0, 3)
}

function columnLabel(
  group: Field | null,
  value: string,
  refLookup: (entityName: string, id: string) => string | null
): string {
  if (!group) return value
  if (group.data_type === 'reference' && group.reference_entity) {
    return refLookup(group.reference_entity, value) ?? value
  }
  return value
}
