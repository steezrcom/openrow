import { useMemo } from 'react'
import { cn } from '@/lib/utils'

// FlowGoal renders an agent goal — the free-text instruction stored on a
// flow — with light syntactic colour. The seeded Agency goals mix prose,
// inline JSON payloads, tool names, entity.field references, SQL-ish
// keywords and single-quoted enum values; rendered as pre-wrapped text
// they get unreadable fast.
//
// This is not a full markdown parser. The shape we get from seeded flows
// is consistent enough to handle with two passes:
//
//  1. Pull out multi-line {...} or [...] blocks → render as a styled code
//     card so the JSON breathes.
//  2. For each remaining prose chunk, tokenise inline patterns:
//     - tool names (connector_x_y, query_rows, add_row, ...)
//     - single-quoted values ('paid')
//     - SQL keywords (AND, OR, IN, IS, NOT, NULL, LIKE)
//     - entity.field references (invoices.status)
//     - <placeholder> hints
//
// Anything we don't recognise stays as plain prose.
export function FlowGoal({ goal }: { goal: string }) {
  const segments = useMemo(() => parseGoal(goal.trim()), [goal])
  return (
    <div className="space-y-3 text-sm leading-relaxed">
      {segments.map((s, i) =>
        s.kind === 'code' ? (
          <CodeBlock key={i} text={s.text} />
        ) : (
          <Prose key={i} text={s.text} />
        ),
      )}
    </div>
  )
}

// --- segmentation ---------------------------------------------------------

type Segment = { kind: 'prose' | 'code'; text: string }

// parseGoal pulls multi-line balanced {...} / [...] regions out of the
// goal as code segments. Single-line `{...}` payloads stay inline so
// short examples don't bloat the layout.
function parseGoal(text: string): Segment[] {
  const out: Segment[] = []
  let i = 0
  let proseStart = 0

  while (i < text.length) {
    const c = text[i]
    if (c !== '{' && c !== '[') {
      i++
      continue
    }
    // Walk forward, tracking bracket depth + whether we crossed a newline.
    let depth = 0
    let crossed = false
    let j = i
    for (; j < text.length; j++) {
      const ch = text[j]
      if (ch === '{' || ch === '[') depth++
      else if (ch === '}' || ch === ']') {
        depth--
        if (depth === 0) {
          j++
          break
        }
      } else if (ch === '\n') {
        crossed = true
      }
    }
    if (depth !== 0 || !crossed) {
      // Unbalanced or single-line — leave it as prose.
      i = j > i ? j : i + 1
      continue
    }
    // Push the prose before, then the code segment.
    if (i > proseStart) {
      const pre = text.slice(proseStart, i).replace(/\s+$/, '')
      if (pre) out.push({ kind: 'prose', text: pre })
    }
    out.push({ kind: 'code', text: text.slice(i, j) })
    proseStart = j
    i = j
    // Skip the newline that typically follows the closing brace.
    while (proseStart < text.length && text[proseStart] === '\n') proseStart++
    i = proseStart
  }
  if (proseStart < text.length) {
    const tail = text.slice(proseStart).replace(/^\s+/, '')
    if (tail) out.push({ kind: 'prose', text: tail })
  }
  return out
}

// --- prose tokenisation ---------------------------------------------------

type Token =
  | { kind: 'text'; value: string }
  | { kind: 'tool'; value: string }
  | { kind: 'sql'; value: string }
  | { kind: 'value'; value: string }
  | { kind: 'field'; value: string }
  | { kind: 'placeholder'; value: string }

// Built-in tool names we know about. connector_* matches any registered
// connector action without needing to enumerate them.
const INTERNAL_TOOLS = new Set([
  'list_entities',
  'create_entity',
  'add_row',
  'update_row',
  'delete_row',
  'add_field',
  'drop_field',
  'query_rows',
  'list_templates',
  'apply_template',
  'list_dashboards',
  'get_dashboard',
  'create_dashboard',
  'add_report',
  'update_report',
  'delete_report',
  'delete_dashboard',
  'render_document',
  'reconcile_bank_transactions',
])

const SQL_KEYWORDS = new Set([
  'AND',
  'OR',
  'NOT',
  'NULL',
  'IN',
  'IS',
  'LIKE',
  'TRUE',
  'FALSE',
])

// Combined match pattern. Order matters: longest / most specific first.
const TOKEN_RE = new RegExp(
  [
    // <placeholder>
    /(?<placeholder><[^<>\n]{1,80}>)/.source,
    // 'single-quoted value' (no newlines inside)
    /(?<value>'[^'\n]{0,60}')/.source,
    // connector_<id>_<action> (greedy across underscores)
    /(?<connector>connector_[a-z][a-z0-9_]+)/.source,
    // entity.field (lowercase only, snake-case allowed on either side)
    /(?<field>\b[a-z][a-z0-9_]+\.[a-z][a-z0-9_]+\b)/.source,
    // Bare identifier — matched only when it's in the tool / SQL set
    // (resolved in the loop below).
    /(?<ident>\b[A-Za-z][A-Za-z0-9_]+\b)/.source,
  ].join('|'),
  'g',
)

function tokenise(text: string): Token[] {
  const out: Token[] = []
  let last = 0
  for (const m of text.matchAll(TOKEN_RE)) {
    const start = m.index ?? 0
    if (start > last) out.push({ kind: 'text', value: text.slice(last, start) })

    const groups = m.groups ?? {}
    if (groups.placeholder) {
      out.push({ kind: 'placeholder', value: groups.placeholder })
    } else if (groups.value) {
      out.push({ kind: 'value', value: groups.value })
    } else if (groups.connector) {
      out.push({ kind: 'tool', value: groups.connector })
    } else if (groups.field) {
      out.push({ kind: 'field', value: groups.field })
    } else if (groups.ident) {
      const v = groups.ident
      if (INTERNAL_TOOLS.has(v)) out.push({ kind: 'tool', value: v })
      else if (SQL_KEYWORDS.has(v)) out.push({ kind: 'sql', value: v })
      else out.push({ kind: 'text', value: v })
    }
    last = start + m[0].length
  }
  if (last < text.length) out.push({ kind: 'text', value: text.slice(last) })
  return out
}

// --- rendering ------------------------------------------------------------

function Prose({ text }: { text: string }) {
  // Split on blank lines to preserve paragraph breaks; collapse internal
  // newlines (the seeded goals use hard wraps for readability in Go
  // source, but they read fine reflowed).
  const paragraphs = text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
  return (
    <div className="space-y-2">
      {paragraphs.map((p, i) => (
        <p key={i} className="whitespace-normal">
          {tokenise(p.replace(/\n/g, ' ')).map((t, j) => (
            <TokenSpan key={j} token={t} />
          ))}
        </p>
      ))}
    </div>
  )
}

function TokenSpan({ token }: { token: Token }) {
  switch (token.kind) {
    case 'tool':
      return (
        <code
          className={cn(
            'rounded px-1.5 py-0.5 font-mono text-[12px]',
            token.value.startsWith('connector_')
              ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
              : 'bg-violet-500/10 text-violet-700 dark:text-violet-300',
          )}
        >
          {token.value}
        </code>
      )
    case 'sql':
      return (
        <span className="font-mono text-[11px] uppercase tracking-wider text-muted-foreground">
          {token.value}
        </span>
      )
    case 'value':
      return (
        <code className="rounded bg-amber-500/10 px-1.5 py-0.5 font-mono text-[12px] text-amber-700 dark:text-amber-300">
          {token.value}
        </code>
      )
    case 'field':
      return (
        <code className="rounded bg-muted/60 px-1.5 py-0.5 font-mono text-[12px] text-foreground/80">
          {token.value}
        </code>
      )
    case 'placeholder':
      return (
        <span className="font-mono text-[12px] italic text-muted-foreground">
          {token.value}
        </span>
      )
    case 'text':
    default:
      return <span>{token.value}</span>
  }
}

function CodeBlock({ text }: { text: string }) {
  // Render JSON-ish blocks with their original indentation. Token
  // colouring is the same as for prose so the JSON keys / values still
  // pop, but the container is monospace from the start.
  const tokens = tokenise(text)
  return (
    <pre className="overflow-x-auto rounded-md border border-border bg-muted/40 px-3 py-2 text-[12px] leading-relaxed">
      <code className="font-mono">
        {tokens.map((t, i) => (
          <TokenSpan key={i} token={t} />
        ))}
      </code>
    </pre>
  )
}
