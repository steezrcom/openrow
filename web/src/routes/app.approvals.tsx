import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ChevronRight, ShieldAlert } from 'lucide-react'
import { api, type FlowApproval } from '@/lib/api'
import { Button, Card } from '@/components/ui'
import { useT } from '@/lib/i18n'

export const Route = createFileRoute('/app/approvals')({
  component: ApprovalsPage,
})

function ApprovalsPage() {
  const t = useT()
  const list = useQuery({ queryKey: ['flow-approvals'], queryFn: api.listFlowApprovals })

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

      {list.isLoading && <p className="text-sm text-muted-foreground">{t('common.loading')}</p>}

      {!list.isLoading && (list.data ?? []).length === 0 && (
        <Card className="p-8 text-center">
          <p className="text-sm text-muted-foreground">{t('approvals.empty')}</p>
        </Card>
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
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ['flow-approvals'] })
      qc.invalidateQueries({ queryKey: ['flow-run', r.run?.id] })
      qc.invalidateQueries({ queryKey: ['flow-runs'] })
      if (r.error) setError(r.error)
    },
    onError: (e) => setError(e instanceof Error ? e.message : 'failed'),
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
        </Button>
      </div>

      {error && <p className="text-xs text-destructive">{error}</p>}
    </Card>
  )
}
