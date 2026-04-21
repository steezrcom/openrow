import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { api, ApiError } from '@/lib/api'
import { Button, Card, Input, Label } from '@/components/ui'
import { useQueryClient } from '@tanstack/react-query'

export const Route = createFileRoute('/login')({
  component: LoginPage,
})

type FormValues = { email: string; password: string }

function LoginPage() {
  const { register, handleSubmit, formState: { isSubmitting } } = useForm<FormValues>()
  const [error, setError] = useState<string | null>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()

  async function onSubmit(values: FormValues) {
    setError(null)
    try {
      await api.login(values)
      await qc.invalidateQueries({ queryKey: ['me'] })
      navigate({ to: '/' })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'something went wrong')
    }
  }

  return (
    <AuthShell title="Log in" subtitle="Welcome back">
      <Card className="p-6">
        <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" autoComplete="email" autoFocus
              {...register('email', { required: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input id="password" type="password" autoComplete="current-password"
              {...register('password', { required: true })} />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button className="w-full" type="submit" disabled={isSubmitting}>
            {isSubmitting ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>
      </Card>
      <div className="mt-4 flex items-center justify-between text-sm">
        <Link to="/forgot" className="text-muted-foreground hover:text-foreground">
          Forgot password?
        </Link>
        <Link to="/signup" className="text-muted-foreground hover:text-foreground">
          No account? <span className="text-primary">Sign up</span>
        </Link>
      </div>
    </AuthShell>
  )
}

export function AuthShell({
  title, subtitle, children,
}: {
  title: string
  subtitle?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <div className="text-lg font-semibold tracking-tight">
            steezr<span className="text-primary">_</span>
          </div>
          <h1 className="mt-6 text-xl font-semibold">{title}</h1>
          {subtitle && <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p>}
        </div>
        {children}
      </div>
    </div>
  )
}
