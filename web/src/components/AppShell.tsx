import { Link, useMatchRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import {
  Building2,
  ChevronsUpDown,
  Clock,
  Database,
  LayoutDashboard,
  LogOut,
  Plus,
  Search,
  Settings,
  Sparkles,
} from 'lucide-react'
import { api, type Dashboard, type Entity, type Membership } from '@/lib/api'
import { useMe } from '@/hooks/useMe'
import { useEntities } from '@/hooks/useEntities'
import { useDashboards } from '@/hooks/useDashboards'
import { cn } from '@/lib/utils'
import { ChatPanel } from '@/components/ChatPanel'
import { CreateDashboardModal } from '@/components/CreateDashboardModal'
import { TimerWidget } from '@/components/TimerWidget'

export function AppShell({ children }: { children: ReactNode }) {
  const me = useMe()
  const entities = useEntities()
  const dashboards = useDashboards()
  if (!me.data) return null

  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar
        active={me.data.active_membership}
        memberships={me.data.memberships}
        email={me.data.user.email}
        entities={entities.data ?? []}
        loadingEntities={entities.isLoading}
        dashboards={dashboards.data ?? []}
        loadingDashboards={dashboards.isLoading}
      />
      <main className="flex-1 overflow-x-hidden">{children}</main>
      <ChatPanel />
    </div>
  )
}

function Sidebar({
  active,
  memberships,
  email,
  entities,
  loadingEntities,
  dashboards,
  loadingDashboards,
}: {
  active: Membership | null
  memberships: Membership[]
  email: string
  entities: Entity[]
  loadingEntities: boolean
  dashboards: Dashboard[]
  loadingDashboards: boolean
}) {
  const match = useMatchRoute()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [newDashboardOpen, setNewDashboardOpen] = useState(false)

  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: async () => {
      await qc.clear()
      navigate({ to: '/login' })
    },
  })

  const isDashboardActive = Boolean(match({ to: '/app', fuzzy: false }))

  return (
    <aside className="sticky top-0 flex h-screen w-64 flex-col border-r border-border bg-card/30">
      <div className="flex items-center justify-between px-4 pt-4">
        <Link to="/app" className="inline-flex items-center gap-2 font-semibold tracking-tight">
          <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Sparkles className="h-3.5 w-3.5" />
          </span>
          steezr<span className="text-primary">_</span>
        </Link>
      </div>

      <div className="mt-4 px-3">
        <OrgSwitcher active={active} memberships={memberships} />
      </div>

      <nav className="mt-6 flex-1 overflow-y-auto px-3 pb-4">
        <NavItem to="/app" icon={<Database className="h-4 w-4" />} active={isDashboardActive}>
          Home
        </NavItem>
        <NavItem
          to="/app/time"
          icon={<Clock className="h-4 w-4" />}
          active={Boolean(match({ to: '/app/time' }))}
        >
          Timesheet
        </NavItem>

        <div className="mt-5 flex items-center justify-between px-3 pb-1">
          <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground/70">
            Dashboards
          </span>
          <button
            onClick={() => setNewDashboardOpen(true)}
            className="rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            title="New dashboard"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>
        {loadingDashboards && (
          <div className="space-y-1 px-2 py-1">
            <div className="h-7 animate-pulse rounded-md bg-muted/40" />
          </div>
        )}
        {!loadingDashboards && dashboards.length === 0 && (
          <p className="px-3 py-2 text-xs text-muted-foreground">
            Create one, or ask Claude to.
          </p>
        )}
        {dashboards.map((d) => {
          const isActive = Boolean(
            match({ to: '/app/dashboards/$slug', params: { slug: d.slug } })
          )
          return (
            <Link
              key={d.id}
              to="/app/dashboards/$slug"
              params={{ slug: d.slug }}
              className={cn(
                'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors hover:bg-accent',
                isActive ? 'bg-accent text-foreground' : 'text-muted-foreground'
              )}
            >
              <LayoutDashboard className="h-3.5 w-3.5" />
              <span className="truncate">{d.name}</span>
            </Link>
          )
        })}

        <SectionLabel>Entities</SectionLabel>

        {loadingEntities && (
          <div className="space-y-1 px-2 py-1">
            <div className="h-7 animate-pulse rounded-md bg-muted/40" />
            <div className="h-7 animate-pulse rounded-md bg-muted/40" />
          </div>
        )}

        {!loadingEntities && entities.length === 0 && (
          <p className="px-3 py-2 text-xs text-muted-foreground">
            None yet. Ask Claude to design one.
          </p>
        )}

        {entities.map((e) => {
          const isActive = Boolean(
            match({ to: '/app/entities/$name', params: { name: e.name } })
          )
          return (
            <Link
              key={e.id}
              to="/app/entities/$name"
              params={{ name: e.name }}
              className={cn(
                'group flex items-center justify-between rounded-md px-3 py-1.5 text-sm',
                'transition-colors hover:bg-accent',
                isActive ? 'bg-accent text-foreground' : 'text-muted-foreground'
              )}
            >
              <span className="truncate">{e.display_name}</span>
              <span className="font-mono text-[10px] text-muted-foreground/70">{e.name}</span>
            </Link>
          )
        })}

        <Link
          to="/app"
          className="mt-3 flex items-center gap-2 rounded-md border border-dashed border-border/80 px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent"
        >
          <Plus className="h-3.5 w-3.5" /> New entity
        </Link>
      </nav>

      <CreateDashboardModal open={newDashboardOpen} onClose={() => setNewDashboardOpen(false)} />
      <div className="border-t border-border px-3 py-2">
        <TimerWidget />
      </div>
      <div className="border-t border-border p-3">
        <div className="flex items-center gap-2 px-2 py-1.5">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/15 text-xs font-medium text-primary">
            {email.charAt(0).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs">{email}</p>
          </div>
          <button
            onClick={() => logout.mutate()}
            title="Log out"
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </div>
    </aside>
  )
}

function NavItem({
  to,
  icon,
  active,
  children,
}: {
  to: string
  icon: ReactNode
  active?: boolean
  children: ReactNode
}) {
  return (
    <Link
      to={to}
      className={cn(
        'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors hover:bg-accent',
        active ? 'bg-accent text-foreground' : 'text-muted-foreground'
      )}
    >
      {icon}
      <span>{children}</span>
    </Link>
  )
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mt-5 px-3 pb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground/70">
      {children}
    </div>
  )
}

function OrgSwitcher({
  active,
  memberships,
}: {
  active: Membership | null
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

  if (!active) {
    return (
      <Link
        to="/orgs"
        className="flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm hover:bg-accent"
      >
        <Building2 className="h-4 w-4" />
        Pick a workspace
      </Link>
    )
  }

  return (
    <div className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm hover:bg-accent"
      >
        <Building2 className="h-4 w-4 text-muted-foreground" />
        <div className="min-w-0 flex-1 text-left">
          <p className="truncate text-sm font-medium">{active.org_name}</p>
          <p className="truncate text-[11px] text-muted-foreground">{active.role}</p>
        </div>
        <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
      </button>
      {open && (
        <div className="absolute left-0 right-0 z-10 mt-1 rounded-md border border-border bg-card shadow-lg">
          <div className="py-1">
            {memberships.map((m) => (
              <button
                key={m.id}
                onClick={() => activate.mutate(m.id)}
                className="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-accent"
              >
                <span className="truncate">{m.org_name}</span>
                {m.id === active.id && <span className="text-primary">●</span>}
              </button>
            ))}
          </div>
          <div className="border-t border-border">
            <Link
              to="/orgs"
              onClick={() => setOpen(false)}
              className="flex items-center gap-2 px-3 py-2 text-sm text-muted-foreground hover:bg-accent"
            >
              <Settings className="h-3.5 w-3.5" />
              Manage workspaces
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}

export function CommandHint() {
  return (
    <div className="flex items-center gap-2 rounded-md border border-border bg-background px-3 py-1.5 text-xs text-muted-foreground">
      <Search className="h-3 w-3" />
      Search
      <kbd className="ml-1 rounded border border-border bg-muted/60 px-1 font-mono text-[10px]">⌘K</kbd>
    </div>
  )
}
