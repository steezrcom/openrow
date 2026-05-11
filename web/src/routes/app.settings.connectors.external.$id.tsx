import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronRight } from 'lucide-react'
import {
  api,
  ApiError,
  type DiscoveryEvidence,
  type ExternalBinding,
  type ExternalBindingReviewItem,
} from '@/lib/api'
import { Button, Card, ErrorAlert } from '@/components/ui'
import { SettingsShell } from '@/components/SettingsShell'
import { toast } from '@/components/Toast'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/settings/connectors/external/$id')({
  component: ExternalBindingDetail,
})

function ExternalBindingDetail() {
  const { id } = Route.useParams()
  const qc = useQueryClient()
  const [actionError, setActionError] = useState<string | null>(null)

  const binding = useQuery({
    queryKey: ['external-binding', id],
    queryFn: () => api.getExternalBinding(id),
    refetchInterval: (q) => (q.state.data?.State === 'discovering' ? 2000 : false),
  })

  const review = useQuery({
    queryKey: ['external-binding-review', id],
    queryFn: () => api.listExternalBindingReviewItems(id),
  })

  const rediscover = useMutation({
    mutationFn: () => api.rediscoverExternalBinding(id),
    onSuccess: () => {
      setActionError(null)
      qc.invalidateQueries({ queryKey: ['external-binding', id] })
      toast.info('Re-discovery started.')
    },
    onError: (err) => setActionError(err instanceof ApiError ? err.message : 'failed'),
  })

  const activate = useMutation({
    mutationFn: () => api.activateExternalBinding(id),
    onSuccess: () => {
      setActionError(null)
      qc.invalidateQueries({ queryKey: ['external-binding', id] })
      qc.invalidateQueries({ queryKey: ['external-bindings'] })
      toast.success('Binding activated.')
    },
    onError: (err) => setActionError(err instanceof ApiError ? err.message : 'failed'),
  })

  const b = binding.data

  return (
    <SettingsShell active="connectors">
      <p className="mb-4 text-xs text-muted-foreground">
        <Link to="/app/settings/connectors" className="hover:text-foreground">
          Connectors
        </Link>
        <ChevronRight className="mx-1 inline h-3 w-3" />
        <Link to="/app/settings/connectors/external" className="hover:text-foreground">
          External (ERP)
        </Link>
        <ChevronRight className="mx-1 inline h-3 w-3" />
        Binding
      </p>

      {binding.isLoading && (
        <p className="text-sm text-muted-foreground">Loading...</p>
      )}
      {binding.error && (
        <ErrorAlert>{(binding.error as Error).message}</ErrorAlert>
      )}

      {b && (
        <>
          <Card className="mb-6 p-6">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="text-lg font-semibold">{b.ConnectorID}</h2>
                <p className="mt-1 font-mono text-xs text-muted-foreground">{b.ID}</p>
                <div className="mt-2 flex items-center gap-2">
                  <StateBadge state={b.State} />
                  {b.State === 'discovering' && (
                    <span className="text-xs text-muted-foreground">polling...</span>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  onClick={() => rediscover.mutate()}
                  disabled={rediscover.isPending || b.State === 'discovering'}
                >
                  {rediscover.isPending ? 'Working...' : 'Re-discover'}
                </Button>
                <Button
                  onClick={() => activate.mutate()}
                  disabled={activate.isPending || b.State !== 'proposed'}
                >
                  {activate.isPending ? 'Working...' : 'Activate'}
                </Button>
              </div>
            </div>
            {b.LastError && (
              <ErrorAlert className="mt-4">{b.LastError}</ErrorAlert>
            )}
            {actionError && <ErrorAlert className="mt-4">{actionError}</ErrorAlert>}
          </Card>

          {b.Mapping && <MappingTable evidences={b.Mapping.evidences} />}

          <ReviewQueue
            bindingID={id}
            items={review.data ?? []}
            isLoading={review.isLoading}
            error={review.error ? (review.error as Error).message : null}
          />
        </>
      )}
    </SettingsShell>
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

function MappingTable({ evidences }: { evidences: Record<string, DiscoveryEvidence> }) {
  const rows = Object.entries(evidences).sort(([a], [b]) => a.localeCompare(b))
  return (
    <Card className="mb-6 overflow-hidden">
      <div className="border-b border-border px-4 py-3">
        <h3 className="text-sm font-medium">Mapping</h3>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/40 text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-2 font-medium">Name</th>
              <th className="px-4 py-2 font-medium">Role</th>
              <th className="px-4 py-2 font-medium">Confidence</th>
              <th className="px-4 py-2 font-medium">Review</th>
              <th className="px-4 py-2 font-medium">Mirror</th>
              <th className="px-4 py-2 text-right font-medium">Rows</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(([name, ev]) => (
              <tr key={name} className="border-b border-border last:border-0">
                <td className="px-4 py-2 font-mono text-xs">{name}</td>
                <td className="px-4 py-2">{ev.role}</td>
                <td className="px-4 py-2 tabular-nums">{ev.confidence.toFixed(2)}</td>
                <td className="px-4 py-2 text-xs text-muted-foreground">{ev.review}</td>
                <td className="px-4 py-2">{ev.mirror ? 'yes' : 'no'}</td>
                <td className="px-4 py-2 text-right tabular-nums">{ev.row_count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

function ReviewQueue({
  bindingID,
  items,
  isLoading,
  error,
}: {
  bindingID: string
  items: ExternalBindingReviewItem[]
  isLoading: boolean
  error: string | null
}) {
  return (
    <Card className="overflow-hidden">
      <div className="border-b border-border px-4 py-3">
        <h3 className="text-sm font-medium">Review queue</h3>
      </div>
      <div className="p-4">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : error ? (
          <ErrorAlert>{error}</ErrorAlert>
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nothing to review.</p>
        ) : (
          <ul className="space-y-3">
            {items.map((it) => (
              <ReviewRow key={it.id} bindingID={bindingID} item={it} />
            ))}
          </ul>
        )}
      </div>
    </Card>
  )
}

function ReviewRow({
  bindingID,
  item,
}: {
  bindingID: string
  item: ExternalBindingReviewItem
}) {
  const qc = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const accept = useMutation({
    mutationFn: () =>
      api.resolveExternalBindingReviewItem(bindingID, item.id, { action: 'accept' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['external-binding-review', bindingID] })
      qc.invalidateQueries({ queryKey: ['external-binding', bindingID] })
      toast.success('Accepted.')
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : 'failed'),
  })

  return (
    <li className="rounded-md border border-border bg-card p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{item.evidence}</span>
            {item.field && (
              <>
                <span className="text-muted-foreground">/</span>
                <span className="font-mono text-xs">{item.field}</span>
              </>
            )}
            <StatusBadge status={item.status} />
          </div>
          <pre className="overflow-x-auto rounded bg-muted/40 p-2 text-xs">
            {JSON.stringify(item.proposed, null, 2)}
          </pre>
          {error && <ErrorAlert>{error}</ErrorAlert>}
        </div>
        {item.status === 'pending' && (
          <Button
            variant="ghost"
            onClick={() => {
              setError(null)
              accept.mutate()
            }}
            disabled={accept.isPending}
          >
            {accept.isPending ? 'Working...' : 'Accept proposed'}
          </Button>
        )}
      </div>
    </li>
  )
}

function StatusBadge({ status }: { status: ExternalBindingReviewItem['status'] }) {
  return (
    <span
      className={cn(
        'rounded px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider',
        status === 'pending' && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
        status === 'accepted' && 'bg-primary/15 text-primary',
        status === 'edited' && 'bg-primary/15 text-primary',
        status === 'rejected' && 'bg-destructive/20 text-destructive',
      )}
    >
      {status}
    </span>
  )
}
