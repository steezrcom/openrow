import { useMemo } from 'react'
import type { Flow } from '@/lib/api'
import {
  classifyTool,
  connectorLabel,
  connectorSubtitle,
  scheduleLabel,
} from '@/lib/flow-meta'
import { cn } from '@/lib/utils'

// FlowDiagram shows a single flow as a centered hero card with its
// connector reads on the left and writes on the right. Used at the top
// of the flow detail page to give a glance-able picture before any text.
//
// If the flow only uses internal tools (no connector_*), we still render
// the centre card so the trigger/mode are visible in the same layout as
// the full graph.
export function FlowDiagram({ flow }: { flow: Flow }) {
  const layout = useMemo(() => computeLayout(flow), [flow])

  return (
    <div className="w-full overflow-x-auto">
      <svg
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width="100%"
        style={{ minWidth: 600, maxHeight: layout.height + 24 }}
        className="text-foreground"
      >
        {/* Trigger pill above the flow card */}
        <g>
          <rect
            x={layout.flow.x}
            y={layout.flow.y - 36}
            width={FLOW_W}
            height={24}
            rx={12}
            className="fill-muted/40 stroke-border"
            strokeWidth={1}
          />
          <text
            x={layout.flow.x + FLOW_W / 2}
            y={layout.flow.y - 19}
            textAnchor="middle"
            className="fill-muted-foreground"
            style={{ fontSize: 11 }}
          >
            {scheduleLabel(flow)}
          </text>
        </g>

        {/* Edges (drawn before nodes) */}
        <g fill="none" stroke="currentColor" strokeOpacity={0.25} strokeWidth={1.25}>
          {layout.edges.map((e, i) => (
            <path key={i} d={e.path} />
          ))}
        </g>

        {/* Input connectors */}
        {layout.inputs.map((n) => (
          <ConnectorNode key={`in-${n.id}`} side="in" x={layout.colsInputX} y={n.y} id={n.id} />
        ))}
        {/* Output connectors */}
        {layout.outputs.map((n) => (
          <ConnectorNode key={`out-${n.id}`} side="out" x={layout.colsOutputX} y={n.y} id={n.id} />
        ))}

        {/* Flow node */}
        <FlowCard x={layout.flow.x} y={layout.flow.y} flow={flow} />

        {/* Empty-state helpers when one side has no connectors */}
        {layout.inputs.length === 0 && (
          <text
            x={layout.colsInputX + NODE_W / 2}
            y={layout.flow.y + FLOW_H / 2}
            textAnchor="middle"
            className="fill-muted-foreground/60"
            style={{ fontSize: 11 }}
          >
            interní spouštěč
          </text>
        )}
        {layout.outputs.length === 0 && (
          <text
            x={layout.colsOutputX + NODE_W / 2}
            y={layout.flow.y + FLOW_H / 2}
            textAnchor="middle"
            className="fill-muted-foreground/60"
            style={{ fontSize: 11 }}
          >
            jen interní zápisy
          </text>
        )}
      </svg>
    </div>
  )
}

// --- nodes ----------------------------------------------------------------

const NODE_W = 170
const NODE_H = 44
const FLOW_W = 280
const FLOW_H = 92
const COL_GAP = 90
const ROW_GAP = 12

function ConnectorNode({
  id,
  side,
  x,
  y,
}: {
  id: string
  side: 'in' | 'out'
  x: number
  y: number
}) {
  return (
    <g>
      <rect x={x} y={y} width={NODE_W} height={NODE_H} rx={6} className="fill-card stroke-border" strokeWidth={1} />
      <text x={x + 12} y={y + 18} className="fill-foreground" style={{ fontSize: 13, fontWeight: 500 }}>
        {connectorLabel(id)}
      </text>
      <text x={x + 12} y={y + 34} className="fill-muted-foreground" style={{ fontSize: 10 }}>
        {connectorSubtitle(id, side)}
      </text>
    </g>
  )
}

function FlowCard({ x, y, flow }: { x: number; y: number; flow: Flow }) {
  const fill =
    flow.mode === 'auto'
      ? 'fill-primary/15'
      : flow.mode === 'approve'
        ? 'fill-amber-500/15'
        : 'fill-muted/40'
  const stroke =
    flow.mode === 'auto'
      ? 'stroke-primary'
      : flow.mode === 'approve'
        ? 'stroke-amber-500'
        : 'stroke-border'
  return (
    <g>
      <rect x={x} y={y} width={FLOW_W} height={FLOW_H} rx={10} strokeWidth={2} className={cn(fill, stroke)} />
      <text x={x + FLOW_W / 2} y={y + 28} textAnchor="middle" className="fill-foreground" style={{ fontSize: 14, fontWeight: 600 }}>
        {truncate(flow.name, 32)}
      </text>
      {flow.description && (
        <text x={x + FLOW_W / 2} y={y + 50} textAnchor="middle" className="fill-muted-foreground" style={{ fontSize: 11 }}>
          {truncate(flow.description, 44)}
        </text>
      )}
      <g style={{ fontSize: 9 }}>
        <rect
          x={x + FLOW_W / 2 - 40}
          y={y + FLOW_H - 26}
          width={80}
          height={18}
          rx={4}
          className={fill}
        />
        <text
          x={x + FLOW_W / 2}
          y={y + FLOW_H - 13}
          textAnchor="middle"
          className={cn(
            flow.mode === 'auto' && 'fill-primary',
            flow.mode === 'approve' && 'fill-amber-600 dark:fill-amber-400',
            flow.mode === 'dry_run' && 'fill-muted-foreground',
          )}
          style={{ fontWeight: 600, letterSpacing: '0.05em' }}
        >
          {flow.mode.toUpperCase()}
        </text>
      </g>
    </g>
  )
}

// --- layout ---------------------------------------------------------------

interface ConnectorRow {
  id: string
  y: number
}

interface Layout {
  width: number
  height: number
  colsInputX: number
  colsOutputX: number
  flow: { x: number; y: number }
  inputs: ConnectorRow[]
  outputs: ConnectorRow[]
  edges: { path: string }[]
}

function computeLayout(flow: Flow): Layout {
  const ins = new Set<string>()
  const outs = new Set<string>()
  for (const tool of flow.tool_allowlist ?? []) {
    const ref = classifyTool(tool)
    if (!ref) continue
    if (ref.side === 'in') ins.add(ref.connector)
    else outs.add(ref.connector)
  }

  const sortedIn = [...ins].sort((a, b) => connectorLabel(a).localeCompare(connectorLabel(b)))
  const sortedOut = [...outs].sort((a, b) => connectorLabel(a).localeCompare(connectorLabel(b)))

  // Vertical span needed to fit the taller column without overlapping the
  // flow card. Pad to a minimum so the layout doesn't collapse on flows
  // with zero or one connector.
  const sideHeight = (count: number) => count * NODE_H + Math.max(0, count - 1) * ROW_GAP
  const innerHeight = Math.max(sideHeight(sortedIn.length), sideHeight(sortedOut.length), FLOW_H)

  // Center each column vertically against the inner height. Use 50 of
  // top padding to leave room for the trigger pill above the flow card.
  const topPad = 50
  const centerY = topPad + innerHeight / 2

  const inputs: ConnectorRow[] = sortedIn.map((id, i) => ({
    id,
    y: centerY - sideHeight(sortedIn.length) / 2 + i * (NODE_H + ROW_GAP),
  }))
  const outputs: ConnectorRow[] = sortedOut.map((id, i) => ({
    id,
    y: centerY - sideHeight(sortedOut.length) / 2 + i * (NODE_H + ROW_GAP),
  }))
  const flowY = centerY - FLOW_H / 2

  const colsInputX = 0
  const flowX = NODE_W + COL_GAP
  const colsOutputX = flowX + FLOW_W + COL_GAP
  const width = colsOutputX + NODE_W

  // Edges
  const edges: { path: string }[] = []
  for (const n of inputs) {
    const x1 = colsInputX + NODE_W
    const y1 = n.y + NODE_H / 2
    const x2 = flowX
    const y2 = flowY + FLOW_H / 2
    edges.push({ path: bezier(x1, y1, x2, y2) })
  }
  for (const n of outputs) {
    const x1 = flowX + FLOW_W
    const y1 = flowY + FLOW_H / 2
    const x2 = colsOutputX
    const y2 = n.y + NODE_H / 2
    edges.push({ path: bezier(x1, y1, x2, y2) })
  }

  return {
    width,
    height: topPad + innerHeight + 20,
    colsInputX,
    colsOutputX,
    flow: { x: flowX, y: flowY },
    inputs,
    outputs,
    edges,
  }
}

function bezier(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(40, (x2 - x1) * 0.45)
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
