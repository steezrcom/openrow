import { createFileRoute, Outlet } from '@tanstack/react-router'
import { Play } from 'lucide-react'
import type { Flow } from '@/lib/api'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

// This file is a layout for /app/flows. Without it, TanStack Router
// would still auto-infer a layout from the presence of nested
// children (app.flows.$id.tsx, app.flows.new.tsx); having it explicit
// lets us keep the badge helpers colocated for reuse.
//
// The actual list page lives in app.flows.index.tsx so that
// navigating to a child route (/app/flows/$id, /app/flows/new) shows
// only the child — not the list stacked on top.
export const Route = createFileRoute('/app/flows')({
  component: FlowsLayout,
})

function FlowsLayout() {
  return <Outlet />
}

export function ModeBadge({ mode }: { mode: Flow['mode'] }) {
  const t = useT()
  const className = cn(
    'rounded px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider',
    mode === 'auto' && 'bg-primary/15 text-primary',
    mode === 'approve' && 'bg-amber-500/20 text-amber-600 dark:text-amber-400',
    mode === 'dry_run' && 'bg-muted text-muted-foreground'
  )
  return <span className={className}>{t(`flows.mode.${mode}` as const)}</span>
}

export function TriggerBadge({ kind }: { kind: Flow['trigger_kind'] }) {
  const t = useT()
  return (
    <span className="rounded border border-border px-2 py-0.5 text-[10px] uppercase tracking-wider text-muted-foreground">
      {kind === 'manual' && <Play className="mr-1 inline h-2.5 w-2.5" />}
      {t(`flows.trigger.${kind}` as const)}
    </span>
  )
}
