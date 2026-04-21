import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { z } from 'zod'
import { api, ApiError } from '@/lib/api'
import { Button, Card, Input, Label } from '@/components/ui'
import { AuthShell } from './login'

const searchSchema = z.object({ token: z.string().optional() })

export const Route = createFileRoute('/reset')({
  validateSearch: searchSchema,
  component: ResetPage,
})

function ResetPage() {
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const { register, handleSubmit, formState: { isSubmitting } } =
    useForm<{ password: string }>()
  const [error, setError] = useState<string | null>(null)

  if (!token) {
    return (
      <AuthShell title="Invalid link" subtitle="This reset link is missing a token.">
        <Card className="p-6 text-sm text-muted-foreground">
          Request a new link from <Link to="/forgot" className="text-primary hover:underline">the forgot password page</Link>.
        </Card>
      </AuthShell>
    )
  }

  async function onSubmit({ password }: { password: string }) {
    setError(null)
    try {
      await api.resetPassword(token!, password)
      navigate({ to: '/login' })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Something went wrong')
    }
  }

  return (
    <AuthShell title="Set a new password" subtitle="Choose a strong password.">
      <Card className="p-6">
        <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-2">
            <Label htmlFor="password">New password</Label>
            <Input id="password" type="password" autoComplete="new-password" autoFocus
              {...register('password', { required: true, minLength: 10 })} />
            <p className="text-xs text-muted-foreground">At least 10 characters.</p>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <Button className="w-full" type="submit" disabled={isSubmitting}>
            {isSubmitting ? 'Saving…' : 'Save new password'}
          </Button>
        </form>
      </Card>
    </AuthShell>
  )
}
