import { createFileRoute, Outlet } from '@tanstack/react-router'
import type { FlowRunStatus } from '@/lib/api'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

// Layout for /app/flows/$id. Detail content lives in
// app.flows.$id.index.tsx so nested routes (runs.$runId) render on
// their own rather than stacking on top of the detail.
export const Route = createFileRoute('/app/flows/$id')({
  component: FlowIdLayout,
})

function FlowIdLayout() {
  return <Outlet />
}

export function RunStatusBadge({ status }: { status: FlowRunStatus }) {
  const t = useT()
  const className = cn(
    'rounded px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider',
    status === 'succeeded' && 'bg-primary/15 text-primary',
    status === 'failed' && 'bg-destructive/20 text-destructive',
    status === 'running' && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
    status === 'awaiting_approval' && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
    status === 'queued' && 'bg-muted text-muted-foreground'
  )
  return <span className={className}>{t(`flows.runs.status.${status}` as const)}</span>
}
