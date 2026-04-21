import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Plus, Workflow } from 'lucide-react'
import { api, type Flow } from '@/lib/api'
import { Button, Card } from '@/components/ui'
import { ModeBadge, TriggerBadge } from './app.flows'
import { useT } from '@/lib/i18n'

export const Route = createFileRoute('/app/flows/')({
  component: FlowsPage,
})

function FlowsPage() {
  const t = useT()
  const navigate = useNavigate()
  const flows = useQuery({ queryKey: ['flows'], queryFn: api.listFlows })

  return (
    <div className="mx-auto max-w-5xl px-8 py-10">
      <header className="mb-6 flex items-start justify-between gap-4">
        <div>
          <p className="text-xs text-muted-foreground">
            <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
            <ChevronRight className="inline h-3 w-3 mx-1" />
            {t('nav.flows')}
          </p>
          <h1 className="mt-2 flex items-center gap-3 text-2xl font-semibold tracking-tight">
            <Workflow className="h-5 w-5 text-primary" />
            {t('nav.flows')}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('flows.hint')}</p>
        </div>
        <Button onClick={() => navigate({ to: '/app/flows/new' })}>
          <Plus className="mr-1 h-3.5 w-3.5" />
          {t('flows.new')}
        </Button>
      </header>

      {flows.isLoading && <p className="text-sm text-muted-foreground">{t('common.loading')}</p>}

      {!flows.isLoading && (flows.data ?? []).length === 0 && (
        <Card className="p-8 text-center">
          <Workflow className="mx-auto mb-3 h-6 w-6 text-muted-foreground" />
          <h2 className="font-medium">{t('flows.empty.title')}</h2>
          <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
            {t('flows.empty.hint')}
          </p>
        </Card>
      )}

      <div className="grid gap-3">
        {(flows.data ?? []).map((f) => (
          <FlowRow key={f.id} flow={f} />
        ))}
      </div>
    </div>
  )
}

function FlowRow({ flow }: { flow: Flow }) {
  const t = useT()
  return (
    <Link
      to="/app/flows/$id"
      params={{ id: flow.id }}
      className="group flex items-center justify-between gap-3 rounded-md border border-border bg-card p-4 transition-colors hover:bg-accent hover:border-primary/40"
    >
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{flow.name}</span>
          <ModeBadge mode={flow.mode} />
          <TriggerBadge kind={flow.trigger_kind} />
          {!flow.enabled && (
            <span className="rounded bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
              {t('flows.disabled')}
            </span>
          )}
        </div>
        {flow.description && (
          <p className="mt-1 text-xs text-muted-foreground">{flow.description}</p>
        )}
        <p className="mt-1 text-xs text-muted-foreground/80 line-clamp-1">{flow.goal}</p>
      </div>
      <ChevronRight className="h-4 w-4 text-muted-foreground" />
    </Link>
  )
}
