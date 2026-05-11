import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { ChevronRight } from 'lucide-react'
import { api, ApiError, type ExternalBinding } from '@/lib/api'
import { Button, Card, ErrorAlert, Input, Label } from '@/components/ui'
import { SettingsShell } from '@/components/SettingsShell'
import { toast } from '@/components/Toast'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/connectors/external/')({
  component: ExternalConnectorsPage,
})

const FLEXI_ID = 'abra-flexi'

type FlexiCreds = {
  base_url: string
  company: string
  username: string
  password: string
}

function ExternalConnectorsPage() {
  const bindings = useQuery({
    queryKey: ['external-bindings'],
    queryFn: api.listExternalBindings,
  })

  return (
    <SettingsShell active="connectors">
      <p className="mb-4 text-xs text-muted-foreground">
        <Link to="/app/settings/connectors" className="hover:text-foreground">
          Connectors
        </Link>
        <ChevronRight className="mx-1 inline h-3 w-3" />
        External (ERP)
      </p>

      <div className="mb-6">
        <h2 className="text-lg font-semibold">External (ERP) connectors</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Connect an ERP system. The agent scans evidence list and proposes a mapping you can review.
        </p>
      </div>

      <Card className="mb-6 p-6">
        <h3 className="mb-3 text-sm font-medium">Connect ABRA Flexi</h3>
        <ConnectForm />
      </Card>

      <div>
        <h3 className="mb-3 text-sm font-medium">Bindings</h3>
        {bindings.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : bindings.error ? (
          <ErrorAlert>{(bindings.error as Error).message}</ErrorAlert>
        ) : (bindings.data?.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">No bindings yet.</p>
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border bg-card">
            {bindings.data!.map((b) => (
              <BindingRow key={b.ID} binding={b} />
            ))}
          </ul>
        )}
      </div>
    </SettingsShell>
  )
}

function ConnectForm() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FlexiCreds>({
    defaultValues: { base_url: '', company: '', username: '', password: '' },
  })

  const create = useMutation({
    mutationFn: (v: FlexiCreds) =>
      api.createExternalBinding({
        connector_id: FLEXI_ID,
        credentials: { ...v },
      }),
    onSuccess: (b) => {
      qc.invalidateQueries({ queryKey: ['external-bindings'] })
      toast.success('Discovery started.')
      navigate({
        to: '/app/settings/connectors/external/$id',
        params: { id: b.ID },
      })
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <form
      className="space-y-4"
      onSubmit={handleSubmit((v) => {
        setError(null)
        create.mutate(v)
      })}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <FormRow
          id="base_url"
          label="Server URL"
          placeholder="https://demo.flexibee.eu"
          help="Root URL of the Flexi server (no trailing /c/<company>)."
          error={errors.base_url?.message}
        >
          <Input
            id="base_url"
            type="url"
            autoComplete="off"
            {...register('base_url', { required: 'Required' })}
          />
        </FormRow>
        <FormRow
          id="company"
          label="Company token"
          placeholder="demo_s_r_o_"
          help="Company segment from your Flexi URL — the part after /c/."
          error={errors.company?.message}
        >
          <Input
            id="company"
            autoComplete="off"
            {...register('company', { required: 'Required' })}
          />
        </FormRow>
        <FormRow id="username" label="Username" error={errors.username?.message}>
          <Input
            id="username"
            autoComplete="username"
            {...register('username', { required: 'Required' })}
          />
        </FormRow>
        <FormRow id="password" label="Password" error={errors.password?.message}>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            {...register('password', { required: 'Required' })}
          />
        </FormRow>
      </div>

      {error && <ErrorAlert>{error}</ErrorAlert>}

      <div className="flex items-center justify-end">
        <Button type="submit" disabled={isSubmitting || create.isPending}>
          {create.isPending ? 'Connecting...' : 'Connect'}
        </Button>
      </div>
    </form>
  )
}

function FormRow({
  id,
  label,
  placeholder,
  help,
  error,
  children,
}: {
  id: string
  label: string
  placeholder?: string
  help?: string
  error?: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {placeholder && !error && !help && (
        <p className="text-xs text-muted-foreground">{placeholder}</p>
      )}
      {help && !error && <p className="text-xs text-muted-foreground">{help}</p>}
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}

function BindingRow({ binding }: { binding: ExternalBinding }) {
  return (
    <li className="flex items-center justify-between px-4 py-3 text-sm">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{binding.ConnectorID}</span>
          <StateBadge state={binding.State} />
        </div>
        {binding.LastError && (
          <p className="mt-1 truncate text-xs text-destructive">{binding.LastError}</p>
        )}
      </div>
      <Link
        to="/app/settings/connectors/external/$id"
        params={{ id: binding.ID }}
        className="text-sm text-primary hover:underline"
      >
        Open
      </Link>
    </li>
  )
}

function StateBadge({ state }: { state: ExternalBinding['State'] }) {
  return (
    <span
      className={cn(
        'rounded px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider',
        state === 'active' && 'bg-primary/15 text-primary',
        state === 'proposed' && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
        state === 'discovering' && 'bg-muted text-muted-foreground',
        state === 'error' && 'bg-destructive/20 text-destructive',
      )}
    >
      {state}
    </span>
  )
}
