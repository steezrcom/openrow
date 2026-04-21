import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { api, ApiError, type DataType, type Entity, type Field, type RowsResponse } from '@/lib/api'
import { Button, Card, Input, Label, Pill, Textarea } from '@/components/ui'

export const Route = createFileRoute('/app/entities/$name')({
  component: EntityDetail,
})

function EntityDetail() {
  const { name } = Route.useParams()
  const rows = useQuery({
    queryKey: ['rows', name],
    queryFn: () => api.listRows(name),
  })

  if (rows.isLoading) return <p className="text-sm text-muted-foreground">Loading.</p>
  if (rows.error) {
    return (
      <p className="text-sm text-destructive">
        {rows.error instanceof Error ? rows.error.message : 'Failed to load'}
      </p>
    )
  }
  if (!rows.data) return null
  const { entity, ref_options } = rows.data

  return (
    <div className="space-y-10">
      <div>
        <Link
          to="/app"
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          ← Back
        </Link>
        <div className="mt-3 flex items-center gap-3">
          <h1 className="text-xl font-semibold">{entity.display_name}</h1>
          <Pill>{entity.name}</Pill>
        </div>
        {entity.description && (
          <p className="mt-2 text-sm text-muted-foreground">{entity.description}</p>
        )}
      </div>

      <FieldsTable entity={entity} />
      <InsertForm entity={entity} refOptions={ref_options} />
      <RowsTable data={rows.data} />
    </div>
  )
}

function FieldsTable({ entity }: { entity: Entity }) {
  return (
    <section>
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-muted-foreground">Fields</h2>
      <Card>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Label</th>
              <th className="px-4 py-2">Type</th>
              <th className="px-4 py-2">Required</th>
              <th className="px-4 py-2">Unique</th>
              <th className="px-4 py-2">References</th>
            </tr>
          </thead>
          <tbody>
            {entity.fields.map((f) => (
              <tr key={f.id} className="border-b border-border last:border-0">
                <td className="px-4 py-2 font-mono text-[13px]">{f.name}</td>
                <td className="px-4 py-2">{f.display_name}</td>
                <td className="px-4 py-2"><Pill>{f.data_type}</Pill></td>
                <td className="px-4 py-2">{f.is_required ? 'yes' : ''}</td>
                <td className="px-4 py-2">{f.is_unique ? 'yes' : ''}</td>
                <td className="px-4 py-2 text-muted-foreground">{f.reference_entity ?? ''}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </section>
  )
}

function InsertForm({
  entity,
  refOptions,
}: {
  entity: Entity
  refOptions: RowsResponse['ref_options']
}) {
  const qc = useQueryClient()
  const { register, handleSubmit, reset, formState: { isSubmitting } } = useForm<Record<string, string>>()
  const [error, setError] = useState<string | null>(null)

  const mut = useMutation({
    mutationFn: (values: Record<string, string>) => api.createRow(entity.name, values),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['rows', entity.name] })
      reset()
      setError(null)
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <section>
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-muted-foreground">Add row</h2>
      <Card className="p-4">
        <form
          className="space-y-4"
          onSubmit={handleSubmit((v) => {
            const filtered = Object.fromEntries(Object.entries(v).filter(([, value]) => value !== ''))
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
          <div className="flex items-center gap-3">
            <Button type="submit" disabled={isSubmitting || mut.isPending}>
              {mut.isPending ? 'Saving…' : 'Insert'}
            </Button>
            {error && <span className="text-sm text-destructive">{error}</span>}
          </div>
        </form>
      </Card>
    </section>
  )
}

type RegisterFn = ReturnType<typeof useForm<Record<string, string>>>['register']

function FieldInput({
  field,
  register,
  refOptions,
}: {
  field: Field
  register: RegisterFn
  refOptions: { ID: string; Label: string }[]
}) {
  const baseClass =
    'flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm'
  const props = register(field.name, { required: field.is_required })
  const label = (
    <Label>
      {field.display_name}
      {field.is_required && <span className="text-destructive"> *</span>}
    </Label>
  )

  switch (field.data_type as DataType) {
    case 'boolean':
      return (
        <div className="flex items-center gap-3">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-input"
            {...register(field.name)}
          />
          {label}
        </div>
      )
    case 'integer':
    case 'bigint':
      return (
        <div className="space-y-2">
          {label}
          <Input type="number" step="1" {...props} />
        </div>
      )
    case 'numeric':
      return (
        <div className="space-y-2">
          {label}
          <Input type="number" step="any" {...props} />
        </div>
      )
    case 'date':
      return (
        <div className="space-y-2">
          {label}
          <Input type="date" {...props} />
        </div>
      )
    case 'timestamptz':
      return (
        <div className="space-y-2">
          {label}
          <Input type="datetime-local" {...props} />
        </div>
      )
    case 'jsonb':
      return (
        <div className="space-y-2">
          {label}
          <Textarea placeholder='{"key":"value"}' rows={3} {...props} />
        </div>
      )
    case 'reference':
      return (
        <div className="space-y-2">
          {label}
          <select className={baseClass} {...props}>
            <option value="">— pick {field.reference_entity} —</option>
            {refOptions.map((o) => (
              <option key={o.ID} value={o.ID}>
                {o.Label}
              </option>
            ))}
          </select>
        </div>
      )
    default:
      return (
        <div className="space-y-2">
          {label}
          <Input type="text" {...props} />
        </div>
      )
  }
}

function displayCell(v: unknown): string {
  if (v == null) return ''
  if (typeof v === 'boolean') return v ? 'yes' : ''
  if (typeof v === 'string') {
    if (/^\d{4}-\d{2}-\d{2}T/.test(v)) {
      const d = new Date(v)
      if (!isNaN(d.getTime())) return d.toISOString().replace('T', ' ').slice(0, 16)
    }
    return v
  }
  return String(v)
}

function RowsTable({ data }: { data: RowsResponse }) {
  const qc = useQueryClient()
  const del = useMutation({
    mutationFn: (id: string) => api.deleteRow(data.entity.name, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['rows', data.entity.name] }),
  })

  return (
    <section>
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-muted-foreground">Rows</h2>
      {data.rows.length === 0 ? (
        <Card className="p-4 text-sm text-muted-foreground">No rows yet.</Card>
      ) : (
        <Card>
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs uppercase tracking-wider text-muted-foreground">
                <th className="px-4 py-2">id</th>
                {data.entity.fields.map((f) => (
                  <th key={f.id} className="px-4 py-2">
                    {f.name}
                  </th>
                ))}
                <th className="px-4 py-2">created</th>
                <th className="px-4 py-2"></th>
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row) => {
                const id = String(row.id ?? '')
                return (
                  <tr key={id} className="border-b border-border last:border-0">
                    <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                      {id.slice(0, 8)}
                    </td>
                    {data.entity.fields.map((f) => (
                      <td key={f.id} className="px-4 py-2">
                        {displayCell(row[f.name])}
                      </td>
                    ))}
                    <td className="px-4 py-2 font-mono text-xs text-muted-foreground">
                      {displayCell(row.created_at)}
                    </td>
                    <td className="px-4 py-2">
                      <button
                        onClick={() => {
                          if (confirm('Delete this row?')) del.mutate(id)
                        }}
                        className="rounded border border-destructive/50 px-2 py-0.5 text-xs text-destructive hover:bg-destructive/10"
                      >
                        ×
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </Card>
      )}
    </section>
  )
}
