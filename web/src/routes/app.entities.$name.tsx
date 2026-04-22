import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useMemo, useState } from 'react'
import { z } from 'zod'
import {
  ChevronLeft,
  ChevronRight,
  ChevronsUpDown,
  ArrowDown,
  ArrowUp,
  Pencil,
  Plus,
  Search,
  Trash2,
  ListTree,
} from 'lucide-react'
import {
  api,
  ApiError,
  type Entity,
  type EntityView,
  type Field,
  type RefOption,
} from '@/lib/api'
import { Button, Card, Input, Pill } from '@/components/ui'
import { Drawer } from '@/components/Drawer'
import { FieldInput } from '@/components/FieldInput'
import { ViewTabs } from '@/components/entities/ViewTabs'
import { CardsView } from '@/components/entities/CardsView'
import { KanbanView } from '@/components/entities/KanbanView'
import { NewViewModal } from '@/components/entities/NewViewModal'
import { buildRefLookup, formatCell, formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  sort: z.string().optional(),
  dir: z.enum(['asc', 'desc']).optional(),
  page: z.number().int().min(1).optional(),
  view: z.string().optional(),
})

export const Route = createFileRoute('/app/entities/$name')({
  component: EntityDetail,
  validateSearch: searchSchema,
})

const PAGE_SIZE = 50

type Mode =
  | { kind: 'closed' }
  | { kind: 'create' }
  | { kind: 'view'; row: Record<string, unknown> }
  | { kind: 'edit'; row: Record<string, unknown> }

function EntityDetail() {
  const { name } = Route.useParams()
  const searchParams = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const sort = searchParams.sort
  const dir = searchParams.dir
  const page = searchParams.page ?? 1

  const rows = useQuery({
    queryKey: ['rows', name, sort ?? '', dir ?? '', page],
    queryFn: () => api.listRows(name, { sort, dir, page, limit: PAGE_SIZE }),
  })
  const views = useQuery({
    queryKey: ['views', name],
    queryFn: () => api.listViews(name),
  })

  const [mode, setMode] = useState<Mode>({ kind: 'closed' })
  const [search, setSearch] = useState('')
  const [newViewOpen, setNewViewOpen] = useState(false)

  // "table" is the implicit default — no row in entity_views, no id.
  const activeView: EntityView | null = useMemo(() => {
    if (!searchParams.view || searchParams.view === 'table') return null
    return (views.data ?? []).find((v) => v.id === searchParams.view) ?? null
  }, [searchParams.view, views.data])

  function setSort(field: string) {
    let nextDir: 'asc' | 'desc' | undefined
    if (sort !== field) nextDir = 'asc'
    else if (dir === 'asc') nextDir = 'desc'
    else nextDir = undefined
    navigate({
      search: (prev) => ({
        ...prev,
        sort: nextDir ? field : undefined,
        dir: nextDir,
        page: 1,
      }),
    })
  }

  function setPage(next: number) {
    navigate({ search: (prev) => ({ ...prev, page: next }) })
  }

  const refOptions = rows.data?.ref_options ?? {}
  const entity = rows.data?.entity
  const allRows = rows.data?.rows ?? []
  const total = rows.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
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
        rowCount={total}
        onAdd={() => setMode({ kind: 'create' })}
      />
      <ViewTabs
        views={views.data ?? []}
        activeId={activeView?.id ?? 'table'}
        onSelect={(id) =>
          navigate({ search: (prev) => ({ ...prev, view: id === 'table' ? undefined : id, page: 1 }) })
        }
        onNew={() => setNewViewOpen(true)}
      />
      <Toolbar search={search} onSearch={setSearch} filtered={filteredRows.length} total={allRows.length} />
      <div className="flex-1 overflow-auto">
        {activeView?.view_type === 'cards' ? (
          <CardsView
            entity={entity}
            rows={filteredRows}
            refLookup={refLookup}
            config={activeView.config}
            onOpen={(row) => setMode({ kind: 'view', row })}
          />
        ) : activeView?.view_type === 'kanban' ? (
          <KanbanView
            entity={entity}
            rows={filteredRows}
            refLookup={refLookup}
            config={activeView.config}
            onOpen={(row) => setMode({ kind: 'view', row })}
          />
        ) : (
          <RowsTable
            entity={entity}
            rows={filteredRows}
            refLookup={refLookup}
            sort={sort}
            dir={dir}
            onSort={setSort}
            onOpen={(row) => setMode({ kind: 'view', row })}
          />
        )}
      </div>
      {!activeView && totalPages > 1 && (
        <Pagination page={page} totalPages={totalPages} total={total} onPage={setPage} />
      )}

      <NewViewModal
        open={newViewOpen}
        entity={entity}
        onClose={() => setNewViewOpen(false)}
        onCreated={(v) => {
          setNewViewOpen(false)
          navigate({ search: (prev) => ({ ...prev, view: v.id, page: 1 }) })
        }}
      />

      <RecordDrawer
        open={mode.kind !== 'closed'}
        mode={mode}
        entity={entity}
        refOptions={refOptions}
        refLookup={refLookup}
        onClose={() => setMode({ kind: 'closed' })}
        onEditFromView={(row) => setMode({ kind: 'edit', row })}
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
        subtitle="Schema"
        widthClass="w-[460px]"
      >
        <FieldsList entity={entity} />
        <AddFieldForm entity={entity} />
      </Drawer>
    </>
  )
}

function FieldsList({ entity }: { entity: Entity }) {
  const qc = useQueryClient()
  const drop = useMutation({
    mutationFn: (fieldName: string) => api.dropField(entity.name, fieldName),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rows', entity.name] })
      qc.invalidateQueries({ queryKey: ['entities'] })
    },
  })

  return (
    <div className="space-y-1">
      {entity.fields.map((f) => (
        <div
          key={f.id}
          className="group flex items-center justify-between rounded-md px-3 py-2 hover:bg-accent"
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
            <button
              onClick={() => {
                if (confirm(`Drop field "${f.name}"? Data in that column will be lost.`)) {
                  drop.mutate(f.name)
                }
              }}
              className="invisible rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive group-hover:visible"
              title="Drop field"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}

const DATA_TYPES: DataTypeOption[] = [
  'text', 'integer', 'bigint', 'numeric', 'boolean',
  'date', 'timestamptz', 'uuid', 'jsonb', 'reference',
]
type DataTypeOption = 'text' | 'integer' | 'bigint' | 'numeric' | 'boolean' | 'date' | 'timestamptz' | 'uuid' | 'jsonb' | 'reference'

function AddFieldForm({ entity }: { entity: Entity }) {
  const qc = useQueryClient()
  const entities = useQuery({ queryKey: ['entities'], queryFn: api.listEntities })
  const [error, setError] = useState<string | null>(null)

  type FormValues = {
    name: string
    display_name: string
    data_type: DataTypeOption
    is_required: boolean
    is_unique: boolean
    reference_entity: string
  }

  const { register, handleSubmit, watch, reset, formState: { isSubmitting } } =
    useForm<FormValues>({
      defaultValues: {
        data_type: 'text',
        is_required: false,
        is_unique: false,
      },
    })
  const currentType = watch('data_type')

  const mut = useMutation({
    mutationFn: (v: FormValues) =>
      api.addField(entity.name, {
        name: v.name,
        display_name: v.display_name,
        data_type: v.data_type,
        is_required: v.is_required,
        is_unique: v.is_unique,
        reference_entity: v.data_type === 'reference' ? v.reference_entity : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['rows', entity.name] })
      qc.invalidateQueries({ queryKey: ['entities'] })
      reset({
        name: '',
        display_name: '',
        data_type: 'text',
        is_required: false,
        is_unique: false,
        reference_entity: '',
      })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  const selectClass =
    'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm'

  return (
    <form
      onSubmit={handleSubmit((v) => {
        setError(null)
        mut.mutate(v)
      })}
      className="mt-6 space-y-3 rounded-md border border-border bg-muted/10 p-4"
    >
      <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Add field</p>
      <div className="grid grid-cols-2 gap-2">
        <Input placeholder="Field name (snake_case)" {...register('name', { required: true })} />
        <Input placeholder="Label" {...register('display_name', { required: true })} />
      </div>
      <select className={selectClass} {...register('data_type')}>
        {DATA_TYPES.map((t) => (
          <option key={t} value={t}>{t}</option>
        ))}
      </select>
      {currentType === 'reference' && (
        <select className={selectClass} {...register('reference_entity', { required: true })}>
          <option value="">— pick target entity —</option>
          {(entities.data ?? []).filter((e) => e.name !== entity.name).map((e) => (
            <option key={e.id} value={e.name}>{e.display_name}</option>
          ))}
        </select>
      )}
      <div className="flex items-center gap-4 text-xs text-muted-foreground">
        <label className="inline-flex items-center gap-1.5">
          <input type="checkbox" {...register('is_required')} /> required
        </label>
        <label className="inline-flex items-center gap-1.5">
          <input type="checkbox" {...register('is_unique')} /> unique
        </label>
      </div>
      {error && <p className="text-xs text-destructive">{error}</p>}
      <Button type="submit" disabled={isSubmitting || mut.isPending}>
        {mut.isPending ? 'Adding…' : 'Add field'}
      </Button>
    </form>
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
  sort,
  dir,
  onSort,
  onOpen,
}: {
  entity: Entity
  rows: Record<string, unknown>[]
  refLookup: (entityName: string, id: string) => string | null
  sort: string | undefined
  dir: 'asc' | 'desc' | undefined
  onSort: (field: string) => void
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
            <SortableTH
              key={f.id}
              label={f.display_name}
              field={f.name}
              sort={sort}
              dir={dir}
              onSort={onSort}
              align={isNumericType(f.data_type) ? 'right' : 'left'}
            />
          ))}
          <SortableTH
            label="Updated"
            field="updated_at"
            sort={sort}
            dir={dir}
            onSort={onSort}
          />
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
  onEditFromView,
}: {
  open: boolean
  mode: Mode
  entity: Entity
  refOptions: Record<string, RefOption[]>
  refLookup: (entityName: string, id: string) => string | null
  onClose: () => void
  onEditFromView: (row: Record<string, unknown>) => void
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
        <ViewRecord
          row={mode.row}
          entity={entity}
          refLookup={refLookup}
          onEdit={() => onEditFromView(mode.row)}
        />
      </Drawer>
    )
  }
  if (mode.kind === 'edit') {
    return (
      <Drawer
        open={open}
        onClose={onClose}
        title="Edit record"
        subtitle={entity.display_name}
      >
        <EditForm
          entity={entity}
          refOptions={refOptions}
          row={mode.row}
          onDone={onClose}
        />
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
  onEdit,
}: {
  row: Record<string, unknown>
  entity: Entity
  refLookup: (entityName: string, id: string) => string | null
  onEdit: () => void
}) {
  return (
    <div>
      <div className="mb-4 flex justify-end">
        <Button variant="ghost" onClick={onEdit}>
          <Pencil className="mr-1 h-3.5 w-3.5" /> Edit
        </Button>
      </div>
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
    </div>
  )
}

function EditForm({
  entity,
  refOptions,
  row,
  onDone,
}: {
  entity: Entity
  refOptions: Record<string, RefOption[]>
  row: Record<string, unknown>
  onDone: () => void
}) {
  const qc = useQueryClient()
  const defaults = useMemo(() => rowToFormValues(entity, row), [entity, row])
  const { register, handleSubmit, formState: { isSubmitting } } =
    useForm<Record<string, string>>({ defaultValues: defaults })
  const [error, setError] = useState<string | null>(null)

  const id = String(row.id ?? '')
  const mut = useMutation({
    mutationFn: (values: Record<string, string>) => api.updateRow(entity.name, id, values),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['rows', entity.name] })
      onDone()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <form
      className="space-y-5"
      onSubmit={handleSubmit((v) => {
        mut.mutate(v)
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
      <div className="sticky bottom-0 flex items-center gap-2 border-t border-border bg-card/95 py-3 backdrop-blur">
        <Button type="submit" disabled={isSubmitting || mut.isPending}>
          {mut.isPending ? 'Saving…' : 'Save changes'}
        </Button>
        <Button variant="ghost" type="button" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  )
}

function rowToFormValues(entity: Entity, row: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const f of entity.fields) {
    const v = row[f.name]
    if (v === null || v === undefined) {
      out[f.name] = ''
      continue
    }
    if (f.data_type === 'boolean') {
      out[f.name] = v ? 'on' : ''
      continue
    }
    if (f.data_type === 'timestamptz' && typeof v === 'string') {
      const d = new Date(v)
      if (!isNaN(d.getTime())) {
        out[f.name] = toDatetimeLocal(d)
        continue
      }
    }
    if (f.data_type === 'jsonb') {
      out[f.name] = typeof v === 'string' ? v : JSON.stringify(v)
      continue
    }
    out[f.name] = String(v)
  }
  return out
}

function toDatetimeLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function SortableTH({
  label,
  field,
  sort,
  dir,
  onSort,
  align = 'left',
}: {
  label: string
  field: string
  sort: string | undefined
  dir: 'asc' | 'desc' | undefined
  onSort: (field: string) => void
  align?: 'left' | 'right'
}) {
  const active = sort === field
  return (
    <th
      className={cn(
        'whitespace-nowrap px-4 py-2.5 text-xs font-medium uppercase tracking-wider text-muted-foreground',
        align === 'right' && 'text-right'
      )}
    >
      <button
        onClick={() => onSort(field)}
        className={cn(
          'group inline-flex items-center gap-1 uppercase tracking-wider hover:text-foreground',
          active && 'text-foreground'
        )}
      >
        <span>{label}</span>
        {active ? (
          dir === 'asc' ? (
            <ArrowUp className="h-3 w-3" />
          ) : (
            <ArrowDown className="h-3 w-3" />
          )
        ) : (
          <ChevronsUpDown className="h-3 w-3 opacity-0 transition-opacity group-hover:opacity-50" />
        )}
      </button>
    </th>
  )
}

function Pagination({
  page,
  totalPages,
  total,
  onPage,
}: {
  page: number
  totalPages: number
  total: number
  onPage: (page: number) => void
}) {
  return (
    <div className="flex items-center justify-between border-t border-border bg-card/60 px-8 py-2.5 text-xs text-muted-foreground">
      <span>
        Page <span className="font-medium text-foreground">{page}</span> of{' '}
        <span className="font-medium text-foreground">{totalPages}</span>
        <span className="mx-2">·</span>
        {total} total
      </span>
      <div className="flex items-center gap-1">
        <button
          onClick={() => onPage(page - 1)}
          disabled={page <= 1}
          className="inline-flex h-7 items-center gap-1 rounded-md border border-border bg-background px-2 text-xs hover:bg-accent disabled:opacity-40"
        >
          <ChevronLeft className="h-3.5 w-3.5" /> Prev
        </button>
        <button
          onClick={() => onPage(page + 1)}
          disabled={page >= totalPages}
          className="inline-flex h-7 items-center gap-1 rounded-md border border-border bg-background px-2 text-xs hover:bg-accent disabled:opacity-40"
        >
          Next <ChevronRight className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  )
}
