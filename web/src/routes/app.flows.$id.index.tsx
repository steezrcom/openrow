import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronRight, Copy, Play, RefreshCw, Trash2 } from 'lucide-react'
import { api, ApiError, type Flow, type FlowMode } from '@/lib/api'
import { Button, Card, Input } from '@/components/ui'
import { ModeBadge, TriggerBadge } from './app.flows'
import { RunStatusBadge } from './app.flows.$id'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/flows/$id/')({
  component: FlowDetailPage,
})

function FlowDetailPage() {
  const { id } = Route.useParams()
  const t = useT()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)

  const flow = useQuery({ queryKey: ['flow', id], queryFn: () => api.getFlow(id) })
  // Poll runs so status transitions (queued → running → succeeded /
  // awaiting_approval) show up without a manual refresh. 3s is fast
  // enough to feel live and cheap enough to run indefinitely.
  const runs = useQuery({
    queryKey: ['flow-runs', id],
    queryFn: () => api.listFlowRuns(id),
    refetchInterval: 3000,
  })

  const trigger = useMutation({
    mutationFn: () => api.triggerFlow(id),
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['flow-runs', id] })
      qc.invalidateQueries({ queryKey: ['flow-approvals'] })
      if (r.error) setError(r.error)
      if (r.run) navigate({ to: '/app/flows/$id/runs/$runId', params: { id, runId: r.run.id } })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  const del = useMutation({
    mutationFn: () => api.deleteFlow(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['flows'] })
      navigate({ to: '/app/flows' })
    },
  })

  const patch = useMutation({
    mutationFn: (body: Parameters<typeof api.patchFlow>[1]) => api.patchFlow(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['flow', id] }),
  })

  if (!flow.data) return <div className="px-8 py-10 text-sm text-muted-foreground">{t('common.loading')}</div>

  const f = flow.data

  return (
    <div className="mx-auto max-w-5xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          <Link to="/app/flows" className="hover:text-foreground">{t('nav.flows')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {f.name}
        </p>
        <div className="mt-2 flex items-start justify-between gap-4">
          <div>
            <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
              {f.name}
              <ModeBadge mode={f.mode} />
              <TriggerBadge kind={f.trigger_kind} />
            </h1>
            {f.description && <p className="mt-1 text-sm text-muted-foreground">{f.description}</p>}
          </div>
          <div className="flex items-center gap-2">
            <Button
              onClick={() => {
                setError(null)
                trigger.mutate()
              }}
              disabled={trigger.isPending || !f.enabled}
            >
              <Play className="mr-1 h-3.5 w-3.5" />
              {trigger.isPending ? t('flows.running') : t('flows.runNow')}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                if (confirm(t('flows.confirmDelete'))) del.mutate()
              }}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        {error && <p className="mt-2 text-sm text-destructive">{error}</p>}
      </header>

      <div className="grid gap-4 md:grid-cols-[2fr,3fr]">
        <Card className="p-5 space-y-4">
          <div>
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.goal')}</h3>
            <p className="mt-1 whitespace-pre-wrap text-sm">{f.goal}</p>
          </div>
          <TriggerConfigBlock flow={f} />
          {f.trigger_kind === 'webhook' && <WebhookBlock flowId={f.id} />}
          <div>
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.mode')}</h3>
            <div className="mt-2 flex flex-wrap gap-1">
              {(['dry_run', 'approve', 'auto'] as FlowMode[]).map((m) => (
                <button
                  key={m}
                  onClick={() => {
                    if (m !== f.mode) patch.mutate({ mode: m })
                  }}
                  className={cn(
                    'rounded-md border border-border px-2 py-1 text-xs',
                    m === f.mode ? 'bg-primary/15 text-primary border-primary/40' : 'hover:bg-accent'
                  )}
                >
                  {t(`flows.mode.${m}` as const)}
                </button>
              ))}
            </div>
          </div>
          <div>
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.allowlist')}</h3>
            <div className="mt-2 flex flex-wrap gap-1">
              {f.tool_allowlist.map((tool) => (
                <span key={tool} className="rounded border border-border px-2 py-0.5 font-mono text-[11px]">
                  {tool}
                </span>
              ))}
            </div>
          </div>
          <div>
            <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.enabled')}</h3>
            <button
              onClick={() => patch.mutate({ enabled: !f.enabled })}
              className="mt-1 text-sm text-primary hover:underline"
            >
              {f.enabled ? t('flows.disable') : t('flows.enable')}
            </button>
          </div>
        </Card>

        <Card className="p-5">
          <h3 className="mb-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.runs')}</h3>
          {(runs.data ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">{t('flows.runs.empty')}</p>
          )}
          <div className="divide-y divide-border">
            {(runs.data ?? []).map((r) => (
              <Link
                key={r.id}
                to="/app/flows/$id/runs/$runId"
                params={{ id, runId: r.id }}
                className="flex items-center justify-between gap-3 py-2 hover:bg-accent -mx-2 px-2 rounded"
              >
                <div className="min-w-0 flex-1">
                  <p className="text-xs text-muted-foreground">
                    {new Date(r.started_at).toLocaleString()}
                  </p>
                  {r.error && <p className="text-xs text-destructive line-clamp-1">{r.error}</p>}
                </div>
                <RunStatusBadge status={r.status} />
              </Link>
            ))}
          </div>
        </Card>
      </div>
    </div>
  )
}

function TriggerConfigBlock({ flow }: { flow: Flow }) {
  const t = useT()
  const cfg = flow.trigger_config
  if (flow.trigger_kind === 'entity_event') {
    const entity = cfg.entity as string | undefined
    const events = Array.isArray(cfg.events) ? (cfg.events as string[]) : ['insert']
    return (
      <div>
        <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.trigger')}</h3>
        <p className="mt-1 text-sm">
          <span className="font-mono">{entity ?? '?'}</span>
          <span className="text-muted-foreground"> · {events.join(', ')}</span>
        </p>
      </div>
    )
  }
  if (flow.trigger_kind === 'cron') {
    const expr = (cfg.cron as string | undefined) ?? ''
    return (
      <div>
        <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{t('flows.trigger.cron.expression')}</h3>
        <p className="mt-1 font-mono text-sm">{expr}</p>
      </div>
    )
  }
  return null
}

function WebhookBlock({ flowId }: { flowId: string }) {
  const t = useT()
  const [info, setInfo] = useState<{ url: string; token: string } | null>(null)
  const rotate = useMutation({
    mutationFn: () => api.rotateFlowWebhookToken(flowId),
    onSuccess: (r) => setInfo({ url: r.webhook_url, token: r.webhook_token_once }),
  })
  return (
    <div>
      <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Webhook</h3>
      {info ? (
        <div className="mt-1 space-y-2">
          <div className="flex items-center gap-2">
            <Input readOnly value={info.url} onFocus={(e) => e.currentTarget.select()} />
            <Button type="button" variant="ghost" onClick={() => navigator.clipboard.writeText(info.url)}>
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </div>
          <p className="text-[11px] text-muted-foreground">{t('flows.webhook.rotatedHint')}</p>
        </div>
      ) : (
        <div className="mt-1">
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              if (confirm(t('flows.webhook.rotateConfirm'))) rotate.mutate()
            }}
            disabled={rotate.isPending}
          >
            <RefreshCw className="mr-1 h-3.5 w-3.5" />
            {t('flows.webhook.rotate')}
          </Button>
        </div>
      )}
    </div>
  )
}
