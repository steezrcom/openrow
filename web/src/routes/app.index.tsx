import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { Sparkles, ArrowUpRight, Table2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useEntities } from '@/hooks/useEntities'
import { useMe } from '@/hooks/useMe'
import { Button, Card, Pill, Textarea } from '@/components/ui'

export const Route = createFileRoute('/app/')({
  component: Dashboard,
})

function Dashboard() {
  const me = useMe()
  const entities = useEntities()
  const firstName = me.data?.user.name.split(' ')[0] ?? ''

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <header className="mb-10">
        <p className="text-sm text-muted-foreground">Welcome back{firstName ? `, ${firstName}` : ''}</p>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">
          What should we build today?
        </h1>
      </header>

      <DescribeForm />

      <section className="mt-14">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
            Your entities
          </h2>
          {entities.data && entities.data.length > 0 && (
            <span className="text-xs text-muted-foreground">{entities.data.length} total</span>
          )}
        </div>

        {entities.isLoading && (
          <div className="grid gap-2">
            <div className="h-16 animate-pulse rounded-md bg-muted/30" />
            <div className="h-16 animate-pulse rounded-md bg-muted/30" />
          </div>
        )}

        {entities.data && entities.data.length === 0 && (
          <Card className="p-6 text-sm text-muted-foreground">
            <p>Nothing yet.</p>
            <p className="mt-1">
              Describe your first entity above. We'll turn it into a real database table.
            </p>
          </Card>
        )}

        {entities.data && entities.data.length > 0 && (
          <div className="grid gap-2">
            {entities.data.map((e) => (
              <Link
                key={e.id}
                to="/app/entities/$name"
                params={{ name: e.name }}
                className="group block"
              >
                <Card className="flex items-center gap-4 p-4 transition-colors hover:bg-accent">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                    <Table2 className="h-4 w-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="truncate font-medium">{e.display_name}</h3>
                      <Pill>{e.name}</Pill>
                    </div>
                    {e.description && (
                      <p className="mt-1 truncate text-xs text-muted-foreground">{e.description}</p>
                    )}
                  </div>
                  <ArrowUpRight className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                </Card>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

const SUGGESTIONS = [
  'A customer with a name, email and optional phone',
  'An invoice with amount, due date, and a customer reference',
  'A project with a name, status, and estimated hours',
]

function DescribeForm() {
  const qc = useQueryClient()
  const { register, handleSubmit, reset, setValue, formState: { isSubmitting } } =
    useForm<{ description: string }>()
  const [error, setError] = useState<string | null>(null)

  const propose = useMutation({
    mutationFn: (description: string) => api.proposeEntity(description),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['entities'] })
      reset()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <Card className="overflow-hidden border-primary/30 bg-gradient-to-br from-card to-card/70">
      <form
        onSubmit={handleSubmit((v) => {
          setError(null)
          propose.mutate(v.description)
        })}
      >
        <div className="flex items-start gap-3 p-4">
          <div className="mt-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Sparkles className="h-4 w-4" />
          </div>
          <div className="flex-1">
            <Textarea
              rows={3}
              placeholder="Describe what you want to track — in plain English."
              className="resize-none border-0 bg-transparent p-0 shadow-none focus-visible:ring-0"
              {...register('description', { required: true, minLength: 8 })}
            />
            <div className="mt-3 flex flex-wrap gap-1.5">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setValue('description', s, { shouldValidate: true })}
                  className="rounded-full border border-border bg-background/60 px-3 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        </div>
        <div className="flex items-center justify-between border-t border-border bg-muted/20 px-4 py-2">
          <span className="text-xs text-muted-foreground">
            Claude will design the schema and create a real table.
          </span>
          <Button type="submit" disabled={isSubmitting || propose.isPending}>
            {propose.isPending ? 'Designing…' : 'Create entity'}
          </Button>
        </div>
        {error && <p className="border-t border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-destructive">{error}</p>}
      </form>
    </Card>
  )
}
