import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { ChevronRight, Copy } from 'lucide-react'
import { api, ApiError, type FlowMode, type FlowTriggerKind } from '@/lib/api'
import { useEntities } from '@/hooks/useEntities'
import { Button, Card, Input, Label, Textarea } from '@/components/ui'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/flows/new')({
  component: NewFlowPage,
})

type FormValues = {
  name: string
  description: string
  goal: string
  mode: FlowMode
  trigger_kind: FlowTriggerKind
  entity: string
  event_insert: boolean
  event_update: boolean
  event_delete: boolean
  cron: string
}

function NewFlowPage() {
  const t = useT()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const tools = useQuery({ queryKey: ['flow-tools'], queryFn: api.listFlowTools })
  const entities = useEntities()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [webhookInfo, setWebhookInfo] = useState<{ url: string; token: string; flowId: string } | null>(null)

  const { register, handleSubmit, watch, formState: { isSubmitting } } = useForm<FormValues>({
    defaultValues: {
      name: '', description: '', goal: '', mode: 'dry_run',
      trigger_kind: 'manual', entity: '',
      event_insert: true, event_update: false, event_delete: false,
      cron: '0 9 * * *',
    },
  })
  const triggerKind = watch('trigger_kind')

  const save = useMutation({
    mutationFn: (v: FormValues) => {
      const triggerConfig: Record<string, unknown> = {}
      if (v.trigger_kind === 'entity_event') {
        triggerConfig.entity = v.entity
        const events: string[] = []
        if (v.event_insert) events.push('insert')
        if (v.event_update) events.push('update')
        if (v.event_delete) events.push('delete')
        triggerConfig.events = events
      }
      if (v.trigger_kind === 'cron') {
        triggerConfig.cron = v.cron
      }
      return api.createFlow({
        name: v.name,
        description: v.description,
        goal: v.goal,
        trigger_kind: v.trigger_kind,
        trigger_config: triggerConfig,
        tool_allowlist: Array.from(selected),
        mode: v.mode,
      })
    },
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['flows'] })
      if (res.webhook_url && res.webhook_token_once) {
        // Show the one-time token before navigating away.
        setWebhookInfo({ url: res.webhook_url, token: res.webhook_token_once, flowId: res.flow.id })
        return
      }
      navigate({ to: '/app/flows/$id', params: { id: res.flow.id } })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  if (webhookInfo) {
    return <WebhookDisplayOnce info={webhookInfo} onDone={() => navigate({ to: '/app/flows/$id', params: { id: webhookInfo.flowId } })} />
  }

  function toggle(name: string) {
    const next = new Set(selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    setSelected(next)
  }

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          <Link to="/app/flows" className="hover:text-foreground">{t('nav.flows')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {t('flows.new')}
        </p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">{t('flows.new')}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('flows.new.hint')}</p>
      </header>

      <Card className="p-6">
        <form
          className="space-y-5"
          onSubmit={handleSubmit((v) => {
            setError(null)
            if (selected.size === 0) {
              setError(t('flows.new.allowlistRequired'))
              return
            }
            save.mutate(v)
          })}
        >
          <div className="space-y-2">
            <Label htmlFor="name">{t('flows.name')}</Label>
            <Input id="name" {...register('name', { required: true })} placeholder="Pair payment with invoice" />
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">{t('flows.description')}</Label>
            <Input id="description" {...register('description')} placeholder={t('flows.description.placeholder')} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="goal">{t('flows.goal')}</Label>
            <Textarea
              id="goal"
              rows={4}
              {...register('goal', { required: true, minLength: 10 })}
              placeholder={t('flows.goal.placeholder')}
            />
          </div>

          <div className="space-y-2">
            <Label>{t('flows.trigger')}</Label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {(['manual', 'entity_event', 'webhook', 'cron'] as FlowTriggerKind[]).map((k) => (
                <label
                  key={k}
                  className="flex cursor-pointer items-start gap-2 rounded-md border border-border bg-background p-3 hover:bg-accent"
                >
                  <input type="radio" value={k} {...register('trigger_kind')} className="mt-1" />
                  <div className="min-w-0">
                    <div className="text-sm font-medium">{t(`flows.trigger.${k}` as const)}</div>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t(`flows.trigger.${k}.hint` as const)}
                    </p>
                  </div>
                </label>
              ))}
            </div>
          </div>

          {triggerKind === 'entity_event' && (
            <div className="space-y-2 rounded-md border border-border bg-muted/10 p-3">
              <Label htmlFor="entity">{t('flows.trigger.entity')}</Label>
              <select
                id="entity"
                {...register('entity', { required: triggerKind === 'entity_event' })}
                className="flex h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
              >
                <option value="">{t('flows.trigger.entity.pick')}</option>
                {(entities.data ?? []).map((e) => (
                  <option key={e.name} value={e.name}>{e.display_name} ({e.name})</option>
                ))}
              </select>
              <div className="flex flex-wrap gap-3">
                <label className="flex items-center gap-1.5 text-xs">
                  <input type="checkbox" {...register('event_insert')} /> insert
                </label>
                <label className="flex items-center gap-1.5 text-xs">
                  <input type="checkbox" {...register('event_update')} /> update
                </label>
                <label className="flex items-center gap-1.5 text-xs">
                  <input type="checkbox" {...register('event_delete')} /> delete
                </label>
              </div>
            </div>
          )}

          {triggerKind === 'webhook' && (
            <div className="rounded-md border border-border bg-muted/10 p-3 text-xs text-muted-foreground">
              {t('flows.trigger.webhook.hint.create')}
            </div>
          )}

          {triggerKind === 'cron' && (
            <div className="space-y-2 rounded-md border border-border bg-muted/10 p-3">
              <Label htmlFor="cron">{t('flows.trigger.cron.expression')}</Label>
              <Input
                id="cron"
                {...register('cron', { required: triggerKind === 'cron' })}
                placeholder="0 9 * * *"
                className="font-mono"
              />
              <p className="text-xs text-muted-foreground">{t('flows.trigger.cron.hint.create')}</p>
            </div>
          )}

          <div className="space-y-2">
            <Label>{t('flows.mode')}</Label>
            <div className="grid grid-cols-3 gap-2">
              {(['dry_run', 'approve', 'auto'] as FlowMode[]).map((m) => (
                <ModeOption key={m} mode={m} register={register} />
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t('flows.allowlist')}</Label>
            <p className="text-xs text-muted-foreground">{t('flows.allowlist.hint')}</p>
            <div className="grid max-h-96 grid-cols-1 gap-1 overflow-y-auto rounded-md border border-border bg-background p-2 sm:grid-cols-2">
              {(tools.data ?? []).map((tool) => (
                <label
                  key={tool.name}
                  className="flex cursor-pointer items-start gap-2 rounded px-2 py-1.5 hover:bg-accent"
                >
                  <input
                    type="checkbox"
                    className="mt-0.5 h-3.5 w-3.5"
                    checked={selected.has(tool.name)}
                    onChange={() => toggle(tool.name)}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <span className="font-mono text-xs">{tool.name}</span>
                      {tool.mutates && (
                        <span className="rounded bg-amber-500/20 px-1 py-0.5 text-[9px] font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
                          {t('flows.tool.writes')}
                        </span>
                      )}
                    </div>
                    <p className="mt-0.5 text-[11px] text-muted-foreground line-clamp-2">
                      {tool.description}
                    </p>
                  </div>
                </label>
              ))}
            </div>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex items-center gap-2">
            <Button type="submit" disabled={isSubmitting || save.isPending}>
              {save.isPending ? t('common.loading') : t('common.create')}
            </Button>
            <Link to="/app/flows">
              <Button type="button" variant="ghost">{t('common.cancel')}</Button>
            </Link>
          </div>
        </form>
      </Card>
    </div>
  )
}

function WebhookDisplayOnce({
  info,
  onDone,
}: {
  info: { url: string; token: string; flowId: string }
  onDone: () => void
}) {
  const t = useT()
  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <Card className="p-6 space-y-4">
        <h1 className="text-xl font-semibold tracking-tight">{t('flows.webhook.created')}</h1>
        <p className="text-sm text-muted-foreground">{t('flows.webhook.createdHint')}</p>
        <div className="space-y-1">
          <Label>{t('flows.webhook.url')}</Label>
          <div className="flex items-center gap-2">
            <Input readOnly value={info.url} onFocus={(e) => e.currentTarget.select()} />
            <Button type="button" variant="ghost" onClick={() => navigator.clipboard.writeText(info.url)}>
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        <Button onClick={onDone}>{t('flows.webhook.done')}</Button>
      </Card>
    </div>
  )
}

function ModeOption({ mode, register }: { mode: FlowMode; register: ReturnType<typeof useForm<FormValues>>['register'] }) {
  const t = useT()
  return (
    <label
      className={cn(
        'flex cursor-pointer items-start gap-2 rounded-md border border-border bg-background p-3 hover:bg-accent',
      )}
    >
      <input type="radio" value={mode} {...register('mode')} className="mt-1" />
      <div className="min-w-0">
        <div className="text-sm font-medium">{t(`flows.mode.${mode}` as const)}</div>
        <p className="mt-0.5 text-xs text-muted-foreground">{t(`flows.mode.${mode}.hint` as const)}</p>
      </div>
    </label>
  )
}
