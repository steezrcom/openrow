import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

// Redirect-only route: approvals link here because they only know run ID,
// not the flow ID. We look up the run and forward to the nested route.
export const Route = createFileRoute('/app/flow_runs/$runId')({
  component: FlowRunRedirect,
})

function FlowRunRedirect() {
  const { runId } = Route.useParams()
  const t = useT()
  const navigate = useNavigate()
  const data = useQuery({ queryKey: ['flow-run', runId], queryFn: () => api.getFlowRun(runId) })

  useEffect(() => {
    if (data.data) {
      navigate({
        to: '/app/flows/$id/runs/$runId',
        params: { id: data.data.run.flow_id, runId },
        replace: true,
      })
    }
  }, [data.data, runId, navigate])

  return <div className="px-8 py-10 text-sm text-muted-foreground">{t('common.loading')}</div>
}
