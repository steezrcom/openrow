import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Clock } from 'lucide-react'
import { api } from '@/lib/api'
import { Card } from '@/components/ui'
import { FlowRunSteps } from '@/components/FlowRunSteps'
import { RunStatusBadge } from './app.flows.$id'
import { useT } from '@/lib/i18n'

export const Route = createFileRoute('/app/flows/$id/runs/$runId')({
  component: FlowRunPage,
})

function FlowRunPage() {
  const { id, runId } = Route.useParams()
  const t = useT()
  const data = useQuery({
    queryKey: ['flow-run', runId],
    queryFn: () => api.getFlowRun(runId),
    // Poll while running/awaiting — cheap, keeps the UI live.
    refetchInterval: (q) => {
      const status = q.state.data?.run?.status
      return status === 'running' || status === 'queued' ? 1500 : false
    },
  })

  const flow = useQuery({ queryKey: ['flow', id], queryFn: () => api.getFlow(id) })

  if (!data.data) return <div className="px-8 py-10 text-sm text-muted-foreground">{t('common.loading')}</div>

  const { run, steps } = data.data
  const started = new Date(run.started_at)
  const finished = run.finished_at ? new Date(run.finished_at) : null
  const duration = finished ? finished.getTime() - started.getTime() : null

  return (
    <div className="mx-auto max-w-4xl px-8 py-8">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          <Link to="/app/flows" className="hover:text-foreground">{t('nav.flows')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          <Link to="/app/flows/$id" params={{ id }} className="hover:text-foreground">
            {flow.data?.name ?? '…'}
          </Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          Run
        </p>
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">
            {started.toLocaleString()}
          </h1>
          <RunStatusBadge status={run.status} />
          {duration != null && (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <Clock className="h-3 w-3" />
              {formatDuration(duration)}
            </span>
          )}
          <span className="text-xs text-muted-foreground">· {steps.length} {steps.length === 1 ? 'step' : 'steps'}</span>
        </div>
        {run.error && (
          <Card className="mt-4 border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
            {run.error}
          </Card>
        )}
      </header>

      {steps.length === 0 ? (
        <Card className="p-6 text-center text-sm text-muted-foreground">
          No steps recorded yet.
        </Card>
      ) : (
        <FlowRunSteps steps={steps} />
      )}
    </div>
  )
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s} s`
  const m = Math.floor(s / 60)
  const rs = s % 60
  return rs === 0 ? `${m} min` : `${m} min ${rs} s`
}
