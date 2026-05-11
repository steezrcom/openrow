import { useMemo, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import type { Flow } from '@/lib/api'
import { cn } from '@/lib/utils'

// FlowGraph renders the tenant's flows as a three-band SVG diagram:
// inputs (connectors a flow reads from) on the left, flow nodes in the
// middle, outputs (connectors a flow writes to) on the right. Edges are
// inferred from each flow's tool_allowlist — no extra metadata required.
// Click a flow to navigate to its detail page; hover highlights the
// connectors it touches.
export function FlowGraph({ flows }: { flows: Flow[] }) {
  const navigate = useNavigate()
  const [hoveredFlow, setHoveredFlow] = useState<string | null>(null)
  const [hoveredConnector, setHoveredConnector] = useState<string | null>(null)

  const layout = useMemo(() => computeLayout(flows), [flows])

  if (flows.length === 0) {
    return null
  }

  const isHighlighted = (flowId: string, side: 'in' | 'out', connector: string) => {
    if (hoveredFlow) return hoveredFlow === flowId
    if (hoveredConnector) return hoveredConnector === `${side}:${connector}`
    return false
  }

  return (
    <div className="w-full overflow-x-auto">
      <svg
        viewBox={`0 0 ${layout.width} ${layout.height}`}
        width="100%"
        style={{ minWidth: layout.width, maxHeight: layout.height + 40 }}
        className="text-foreground"
      >
        {/* Column labels */}
        <g className="text-xs fill-muted-foreground" style={{ fontSize: 11 }}>
          <text x={layout.cols.input.x + NODE_W / 2} y={20} textAnchor="middle">
            Vstupy
          </text>
          <text x={layout.cols.flow.x + FLOW_W / 2} y={20} textAnchor="middle">
            Toky
          </text>
          <text x={layout.cols.output.x + NODE_W / 2} y={20} textAnchor="middle">
            Výstupy
          </text>
        </g>

        {/* Edges (drawn first so nodes sit on top) */}
        <g fill="none">
          {layout.edges.map((e, i) => {
            const dim = (hoveredFlow && hoveredFlow !== e.flowId) ||
              (hoveredConnector && hoveredConnector !== `${e.side}:${e.connector}`)
            const active = isHighlighted(e.flowId, e.side, e.connector)
            return (
              <path
                key={i}
                d={e.path}
                stroke={active ? 'currentColor' : 'currentColor'}
                strokeOpacity={active ? 0.6 : dim ? 0.06 : 0.18}
                strokeWidth={active ? 1.5 : 1}
              />
            )
          })}
        </g>

        {/* Input nodes */}
        {layout.inputs.map((n) => (
          <ConnectorNode
            key={`in-${n.id}`}
            node={n}
            x={layout.cols.input.x}
            highlighted={hoveredConnector === `in:${n.id}` || (!!hoveredFlow && n.flowIds.has(hoveredFlow))}
            onEnter={() => setHoveredConnector(`in:${n.id}`)}
            onLeave={() => setHoveredConnector(null)}
          />
        ))}

        {/* Output nodes */}
        {layout.outputs.map((n) => (
          <ConnectorNode
            key={`out-${n.id}`}
            node={n}
            x={layout.cols.output.x}
            highlighted={hoveredConnector === `out:${n.id}` || (!!hoveredFlow && n.flowIds.has(hoveredFlow))}
            onEnter={() => setHoveredConnector(`out:${n.id}`)}
            onLeave={() => setHoveredConnector(null)}
          />
        ))}

        {/* Flow nodes */}
        {layout.flows.map((n) => (
          <FlowNode
            key={n.id}
            node={n}
            x={layout.cols.flow.x}
            highlighted={hoveredFlow === n.id}
            dim={!!hoveredFlow && hoveredFlow !== n.id}
            onEnter={() => setHoveredFlow(n.id)}
            onLeave={() => setHoveredFlow(null)}
            onClick={() => navigate({ to: '/app/flows/$id', params: { id: n.id } })}
          />
        ))}
      </svg>
    </div>
  )
}

// --- nodes ----------------------------------------------------------------

const NODE_W = 180
const NODE_H = 44
const FLOW_W = 320
const FLOW_H = 70
const COL_GAP = 140
const ROW_GAP = 14

function ConnectorNode({
  node,
  x,
  highlighted,
  onEnter,
  onLeave,
}: {
  node: ConnectorLayoutNode
  x: number
  highlighted: boolean
  onEnter: () => void
  onLeave: () => void
}) {
  return (
    <g onMouseEnter={onEnter} onMouseLeave={onLeave} style={{ cursor: 'default' }}>
      <rect
        x={x}
        y={node.y}
        width={NODE_W}
        height={NODE_H}
        rx={6}
        className={cn(
          'transition-colors',
          highlighted ? 'fill-primary/15 stroke-primary' : 'fill-card stroke-border',
        )}
        strokeWidth={1}
      />
      <text
        x={x + 12}
        y={node.y + 18}
        className="fill-foreground"
        style={{ fontSize: 13, fontWeight: 500 }}
      >
        {node.label}
      </text>
      <text
        x={x + 12}
        y={node.y + 34}
        className="fill-muted-foreground"
        style={{ fontSize: 10 }}
      >
        {node.subtitle}
      </text>
    </g>
  )
}

function FlowNode({
  node,
  x,
  highlighted,
  dim,
  onEnter,
  onLeave,
  onClick,
}: {
  node: FlowLayoutNode
  x: number
  highlighted: boolean
  dim: boolean
  onEnter: () => void
  onLeave: () => void
  onClick: () => void
}) {
  const modeFill =
    node.flow.mode === 'auto'
      ? 'fill-primary/15'
      : node.flow.mode === 'approve'
        ? 'fill-amber-500/15'
        : 'fill-muted/40'
  const modeStroke =
    node.flow.mode === 'auto'
      ? 'stroke-primary'
      : node.flow.mode === 'approve'
        ? 'stroke-amber-500'
        : 'stroke-border'
  return (
    <g
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      onClick={onClick}
      style={{ cursor: 'pointer', opacity: dim ? 0.4 : 1 }}
    >
      <rect
        x={x}
        y={node.y}
        width={FLOW_W}
        height={FLOW_H}
        rx={8}
        className={cn('transition-colors', modeFill, modeStroke, highlighted && 'stroke-2')}
        strokeWidth={highlighted ? 2 : 1}
      />
      <text
        x={x + 14}
        y={node.y + 20}
        className="fill-foreground"
        style={{ fontSize: 13, fontWeight: 600 }}
      >
        {truncate(node.flow.name, 36)}
      </text>
      <text
        x={x + 14}
        y={node.y + 38}
        className="fill-muted-foreground"
        style={{ fontSize: 10 }}
      >
        {node.scheduleLabel}
      </text>
      <g style={{ fontSize: 9 }}>
        <rect
          x={x + 14}
          y={node.y + FLOW_H - 22}
          width={56}
          height={16}
          rx={3}
          className={cn('stroke-0', modeFill)}
        />
        <text
          x={x + 14 + 28}
          y={node.y + FLOW_H - 10}
          textAnchor="middle"
          className={cn(
            node.flow.mode === 'auto' && 'fill-primary',
            node.flow.mode === 'approve' && 'fill-amber-600 dark:fill-amber-400',
            node.flow.mode === 'dry_run' && 'fill-muted-foreground',
          )}
        >
          {node.flow.mode.toUpperCase()}
        </text>
        {!node.flow.enabled && (
          <text
            x={x + 80}
            y={node.y + FLOW_H - 10}
            className="fill-muted-foreground"
            style={{ fontSize: 10 }}
          >
            disabled
          </text>
        )}
      </g>
    </g>
  )
}

// --- layout ---------------------------------------------------------------

interface ConnectorLayoutNode {
  id: string
  label: string
  subtitle: string
  y: number
  flowIds: Set<string>
}

interface FlowLayoutNode {
  id: string
  flow: Flow
  y: number
  scheduleLabel: string
}

interface EdgeLayout {
  flowId: string
  connector: string
  side: 'in' | 'out'
  path: string
}

interface Layout {
  width: number
  height: number
  cols: {
    input: { x: number }
    flow: { x: number }
    output: { x: number }
  }
  inputs: ConnectorLayoutNode[]
  outputs: ConnectorLayoutNode[]
  flows: FlowLayoutNode[]
  edges: EdgeLayout[]
}

function computeLayout(flows: Flow[]): Layout {
  // 1. Bucket each flow's tools into input / output connector references.
  type Edge = { flowId: string; connector: string; side: 'in' | 'out' }
  const edges: Edge[] = []
  for (const f of flows) {
    for (const tool of f.tool_allowlist ?? []) {
      const ref = classifyTool(tool)
      if (!ref) continue
      edges.push({ flowId: f.id, connector: ref.connector, side: ref.side })
    }
  }

  // De-dupe (a flow might reference list_X and create_X on the same
  // connector — show one edge per side).
  const seen = new Set<string>()
  const uniqueEdges = edges.filter((e) => {
    const key = `${e.flowId}|${e.side}|${e.connector}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })

  // 2. Collect unique connectors per side, with the flows that touch them.
  const inMap = new Map<string, Set<string>>()
  const outMap = new Map<string, Set<string>>()
  for (const e of uniqueEdges) {
    const map = e.side === 'in' ? inMap : outMap
    if (!map.has(e.connector)) map.set(e.connector, new Set())
    map.get(e.connector)!.add(e.flowId)
  }

  // Sort connectors alphabetically for a stable layout.
  const inputs = [...inMap.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([id, flowIds], i) => ({
      id,
      label: connectorLabel(id),
      subtitle: connectorSubtitle(id, 'in'),
      y: 40 + i * (NODE_H + ROW_GAP),
      flowIds,
    }))
  const outputs = [...outMap.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([id, flowIds], i) => ({
      id,
      label: connectorLabel(id),
      subtitle: connectorSubtitle(id, 'out'),
      y: 40 + i * (NODE_H + ROW_GAP),
      flowIds,
    }))

  // 3. Order flows by a stable category + cron time for top-to-bottom flow.
  const ordered = [...flows].sort((a, b) => {
    const ca = categoryRank(a)
    const cb = categoryRank(b)
    if (ca !== cb) return ca - cb
    const ta = cronTime(a)
    const tb = cronTime(b)
    if (ta !== tb) return ta - tb
    return a.name.localeCompare(b.name)
  })
  const flowNodes: FlowLayoutNode[] = ordered.map((f, i) => ({
    id: f.id,
    flow: f,
    y: 40 + i * (FLOW_H + ROW_GAP),
    scheduleLabel: scheduleLabel(f),
  }))

  // 4. Column positions.
  const colX = {
    input: 0,
    flow: NODE_W + COL_GAP,
    output: NODE_W + COL_GAP + FLOW_W + COL_GAP,
  }
  const width = colX.output + NODE_W + 8

  // 5. Edge paths (cubic bezier with horizontal control points).
  const flowById = new Map(flowNodes.map((n) => [n.id, n]))
  const inputById = new Map(inputs.map((n) => [n.id, n]))
  const outputById = new Map(outputs.map((n) => [n.id, n]))

  const edgePaths: EdgeLayout[] = []
  for (const e of uniqueEdges) {
    const fn = flowById.get(e.flowId)
    if (!fn) continue
    if (e.side === 'in') {
      const cn = inputById.get(e.connector)
      if (!cn) continue
      const x1 = colX.input + NODE_W
      const y1 = cn.y + NODE_H / 2
      const x2 = colX.flow
      const y2 = fn.y + FLOW_H / 2
      edgePaths.push({
        flowId: e.flowId,
        connector: e.connector,
        side: 'in',
        path: bezier(x1, y1, x2, y2),
      })
    } else {
      const cn = outputById.get(e.connector)
      if (!cn) continue
      const x1 = colX.flow + FLOW_W
      const y1 = fn.y + FLOW_H / 2
      const x2 = colX.output
      const y2 = cn.y + NODE_H / 2
      edgePaths.push({
        flowId: e.flowId,
        connector: e.connector,
        side: 'out',
        path: bezier(x1, y1, x2, y2),
      })
    }
  }

  // 6. Compute final canvas height.
  const colHeight = Math.max(
    inputs.length > 0 ? inputs[inputs.length - 1].y + NODE_H : 0,
    outputs.length > 0 ? outputs[outputs.length - 1].y + NODE_H : 0,
    flowNodes.length > 0 ? flowNodes[flowNodes.length - 1].y + FLOW_H : 0,
  )

  return {
    width,
    height: colHeight + 30,
    cols: {
      input: { x: colX.input },
      flow: { x: colX.flow },
      output: { x: colX.output },
    },
    inputs,
    outputs,
    flows: flowNodes,
    edges: edgePaths,
  }
}

function bezier(x1: number, y1: number, x2: number, y2: number): string {
  const dx = Math.max(40, (x2 - x1) * 0.45)
  return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`
}

// --- tool classification --------------------------------------------------

// classifyTool inspects a tool name from the flow's allowlist and returns
// the connector reference it implies, plus whether the flow reads or
// writes through it. Internal tools (query_rows, update_row, etc.) and
// the deterministic reconcile/render tools are ignored — they don't
// connect to external systems.
function classifyTool(tool: string): { connector: string; side: 'in' | 'out' } | null {
  // Connector tools: connector_<id>_<action>
  if (tool.startsWith('connector_')) {
    const rest = tool.slice('connector_'.length)
    const [connector, ...actionParts] = rest.split('_')
    const action = actionParts.join('_')
    const side = inferSide(connector, action)
    return { connector, side }
  }
  return null
}

function inferSide(_connector: string, action: string): 'in' | 'out' {
  // Heuristic: writes if the action name implies mutation, else read.
  const writeVerbs = ['create', 'post', 'send', 'update', 'mark', 'pay', 'add']
  if (writeVerbs.some((v) => action.startsWith(v))) return 'out'
  return 'in'
}

const CONNECTOR_LABELS: Record<string, string> = {
  csas: 'Česká spořitelna',
  csob: 'ČSOB',
  fio: 'Fio banka',
  revolut: 'Revolut Business',
  uol: 'ÚOL',
  fakturoid: 'Fakturoid',
  notion: 'Notion',
  resend: 'Resend',
  slack: 'Slack',
  discord: 'Discord',
  github: 'GitHub',
  linear: 'Linear',
  ares: 'ARES',
  cnb: 'ČNB',
  vies: 'VIES',
  stripe: 'Stripe',
}

function connectorLabel(id: string): string {
  return CONNECTOR_LABELS[id] ?? id
}

function connectorSubtitle(id: string, side: 'in' | 'out'): string {
  if (['csas', 'csob', 'fio', 'revolut'].includes(id)) return side === 'in' ? 'banking · read' : 'banking'
  if (id === 'uol') return side === 'in' ? 'accounting · read' : 'accounting · write'
  if (id === 'notion') return side === 'in' ? 'notion · read' : 'notion · write'
  if (id === 'resend') return 'e-mail'
  if (id === 'slack' || id === 'discord') return 'messaging'
  if (id === 'fakturoid') return side === 'in' ? 'invoicing · read' : 'invoicing · write'
  return 'connector'
}

// --- flow categorisation + schedule ---------------------------------------

function categoryRank(f: Flow): number {
  const name = f.name.toLowerCase()
  if (name.includes('sync banky')) return 1
  if (name.includes('sync přijatých')) return 2
  if (name.includes('párování')) return 3
  if (name.includes('klasifikace')) return 4
  if (name.includes('schválení')) return 5
  if (name.includes('vystavit fakturu v uol')) return 6
  if (name.includes('odeslat fakturu')) return 7
  if (name.includes('upomínky')) return 8
  if (name.includes('mirror')) return 9
  if (name.includes('deal')) return 10
  if (name.includes('objednávka')) return 11
  if (name.includes('projekt dokon')) return 12
  if (name.includes('kontrola')) return 13
  return 50
}

function cronTime(f: Flow): number {
  if (f.trigger_kind !== 'cron') return 0
  const cron = (f.trigger_config?.cron as string) ?? ''
  // Parse "minute hour ..." — sort by hour*60+minute. Wildcards => 0.
  const [m = '*', h = '*'] = cron.split(/\s+/)
  const mNum = m.startsWith('*') ? 0 : parseInt(m.replace('*/', '0'), 10) || 0
  const hNum = h.startsWith('*') ? 0 : parseInt(h, 10) || 0
  return hNum * 60 + mNum
}

function scheduleLabel(f: Flow): string {
  if (f.trigger_kind !== 'cron') {
    if (f.trigger_kind === 'manual') return 'Spouští se ručně'
    if (f.trigger_kind === 'webhook') return 'Webhook'
    if (f.trigger_kind === 'entity_event') return 'Při změně řádku'
    return f.trigger_kind
  }
  const cron = (f.trigger_config?.cron as string) ?? ''
  return humanCron(cron)
}

function humanCron(cron: string): string {
  if (!cron) return 'cron'
  const parts = cron.split(/\s+/)
  if (parts.length < 5) return cron
  const [min, hour, dom, mon, dow] = parts

  // Every N minutes
  if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Každých ${min.slice(2)} min`
  }
  // Every N hours
  if (min === '0' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
    return `Každé ${hour.slice(2)} h`
  }
  // At HH:MM every day
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '*') {
    return `Denně v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  // At HH:MM every Monday
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1') {
    return `Po v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  // At HH:MM every weekday
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1-5') {
    return `Po-Pá v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  // At HH:MM every Friday
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '5') {
    return `Pá v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  // At MM past every hour
  if (!min.includes('*') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Každou hodinu v :${min.padStart(2, '0')}`
  }
  return cron
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
