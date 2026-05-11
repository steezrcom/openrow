import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronRight, ShieldAlert } from 'lucide-react'
import { api, type FlowApproval } from '@/lib/api'
import { Button, Card, Kbd, KBD } from '@/components/ui'
import { EmptyState } from '@/components/EmptyState'
import { SkeletonRows } from '@/components/Skeleton'
import { toast } from '@/components/Toast'
import { useT } from '@/lib/i18n'
import { mod } from '@/lib/platform'

export const Route = createFileRoute('/app/approvals')({
  component: ApprovalsPage,
})

function ApprovalsPage() {
  const t = useT()
  // Poll while the tab is open — approvals are created by background
  // workers, so the list can change without any action on this page.
  const list = useQuery({
    queryKey: ['flow-approvals'],
    queryFn: api.listFlowApprovals,
    refetchInterval: 5000,
  })

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <header className="mb-6">
        <p className="text-xs text-muted-foreground">
          <Link to="/app" className="hover:text-foreground">{t('nav.home')}</Link>
          <ChevronRight className="inline h-3 w-3 mx-1" />
          {t('approvals.title')}
        </p>
        <h1 className="mt-2 flex items-center gap-2 text-2xl font-semibold tracking-tight">
          <ShieldAlert className="h-5 w-5 text-primary" />
          {t('approvals.title')}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">{t('approvals.hint')}</p>
      </header>

      {list.isLoading && <SkeletonRows count={3} height="h-32" />}

      {!list.isLoading && (list.data ?? []).length === 0 && (
        <EmptyState
          icon={ShieldAlert}
          title={t('approvals.empty')}
          description={t('approvals.empty.hint')}
          action={{ label: t('approvals.empty.action'), to: '/app/flows' }}
        />
      )}

      <div className="space-y-3">
        {(list.data ?? []).map((a) => <ApprovalCard key={a.id} approval={a} />)}
      </div>
    </div>
  )
}

function ApprovalCard({ approval }: { approval: FlowApproval }) {
  const t = useT()
  const qc = useQueryClient()
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)

  const resolve = useMutation({
    mutationFn: (body: { approve: boolean; rejection_reason?: string }) =>
      api.resolveFlowApproval(approval.id, body),
    onSuccess: (r, body) => {
      qc.invalidateQueries({ queryKey: ['flow-approvals'] })
      qc.invalidateQueries({ queryKey: ['flow-run', r.run?.id] })
      qc.invalidateQueries({ queryKey: ['flow-runs'] })
      if (r.error) {
        setError(r.error)
        toast.error(r.error)
      } else {
        toast.success(body.approve ? 'Schváleno.' : 'Zamítnuto.')
      }
    },
    onError: (e) => {
      const msg = e instanceof Error ? e.message : 'failed'
      setError(msg)
      toast.error(msg)
    },
  })

  return (
    <Card className="p-4 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm">{approval.tool_name}</span>
            <span className="text-xs text-muted-foreground">
              {new Date(approval.requested_at).toLocaleString()}
            </span>
          </div>
          <Link
            to="/app/flow_runs/$runId"
            params={{ runId: approval.flow_run_id }}
            className="text-xs text-primary hover:underline"
          >
            {t('approvals.viewRun')}
          </Link>
        </div>
      </div>

      <pre className="max-h-48 overflow-auto rounded-md border border-border bg-muted/10 p-3 text-xs font-mono">
        {JSON.stringify(approval.tool_input, null, 2)}
      </pre>

      <div className="flex items-center gap-2">
        <input
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
              e.preventDefault()
              resolve.mutate({ approve: true })
            } else if (e.key === 'Escape') {
              e.currentTarget.blur()
            }
          }}
          placeholder={t('approvals.rejectionReason')}
          className="flex h-9 flex-1 rounded-md border border-input bg-background px-3 text-sm"
        />
        <Button
          variant="ghost"
          onClick={() => resolve.mutate({ approve: false, rejection_reason: reason })}
          disabled={resolve.isPending}
        >
          {t('approvals.reject')}
        </Button>
        <Button onClick={() => resolve.mutate({ approve: true })} disabled={resolve.isPending}>
          {t('approvals.approve')}
          <span className="ml-2 hidden gap-0.5 opacity-60 sm:inline-flex">
            <Kbd>{mod()}</Kbd>
            <Kbd>{KBD.enter}</Kbd>
          </span>
        </Button>
      </div>

      {error && <p className="text-xs text-destructive">{error}</p>}
    </Card>
  )
}
