import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { AlertTriangle, Bot, Check, ChevronDown, ChevronRight, Copy, ShieldAlert, Wrench } from 'lucide-react'
import type { FlowRunStep } from '@/lib/api'
import { useT } from '@/lib/i18n'
import { cn } from '@/lib/utils'

// FlowRunSteps renders the timeline of a flow run with much friendlier
// formatting than a raw JSON dump:
//   - Agent messages are the dominant cards
//   - Tool calls + their result(s) pair into one indented block
//   - Arrays of objects render as compact tables with "show more"
//   - Anything large is collapsed by default with an expand toggle
//   - Untrusted-content delimiters from connector tools render as a callout

export function FlowRunSteps({ steps }: { steps: FlowRunStep[] }) {
  // Pair tool_call with the following tool_result so we can render them
  // together. Other step kinds pass through one-by-one.
  const groups = useMemo(() => groupSteps(steps), [steps])
  return (
    <div className="space-y-3">
      {groups.map((g, i) => (
        <StepGroup key={i} group={g} />
      ))}
    </div>
  )
}

// --- grouping -------------------------------------------------------------

type Group =
  | { kind: 'agent'; step: FlowRunStep }
  | { kind: 'tool'; call: FlowRunStep; result?: FlowRunStep }
  | { kind: 'blocked'; step: FlowRunStep }
  | { kind: 'approval'; step: FlowRunStep }

function groupSteps(steps: FlowRunStep[]): Group[] {
  const out: Group[] = []
  for (let i = 0; i < steps.length; i++) {
    const s = steps[i]
    if (s.kind === 'agent_message') {
      out.push({ kind: 'agent', step: s })
    } else if (s.kind === 'tool_call') {
      const next = steps[i + 1]
      if (next && next.kind === 'tool_result') {
        out.push({ kind: 'tool', call: s, result: next })
        i++
      } else {
        out.push({ kind: 'tool', call: s })
      }
    } else if (s.kind === 'mutation_blocked') {
      out.push({ kind: 'blocked', step: s })
    } else if (s.kind === 'approval_requested') {
      out.push({ kind: 'approval', step: s })
    }
  }
  return out
}

// --- group renderer -------------------------------------------------------

function StepGroup({ group }: { group: Group }) {
  const t = useT()
  if (group.kind === 'agent') {
    const c = group.step.content as Record<string, unknown>
    return (
      <div className="flex gap-3">
        <Avatar tone="primary" icon={Bot} />
        <div className="min-w-0 flex-1 rounded-lg border border-border bg-card p-4">
          <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {t('flows.step.agent')}
          </p>
          <p className="whitespace-pre-wrap text-sm leading-relaxed">{String(c.text ?? '')}</p>
        </div>
      </div>
    )
  }
  if (group.kind === 'blocked') {
    const c = group.step.content as Record<string, unknown>
    return (
      <div className="flex gap-3">
        <Avatar tone="amber" icon={AlertTriangle} />
        <div className="min-w-0 flex-1 rounded-lg border border-amber-500/40 bg-amber-500/5 p-4">
          <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
            {t('flows.step.blocked')}
          </p>
          <p className="font-mono text-xs text-muted-foreground">
            {String(c.name)} — {String(c.reason)}
          </p>
          {c.synthetic ? <p className="mt-1 text-xs">{String(c.synthetic)}</p> : null}
        </div>
      </div>
    )
  }
  if (group.kind === 'approval') {
    const c = group.step.content as Record<string, unknown>
    return (
      <div className="flex gap-3">
        <Avatar tone="amber" icon={ShieldAlert} />
        <div className="min-w-0 flex-1 rounded-lg border border-amber-500/40 bg-amber-500/5 p-4">
          <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
            {t('flows.step.approval')}
          </p>
          <p className="font-mono text-xs">{String(c.name)}</p>
          <p className="mt-2">
            <Link to="/app/approvals" className="text-sm text-primary hover:underline">
              {t('flows.step.approval.goto')}
            </Link>
          </p>
        </div>
      </div>
    )
  }
  // tool
  return <ToolCallGroup call={group.call} result={group.result} />
}

// --- tool call + result --------------------------------------------------

function ToolCallGroup({ call, result }: { call: FlowRunStep; result?: FlowRunStep }) {
  const callC = call.content as Record<string, unknown>
  const resC = (result?.content ?? {}) as Record<string, unknown>
  const toolName = String(callC.name ?? 'tool')
  const inputObj = callC.input
  const hasError = result && resC.error != null
  const resultText = result ? String(resC.result ?? '') : null

  return (
    <div className="flex gap-3">
      <Avatar tone={hasError ? 'destructive' : 'muted'} icon={Wrench} />
      <div className="min-w-0 flex-1 space-y-2">
        <div className="rounded-lg border border-border bg-card/60 p-4">
          <header className="mb-2 flex items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                Tool call
              </span>
              <code className={cn(
                'rounded px-1.5 py-0.5 font-mono text-[12px]',
                toolName.startsWith('connector_')
                  ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                  : 'bg-violet-500/10 text-violet-700 dark:text-violet-300',
              )}>
                {toolName}
              </code>
            </div>
            {result && (
              <span
                className={cn(
                  'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider',
                  hasError
                    ? 'bg-destructive/10 text-destructive'
                    : 'bg-primary/10 text-primary',
                )}
              >
                {hasError ? <AlertTriangle className="h-2.5 w-2.5" /> : <Check className="h-2.5 w-2.5" />}
                {hasError ? 'error' : 'ok'}
              </span>
            )}
          </header>

          {/* Input as labelled fields if it's a small object */}
          <InputBlock value={inputObj} />

          {/* Result panel */}
          {result && (
            <div className="mt-3 border-t border-border/60 pt-3">
              {hasError ? (
                <p className="whitespace-pre-wrap font-mono text-xs text-destructive">
                  {String(resC.error)}
                </p>
              ) : (
                <ResultBlock toolName={toolName} text={resultText ?? ''} />
              )}
            </div>
          )}

          {!result && (
            <p className="mt-2 text-xs text-muted-foreground">(awaiting result)</p>
          )}
        </div>
      </div>
    </div>
  )
}

// --- input -----------------------------------------------------------------

function InputBlock({ value }: { value: unknown }) {
  if (value == null || (typeof value === 'object' && Object.keys(value as object).length === 0)) {
    return <p className="text-xs italic text-muted-foreground">(no arguments)</p>
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length <= 6 && entries.every(([, v]) => isScalar(v))) {
      return (
        <dl className="grid grid-cols-[max-content,1fr] gap-x-3 gap-y-1 text-xs">
          {entries.map(([k, v]) => (
            <Inline key={k} k={k} v={v} />
          ))}
        </dl>
      )
    }
  }
  return <PrettyJSON value={value} initiallyOpen />
}

function Inline({ k, v }: { k: string; v: unknown }) {
  return (
    <>
      <dt className="font-mono text-muted-foreground">{k}</dt>
      <dd className="font-mono break-all">
        <ScalarValue v={v} />
      </dd>
    </>
  )
}

function ScalarValue({ v }: { v: unknown }) {
  if (v === null) return <span className="text-muted-foreground">null</span>
  if (typeof v === 'string') return <span className="text-amber-700 dark:text-amber-300">"{v}"</span>
  if (typeof v === 'boolean' || typeof v === 'number') return <span className="text-violet-700 dark:text-violet-300">{String(v)}</span>
  return <span>{JSON.stringify(v)}</span>
}

function isScalar(v: unknown) {
  return v === null || ['string', 'number', 'boolean'].includes(typeof v)
}

// --- result ----------------------------------------------------------------

function ResultBlock({ toolName, text }: { toolName: string; text: string }) {
  // Strip our prompt-injection delimiters before parsing. Render an
  // explicit pill when they were present so the operator sees we treated
  // the content as untrusted.
  const stripped = stripUntrustedMarkers(text)

  // Try to parse as JSON to give a structured rendering. If parsing
  // fails, fall back to a collapsible plain-text panel.
  const parsed = useMemo(() => safeJSON(stripped.text), [stripped.text])

  return (
    <div className="space-y-2">
      {stripped.wasUntrusted && (
        <p className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-[10px] uppercase tracking-wider text-muted-foreground">
          <ShieldAlert className="h-2.5 w-2.5" />
          untrusted external content
        </p>
      )}
      {parsed.ok ? (
        <ParsedResult toolName={toolName} value={parsed.value} raw={stripped.text} />
      ) : (
        <CollapsibleText text={stripped.text} />
      )}
    </div>
  )
}

function ParsedResult({ toolName, value, raw }: { toolName: string; value: unknown; raw: string }) {
  // If the value is an array of homogeneous objects, render as a table.
  if (Array.isArray(value) && value.length > 0 && typeof value[0] === 'object' && value[0] !== null) {
    return <ArrayTable toolName={toolName} rows={value as Record<string, unknown>[]} raw={raw} />
  }
  // If it's a small object, render inline-fields style.
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const entries = Object.entries(value as Record<string, unknown>)
    if (entries.length <= 8 && entries.every(([, v]) => isScalar(v))) {
      return (
        <dl className="grid grid-cols-[max-content,1fr] gap-x-3 gap-y-1 text-xs">
          {entries.map(([k, v]) => (
            <Inline key={k} k={k} v={v} />
          ))}
        </dl>
      )
    }
  }
  // Fallback: pretty-print collapsible JSON.
  return <PrettyJSON value={value} />
}

function ArrayTable({ toolName, rows, raw }: { toolName: string; rows: Record<string, unknown>[]; raw: string }) {
  const previewN = 5
  const showing = rows.slice(0, previewN)
  const moreCount = rows.length - showing.length
  // Pick columns: prefer common entity/transaction fields; fall back to first 4 keys.
  const cols = useMemo(() => pickColumns(rows), [rows])
  const [showAll, setShowAll] = useState(false)
  const data = showAll ? rows : showing
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          <span className="font-medium text-foreground">{rows.length}</span>{' '}
          {rows.length === 1 ? 'row' : 'rows'}
          {toolName === 'list_entities' && ' (entities)'}
        </span>
        <CopyButton text={raw} />
      </div>
      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-xs">
          <thead className="bg-muted/30 text-[10px] uppercase tracking-wider text-muted-foreground">
            <tr>
              {cols.map((c) => (
                <th key={c} className="px-2 py-1.5 text-left font-medium">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {data.map((r, i) => (
              <tr key={i} className="hover:bg-accent/30">
                {cols.map((c) => (
                  <td key={c} className="max-w-[260px] truncate px-2 py-1.5 font-mono">
                    {renderCell(r[c])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {moreCount > 0 && !showAll && (
        <button
          type="button"
          onClick={() => setShowAll(true)}
          className="text-xs text-primary hover:underline"
        >
          Show {moreCount} more {moreCount === 1 ? 'row' : 'rows'}
        </button>
      )}
      {showAll && rows.length > previewN && (
        <button
          type="button"
          onClick={() => setShowAll(false)}
          className="text-xs text-primary hover:underline"
        >
          Collapse
        </button>
      )}
    </div>
  )
}

// pickColumns chooses a small, readable set of columns from a heterogeneous
// row set. Prefers semantic fields when present; falls back to the first
// few keys of the first row.
function pickColumns(rows: Record<string, unknown>[]): string[] {
  const preferred = [
    'id', 'name', 'display_name', 'title', 'number', 'status', 'kind',
    'booking_date', 'date', 'amount', 'currency', 'counterparty', 'description',
    'variable_symbol', 'vs', 'total', 'due_date',
  ]
  const keys = new Set<string>()
  for (const r of rows.slice(0, 10)) {
    for (const k of Object.keys(r)) keys.add(k)
  }
  const present = preferred.filter((k) => keys.has(k))
  if (present.length >= 2) return present.slice(0, 6)
  // Fallback to the first object's keys, up to 5.
  return Object.keys(rows[0]).slice(0, 5)
}

function renderCell(v: unknown): React.ReactNode {
  if (v == null || v === '') return <span className="text-muted-foreground/40">—</span>
  if (typeof v === 'object') return <span className="text-muted-foreground">{summariseObject(v)}</span>
  if (typeof v === 'boolean') return <span>{v ? '✓' : '·'}</span>
  return String(v)
}

function summariseObject(v: object): string {
  if (Array.isArray(v)) return `[${v.length}]`
  const keys = Object.keys(v)
  if (keys.length === 0) return '{}'
  return `{${keys.length} keys}`
}

// --- pretty JSON -----------------------------------------------------------

function PrettyJSON({ value, initiallyOpen }: { value: unknown; initiallyOpen?: boolean }) {
  const formatted = useMemo(() => JSON.stringify(value, null, 2), [value])
  const big = formatted.length > 600 || formatted.split('\n').length > 14
  const [open, setOpen] = useState(initiallyOpen ?? !big)
  if (!open) {
    return (
      <div className="space-y-1">
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
        >
          <ChevronRight className="h-3 w-3" />
          Show JSON ({formatted.length.toLocaleString()} chars)
        </button>
      </div>
    )
  }
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ChevronDown className="h-3 w-3" />
          Hide JSON
        </button>
        <CopyButton text={formatted} />
      </div>
      <pre className="max-h-[480px] overflow-auto rounded-md border border-border bg-muted/20 p-3 text-[11px] leading-relaxed">
        <code className="font-mono">{colourJSON(formatted)}</code>
      </pre>
    </div>
  )
}

// colourJSON tokenises a pretty-printed JSON string and returns it with
// keys/strings/numbers/booleans/null distinguished. Cheap: one regex, no
// parsing-tree. Good enough for the run-step view.
function colourJSON(s: string): React.ReactNode[] {
  const out: React.ReactNode[] = []
  const re = /("([^"\\]|\\.)*"\s*:|"([^"\\]|\\.)*")|(\b(true|false|null)\b)|(-?\d+(\.\d+)?([eE][-+]?\d+)?)|([\[\]{}:,])/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(s)) !== null) {
    if (m.index > last) out.push(s.slice(last, m.index))
    const tok = m[0]
    if (tok.endsWith(':') || (m[1] && tok.endsWith(':'))) {
      out.push(<span key={m.index} className="text-violet-700 dark:text-violet-300">{tok}</span>)
    } else if (m[1]) {
      // string value
      out.push(<span key={m.index} className="text-amber-700 dark:text-amber-300">{tok}</span>)
    } else if (m[4]) {
      out.push(<span key={m.index} className="text-emerald-700 dark:text-emerald-300">{tok}</span>)
    } else if (m[6]) {
      out.push(<span key={m.index} className="text-emerald-700 dark:text-emerald-300">{tok}</span>)
    } else {
      out.push(<span key={m.index} className="text-muted-foreground">{tok}</span>)
    }
    last = m.index + tok.length
  }
  if (last < s.length) out.push(s.slice(last))
  return out
}

function safeJSON(text: string): { ok: true; value: unknown } | { ok: false } {
  const trimmed = text.trim()
  if (!trimmed) return { ok: false }
  if (!(trimmed.startsWith('{') || trimmed.startsWith('['))) return { ok: false }
  try {
    return { ok: true, value: JSON.parse(trimmed) }
  } catch {
    return { ok: false }
  }
}

// --- collapsible plain text ------------------------------------------------

function CollapsibleText({ text }: { text: string }) {
  const big = text.length > 600 || text.split('\n').length > 14
  const [open, setOpen] = useState(!big)
  if (!open) {
    const preview = text.slice(0, 280).replace(/\s+/g, ' ')
    return (
      <div className="space-y-1">
        <p className="line-clamp-2 text-xs text-muted-foreground">{preview}…</p>
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
        >
          <ChevronRight className="h-3 w-3" />
          Show full output ({text.length.toLocaleString()} chars)
        </button>
      </div>
    )
  }
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ChevronDown className="h-3 w-3" />
          Collapse
        </button>
        <CopyButton text={text} />
      </div>
      <pre className="max-h-[480px] overflow-auto whitespace-pre-wrap rounded-md border border-border bg-muted/20 p-3 text-[11px] leading-relaxed font-mono">
        {text}
      </pre>
    </div>
  )
}

// --- untrusted marker stripper ---------------------------------------------

const BEGIN_MARK = 'BEGIN_UNTRUSTED_TOOL_RESULT'
const END_MARK = 'END_UNTRUSTED_TOOL_RESULT'

function stripUntrustedMarkers(text: string): { text: string; wasUntrusted: boolean } {
  if (!text.includes(BEGIN_MARK)) return { text, wasUntrusted: false }
  // Find the BEGIN line, take everything after it up to END.
  const begin = text.indexOf('\n', text.indexOf(BEGIN_MARK))
  const end = text.lastIndexOf(END_MARK)
  if (begin < 0 || end < 0 || end <= begin) return { text, wasUntrusted: true }
  let inner = text.slice(begin + 1, end).trim()
  // Strip the leading separator line ("---") if present.
  if (inner.startsWith('---')) {
    const nl = inner.indexOf('\n')
    if (nl > 0) inner = inner.slice(nl + 1).trim()
  }
  if (inner.endsWith('---')) {
    inner = inner.slice(0, inner.lastIndexOf('---')).trim()
  }
  return { text: inner, wasUntrusted: true }
}

// --- avatar + copy --------------------------------------------------------

function Avatar({
  tone,
  icon: Icon,
}: {
  tone: 'primary' | 'muted' | 'amber' | 'destructive'
  icon: React.ComponentType<{ className?: string }>
}) {
  return (
    <div
      className={cn(
        'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full',
        tone === 'primary' && 'bg-primary/15 text-primary',
        tone === 'muted' && 'bg-muted text-muted-foreground',
        tone === 'amber' && 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
        tone === 'destructive' && 'bg-destructive/15 text-destructive',
      )}
    >
      <Icon className="h-3.5 w-3.5" />
    </div>
  )
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text)
          setCopied(true)
          setTimeout(() => setCopied(false), 1500)
        } catch {
          /* ignore */
        }
      }}
      className="inline-flex items-center gap-1 text-[10px] text-muted-foreground hover:text-foreground"
      aria-label="copy"
    >
      {copied ? <Check className="h-3 w-3 text-primary" /> : <Copy className="h-3 w-3" />}
      {copied ? 'copied' : 'copy'}
    </button>
  )
}
