import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useEffect, useState } from 'react'
import { Check, ChevronRight, Pencil, Trash2 } from 'lucide-react'
import { api, ApiError, type Report } from '@/lib/api'
import { useDashboard } from '@/hooks/useDashboards'
import { Button, Card, Input, Label, Pill } from '@/components/ui'
import { Drawer } from '@/components/Drawer'
import { SortableReports } from '@/components/SortableReports'
import { DateRangePicker, type DateRange } from '@/components/DateRangePicker'

export const Route = createFileRoute('/app/dashboards/$slug')({
  component: DashboardPage,
})

function DashboardPage() {
  const { slug } = Route.useParams()
  const dashboard = useDashboard(slug)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [range, setRange] = useState<DateRange>({ presetKey: 'all' })
  const [editingReport, setEditingReport] = useState<Report | null>(null)

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

      <ReportEditDrawer
        report={editingReport}
        onClose={() => setEditingReport(null)}
      />

      {reports.length === 0 ? (
        <Card className="p-6 text-center text-sm text-muted-foreground">
          <p>No reports yet.</p>
          <p className="mt-1">Ask Claude to add one — e.g. "add a revenue-by-month line chart."</p>
        </Card>
      ) : (
        <SortableReports
          dashboard={d}
          range={{ from: range.from, to: range.to }}
          onEditReport={setEditingReport}
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

function ReportEditDrawer({
  report,
  onClose,
}: {
  report: Report | null
  onClose: () => void
}) {
  const qc = useQueryClient()
  const { register, handleSubmit, reset, formState: { isSubmitting } } =
    useForm<{ title: string; subtitle: string; width: number }>()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (report) {
      reset({ title: report.title, subtitle: report.subtitle ?? '', width: report.width })
      setError(null)
    }
  }, [report, reset])

  const mut = useMutation({
    mutationFn: (body: { title: string; subtitle: string; width: number }) =>
      api.updateReport(report!.id, {
        title: body.title,
        subtitle: body.subtitle,
        width: Number(body.width),
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['dashboard'] })
      await qc.invalidateQueries({ queryKey: ['report-exec'] })
      onClose()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <Drawer
      open={report !== null}
      onClose={onClose}
      title="Edit report"
      subtitle={report?.title}
    >
      <form
        className="space-y-5"
        onSubmit={handleSubmit((v) => mut.mutate({ ...v, width: Number(v.width) }))}
      >
        <div className="space-y-1.5">
          <Label>Title</Label>
          <Input {...register('title', { required: true })} />
        </div>
        <div className="space-y-1.5">
          <Label>Subtitle</Label>
          <Input {...register('subtitle')} />
        </div>
        <div className="space-y-1.5">
          <Label>Width (1–12 columns)</Label>
          <Input type="number" min={1} max={12} step={1} {...register('width', { required: true })} />
        </div>
        {error && (
          <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            {error}
          </div>
        )}
        <div className="flex items-center gap-2">
          <Button type="submit" disabled={isSubmitting || mut.isPending}>
            {mut.isPending ? 'Saving…' : 'Save'}
          </Button>
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          To change the chart type or query, ask Claude in the chat — e.g. "turn this into a pie chart" or "group by month instead."
        </p>
      </form>
    </Drawer>
  )
}
