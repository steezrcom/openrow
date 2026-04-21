import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import {
  Check,
  ChevronRight,
  Cpu,
  RefreshCw,
  Server,
  Trash2,
  X,
} from 'lucide-react'
import { api, ApiError, type LLMProvider, type LLMTestResult } from '@/lib/api'
import { Button, Card, Input, Label } from '@/components/ui'
import { SettingsTabs } from '@/components/SettingsTabs'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/llm')({
  component: LLMSettingsPage,
})

type FormValues = {
  provider: string
  base_url: string
  api_key: string
  model: string
  keep_existing_key: boolean
}

function LLMSettingsPage() {
  const qc = useQueryClient()
  const providers = useQuery({ queryKey: ['llm-providers'], queryFn: api.llmProviders })
  const config = useQuery({ queryKey: ['llm-config'], queryFn: api.llmConfig })

  const { register, handleSubmit, watch, setValue, reset, formState: { isSubmitting } } =
    useForm<FormValues>({
      defaultValues: {
        provider: '',
        base_url: '',
        api_key: '',
        model: '',
        keep_existing_key: false,
      },
    })

  const providerID = watch('provider')
  const baseURL = watch('base_url')
  const apiKey = watch('api_key')
  const model = watch('model')
  const keepKey = watch('keep_existing_key')

  const selectedProvider = useMemo(
    () => providers.data?.find((p) => p.id === providerID),
    [providers.data, providerID]
  )

  // Seed the form from the saved config once both queries are done.
  useEffect(() => {
    if (!config.data) return
    const existing = config.data
    reset({
      provider: existing.provider ?? '',
      base_url: existing.base_url ?? '',
      api_key: '',
      model: existing.model ?? '',
      keep_existing_key: Boolean(existing.has_api_key),
    })
  }, [config.data, reset])

  // When the user picks a preset, auto-fill its base URL and default model.
  function pickProvider(p: LLMProvider) {
    setValue('provider', p.id)
    setValue('base_url', p.base_url)
    if (p.default_model) setValue('model', p.default_model)
  }

  const [fetchedModels, setFetchedModels] = useState<string[]>([])
  const [modelsError, setModelsError] = useState<string | null>(null)

  const fetchModels = useMutation({
    mutationFn: () =>
      api.llmListModels({
        base_url: baseURL,
        api_key: apiKey || (keepKey && config.data?.has_api_key ? '__keep__' : ''),
      }),
    onMutate: () => setModelsError(null),
    onSuccess: (list) => setFetchedModels(list.map((m) => m.id)),
    onError: (err) => setModelsError(err instanceof ApiError ? err.message : 'failed'),
  })

  const [testResult, setTestResult] = useState<LLMTestResult | null>(null)
  const [testError, setTestError] = useState<string | null>(null)

  // "Clean" means the user is looking at the saved config without edits: they
  // haven't typed a new api_key, and the URL/model match what's persisted.
  // In that case the test button should probe the actual saved config (which
  // also records the outcome for the status banner).
  const formIsClean = (() => {
    const saved = config.data
    if (!saved || saved.source !== 'tenant') return false
    if (apiKey.length > 0) return false
    if ((saved.base_url ?? '') !== baseURL) return false
    if ((saved.model ?? '') !== model) return false
    if ((saved.provider ?? '') !== providerID) return false
    return true
  })()

  const test = useMutation({
    mutationFn: async () => {
      if (formIsClean) return api.llmSelfTest()
      return api.llmTest({ base_url: baseURL, api_key: apiKey, model })
    },
    onMutate: () => {
      setTestResult(null)
      setTestError(null)
    },
    onSuccess: async (r) => {
      setTestResult(r)
      if (formIsClean) await qc.invalidateQueries({ queryKey: ['llm-config'] })
    },
    onError: (err) => setTestError(err instanceof ApiError ? err.message : 'failed'),
  })

  const save = useMutation({
    mutationFn: (v: FormValues) =>
      api.putLLMConfig({
        provider: v.provider,
        base_url: v.base_url,
        api_key: v.keep_existing_key && !v.api_key ? undefined : v.api_key,
        model: v.model,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['llm-config'] })
    },
  })

  const del = useMutation({
    mutationFn: () => api.deleteLLMConfig(),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['llm-config'] })
      reset({
        provider: '',
        base_url: '',
        api_key: '',
        model: '',
        keep_existing_key: false,
      })
    },
  })

  const source = config.data?.source
  const showFallbackBanner = source === 'env-fallback'
  const hasPersistedConfig = source === 'tenant'

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">Home</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          Settings
          <ChevronRight className="inline h-3 w-3 mx-1" />
          LLM
        </p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">Language model</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Pick any OpenAI-compatible provider — cloud or local. Your API key is encrypted
          before it hits the database. Tool calling requires a capable model
          (GPT-4o, Claude 3.5+, Llama 3.1 8B+, Qwen 2.5 7B+).
        </p>
      </header>

      <SettingsTabs active="llm" />

      {showFallbackBanner && (
        <Card className="mb-6 border-primary/30 bg-primary/5 p-4 text-sm">
          This workspace is currently using the fallback <code>ANTHROPIC_API_KEY</code> from
          the server environment. Save a config below to use your own provider and key.
        </Card>
      )}

      {hasPersistedConfig && config.data?.last_tested_at && (
        <StatusBanner config={config.data} />
      )}

      <form
        className="space-y-6"
        onSubmit={handleSubmit((v) => {
          save.mutate(v)
        })}
      >
        <Section title="Provider">
          <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
            {(providers.data ?? []).map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => pickProvider(p)}
                className={cn(
                  'flex items-start gap-2 rounded-md border border-border bg-card p-3 text-left text-sm hover:bg-accent',
                  providerID === p.id && 'border-primary ring-2 ring-primary/30'
                )}
              >
                {p.local ? (
                  <Cpu className="mt-0.5 h-4 w-4 text-muted-foreground" />
                ) : (
                  <Server className="mt-0.5 h-4 w-4 text-muted-foreground" />
                )}
                <div className="min-w-0">
                  <div className="truncate font-medium">{p.name}</div>
                  <div className="truncate text-[11px] text-muted-foreground">
                    {p.base_url || 'custom URL'}
                  </div>
                </div>
              </button>
            ))}
          </div>
          {selectedProvider?.notes && (
            <p className="mt-2 text-xs text-muted-foreground">{selectedProvider.notes}</p>
          )}
        </Section>

        <Section title="Endpoint">
          <Field label="Base URL">
            <Input
              placeholder="https://api.openai.com/v1"
              {...register('base_url', { required: true })}
            />
          </Field>
          <Field label={`API key${selectedProvider?.requires_api_key === false ? ' (optional for local)' : ''}`}>
            <Input
              type="password"
              placeholder={
                keepKey
                  ? '•••••••••••• (saved; leave blank to keep)'
                  : 'sk-... or your provider key'
              }
              {...register('api_key')}
            />
            {keepKey && (
              <p className="text-[11px] text-muted-foreground">
                A key is already saved. Typing a new value replaces it. To clear, save an empty key.
              </p>
            )}
          </Field>
        </Section>

        <Section title="Model">
          <div className="flex items-end gap-2">
            <Field label="Model name" className="flex-1">
              <Input
                placeholder={selectedProvider?.default_model || 'gpt-4o'}
                {...register('model', { required: true })}
                list="llm-models-list"
              />
              {fetchedModels.length > 0 && (
                <datalist id="llm-models-list">
                  {fetchedModels.map((m) => <option key={m} value={m} />)}
                </datalist>
              )}
            </Field>
            <Button
              type="button"
              variant="ghost"
              onClick={() => fetchModels.mutate()}
              disabled={!baseURL || fetchModels.isPending}
            >
              <RefreshCw className={cn('mr-1 h-3.5 w-3.5', fetchModels.isPending && 'animate-spin')} />
              Fetch models
            </Button>
          </div>
          {fetchedModels.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {fetchedModels.slice(0, 40).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => setValue('model', m)}
                  className={cn(
                    'rounded-full border border-border bg-background/60 px-2 py-0.5 text-[11px]',
                    model === m ? 'border-primary text-primary' : 'text-muted-foreground hover:bg-accent'
                  )}
                >
                  {m}
                </button>
              ))}
              {fetchedModels.length > 40 && (
                <span className="text-[11px] text-muted-foreground self-center">
                  …and {fetchedModels.length - 40} more
                </span>
              )}
            </div>
          )}
          {modelsError && (
            <p className="text-xs text-destructive">{modelsError}</p>
          )}
        </Section>

        <Section title="Test">
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => test.mutate()}
              disabled={!baseURL || !model || test.isPending}
            >
              {test.isPending
                ? 'Testing…'
                : formIsClean
                ? 'Test saved config'
                : 'Test these values'}
            </Button>
            {testResult && <TestBadge result={testResult} />}
          </div>
          {testResult?.message && (
            <p className="text-xs text-muted-foreground">{testResult.message}</p>
          )}
          {testError && <p className="text-xs text-destructive">{testError}</p>}
          {formIsClean && (
            <p className="text-[11px] text-muted-foreground">
              Tests against the saved API key on the server. The result is recorded and
              shown above.
            </p>
          )}
        </Section>

        <div className="flex items-center justify-between border-t border-border pt-5">
          <div className="flex items-center gap-2">
            <Button type="submit" disabled={isSubmitting || save.isPending}>
              {save.isPending ? 'Saving…' : 'Save'}
            </Button>
            {save.isSuccess && (
              <span className="inline-flex items-center gap-1 text-xs text-primary">
                <Check className="h-3 w-3" /> Saved
              </span>
            )}
            {save.isError && (
              <span className="text-xs text-destructive">
                {save.error instanceof ApiError ? save.error.message : 'failed'}
              </span>
            )}
          </div>
          {hasPersistedConfig && (
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                if (confirm('Remove this workspace\'s LLM config? The env fallback (if any) will take over.')) {
                  del.mutate()
                }
              }}
            >
              <Trash2 className="mr-1 h-3.5 w-3.5" />
              Remove
            </Button>
          )}
        </div>
      </form>

      <section className="mt-10 border-t border-border pt-6 text-xs text-muted-foreground">
        <h3 className="mb-2 text-sm font-medium text-foreground">Recommended models for tool use</h3>
        <ul className="space-y-1">
          <li>
            <span className="font-medium text-foreground">Cloud:</span> GPT-4o, GPT-4o-mini,
            Claude Sonnet 4/4.5, Gemini 2.0 Flash, Llama 3.3 70B on Groq.
          </li>
          <li>
            <span className="font-medium text-foreground">Local:</span> Llama 3.1 8B+,
            Qwen 2.5 7B+, Mistral Nemo, Hermes 3. Anything under 7B struggles with the
            tool schema and will often fail silently.
          </li>
        </ul>
        <p className="mt-2">
          Running Ollama in a separate container? From the OpenRow app container use
          {' '}<code>http://host.docker.internal:11434/v1</code> on Mac/Windows,
          or add <code>--network host</code> on Linux.
        </p>
      </section>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </p>
      <div className="space-y-3">{children}</div>
    </section>
  )
}

function Field({
  label,
  children,
  className,
}: {
  label: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('space-y-1.5', className)}>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function StatusBanner({ config }: { config: { last_tested_at?: string | null; last_test_ok?: boolean | null; last_test_tools_ok?: boolean | null; last_test_message?: string } }) {
  const ok = config.last_test_ok === true
  const tools = config.last_test_tools_ok === true
  const tested = config.last_tested_at ? new Date(config.last_tested_at) : null
  const rel = tested ? relativeTime(tested) : ''
  return (
    <Card
      className={cn(
        'mb-6 flex items-start gap-3 p-4 text-sm',
        ok ? 'border-primary/30 bg-primary/5' : 'border-destructive/30 bg-destructive/5'
      )}
    >
      {ok ? (
        <Check className="mt-0.5 h-4 w-4 text-primary" />
      ) : (
        <X className="mt-0.5 h-4 w-4 text-destructive" />
      )}
      <div className="min-w-0 flex-1">
        <div className="font-medium">
          {ok ? (tools ? 'Config OK' : 'Chat OK, tool calls unreliable') : 'Config is failing'}
        </div>
        <div className="mt-0.5 text-xs text-muted-foreground">
          Last checked {rel || tested?.toISOString().slice(0, 16).replace('T', ' ')}
          {config.last_test_message ? ' · ' + config.last_test_message : ''}
        </div>
      </div>
    </Card>
  )
}

function relativeTime(d: Date): string {
  const diffSec = Math.round((Date.now() - d.getTime()) / 1000)
  if (diffSec < 45) return 'just now'
  if (diffSec < 60 * 60) return `${Math.round(diffSec / 60)} min ago`
  if (diffSec < 60 * 60 * 24) return `${Math.round(diffSec / 3600)} h ago`
  return `${Math.round(diffSec / 86400)} d ago`
}

function TestBadge({ result }: { result: LLMTestResult }) {
  const step = (label: string, ok: boolean) => (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]',
        ok
          ? 'bg-primary/10 text-primary'
          : 'bg-muted/60 text-muted-foreground'
      )}
    >
      {ok ? <Check className="h-3 w-3" /> : <X className="h-3 w-3" />}
      {label}
    </span>
  )
  return (
    <div className="flex items-center gap-1">
      {step('models', result.models_ok)}
      {step('chat', result.chat_ok)}
      {step('tools', result.tools_ok)}
    </div>
  )
}
