import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { api, ApiError, type Entity, type RefOption } from '@/lib/api'
import { Button, ErrorAlert, FormActions } from '@/components/ui'
import { FieldInput } from '@/components/FieldInput'

export function CreateForm({
  entity,
  refOptions,
  onDone,
}: {
  entity: Entity
  refOptions: Record<string, RefOption[]>
  onDone: () => void
}) {
  const qc = useQueryClient()
  const { register, handleSubmit, formState: { isSubmitting } } = useForm<Record<string, string>>()
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
      {error && <ErrorAlert>{error}</ErrorAlert>}
      <FormActions>
        <Button type="submit" disabled={isSubmitting || mut.isPending}>
          {mut.isPending ? 'Saving…' : 'Save record'}
        </Button>
      </FormActions>
    </form>
  )
}
