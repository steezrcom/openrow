import { createFileRoute, Link, Outlet, redirect, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, ApiError, type Membership } from '@/lib/api'
import { useMe } from '@/hooks/useMe'
import { Button } from '@/components/ui'

export const Route = createFileRoute('/app')({
  beforeLoad: async ({ context }) => {
    try {
      const me = await context.queryClient.fetchQuery({
        queryKey: ['me'],
        queryFn: api.me,
        staleTime: 10_000,
      })
      if (!me.active_membership) throw redirect({ to: '/orgs' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: '/login' })
      }
      throw err
    }
  },
  component: AppLayout,
})

function AppLayout() {
  const me = useMe()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      await qc.clear()
      navigate({ to: '/login' })
    },
  })

  if (!me.data) return null
  const active = me.data.active_membership

  return (
    <div className="min-h-screen">
      <header className="flex items-center justify-between border-b border-border px-6 py-3">
        <div className="flex items-center gap-6">
          <Link to="/app" className="font-semibold tracking-tight">
            steezr<span className="text-primary">_</span>
          </Link>
          {active && <OrgSwitcher active={active} memberships={me.data.memberships} />}
        </div>
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <span>{me.data.user.email}</span>
          <Button variant="ghost" onClick={() => logout.mutate()}>
            Log out
          </Button>
        </div>
      </header>
      <main className="mx-auto max-w-5xl p-6">
        <Outlet />
      </main>
    </div>
  )
}

function OrgSwitcher({
  active,
  memberships,
}: {
  active: Membership
  memberships: Membership[]
}) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const activate = useMutation({
    mutationFn: api.activateMembership,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['me'] })
      await qc.invalidateQueries({ queryKey: ['entities'] })
      setOpen(false)
      navigate({ to: '/app' })
    },
  })

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-2 rounded-md border border-border bg-card px-3 py-1.5 text-sm hover:bg-accent"
      >
        <span>{active.org_name}</span>
        <span className="text-muted-foreground">▾</span>
      </button>
      {open && (
        <div className="absolute left-0 mt-1 w-64 rounded-md border border-border bg-card shadow-lg">
          <div className="py-1">
            {memberships.map((m) => (
              <button
                key={m.id}
                onClick={() => activate.mutate(m.id)}
                className="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-accent"
              >
                <span>{m.org_name}</span>
                {m.id === active.id && <span className="text-primary">●</span>}
              </button>
            ))}
          </div>
          <div className="border-t border-border p-1">
            <Link
              to="/orgs"
              onClick={() => setOpen(false)}
              className="block rounded-sm px-3 py-2 text-sm text-muted-foreground hover:bg-accent"
            >
              Manage workspaces
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}
