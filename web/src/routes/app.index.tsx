import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { api, ApiError, type Entity } from '@/lib/api'
import { Button, Card, Pill, Textarea } from '@/components/ui'

export const Route = createFileRoute('/app/')({
  component: Dashboard,
})

function Dashboard() {
  const entities = useQuery({ queryKey: ['entities'], queryFn: api.listEntities })

  return (
    <div className="space-y-10">
      <section>
        <h1 className="text-xl font-semibold">Describe an entity</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Say what you want to track. We generate a real table for it.
        </p>
        <DescribeForm />
      </section>

      <section>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">Entities</h2>
        </div>
        {entities.isLoading && <p className="text-sm text-muted-foreground">Loading.</p>}
        {entities.error && (
          <p className="text-sm text-destructive">
            {entities.error instanceof Error ? entities.error.message : 'Failed to load'}
          </p>
        )}
        {entities.data && entities.data.length === 0 && (
          <Card className="p-6 text-sm text-muted-foreground">
            Nothing yet. Describe your first entity above.
          </Card>
        )}
        {entities.data && entities.data.length > 0 && (
          <div className="grid gap-3">
            {entities.data.map((e: Entity) => (
              <Link
                key={e.id}
                to="/app/entities/$name"
                params={{ name: e.name }}
                className="block"
              >
                <Card className="p-4 transition-colors hover:bg-accent">
                  <div className="flex items-baseline justify-between">
                    <div>
                      <h3 className="text-base font-medium">{e.display_name}</h3>
                      {e.description && (
                        <p className="mt-1 text-sm text-muted-foreground">{e.description}</p>
                      )}
                    </div>
                    <Pill>{e.name}</Pill>
                  </div>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

function DescribeForm() {
  const qc = useQueryClient()
  const { register, handleSubmit, reset, formState: { isSubmitting } } = useForm<{ description: string }>()
  const [error, setError] = useState<string | null>(null)

  const propose = useMutation({
    mutationFn: (description: string) => api.proposeEntity(description),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['entities'] })
      reset()
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : 'failed')
    },
  })

  return (
    <form
      className="mt-4 space-y-3"
      onSubmit={handleSubmit((v) => {
        setError(null)
        propose.mutate(v.description)
      })}
    >
      <Textarea
        rows={3}
        placeholder="e.g. A customer with a name, email, and optional phone. Emails should be unique."
        {...register('description', { required: true, minLength: 8 })}
      />
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={isSubmitting || propose.isPending}>
          {propose.isPending ? 'Asking Claude…' : 'Propose entity'}
        </Button>
        {error && <span className="text-sm text-destructive">{error}</span>}
      </div>
    </form>
  )
}
