import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { Copy, ExternalLink, ShieldCheck, Trash2 } from 'lucide-react'
import {
  api,
  ApiError,
  type Connector,
  type ConnectorConfigSafe,
} from '@/lib/api'
import { Button, Input, Kbd, KBD, Label } from '@/components/ui'
import { Modal } from '@/components/Modal'
import { SettingsShell } from '@/components/SettingsShell'
import { toast } from '@/components/Toast'
import { useT } from '@/lib/i18n'
import { mod } from '@/lib/platform'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/connectors/')({
  component: ConnectorsPage,
})

function ConnectorsPage() {
  const t = useT()
  const qc = useQueryClient()
  const connectors = useQuery({ queryKey: ['connectors'], queryFn: api.listConnectors })
  const configs = useQuery({
    queryKey: ['connector-configs'],
    queryFn: api.listConnectorConfigs,
  })
  const [active, setActive] = useState<Connector | null>(null)

  const configByID = new Map<string, ConnectorConfigSafe>()
  for (const c of configs.data ?? []) configByID.set(c.connector_id, c)

  // Surface oauth callback result as a toast, then strip the params so a
  // refresh doesn't re-fire.
  useEffect(() => {
    if (typeof window === 'undefined') return
    const params = new URLSearchParams(window.location.search)
    const ok = params.get('oauth_connected')
    const err = params.get('oauth_error')
    if (!ok && !err) return
    const list = connectors.data ?? []
    const id = ok ?? params.get('oauth_connector')
    const name = list.find((c) => c.id === id)?.name ?? id ?? 'connector'
    if (ok) {
      toast.success(`${name} připojen.`)
      qc.invalidateQueries({ queryKey: ['connector-configs'] })
    } else if (err) {
      toast.error(`Autorizace selhala (${err}).`)
    }
    // Strip oauth_* params without a router round-trip.
    const next = new URL(window.location.href)
    next.searchParams.delete('oauth_connected')
    next.searchParams.delete('oauth_error')
    next.searchParams.delete('oauth_connector')
    window.history.replaceState(null, '', next.pathname + next.search)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connectors.data])

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

  const oauthEnabled = Boolean(connector.oauth_supported)
  const refreshTokenField = 'refresh_token'
  const visibleCredentials = oauthEnabled
    ? connector.credentials.filter((f) => f.name !== refreshTokenField)
    : connector.credentials
  const refreshCaptured = Boolean(existing?.fields_present?.[refreshTokenField])

  type FormValues = Record<string, string>
  const defaults: FormValues = {}
  for (const f of visibleCredentials) {
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connector.id, existing?.id])

  const save = useMutation({
    mutationFn: async (v: FormValues) => {
      const fields: Record<string, string | null> = {}
      for (const f of visibleCredentials) {
        const raw = v[f.name] ?? ''
        if (f.kind === 'secret') {
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
      if (oauthEnabled) {
        // Keep the modal open so the user can click Authorize next.
        toast.success(`${connector.name} uložen. Klikněte Autorizovat pro dokončení.`)
      } else {
        toast.success(`${connector.name} uložen.`)
        onClose()
      }
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  const del = useMutation({
    mutationFn: () => api.deleteConnectorConfig(connector.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['connector-configs'] })
      toast.info(`${connector.name} odpojen.`)
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
            setError(t('connectors.fillRequired'))
          },
        )}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault()
            handleSubmit(
              (v) => {
                setError(null)
                save.mutate(v)
              },
              () => {
                setError(t('connectors.fillRequired'))
              },
            )()
          }
        }}
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

        {oauthEnabled && connector.callback_url && (
          <CallbackBanner
            url={connector.callback_url}
            captured={refreshCaptured}
            connectorName={connector.name}
          />
        )}

        <div className="space-y-3">
          {visibleCredentials.map((f) => {
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
          <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
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
            {oauthEnabled && (
              <Button
                type="button"
                variant="ghost"
                className="ml-auto inline-flex items-center gap-1.5"
                onClick={() => {
                  window.location.href = `/api/v1/connectors/${connector.id}/oauth/start`
                }}
              >
                <ShieldCheck className="h-3.5 w-3.5" />
                {refreshCaptured ? 'Re-autorizovat' : 'Autorizovat'}
              </Button>
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
              <span className="ml-2 hidden gap-0.5 opacity-60 sm:inline-flex">
                <Kbd>{mod()}</Kbd>
                <Kbd>{KBD.enter}</Kbd>
              </span>
            </Button>
          </div>
        </div>
      </form>
    </Modal>
  )
}

function CallbackBanner({
  url,
  captured,
  connectorName,
}: {
  url: string
  captured: boolean
  connectorName: string
}) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="rounded-md border border-primary/30 bg-primary/5 p-3 text-xs">
      <p className="font-medium text-foreground">
        Callback URL pro {connectorName}
      </p>
      <p className="mt-1 text-muted-foreground">
        Vložte ji jako Redirect URI v portálu poskytovatele před prvním kliknutím na Autorizovat.
      </p>
      <div className="mt-2 flex items-center gap-2 rounded-md border border-border bg-background px-2 py-1.5 font-mono text-[11px]">
        <span className="flex-1 truncate">{url}</span>
        <button
          type="button"
          onClick={() => {
            navigator.clipboard.writeText(url).then(() => {
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            })
          }}
          className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground hover:text-foreground"
          aria-label="Copy callback URL"
        >
          <Copy className="h-3 w-3" />
          {copied ? 'Zkopírováno' : 'Kopírovat'}
        </button>
      </div>
      <p className="mt-2 text-muted-foreground">
        {captured ? (
          <>Refresh token uložen. Re-autorizace ho přepíše novým.</>
        ) : (
          <>Po uložení Client ID + Client Secret klikněte Autorizovat — vrátíme se sem s uloženým refresh tokenem.</>
        )}
      </p>
    </div>
  )
}
