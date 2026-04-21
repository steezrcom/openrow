import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { api, ApiError } from '@/lib/api'
import { useMe } from '@/hooks/useMe'
import { Button, Card, Input, Label } from '@/components/ui'

export const Route = createFileRoute('/orgs')({
  beforeLoad: async ({ context }) => {
    try {
      await context.queryClient.fetchQuery({
        queryKey: ['me'],
        queryFn: api.me,
        staleTime: 10_000,
      })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: '/login' })
      }
      throw err
    }
  },
  component: OrgsPage,
})

function slugify(s: string) {
  return s.toLowerCase().trim().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 30)
}

function OrgsPage() {
  const me = useMe()
  const qc = useQueryClient()
  const navigate = useNavigate()

  const activate = useMutation({
    mutationFn: api.activateMembership,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['me'] })
      navigate({ to: '/app' })
    },
  })

  if (!me.data) return null

  return (
    <div className="mx-auto max-w-xl p-6">
      <h1 className="text-xl font-semibold">Workspaces</h1>
      <p className="mt-1 text-sm text-muted-foreground">Pick a workspace to open, or create a new one.</p>

      <div className="mt-6 grid gap-2">
        {me.data.memberships.map((m) => (
          <Card
            key={m.id}
            onClick={() => activate.mutate(m.id)}
            className="cursor-pointer p-4 transition-colors hover:bg-accent"
          >
            <div className="flex items-baseline justify-between">
              <div>
                <p className="font-medium">{m.org_name}</p>
                <p className="text-xs text-muted-foreground">{m.org_slug} · {m.role}</p>
              </div>
              <span className="text-muted-foreground">→</span>
            </div>
          </Card>
        ))}
        {me.data.memberships.length === 0 && (
          <Card className="p-4 text-sm text-muted-foreground">
            No workspaces yet. Create one below.
          </Card>
        )}
      </div>

      <CreateOrgForm />
    </div>
  )
}

function CreateOrgForm() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { register, handleSubmit, watch, setValue, formState: { isSubmitting }, reset } =
    useForm<{ name: string; slug: string }>()
  const [error, setError] = useState<string | null>(null)
  const name = watch('name')

  const mut = useMutation({
    mutationFn: (body: { name: string; slug: string }) => api.createOrg(body),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['me'] })
      reset()
      navigate({ to: '/app' })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <div className="mt-8">
      <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-muted-foreground">
        New workspace
      </h2>
      <Card className="p-4">
        <form
          className="space-y-4"
          onSubmit={handleSubmit((v) => {
            setError(null)
            mut.mutate(v)
          })}
        >
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="Globex GmbH"
              {...register('name', {
                required: true,
                onChange: (e) => setValue('slug', slugify(e.target.value)),
              })}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="slug">URL slug</Label>
            <Input
              id="slug"
              className="font-mono"
              placeholder={name ? slugify(name) : 'globex'}
              pattern="[a-z][a-z0-9_]*"
              {...register('slug', { required: true })}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button type="submit" disabled={isSubmitting || mut.isPending}>
            {mut.isPending ? 'Creating…' : 'Create workspace'}
          </Button>
        </form>
      </Card>
    </div>
  )
}
