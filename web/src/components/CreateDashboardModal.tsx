import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { api, ApiError } from '@/lib/api'
import { Button, Input, Label, Textarea } from '@/components/ui'
import { Modal } from '@/components/Modal'

export function CreateDashboardModal({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { register, handleSubmit, reset, formState: { isSubmitting } } =
    useForm<{ name: string; description?: string }>()
  const [error, setError] = useState<string | null>(null)

  const mut = useMutation({
    mutationFn: (body: { name: string; description?: string }) => api.createDashboard(body),
    onSuccess: async (d) => {
      await qc.invalidateQueries({ queryKey: ['dashboards'] })
      reset()
      onClose()
      navigate({ to: '/app/dashboards/$slug', params: { slug: d.slug } })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <Modal open={open} onClose={onClose} title="New dashboard">
      <form
        className="space-y-4"
        onSubmit={handleSubmit((v) => {
          setError(null)
          mut.mutate({ name: v.name, description: v.description })
        })}
      >
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input
            id="name"
            autoFocus
            placeholder="Financial overview"
            {...register('name', { required: true, minLength: 2 })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="description">Description (optional)</Label>
          <Textarea
            id="description"
            rows={2}
            placeholder="What this dashboard is for"
            {...register('description')}
          />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="flex items-center gap-2">
          <Button type="submit" disabled={isSubmitting || mut.isPending}>
            {mut.isPending ? 'Creating…' : 'Create dashboard'}
          </Button>
          <Button variant="ghost" type="button" onClick={onClose}>
            Cancel
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          After creating, ask Claude in the chat to add reports — e.g. "add a revenue by month line chart."
        </p>
      </form>
    </Modal>
  )
}
