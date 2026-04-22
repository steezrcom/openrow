import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { ExternalLink, Trash2 } from 'lucide-react'
import {
  api,
  ApiError,
  type Connector,
  type ConnectorConfigSafe,
} from '@/lib/api'
import { Button, Input, Label } from '@/components/ui'
import { Modal } from '@/components/Modal'
import { SettingsShell } from '@/components/SettingsShell'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/connectors')({
  component: ConnectorsPage,
})

function ConnectorsPage() {
  const t = useT()
  const connectors = useQuery({ queryKey: ['connectors'], queryFn: api.listConnectors })
  const configs = useQuery({
    queryKey: ['connector-configs'],
    queryFn: api.listConnectorConfigs,
  })
  const [active, setActive] = useState<Connector | null>(null)

  const configByID = new Map<string, ConnectorConfigSafe>()
  for (const c of configs.data ?? []) configByID.set(c.connector_id, c)

  return (
    <SettingsShell active="connectors" hint={t('settings.connectors.hint')}>
      <div className="grid gap-3 sm:grid-cols-2">
        {(connectors.data ?? []).map((c) => (
          <ConnectorCard
            key={c.id}
            connector={c}
            config={configByID.get(c.id) ?? null}
            onClick={() => {
              if (c.status === 'available') setActive(c)
            }}
          />
        ))}
      </div>

      {active && (
        <ConfigureModal
          connector={active}
          existing={configByID.get(active.id) ?? null}
          onClose={() => setActive(null)}
        />
      )}
    </SettingsShell>
  )
}

function ConnectorCard({
  connector,
  config,
  onClick,
}: {
  connector: Connector
  config: ConnectorConfigSafe | null
  onClick: () => void
}) {
  const t = useT()
  const comingSoon = connector.status === 'coming_soon'
  const installed = Boolean(config)

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={comingSoon}
      className={cn(
        'group relative flex items-start gap-3 rounded-md border border-border bg-card p-4 text-left transition-colors',
        comingSoon
          ? 'cursor-not-allowed opacity-70'
          : 'hover:bg-accent hover:border-primary/40'
      )}
    >
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary/10 text-sm font-semibold text-primary">
        {connector.name.charAt(0)}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{connector.name}</span>
          <span className="rounded-full border border-border px-2 py-0.5 text-[10px] uppercase tracking-wider text-muted-foreground">
            {connector.category}
          </span>
        </div>
        <p className="mt-1 text-xs text-muted-foreground">{connector.description}</p>
        <div className="mt-2 flex items-center gap-2">
          {comingSoon && (
            <span className="rounded bg-muted px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
              {t('connectors.status.comingSoon')}
            </span>
          )}
          {installed && (
            <span className="rounded bg-primary/15 px-2 py-0.5 text-[10px] font-medium text-primary">
              {t('connectors.status.installed')}
            </span>
          )}
        </div>
      </div>
    </button>
  )
}

function ConfigureModal({
  connector,
  existing,
  onClose,
}: {
  connector: Connector
  existing: ConnectorConfigSafe | null
  onClose: () => void
}) {
  const qc = useQueryClient()
  const t = useT()
  const [error, setError] = useState<string | null>(null)

  type FormValues = Record<string, string>
  const defaults: FormValues = {}
  for (const f of connector.credentials) {
    if (f.kind === 'secret') {
      defaults[f.name] = ''
    } else {
      defaults[f.name] = typeof existing?.fields[f.name] === 'string'
        ? (existing.fields[f.name] as string)
        : ''
    }
  }

  const {
    register,
    handleSubmit,
    reset,
    formState: { isSubmitting, errors },
  } = useForm<FormValues>({ defaultValues: defaults })

  useEffect(() => {
    reset(defaults)
    // Only reset when switching connector/config.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connector.id, existing?.id])

  const save = useMutation({
    mutationFn: async (v: FormValues) => {
      const fields: Record<string, string | null> = {}
      for (const f of connector.credentials) {
        const raw = v[f.name] ?? ''
        if (f.kind === 'secret') {
          // Empty means "leave unchanged" for existing configs; for new
          // configs it's treated as missing and validation will catch it.
          if (raw === '' && existing?.fields_present?.[f.name]) continue
          fields[f.name] = raw
        } else {
          fields[f.name] = raw
        }
      }
      return api.putConnectorConfig(connector.id, { fields })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connector-configs'] })
      onClose()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  const del = useMutation({
    mutationFn: () => api.deleteConnectorConfig(connector.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connector-configs'] })
      onClose()
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null)
  const test = useMutation({
    mutationFn: () => api.testConnectorConfig(connector.id),
    onSuccess: (r) => setTestResult(r),
    onError: (err) => setTestResult({ ok: false, message: err instanceof ApiError ? err.message : 'failed' }),
  })

  return (
    <Modal open onClose={onClose} title={connector.name} widthClass="max-w-lg">
      <form
        className="space-y-4"
        onSubmit={handleSubmit(
          (v) => {
            setError(null)
            save.mutate(v)
          },
          () => {
            // RHF blocked submit; surface the reason so the user isn't
            // staring at a Save button that apparently does nothing.
            setError(t('connectors.fillRequired'))
          },
        )}
      >
        <p className="text-sm text-muted-foreground">{connector.description}</p>
        {connector.homepage && (
          <a
            href={connector.homepage}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
          >
            {connector.homepage}
            <ExternalLink className="h-3 w-3" />
          </a>
        )}

        <div className="space-y-3">
          {connector.credentials.map((f) => {
            const present = Boolean(existing?.fields_present?.[f.name])
            const fieldErr = errors[f.name]
            return (
              <div key={f.name} className="space-y-1">
                <div className="flex items-center justify-between">
                  <Label htmlFor={f.name}>
                    {f.label}
                    {f.required && <span className="ml-1 text-destructive">*</span>}
                  </Label>
                  {f.kind === 'secret' && present && (
                    <span className="text-[10px] uppercase tracking-wider text-primary">
                      {t('connectors.secretSaved')}
                    </span>
                  )}
                </div>
                <Input
                  id={f.name}
                  type={f.kind === 'secret' ? 'password' : 'text'}
                  autoComplete={f.kind === 'secret' ? 'new-password' : 'off'}
                  placeholder={
                    f.kind === 'secret' && present ? t('connectors.secretPlaceholder') : f.placeholder
                  }
                  className={cn(fieldErr && 'border-destructive focus-visible:ring-destructive')}
                  {...register(f.name, {
                    required: f.required && !(f.kind === 'secret' && present),
                  })}
                />
                {fieldErr && (
                  <p className="text-xs text-destructive">{t('connectors.fieldRequired')}</p>
                )}
                {f.help && !fieldErr && <p className="text-xs text-muted-foreground">{f.help}</p>}
              </div>
            )
          })}
        </div>

        {error && <p className="text-sm text-destructive">{error}</p>}

        {existing && (
          <div className="flex items-center gap-2 border-t border-border pt-3">
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setTestResult(null)
                test.mutate()
              }}
              disabled={test.isPending}
            >
              {test.isPending ? t('common.loading') : t('connectors.test')}
            </Button>
            {testResult && (
              <span className={cn('text-xs', testResult.ok ? 'text-primary' : 'text-destructive')}>
                {testResult.ok ? t('connectors.test.ok') : (testResult.message ?? t('connectors.test.fail'))}
              </span>
            )}
          </div>
        )}

        <div className="flex items-center justify-between gap-2 pt-2">
          {existing ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                if (confirm(t('connectors.confirmRemove'))) del.mutate()
              }}
              disabled={del.isPending}
            >
              <Trash2 className="mr-1 h-3.5 w-3.5" />
              {t('common.delete')}
            </Button>
          ) : (
            <span />
          )}
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={isSubmitting || save.isPending}>
              {save.isPending ? t('common.loading') : t('common.save')}
            </Button>
          </div>
        </div>
      </form>
    </Modal>
  )
}

