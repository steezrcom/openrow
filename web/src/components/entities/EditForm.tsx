import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useMemo, useState } from 'react'
import { api, ApiError, type Entity, type RefOption } from '@/lib/api'
import { Button, ErrorAlert, FormActions } from '@/components/ui'
import { FieldInput } from '@/components/FieldInput'

export function EditForm({
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
      {error && <ErrorAlert>{error}</ErrorAlert>}
      <FormActions>
        <Button type="submit" disabled={isSubmitting || mut.isPending}>
          {mut.isPending ? 'Saving…' : 'Save changes'}
        </Button>
        <Button variant="ghost" type="button" onClick={onDone}>
          Cancel
        </Button>
      </FormActions>
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
