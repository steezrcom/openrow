import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Check, ChevronRight, Pencil, Plus, Trash2 } from 'lucide-react'
import { api, type Report } from '@/lib/api'
import { useDashboard } from '@/hooks/useDashboards'
import { Button, Card, Input, Pill } from '@/components/ui'
import { SortableReports } from '@/components/SortableReports'
import { DateRangePicker, type DateRange } from '@/components/DateRangePicker'
import { ReportEditor } from '@/components/ReportEditor'

export const Route = createFileRoute('/app/dashboards/$slug')({
  component: DashboardPage,
})

function DashboardPage() {
  const { slug } = Route.useParams()
  const dashboard = useDashboard(slug)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [range, setRange] = useState<DateRange>({ presetKey: 'all' })
  const [editorMode, setEditorMode] = useState<
    { kind: 'create'; slug: string } | { kind: 'edit'; report: Report } | null
  >(null)

  const del = useMutation({
    mutationFn: () => api.deleteDashboard(slug),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['dashboards'] })
      navigate({ to: '/app' })
    },
  })

  if (dashboard.isLoading) {
    return (
      <div className="px-8 py-10">
        <div className="mx-auto max-w-6xl space-y-6">
          <div className="h-8 w-64 animate-pulse rounded-md bg-muted/30" />
          <div className="grid grid-cols-12 gap-4">
            <div className="col-span-6 h-48 animate-pulse rounded-md bg-muted/30" />
            <div className="col-span-6 h-48 animate-pulse rounded-md bg-muted/30" />
          </div>
        </div>
      </div>
    )
  }
  if (dashboard.error || !dashboard.data) {
    return (
      <div className="mx-auto max-w-6xl px-8 py-10">
        <Card className="border-destructive/30 bg-destructive/5 p-6 text-sm text-destructive">
          {dashboard.error instanceof Error ? dashboard.error.message : 'Dashboard not found'}
        </Card>
      </div>
    )
  }
  const d = dashboard.data
  const reports = d.reports ?? []

  return (
    <div className="px-8 py-8">
      <header className="mb-6 flex items-start justify-between gap-6">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Link to="/app" className="hover:text-foreground">Home</Link>
            <ChevronRight className="h-3 w-3" />
            <span>Dashboards</span>
          </div>
          <div className="mt-2 flex items-center gap-3">
            <DashboardHeading dashboard={d} />
            <Pill>{d.slug}</Pill>
          </div>
          {d.description && (
            <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{d.description}</p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <DateRangePicker value={range} onChange={setRange} />
          <Button onClick={() => setEditorMode({ kind: 'create', slug })}>
            <Plus className="mr-1 h-3.5 w-3.5" /> Add report
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              if (confirm(`Delete "${d.name}" and all its reports?`)) del.mutate()
            }}
          >
            <Trash2 className="mr-1 h-3.5 w-3.5" /> Delete
          </Button>
        </div>
      </header>

      <ReportEditor mode={editorMode} onClose={() => setEditorMode(null)} />

      {reports.length === 0 ? (
        <Card className="p-6 text-center text-sm text-muted-foreground">
          <p>No reports yet.</p>
          <p className="mt-1">Ask Claude to add one — e.g. "add a revenue-by-month line chart."</p>
        </Card>
      ) : (
        <SortableReports
          dashboard={d}
          range={{ from: range.from, to: range.to }}
          onEditReport={(r) => setEditorMode({ kind: 'edit', report: r })}
        />
      )}
    </div>
  )
}

function DashboardHeading({ dashboard }: { dashboard: { slug: string; name: string } }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(dashboard.name)
  useEffect(() => setName(dashboard.name), [dashboard.name])

  const mut = useMutation({
    mutationFn: (newName: string) => api.updateDashboard(dashboard.slug, { name: newName }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['dashboard', dashboard.slug] })
      qc.invalidateQueries({ queryKey: ['dashboards'] })
      setEditing(false)
    },
  })

  if (!editing) {
    return (
      <div className="group flex items-center gap-2">
        <h1 className="truncate text-2xl font-semibold tracking-tight">{dashboard.name}</h1>
        <button
          onClick={() => setEditing(true)}
          className="invisible rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground group-hover:visible"
          title="Rename"
        >
          <Pencil className="h-3.5 w-3.5" />
        </button>
      </div>
    )
  }
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        if (name.trim() && name !== dashboard.name) mut.mutate(name.trim())
        else setEditing(false)
      }}
      className="flex items-center gap-2"
    >
      <Input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onBlur={() => {
          if (name.trim() && name !== dashboard.name) mut.mutate(name.trim())
          else setEditing(false)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setName(dashboard.name)
            setEditing(false)
          }
        }}
        className="h-9 text-xl font-semibold"
      />
      <button type="submit" className="rounded-md p-1.5 text-primary hover:bg-accent">
        <Check className="h-4 w-4" />
      </button>
    </form>
  )
}

