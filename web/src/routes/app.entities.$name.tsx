import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useMemo, useState } from 'react'
import {
  ChevronRight,
  Plus,
  Search,
  Trash2,
  ListTree,
} from 'lucide-react'
import {
  api,
  ApiError,
  type Entity,
  type Field,
  type RefOption,
} from '@/lib/api'
import { Button, Card, Input, Pill } from '@/components/ui'
import { Drawer } from '@/components/Drawer'
import { FieldInput } from '@/components/FieldInput'
import { buildRefLookup, formatCell, formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/entities/$name')({
  component: EntityDetail,
})

type Mode =
  | { kind: 'closed' }
  | { kind: 'create' }
  | { kind: 'view'; row: Record<string, unknown> }

function EntityDetail() {
  const { name } = Route.useParams()
  const rows = useQuery({
    queryKey: ['rows', name],
    queryFn: () => api.listRows(name),
  })

  const [mode, setMode] = useState<Mode>({ kind: 'closed' })
  const [search, setSearch] = useState('')

  const refOptions = rows.data?.ref_options ?? {}
  const entity = rows.data?.entity
  const allRows = rows.data?.rows ?? []
  const refLookup = useMemo(() => buildRefLookup(refOptions), [refOptions])

  const filteredRows = useMemo(() => {
    if (!search.trim() || !entity) return allRows
    const q = search.toLowerCase()
    return allRows.filter((row) =>
      entity.fields.some((f) => {
        const v = row[f.name]
        if (v == null) return false
        return String(v).toLowerCase().includes(q)
      })
    )
  }, [allRows, entity, search])

  if (rows.isLoading)
    return (
      <div className="px-8 py-10">
        <div className="mx-auto max-w-6xl space-y-6">
          <div className="h-8 w-64 animate-pulse rounded-md bg-muted/30" />
          <div className="h-48 animate-pulse rounded-md bg-muted/30" />
        </div>
      </div>
    )
  if (rows.error) {
    return (
      <div className="mx-auto max-w-6xl px-8 py-10">
        <Card className="border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">
          {rows.error instanceof Error ? rows.error.message : 'Failed to load'}
        </Card>
      </div>
    )
  }
  if (!rows.data || !entity) return null

  return (
    <div className="flex h-screen flex-col">
      <TitleBar
        entity={entity}
        rowCount={allRows.length}
        onAdd={() => setMode({ kind: 'create' })}
      />
      <Toolbar search={search} onSearch={setSearch} filtered={filteredRows.length} total={allRows.length} />
      <div className="flex-1 overflow-auto">
        <RowsTable
          entity={entity}
          rows={filteredRows}
          refLookup={refLookup}
          onOpen={(row) => setMode({ kind: 'view', row })}
        />
      </div>

      <RecordDrawer
        open={mode.kind !== 'closed'}
        mode={mode}
        entity={entity}
        refOptions={refOptions}
        refLookup={refLookup}
        onClose={() => setMode({ kind: 'closed' })}
      />
    </div>
  )
}

function TitleBar({
  entity,
  rowCount,
  onAdd,
}: {
  entity: Entity
  rowCount: number
  onAdd: () => void
}) {
  return (
    <header className="flex items-start justify-between gap-6 border-b border-border px-8 py-5">
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">Home</Link>
          <ChevronRight className="h-3 w-3" />
          <span>Entities</span>
        </div>
        <div className="mt-2 flex items-center gap-3">
          <h1 className="truncate text-xl font-semibold">{entity.display_name}</h1>
          <Pill>{entity.name}</Pill>
          <span className="text-xs text-muted-foreground">
            {rowCount} {rowCount === 1 ? 'record' : 'records'}
          </span>
        </div>
        {entity.description && (
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{entity.description}</p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <SchemaHint entity={entity} />
        <Button onClick={onAdd}>
          <Plus className="mr-1 h-4 w-4" /> Add record
        </Button>
      </div>
    </header>
  )
}

function SchemaHint({ entity }: { entity: Entity }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button variant="ghost" onClick={() => setOpen(true)}>
        <ListTree className="mr-1 h-4 w-4" />
        Schema
      </Button>
      <Drawer
        open={open}
        onClose={() => setOpen(false)}
        title={entity.display_name}
        subtitle="Fields"
        widthClass="w-[460px]"
      >
        <div className="space-y-1">
          {entity.fields.map((f) => (
            <div
              key={f.id}
              className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-accent"
            >
              <div className="min-w-0">
                <p className="truncate text-sm">{f.display_name}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">{f.name}</p>
              </div>
              <div className="flex items-center gap-1.5">
                {f.is_required && (
                  <span className="rounded border border-destructive/30 bg-destructive/5 px-1.5 py-0.5 text-[10px] text-destructive/90">req</span>
                )}
                {f.is_unique && (
                  <span className="rounded border border-border bg-muted/40 px-1.5 py-0.5 text-[10px] text-muted-foreground">unique</span>
                )}
                <Pill>{f.data_type}</Pill>
              </div>
            </div>
          ))}
        </div>
      </Drawer>
    </>
  )
}

function Toolbar({
  search,
  onSearch,
  filtered,
  total,
}: {
  search: string
  onSearch: (s: string) => void
  filtered: number
  total: number
}) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border bg-muted/10 px-8 py-3">
      <div className="relative w-80 max-w-full">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="h-9 pl-9"
          placeholder="Search in this table…"
          value={search}
          onChange={(e) => onSearch(e.target.value)}
        />
      </div>
      {search && (
        <span className="text-xs text-muted-foreground">
          Showing {filtered} of {total}
        </span>
      )}
    </div>
  )
}

function RowsTable({
  entity,
  rows,
  refLookup,
  onOpen,
}: {
  entity: Entity
  rows: Record<string, unknown>[]
  refLookup: (entityName: string, id: string) => string | null
  onOpen: (row: Record<string, unknown>) => void
}) {
  if (rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="max-w-sm text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-lg bg-muted/40 text-muted-foreground">
            <ListTree className="h-5 w-5" />
          </div>
          <h3 className="mt-4 font-medium">No records yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Add your first {entity.display_name.toLowerCase()} to get started.
          </p>
        </div>
      </div>
    )
  }

  return (
    <table className="min-w-full text-sm">
      <thead className="sticky top-0 z-10 bg-card/95 backdrop-blur">
        <tr className="border-b border-border text-left">
          {entity.fields.map((f) => (
            <th
              key={f.id}
              className={cn(
                'whitespace-nowrap px-4 py-2.5 text-xs font-medium uppercase tracking-wider text-muted-foreground',
                isNumericType(f.data_type) && 'text-right'
              )}
            >
              {f.display_name}
            </th>
          ))}
          <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium uppercase tracking-wider text-muted-foreground">
            Updated
          </th>
          <th className="w-10 px-2" />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => {
          const id = String(row.id ?? '')
          return (
            <RowLine
              key={id}
              id={id}
              row={row}
              entity={entity}
              refLookup={refLookup}
              onOpen={() => onOpen(row)}
            />
          )
        })}
      </tbody>
    </table>
  )
}

function RowLine({
  id,
  row,
  entity,
  refLookup,
  onOpen,
}: {
  id: string
  row: Record<string, unknown>
  entity: Entity
  refLookup: (entityName: string, id: string) => string | null
  onOpen: () => void
}) {
  const qc = useQueryClient()
  const del = useMutation({
    mutationFn: () => api.deleteRow(entity.name, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['rows', entity.name] }),
  })

  return (
    <tr
      onClick={onOpen}
      className="group cursor-pointer border-b border-border/60 transition-colors hover:bg-accent/50"
    >
      {entity.fields.map((f) => (
        <td
          key={f.id}
          className={cn(
            'max-w-[280px] truncate px-4 py-2.5',
            isNumericType(f.data_type) && 'text-right font-mono',
            f.data_type === 'boolean' && 'text-center'
          )}
        >
          {renderCell(row[f.name], f, refLookup)}
        </td>
      ))}
      <td className="whitespace-nowrap px-4 py-2.5 text-xs text-muted-foreground">
        {formatTimestampRelative(row.updated_at)}
      </td>
      <td className="px-2 py-2.5 text-right">
        <button
          onClick={(e) => {
            e.stopPropagation()
            if (confirm('Delete this record?')) del.mutate()
          }}
          className="invisible rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive group-hover:visible"
          title="Delete"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
      </td>
    </tr>
  )
}

function renderCell(
  value: unknown,
  field: Field,
  refLookup: (entityName: string, id: string) => string | null
) {
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground/40">—</span>
  }
  if (field.data_type === 'boolean') {
    return value ? <span className="text-primary">✓</span> : <span className="text-muted-foreground/40">—</span>
  }
  if (field.data_type === 'reference' && field.reference_entity && typeof value === 'string') {
    const label = refLookup(field.reference_entity, value)
    return (
      <span className="inline-flex items-center rounded-md bg-primary/10 px-2 py-0.5 text-xs text-primary">
        {label ?? value.slice(0, 8)}
      </span>
    )
  }
  return formatCell(value, field, refLookup)
}

function isNumericType(t: string): boolean {
  return t === 'integer' || t === 'bigint' || t === 'numeric'
}

function RecordDrawer({
  open,
  mode,
  entity,
  refOptions,
  refLookup,
  onClose,
}: {
  open: boolean
  mode: Mode
  entity: Entity
  refOptions: Record<string, RefOption[]>
  refLookup: (entityName: string, id: string) => string | null
  onClose: () => void
}) {
  if (mode.kind === 'create') {
    return (
      <Drawer
        open={open}
        onClose={onClose}
        title={`Add ${entity.display_name.toLowerCase()}`}
        subtitle={entity.name}
      >
        <CreateForm entity={entity} refOptions={refOptions} onDone={onClose} />
      </Drawer>
    )
  }
  if (mode.kind === 'view') {
    return (
      <Drawer
        open={open}
        onClose={onClose}
        title={
          <span className="font-mono">
            {String(mode.row.id ?? '').slice(0, 8)}
          </span>
        }
        subtitle={entity.display_name}
      >
        <ViewRecord row={mode.row} entity={entity} refLookup={refLookup} />
      </Drawer>
    )
  }
  return null
}

function CreateForm({
  entity,
  refOptions,
  onDone,
}: {
  entity: Entity
  refOptions: Record<string, RefOption[]>
  onDone: () => void
}) {
  const qc = useQueryClient()
  const { register, handleSubmit, formState: { isSubmitting } } =
    useForm<Record<string, string>>()
  const [error, setError] = useState<string | null>(null)

  const mut = useMutation({
    mutationFn: (values: Record<string, string>) => api.createRow(entity.name, values),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['rows', entity.name] })
      onDone()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <form
      id="create-record-form"
      className="space-y-5"
      onSubmit={handleSubmit((v) => {
        const filtered = Object.fromEntries(Object.entries(v).filter(([, val]) => val !== ''))
        mut.mutate(filtered)
      })}
    >
      {entity.fields.map((f) => (
        <FieldInput
          key={f.id}
          field={f}
          register={register}
          refOptions={refOptions[f.name] ?? []}
        />
      ))}
      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </div>
      )}
      <div className="sticky bottom-0 flex items-center gap-2 border-t border-border bg-card/95 px-0 py-3 backdrop-blur">
        <Button type="submit" disabled={isSubmitting || mut.isPending}>
          {mut.isPending ? 'Saving…' : 'Save record'}
        </Button>
      </div>
    </form>
  )
}

function ViewRecord({
  row,
  entity,
  refLookup,
}: {
  row: Record<string, unknown>
  entity: Entity
  refLookup: (entityName: string, id: string) => string | null
}) {
  return (
    <dl className="space-y-4">
      {entity.fields.map((f) => {
        const value = row[f.name]
        return (
          <div key={f.id} className="grid grid-cols-[140px_1fr] gap-4">
            <dt className="truncate text-xs text-muted-foreground">{f.display_name}</dt>
            <dd className="break-words text-sm">
              {value == null || value === '' ? (
                <span className="text-muted-foreground/40">—</span>
              ) : f.data_type === 'boolean' ? (
                value ? 'yes' : 'no'
              ) : f.data_type === 'reference' && typeof value === 'string' && f.reference_entity ? (
                <span className="inline-flex items-center rounded-md bg-primary/10 px-2 py-0.5 text-xs text-primary">
                  {refLookup(f.reference_entity, value) ?? value}
                </span>
              ) : f.data_type === 'jsonb' ? (
                <pre className="whitespace-pre-wrap rounded-md bg-muted/40 p-2 font-mono text-[11px]">
                  {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
                </pre>
              ) : (
                formatCell(value, f, refLookup)
              )}
            </dd>
          </div>
        )
      })}
      <div className="grid grid-cols-[140px_1fr] gap-4 border-t border-border pt-4 text-xs text-muted-foreground">
        <dt>id</dt>
        <dd className="break-all font-mono">{String(row.id ?? '')}</dd>
        <dt>created</dt>
        <dd>{formatCell(row.created_at, undefined, refLookup)}</dd>
        <dt>updated</dt>
        <dd>{formatCell(row.updated_at, undefined, refLookup)}</dd>
      </div>
    </dl>
  )
}
