import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowRight,
  BarChart3,
  ChevronRight,
  KeyRound,
  LayoutGrid,
  Search,
  Settings,
  ShieldAlert,
  Sparkles,
  Table2,
  Workflow,
} from 'lucide-react'
import { api } from '@/lib/api'
import { useEntities } from '@/hooks/useEntities'
import { useDashboards } from '@/hooks/useDashboards'
import { cn } from '@/lib/utils'

// CommandPalette is a global Cmd/Ctrl+K launcher. It indexes entities,
// dashboards, flows and a handful of static destinations (settings,
// approvals, new flow, ...). Fuzzy match is intentionally cheap —
// substring + token order, no full Levenshtein.
//
// Mounted once in AppShell. Keeps its own open/close state but listens
// for the global hotkey directly so it works regardless of which route
// is rendered.

interface CmdItem {
  id: string
  group: string
  label: string
  hint?: string
  icon: React.ComponentType<{ className?: string }>
  go: () => void
  keywords?: string
}

export function CommandPalette() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const entities = useEntities()
  const dashboards = useDashboards()
  const flows = useQuery({ queryKey: ['flows'], queryFn: api.listFlows })

  // Open with Cmd/Ctrl+K from anywhere; close with Esc.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey
      if (mod && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        setOpen((v) => !v)
        return
      }
      if (e.key === 'Escape' && open) {
        e.preventDefault()
        setOpen(false)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  // Auto-focus on open, reset query on close.
  useEffect(() => {
    if (open) {
      setTimeout(() => inputRef.current?.focus(), 0)
    } else {
      setQuery('')
      setActive(0)
    }
  }, [open])

  const items = useMemo<CmdItem[]>(() => {
    const out: CmdItem[] = []

    // Static top-level destinations.
    out.push(
      { id: 'home', group: 'Přejít na', label: 'Domů', icon: LayoutGrid, go: () => navigate({ to: '/app' }) },
      { id: 'flows', group: 'Přejít na', label: 'Toky', icon: Workflow, go: () => navigate({ to: '/app/flows' }) },
      { id: 'flows-new', group: 'Akce', label: 'Vytvořit nový tok', icon: Sparkles, go: () => navigate({ to: '/app/flows/new' }) },
      { id: 'approvals', group: 'Přejít na', label: 'Schválení', icon: ShieldAlert, go: () => navigate({ to: '/app/approvals' }) },
      { id: 'time', group: 'Přejít na', label: 'Časové záznamy', icon: ChevronRight, go: () => navigate({ to: '/app/time' }) },
      { id: 'settings-connectors', group: 'Nastavení', label: 'Konektory', icon: Settings, go: () => navigate({ to: '/app/settings/connectors' }) },
      { id: 'settings-llm', group: 'Nastavení', label: 'LLM', icon: KeyRound, go: () => navigate({ to: '/app/settings/llm' }) },
      { id: 'settings-preferences', group: 'Nastavení', label: 'Předvolby', icon: Settings, go: () => navigate({ to: '/app/settings/preferences' }) },
    )

    for (const e of entities.data ?? []) {
      out.push({
        id: `entity-${e.id}`,
        group: 'Entity',
        label: e.display_name || e.name,
        hint: e.name,
        icon: Table2,
        go: () => navigate({ to: '/app/entities/$name', params: { name: e.name } }),
        keywords: `${e.name} ${e.description ?? ''}`,
      })
    }
    for (const d of dashboards.data ?? []) {
      out.push({
        id: `dash-${d.id}`,
        group: 'Dashboardy',
        label: d.name,
        hint: d.slug,
        icon: BarChart3,
        go: () => navigate({ to: '/app/dashboards/$slug', params: { slug: d.slug } }),
        keywords: d.description ?? '',
      })
    }
    for (const f of flows.data ?? []) {
      out.push({
        id: `flow-${f.id}`,
        group: 'Toky',
        label: f.name,
        hint: f.description,
        icon: Workflow,
        go: () => navigate({ to: '/app/flows/$id', params: { id: f.id } }),
        keywords: `${f.description ?? ''} ${f.goal ?? ''}`,
      })
    }
    return out
  }, [navigate, entities.data, dashboards.data, flows.data])

  const filtered = useMemo(() => filter(items, query), [items, query])
  const grouped = useMemo(() => groupBy(filtered), [filtered])
  const flat = useMemo(() => grouped.flatMap((g) => g.items), [grouped])

  // Reset active when result set changes.
  useEffect(() => {
    setActive(0)
  }, [query, items.length])

  if (!open) return null

  const onPick = (item: CmdItem) => {
    item.go()
    setOpen(false)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 px-4 pt-[12vh]"
      onClick={() => setOpen(false)}
    >
      <div
        className="w-full max-w-xl overflow-hidden rounded-lg border border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Search className="h-4 w-4 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Najít entitu, tok, nastavení..."
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') {
                e.preventDefault()
                setActive((i) => Math.min(flat.length - 1, i + 1))
              } else if (e.key === 'ArrowUp') {
                e.preventDefault()
                setActive((i) => Math.max(0, i - 1))
              } else if (e.key === 'Enter') {
                e.preventDefault()
                const it = flat[active]
                if (it) onPick(it)
              }
            }}
          />
          <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
            ESC
          </kbd>
        </div>
        <div className="max-h-[60vh] overflow-y-auto py-1">
          {flat.length === 0 && (
            <p className="px-3 py-6 text-center text-sm text-muted-foreground">Nic neodpovídá.</p>
          )}
          {grouped.map((g) => (
            <div key={g.name} className="py-1">
              <p className="px-3 pb-1 pt-2 text-[10px] uppercase tracking-wider text-muted-foreground">
                {g.name}
              </p>
              {g.items.map((it) => {
                const idx = flat.indexOf(it)
                const Icon = it.icon
                return (
                  <button
                    key={it.id}
                    type="button"
                    onMouseEnter={() => setActive(idx)}
                    onClick={() => onPick(it)}
                    className={cn(
                      'flex w-full items-center gap-3 px-3 py-2 text-left text-sm',
                      idx === active ? 'bg-accent text-foreground' : 'text-foreground/90 hover:bg-accent/50',
                    )}
                  >
                    <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate">{it.label}</span>
                    {it.hint && (
                      <span className="truncate font-mono text-[11px] text-muted-foreground/80">
                        {it.hint}
                      </span>
                    )}
                    {idx === active && <ArrowRight className="h-3 w-3 text-muted-foreground" />}
                  </button>
                )
              })}
            </div>
          ))}
        </div>
        <div className="flex items-center justify-between border-t border-border px-3 py-2 text-[11px] text-muted-foreground">
          <span>
            <kbd className="mr-1 rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">↑↓</kbd>
            pohyb
            <kbd className="mx-1 ml-3 rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">↵</kbd>
            otevřít
          </span>
          <span>
            <kbd className="mr-1 rounded border border-border bg-muted px-1 py-0.5 font-mono text-[10px]">⌘K</kbd>
            přepnout
          </span>
        </div>
      </div>
    </div>
  )
}

// --- helpers --------------------------------------------------------------

function filter(items: CmdItem[], query: string): CmdItem[] {
  const q = query.trim().toLowerCase()
  if (!q) return items
  const terms = q.split(/\s+/)
  return items.filter((it) => {
    const hay = `${it.label} ${it.hint ?? ''} ${it.keywords ?? ''}`.toLowerCase()
    return terms.every((t) => hay.includes(t))
  })
}

function groupBy(items: CmdItem[]): { name: string; items: CmdItem[] }[] {
  const order: string[] = []
  const map = new Map<string, CmdItem[]>()
  for (const it of items) {
    if (!map.has(it.group)) {
      map.set(it.group, [])
      order.push(it.group)
    }
    map.get(it.group)!.push(it)
  }
  return order.map((name) => ({ name, items: map.get(name)! }))
}
