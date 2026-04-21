import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import { Button, Card, Input, Label } from '@/components/ui'
import { AuthShell } from './login'

export const Route = createFileRoute('/signup')({
  component: SignupPage,
})

type FormValues = {
  name: string
  email: string
  password: string
  org_name: string
  org_slug: string
}

function slugify(s: string) {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9_]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 30)
}

function SignupPage() {
  const { register, handleSubmit, watch, setValue, formState: { isSubmitting } } = useForm<FormValues>()
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const orgName = watch('org_name')

  async function onSubmit(values: FormValues) {
    setError(null)
    try {
      await api.signup({
        email: values.email,
        name: values.name,
        password: values.password,
        org_name: values.org_name,
        org_slug: values.org_slug,
      })
      await qc.invalidateQueries({ queryKey: ['me'] })
      navigate({ to: '/' })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'something went wrong')
    }
  }

  return (
    <AuthShell title="Create your account" subtitle="Start your workspace in 30 seconds.">
      <Card className="p-6">
        <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-2">
            <Label htmlFor="name">Your name</Label>
            <Input id="name" autoComplete="name" autoFocus
              {...register('name', { required: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="email">Work email</Label>
            <Input id="email" type="email" autoComplete="email"
              {...register('email', { required: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input id="password" type="password" autoComplete="new-password"
              {...register('password', { required: true, minLength: 10 })} />
            <p className="text-xs text-muted-foreground">At least 10 characters.</p>
          </div>
          <div className="border-t border-border pt-4 space-y-4">
            <div className="space-y-2">
              <Label htmlFor="org_name">Workspace name</Label>
              <Input
                id="org_name" placeholder="Acme Inc."
                {...register('org_name', {
                  required: true,
                  onChange: (e) => setValue('org_slug', slugify(e.target.value)),
                })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="org_slug">Workspace URL</Label>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span className="rounded-md border border-border bg-muted/30 px-2 py-2 font-mono">openrow.app/</span>
                <Input
                  id="org_slug"
                  className="font-mono"
                  pattern="[a-z][a-z0-9_]*"
                  {...register('org_slug', { required: true })}
                  placeholder={orgName ? slugify(orgName) : 'acme'}
                />
              </div>
            </div>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button className="w-full" type="submit" disabled={isSubmitting}>
            {isSubmitting ? 'Creating…' : 'Create account'}
          </Button>
        </form>
      </Card>
      <p className="mt-4 text-center text-sm text-muted-foreground">
        Have an account?{' '}
        <Link to="/login" className="text-primary hover:underline">Log in</Link>
      </p>
    </AuthShell>
  )
}
