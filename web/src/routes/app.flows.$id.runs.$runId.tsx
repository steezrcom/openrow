import { createFileRoute, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import { api, type FlowRunStep } from '@/lib/api'
import { Card } from '@/components/ui'
import { RunStatusBadge } from './app.flows.$id'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

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

  return (
    <div className="mx-auto max-w-4xl px-8 py-10">
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
        <div className="mt-2 flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">
            {new Date(run.started_at).toLocaleString()}
          </h1>
          <RunStatusBadge status={run.status} />
        </div>
        {run.error && <p className="mt-1 text-sm text-destructive">{run.error}</p>}
      </header>

      <Card className="p-4">
        <div className="space-y-2">
          {steps.length === 0 && <p className="text-sm text-muted-foreground">—</p>}
          {steps.map((s) => <StepRow key={s.id} step={s} />)}
        </div>
      </Card>
    </div>
  )
}

function StepRow({ step }: { step: FlowRunStep }) {
  const t = useT()
  const c = step.content as Record<string, unknown>
  switch (step.kind) {
    case 'agent_message':
      return (
        <div className="rounded-md border border-border bg-card/50 p-3">
          <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('flows.step.agent')}
          </div>
          <p className="whitespace-pre-wrap text-sm">{String(c.text ?? '')}</p>
        </div>
      )
    case 'tool_call':
      return (
        <div className="rounded-md border border-border bg-muted/10 p-3">
          <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('flows.step.call')} <span className="font-mono">{String(c.name)}</span>
          </div>
          <pre className="whitespace-pre-wrap text-xs font-mono text-muted-foreground">
            {JSON.stringify(c.input, null, 2)}
          </pre>
        </div>
      )
    case 'tool_result': {
      const err = c.error != null
      return (
        <div className={cn('rounded-md border p-3', err ? 'border-destructive/40 bg-destructive/5' : 'border-border bg-muted/5')}>
          <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('flows.step.result')}
          </div>
          <p className="whitespace-pre-wrap text-xs font-mono">{String(c.result ?? '')}</p>
          {err && <p className="mt-1 text-xs text-destructive">{String(c.error)}</p>}
        </div>
      )
    }
    case 'mutation_blocked':
      return (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
          <div className="mb-1 font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
            {t('flows.step.blocked')}
          </div>
          <p className="font-mono text-muted-foreground">
            {String(c.name)} — {String(c.reason)}
          </p>
          {c.synthetic ? <p className="mt-1">{String(c.synthetic)}</p> : null}
        </div>
      )
    case 'approval_requested':
      return (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
          <div className="mb-1 font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
            {t('flows.step.approval')}
          </div>
          <p className="font-mono text-muted-foreground">{String(c.name)}</p>
          <p className="mt-1">
            <Link to="/app/approvals" className="text-primary hover:underline">
              {t('flows.step.approval.goto')}
            </Link>
          </p>
        </div>
      )
  }
  return null
}
