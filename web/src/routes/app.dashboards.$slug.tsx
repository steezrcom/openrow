import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronRight, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { useDashboard } from '@/hooks/useDashboards'
import { Button, Card, Pill } from '@/components/ui'
import { ReportCard } from '@/components/ReportCard'
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
            <h1 className="truncate text-2xl font-semibold tracking-tight">{d.name}</h1>
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

      {reports.length === 0 ? (
        <Card className="p-6 text-center text-sm text-muted-foreground">
          <p>No reports yet.</p>
          <p className="mt-1">Ask Claude to add one — e.g. "add a revenue-by-month line chart."</p>
        </Card>
      ) : (
        <div className="grid grid-cols-12 gap-4">
          {reports.map((r) => (
            <ReportCard
              key={r.id}
              report={r}
              range={{ from: range.from, to: range.to }}
            />
          ))}
        </div>
      )}
    </div>
  )
}
