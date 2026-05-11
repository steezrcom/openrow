import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import {
  AlertCircle,
  Calendar,
  ChevronRight,
  CircleCheck,
  CircleDot,
  CircleX,
  Clock,
  Copy,
  ExternalLink,
  Hourglass,
  Pencil,
  Play,
  Power,
  RefreshCw,
  Trash2,
  Webhook,
  Zap,
} from 'lucide-react'
import { api, ApiError, type Flow, type FlowMode, type FlowRun, type FlowRunStatus } from '@/lib/api'
import { Button, Card, Input, Kbd } from '@/components/ui'
import { EmptyState } from '@/components/EmptyState'
import { Skeleton, SkeletonRows } from '@/components/Skeleton'
import { FlowDiagram } from '@/components/FlowDiagram'
import { FlowGoal } from '@/components/FlowGoal'
import { toast } from '@/components/Toast'
import {
  CONNECTOR_LABELS,
  connectorLabel,
  groupTools,
} from '@/lib/flow-meta'
import { ModeBadge, TriggerBadge } from './app.flows'
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
      toast.info('Tok smazán.')
      navigate({ to: '/app/flows' })
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Smazání selhalo.'),
  })

  const patch = useMutation({
    mutationFn: (body: Parameters<typeof api.patchFlow>[1]) => api.patchFlow(id, body),
    onSuccess: (_, body) => {
      qc.invalidateQueries({ queryKey: ['flow', id] })
      if ('mode' in body) toast.success(`Režim změněn na ${body.mode}.`)
      else if ('enabled' in body) toast.info(body.enabled ? 'Tok zapnut.' : 'Tok vypnut.')
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Změna selhala.'),
  })

  const canRun = Boolean(flow.data?.enabled) && !trigger.isPending
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== 'r' && e.key !== 'R') return
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (!canRun) return
      e.preventDefault()
      setError(null)
      trigger.mutate()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [canRun, trigger])

  if (!flow.data) return <FlowDetailSkeleton />

  const f = flow.data
  const tools = groupTools(f.tool_allowlist ?? [])

  return (
    <div className="mx-auto max-w-6xl px-8 py-8">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          <Link to="/app/flows" className="hover:text-foreground">{t('nav.flows')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {f.name}
        </p>
        <div className="mt-2 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight">
              {f.name}
              <ModeBadge mode={f.mode} />
              <TriggerBadge kind={f.trigger_kind} />
              {!f.enabled && (
                <span className="rounded bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                  {t('flows.disabled')}
                </span>
              )}
            </h1>
            {f.description && <p className="mt-1 text-sm text-muted-foreground">{f.description}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              onClick={() => {
                setError(null)
                trigger.mutate()
              }}
              disabled={trigger.isPending || !f.enabled}
            >
              <Play className="mr-1 h-3.5 w-3.5" />
              {trigger.isPending ? t('flows.running') : t('flows.runNow')}
              <span className="ml-2 hidden opacity-60 sm:inline-flex">
                <Kbd>R</Kbd>
              </span>
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                if (confirm(t('flows.confirmDelete'))) del.mutate()
              }}
              title={t('common.delete')}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        {error && (
          <div className="mt-3 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-sm text-destructive">
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <p>{error}</p>
          </div>
        )}
      </header>

      <Card className="mb-4 p-4">
        <FlowDiagram flow={f} />
      </Card>

      <div className="grid gap-4 lg:grid-cols-[3fr,2fr]">
        <div className="space-y-4">
          <GoalCard flow={f} />
          <ToolsCard internal={tools.internal} connectors={tools.connectors} />
        </div>
        <div className="space-y-4">
          <TriggerCard flow={f} />
          <ModeCard
            mode={f.mode}
            onChange={(m) => patch.mutate({ mode: m })}
            disabled={patch.isPending}
          />
          <StatusCard
            enabled={f.enabled}
            onToggle={() => patch.mutate({ enabled: !f.enabled })}
            onEdit={() => navigate({ to: '/app/flows/$id', params: { id: f.id } })}
          />
        </div>
      </div>

      <RunsCard
        flowId={f.id}
        runs={runs.data ?? []}
        loading={runs.isLoading}
        enabled={f.enabled}
        onRunNow={() => {
          setError(null)
          trigger.mutate()
        }}
        runDisabled={trigger.isPending || !f.enabled}
      />
    </div>
  )
}

function FlowDetailSkeleton() {
  return (
    <div className="mx-auto max-w-6xl px-8 py-8">
      <header className="mb-6 space-y-3">
        <Skeleton className="h-3 w-48" />
        <Skeleton className="h-7 w-72" />
      </header>
      <Card className="mb-4 p-4">
        <Skeleton className="h-48 w-full" />
      </Card>
      <div className="grid gap-4 lg:grid-cols-[3fr,2fr]">
        <div className="space-y-4">
          <Card className="p-5">
            <SkeletonRows count={3} height="h-5" />
          </Card>
          <Card className="p-5">
            <SkeletonRows count={4} height="h-5" />
          </Card>
        </div>
        <div className="space-y-4">
          <Card className="p-5">
            <SkeletonRows count={2} height="h-5" />
          </Card>
          <Card className="p-5">
            <SkeletonRows count={2} height="h-5" />
          </Card>
        </div>
      </div>
    </div>
  )
}

// --- top-row cards --------------------------------------------------------

function GoalCard({ flow }: { flow: Flow }) {
  const t = useT()
  return (
    <Card className="p-5">
      <h3 className="mb-3 flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        <Zap className="h-3.5 w-3.5" />
        {t('flows.goal')}
      </h3>
      <FlowGoal goal={flow.goal} />
    </Card>
  )
}

function TriggerCard({ flow }: { flow: Flow }) {
  const t = useT()
  const cfg = flow.trigger_config ?? {}

  return (
    <Card className="p-5">
      <h3 className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        <Calendar className="h-3.5 w-3.5" />
        {t('flows.trigger')}
      </h3>
      {flow.trigger_kind === 'cron' && <CronTriggerBody cfg={cfg} />}
      {flow.trigger_kind === 'entity_event' && <EntityTriggerBody cfg={cfg} />}
      {flow.trigger_kind === 'webhook' && <WebhookTriggerBody flowId={flow.id} />}
      {flow.trigger_kind === 'manual' && (
        <p className="text-sm text-muted-foreground">{t('flows.trigger.manual.hint')}</p>
      )}
    </Card>
  )
}

function CronTriggerBody({ cfg }: { cfg: Record<string, unknown> }) {
  const expr = (cfg.cron as string | undefined) ?? ''
  return (
    <div className="space-y-2">
      <p className="text-sm">{humanCronCs(expr)}</p>
      <p className="font-mono text-[11px] text-muted-foreground">{expr}</p>
    </div>
  )
}

function EntityTriggerBody({ cfg }: { cfg: Record<string, unknown> }) {
  const entity = cfg.entity as string | undefined
  const events = Array.isArray(cfg.events) ? (cfg.events as string[]) : ['insert']
  return (
    <p className="text-sm">
      <span className="font-mono">{entity ?? '?'}</span>
      <span className="text-muted-foreground"> · {events.join(', ')}</span>
    </p>
  )
}

function WebhookTriggerBody({ flowId }: { flowId: string }) {
  const t = useT()
  const [info, setInfo] = useState<{ url: string; token: string } | null>(null)
  const rotate = useMutation({
    mutationFn: () => api.rotateFlowWebhookToken(flowId),
    onSuccess: (r) => setInfo({ url: r.webhook_url, token: r.webhook_token_once }),
  })
  return (
    <div className="space-y-2">
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <Webhook className="h-3.5 w-3.5" />
        {t('flows.webhook.hint')}
      </p>
      {info && (
        <>
          <div className="flex items-center gap-2">
            <Input readOnly value={info.url} onFocus={(e) => e.currentTarget.select()} />
            <Button type="button" variant="ghost" onClick={() => navigator.clipboard.writeText(info.url)} title="Copy">
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </div>
          <p className="text-[11px] text-muted-foreground">{t('flows.webhook.rotatedHint')}</p>
        </>
      )}
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
  )
}

function ModeCard({
  mode,
  onChange,
  disabled,
}: {
  mode: FlowMode
  onChange: (m: FlowMode) => void
  disabled: boolean
}) {
  const t = useT()
  return (
    <Card className="p-5">
      <h3 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {t('flows.mode')}
      </h3>
      <div className="flex flex-wrap gap-1">
        {(['dry_run', 'approve', 'auto'] as FlowMode[]).map((m) => (
          <button
            key={m}
            disabled={disabled || m === mode}
            onClick={() => onChange(m)}
            className={cn(
              'rounded-md border px-3 py-1.5 text-xs transition-colors disabled:cursor-default',
              m === mode
                ? m === 'auto'
                  ? 'border-primary bg-primary/15 text-primary'
                  : m === 'approve'
                    ? 'border-amber-500 bg-amber-500/15 text-amber-600 dark:text-amber-400'
                    : 'border-border bg-muted text-foreground'
                : 'border-border hover:bg-accent',
            )}
          >
            {t(`flows.mode.${m}` as const)}
          </button>
        ))}
      </div>
      <p className="mt-2 text-[11px] text-muted-foreground">{t(`flows.mode.${mode}.hint` as const)}</p>
    </Card>
  )
}

function StatusCard({
  enabled,
  onToggle,
  onEdit,
}: {
  enabled: boolean
  onToggle: () => void
  onEdit: () => void
}) {
  const t = useT()
  return (
    <Card className="p-5">
      <h3 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {t('flows.status')}
      </h3>
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm">
          <span
            className={cn(
              'inline-block h-2 w-2 rounded-full',
              enabled ? 'bg-primary' : 'bg-muted-foreground/40',
            )}
          />
          {enabled ? t('flows.enabled') : t('flows.disabled')}
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" onClick={onToggle} title={enabled ? t('flows.disable') : t('flows.enable')}>
            <Power className="h-3.5 w-3.5" />
          </Button>
          <Button variant="ghost" onClick={onEdit} title={t('common.edit')}>
            <Pencil className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </Card>
  )
}

// --- tools card -----------------------------------------------------------

function ToolsCard({
  internal,
  connectors,
}: {
  internal: string[]
  connectors: { connector: string; reads: string[]; writes: string[] }[]
}) {
  const t = useT()
  if (internal.length === 0 && connectors.length === 0) return null
  return (
    <Card className="p-5">
      <h3 className="mb-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {t('flows.allowlist')}
      </h3>
      {connectors.length > 0 && (
        <div className="space-y-3">
          {connectors.map((c) => (
            <div key={c.connector} className="rounded-md border border-border p-3">
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm font-medium">{connectorLabel(c.connector)}</span>
                <span className="font-mono text-[10px] text-muted-foreground">{c.connector}</span>
              </div>
              <div className="mt-2 flex flex-wrap gap-1">
                {c.reads.map((a) => (
                  <ActionPill key={`r-${a}`} action={a} side="in" />
                ))}
                {c.writes.map((a) => (
                  <ActionPill key={`w-${a}`} action={a} side="out" />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
      {internal.length > 0 && (
        <div className="mt-3">
          <p className="mb-1 text-[10px] uppercase tracking-wider text-muted-foreground">
            {t('flows.allowlist.internal')}
          </p>
          <div className="flex flex-wrap gap-1">
            {internal.map((tool) => (
              <span key={tool} className="rounded border border-border px-2 py-0.5 font-mono text-[11px]">
                {tool}
              </span>
            ))}
          </div>
        </div>
      )}
    </Card>
  )
}

function ActionPill({ action, side }: { action: string; side: 'in' | 'out' }) {
  return (
    <span
      className={cn(
        'rounded border px-2 py-0.5 font-mono text-[11px]',
        side === 'in'
          ? 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300'
          : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
      )}
    >
      {action}
    </span>
  )
}

// --- runs card ------------------------------------------------------------

function RunsCard({
  flowId,
  runs,
  loading,
  enabled,
  onRunNow,
  runDisabled,
}: {
  flowId: string
  runs: FlowRun[]
  loading: boolean
  enabled: boolean
  onRunNow: () => void
  runDisabled: boolean
}) {
  const t = useT()
  return (
    <Card className="mt-4 p-5">
      <h3 className="mb-3 flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        <Clock className="h-3.5 w-3.5" />
        {t('flows.runs')}
      </h3>
      {loading && runs.length === 0 && <SkeletonRows count={3} height="h-12" />}
      {!loading && runs.length === 0 && (
        <EmptyState
          icon={Clock}
          title={t('flows.runs.empty.title')}
          description={t('flows.runs.empty.hint')}
          action={
            enabled
              ? {
                  label: t('flows.runs.empty.action'),
                  onClick: runDisabled ? undefined : onRunNow,
                }
              : undefined
          }
        />
      )}
      {runs.length > 0 && (
        <ol className="space-y-2">
          {runs.map((r) => (
            <RunRow key={r.id} flowId={flowId} run={r} />
          ))}
        </ol>
      )}
    </Card>
  )
}

function RunRow({ flowId, run }: { flowId: string; run: FlowRun }) {
  const started = new Date(run.started_at)
  const finished = run.finished_at ? new Date(run.finished_at) : null
  const durationMs = finished ? finished.getTime() - started.getTime() : null
  return (
    <li>
      <Link
        to="/app/flows/$id/runs/$runId"
        params={{ id: flowId, runId: run.id }}
        className="flex items-center gap-3 rounded-md border border-border bg-card p-3 transition-colors hover:bg-accent"
      >
        <RunStatusIcon status={run.status} />
        <div className="min-w-0 flex-1">
          <p className="text-sm">
            {started.toLocaleString()}
            {durationMs != null && (
              <span className="ml-2 text-xs text-muted-foreground">· {formatDuration(durationMs)}</span>
            )}
          </p>
          {run.error && <p className="mt-0.5 text-xs text-destructive line-clamp-1">{run.error}</p>}
        </div>
        <ExternalLink className="h-3.5 w-3.5 text-muted-foreground" />
      </Link>
    </li>
  )
}

function RunStatusIcon({ status }: { status: FlowRunStatus }) {
  const cls = 'h-4 w-4 shrink-0'
  switch (status) {
    case 'succeeded':
      return <CircleCheck className={cn(cls, 'text-primary')} />
    case 'failed':
      return <CircleX className={cn(cls, 'text-destructive')} />
    case 'awaiting_approval':
      return <Hourglass className={cn(cls, 'text-amber-500')} />
    case 'running':
      return <CircleDot className={cn(cls, 'text-blue-500 animate-pulse')} />
    case 'queued':
    default:
      return <CircleDot className={cn(cls, 'text-muted-foreground')} />
  }
}

// --- helpers --------------------------------------------------------------

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s} s`
  const m = Math.floor(s / 60)
  const rs = s % 60
  return rs === 0 ? `${m} min` : `${m} min ${rs} s`
}

// Inline copy of the cron humaniser used for the trigger card. Keeping
// it here so the detail view doesn't need to import scheduleLabel and
// strip the non-cron branches.
function humanCronCs(cron: string): string {
  if (!cron) return 'cron'
  const parts = cron.split(/\s+/)
  if (parts.length < 5) return cron
  const [min, hour, dom, mon, dow] = parts
  if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') return `Každých ${min.slice(2)} minut`
  if (min === '0' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') return `Každé ${hour.slice(2)} hodiny`
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '*') return `Denně v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1') return `Každé pondělí v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1-5') return `Pracovní dny v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '5') return `Každý pátek v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  if (!min.includes('*') && hour === '*' && dom === '*' && mon === '*' && dow === '*') return `Každou hodinu v :${min.padStart(2, '0')}`
  return cron
}

// Suppress unused-import lint when CONNECTOR_LABELS is re-exported only.
void CONNECTOR_LABELS
