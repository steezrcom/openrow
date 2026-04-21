import { createFileRoute, Link } from '@tanstack/react-router'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import { Mail } from 'lucide-react'
import { api } from '@/lib/api'
import { Button, Card, Input, Label } from '@/components/ui'
import { AuthShell } from './login'

export const Route = createFileRoute('/forgot')({
  component: ForgotPage,
})

function ForgotPage() {
  const { register, handleSubmit, formState: { isSubmitting } } = useForm<{ email: string }>()
  const [sent, setSent] = useState(false)

  async function onSubmit({ email }: { email: string }) {
    try {
      await api.forgotPassword(email)
    } finally {
      setSent(true)
    }
  }

  if (sent) {
    return (
      <AuthShell title="Check your email" subtitle="We just sent you a password reset link.">
        <Card className="p-6 text-sm">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-full bg-primary/15 text-primary">
            <Mail className="h-5 w-5" />
          </div>
          <p className="text-muted-foreground">
            If there's an account for that email, a link is on its way. It expires in 1 hour.
          </p>
          <p className="mt-3 text-xs text-muted-foreground">
            (In dev, the link prints to the server log.)
          </p>
        </Card>
        <p className="mt-4 text-center text-sm text-muted-foreground">
          <Link to="/login" className="hover:text-foreground">Back to sign in</Link>
        </p>
      </AuthShell>
    )
  }

  return (
    <AuthShell title="Reset your password" subtitle="Enter your email; we'll send you a link.">
      <Card className="p-6">
        <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" autoComplete="email" autoFocus
              {...register('email', { required: true })} />
          </div>
          <Button className="w-full" type="submit" disabled={isSubmitting}>
            {isSubmitting ? 'Sending…' : 'Send reset link'}
          </Button>
        </form>
      </Card>
      <p className="mt-4 text-center text-sm text-muted-foreground">
        <Link to="/login" className="hover:text-foreground">Back to sign in</Link>
      </p>
    </AuthShell>
  )
}
