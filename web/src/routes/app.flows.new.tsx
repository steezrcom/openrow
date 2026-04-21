import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { ChevronRight } from 'lucide-react'
import { api, ApiError, type FlowMode } from '@/lib/api'
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
}

function NewFlowPage() {
  const t = useT()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const tools = useQuery({ queryKey: ['flow-tools'], queryFn: api.listFlowTools })
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)

  const { register, handleSubmit, formState: { isSubmitting } } = useForm<FormValues>({
    defaultValues: { name: '', description: '', goal: '', mode: 'dry_run' },
  })

  const save = useMutation({
    mutationFn: (v: FormValues) =>
      api.createFlow({
        name: v.name,
        description: v.description,
        goal: v.goal,
        trigger_kind: 'manual',
        tool_allowlist: Array.from(selected),
        mode: v.mode,
      }),
    onSuccess: (flow) => {
      qc.invalidateQueries({ queryKey: ['flows'] })
      navigate({ to: '/app/flows/$id', params: { id: flow.id } })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

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
