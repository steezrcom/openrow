import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { LayoutGrid, Columns, Table2 } from 'lucide-react'
import { api, ApiError, type Entity, type EntityView, type ViewType } from '@/lib/api'
import { Button, Input, Label } from '@/components/ui'
import { Modal } from '@/components/Modal'
import { cn } from '@/lib/utils'

interface FormValues {
  name: string
  view_type: ViewType
  group_by: string
}

const KINDS: { id: ViewType; label: string; description: string; icon: typeof Table2 }[] = [
  { id: 'table', label: 'Table', description: 'Spreadsheet-style rows and columns.', icon: Table2 },
  { id: 'cards', label: 'Cards', description: 'Each record as a tile, stacked in a grid.', icon: LayoutGrid },
  { id: 'kanban', label: 'Kanban', description: 'Columns grouped by a field; drag to change value.', icon: Columns },
]

export function NewViewModal({
  open,
  entity,
  onClose,
  onCreated,
}: {
  open: boolean
  entity: Entity
  onClose: () => void
  onCreated: (view: EntityView) => void
}) {
  const qc = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const { register, handleSubmit, watch, formState: { isSubmitting } } = useForm<FormValues>({
    defaultValues: {
      name: '',
      view_type: 'cards',
      group_by: defaultGroupBy(entity),
    },
  })

  const kind = watch('view_type')

  const create = useMutation({
    mutationFn: (v: FormValues) => {
      const config: Record<string, unknown> = {}
      if (v.view_type === 'kanban') {
        if (!v.group_by) throw new Error('Kanban needs a group-by field')
        config.group_by = v.group_by
      }
      return api.createView(entity.name, {
        name: v.name,
        view_type: v.view_type,
        config,
      })
    },
    onSuccess: (view) => {
      qc.invalidateQueries({ queryKey: ['views', entity.name] })
      onCreated(view)
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : (err as Error).message),
  })

  // Fields eligible for kanban group_by: text or reference.
  const groupable = entity.fields.filter((f) => f.data_type === 'text' || f.data_type === 'reference')

  return (
    <Modal open={open} onClose={onClose} title="New view" widthClass="max-w-lg">
      <form
        className="space-y-4"
        onSubmit={handleSubmit((v) => {
          setError(null)
          create.mutate(v)
        })}
      >
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input id="name" autoFocus placeholder="Open clients" {...register('name', { required: true })} />
        </div>

        <div className="space-y-2">
          <Label>Type</Label>
          <div className="grid grid-cols-1 gap-2">
            {KINDS.map((k) => (
              <label
                key={k.id}
                className={cn(
                  'flex cursor-pointer items-start gap-3 rounded-md border border-border bg-background p-3 hover:bg-accent',
                  kind === k.id && 'border-primary ring-2 ring-primary/30'
                )}
              >
                <input type="radio" value={k.id} {...register('view_type')} className="mt-1" />
                <k.icon className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                <div className="min-w-0">
                  <div className="text-sm font-medium">{k.label}</div>
                  <p className="mt-0.5 text-xs text-muted-foreground">{k.description}</p>
                </div>
              </label>
            ))}
          </div>
        </div>

        {kind === 'kanban' && (
          <div className="space-y-2 rounded-md border border-border bg-muted/10 p-3">
            <Label htmlFor="group_by">Group by</Label>
            <select
              id="group_by"
              {...register('group_by', { required: kind === 'kanban' })}
              className="flex h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
            >
              <option value="">Pick a field…</option>
              {groupable.map((f) => (
                <option key={f.name} value={f.name}>
                  {f.display_name} ({f.name} · {f.data_type})
                </option>
              ))}
            </select>
            <p className="text-[11px] text-muted-foreground">
              Columns come from the distinct values in this field. Dragging a card between columns updates the field.
            </p>
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

        <div className="flex items-center gap-2">
          <Button type="submit" disabled={isSubmitting || create.isPending}>
            {create.isPending ? 'Creating…' : 'Create view'}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
        </div>
      </form>
    </Modal>
  )
}

function defaultGroupBy(entity: Entity): string {
  for (const candidate of ['status', 'state', 'stage']) {
    if (entity.fields.some((f) => f.name === candidate)) return candidate
  }
  return ''
}
