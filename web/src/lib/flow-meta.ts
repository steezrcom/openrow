// Shared helpers for rendering flow metadata in graph + detail views.
// Kept in /lib (not /components) so non-React code paths can reuse the
// schedule humaniser if needed.

import type { Flow } from '@/lib/api'

export type ConnectorSide = 'in' | 'out'

export interface ConnectorRef {
  connector: string
  side: ConnectorSide
  action: string
}

// classifyTool maps a tool name from a flow's allowlist to the connector
// it touches and whether the flow reads or writes through it. Returns
// null for internal tools (query_rows, update_row, render_document, ...)
// that don't connect to an external system.
export function classifyTool(tool: string): ConnectorRef | null {
  if (!tool.startsWith('connector_')) return null
  const rest = tool.slice('connector_'.length)
  const [connector, ...actionParts] = rest.split('_')
  if (!connector) return null
  const action = actionParts.join('_')
  return { connector, action, side: inferSide(action) }
}

function inferSide(action: string): ConnectorSide {
  // Heuristic: writes if the action name implies mutation, else read.
  const writeVerbs = ['create', 'post', 'send', 'update', 'mark', 'pay', 'add', 'rotate', 'delete']
  if (writeVerbs.some((v) => action.startsWith(v))) return 'out'
  return 'in'
}

export const CONNECTOR_LABELS: Record<string, string> = {
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

export function connectorLabel(id: string): string {
  return CONNECTOR_LABELS[id] ?? id
}

export function connectorSubtitle(id: string, side: ConnectorSide): string {
  if (['csas', 'csob', 'fio', 'revolut'].includes(id)) return side === 'in' ? 'banking · read' : 'banking'
  if (id === 'uol') return side === 'in' ? 'accounting · read' : 'accounting · write'
  if (id === 'notion') return side === 'in' ? 'notion · read' : 'notion · write'
  if (id === 'resend') return 'e-mail'
  if (id === 'slack' || id === 'discord') return 'messaging'
  if (id === 'fakturoid') return side === 'in' ? 'invoicing · read' : 'invoicing · write'
  return 'connector'
}

// scheduleLabel renders a cron expression as a localised Czech phrase.
// Falls back to the raw cron string for patterns we haven't templated.
export function scheduleLabel(f: Flow): string {
  if (f.trigger_kind !== 'cron') {
    if (f.trigger_kind === 'manual') return 'Spouští se ručně'
    if (f.trigger_kind === 'webhook') return 'Webhook'
    if (f.trigger_kind === 'entity_event') return 'Při změně řádku'
    return f.trigger_kind
  }
  const cron = (f.trigger_config?.cron as string) ?? ''
  return humanCron(cron)
}

export function humanCron(cron: string): string {
  if (!cron) return 'cron'
  const parts = cron.split(/\s+/)
  if (parts.length < 5) return cron
  const [min, hour, dom, mon, dow] = parts

  if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Každých ${min.slice(2)} min`
  }
  if (min === '0' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
    return `Každé ${hour.slice(2)} h`
  }
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '*') {
    return `Denně v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1') {
    return `Po v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '1-5') {
    return `Po-Pá v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  if (!min.includes('*') && !hour.includes('*') && dom === '*' && mon === '*' && dow === '5') {
    return `Pá v ${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  }
  if (!min.includes('*') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Každou hodinu v :${min.padStart(2, '0')}`
  }
  return cron
}

// cronTime returns the daily firing time as minutes since midnight, used
// to sort flow rows in a stable timeline. Wildcards collapse to 0.
export function cronTime(f: Flow): number {
  if (f.trigger_kind !== 'cron') return 0
  const cron = (f.trigger_config?.cron as string) ?? ''
  const [m = '*', h = '*'] = cron.split(/\s+/)
  const mNum = m.startsWith('*') ? 0 : parseInt(m.replace('*/', '0'), 10) || 0
  const hNum = h.startsWith('*') ? 0 : parseInt(h, 10) || 0
  return hNum * 60 + mNum
}

// Group a flow's tool_allowlist into internal verbs and per-connector
// action lists, in stable display order. Connector entries split actions
// into read vs write sides.
export interface ConnectorGroup {
  connector: string
  reads: string[]
  writes: string[]
}

export function groupTools(allowlist: string[]): {
  internal: string[]
  connectors: ConnectorGroup[]
} {
  const internal: string[] = []
  const byConnector = new Map<string, ConnectorGroup>()

  for (const tool of allowlist) {
    const ref = classifyTool(tool)
    if (!ref) {
      internal.push(tool)
      continue
    }
    if (!byConnector.has(ref.connector)) {
      byConnector.set(ref.connector, { connector: ref.connector, reads: [], writes: [] })
    }
    const g = byConnector.get(ref.connector)!
    if (ref.side === 'out') g.writes.push(ref.action)
    else g.reads.push(ref.action)
  }
  // Stable order: internal alphabetised, connectors alphabetised by label.
  internal.sort()
  const connectors = [...byConnector.values()].sort((a, b) =>
    connectorLabel(a.connector).localeCompare(connectorLabel(b.connector)),
  )
  for (const g of connectors) {
    g.reads.sort()
    g.writes.sort()
  }
  return { internal, connectors }
}
