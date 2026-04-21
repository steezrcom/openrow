import { useEffect, useRef, useState } from 'react'
import { Calendar, ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface DateRange {
  from?: string // YYYY-MM-DD
  to?: string   // YYYY-MM-DD (exclusive upper bound when passed to server)
  presetKey?: PresetKey
}

type PresetKey =
  | 'all'
  | '7d'
  | '30d'
  | '90d'
  | 'mtd'
  | 'qtd'
  | 'ytd'
  | 'prev_month'
  | 'last_12m'

interface Preset {
  key: PresetKey
  label: string
  compute: () => DateRange
}

const today = () => new Date()

function iso(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

const PRESETS: Preset[] = [
  { key: 'all', label: 'All time', compute: () => ({ presetKey: 'all' }) },
  {
    key: '7d',
    label: 'Last 7 days',
    compute: () => {
      const end = today()
      const start = new Date(end)
      start.setDate(end.getDate() - 6)
      return { from: iso(start), to: iso(addDays(end, 1)), presetKey: '7d' }
    },
  },
  {
    key: '30d',
    label: 'Last 30 days',
    compute: () => {
      const end = today()
      const start = new Date(end)
      start.setDate(end.getDate() - 29)
      return { from: iso(start), to: iso(addDays(end, 1)), presetKey: '30d' }
    },
  },
  {
    key: '90d',
    label: 'Last 90 days',
    compute: () => {
      const end = today()
      const start = new Date(end)
      start.setDate(end.getDate() - 89)
      return { from: iso(start), to: iso(addDays(end, 1)), presetKey: '90d' }
    },
  },
  {
    key: 'mtd',
    label: 'Month to date',
    compute: () => {
      const now = today()
      const start = new Date(now.getFullYear(), now.getMonth(), 1)
      return { from: iso(start), to: iso(addDays(now, 1)), presetKey: 'mtd' }
    },
  },
  {
    key: 'prev_month',
    label: 'Previous month',
    compute: () => {
      const now = today()
      const start = new Date(now.getFullYear(), now.getMonth() - 1, 1)
      const end = new Date(now.getFullYear(), now.getMonth(), 1)
      return { from: iso(start), to: iso(end), presetKey: 'prev_month' }
    },
  },
  {
    key: 'qtd',
    label: 'Quarter to date',
    compute: () => {
      const now = today()
      const qStart = new Date(now.getFullYear(), Math.floor(now.getMonth() / 3) * 3, 1)
      return { from: iso(qStart), to: iso(addDays(now, 1)), presetKey: 'qtd' }
    },
  },
  {
    key: 'ytd',
    label: 'Year to date',
    compute: () => {
      const now = today()
      const start = new Date(now.getFullYear(), 0, 1)
      return { from: iso(start), to: iso(addDays(now, 1)), presetKey: 'ytd' }
    },
  },
  {
    key: 'last_12m',
    label: 'Last 12 months',
    compute: () => {
      const now = today()
      const start = new Date(now.getFullYear() - 1, now.getMonth(), now.getDate())
      return { from: iso(start), to: iso(addDays(now, 1)), presetKey: 'last_12m' }
    },
  },
]

function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(d.getDate() + n)
  return r
}

function labelFor(range: DateRange): string {
  const preset = PRESETS.find((p) => p.key === range.presetKey)
  if (preset) return preset.label
  if (range.from && range.to) return `${range.from} → ${range.to}`
  if (range.from) return `from ${range.from}`
  if (range.to) return `until ${range.to}`
  return 'All time'
}

export function DateRangePicker({
  value,
  onChange,
}: {
  value: DateRange
  onChange: (r: DateRange) => void
}) {
  const [open, setOpen] = useState(false)
  const [customFrom, setCustomFrom] = useState(value.from ?? '')
  const [customTo, setCustomTo] = useState(value.to ?? '')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handler(e: MouseEvent) {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'inline-flex items-center gap-2 rounded-md border border-border bg-background px-3 py-1.5 text-sm',
          'hover:bg-accent'
        )}
      >
        <Calendar className="h-3.5 w-3.5 text-muted-foreground" />
        <span>{labelFor(value)}</span>
        <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
      </button>
      {open && (
        <div className="absolute right-0 z-20 mt-1 w-72 rounded-md border border-border bg-card p-2 shadow-lg">
          <div className="grid grid-cols-2 gap-1">
            {PRESETS.map((p) => (
              <button
                key={p.key}
                onClick={() => {
                  onChange(p.compute())
                  setOpen(false)
                }}
                className={cn(
                  'rounded-md px-2 py-1.5 text-left text-xs hover:bg-accent',
                  value.presetKey === p.key && 'bg-accent text-foreground'
                )}
              >
                {p.label}
              </button>
            ))}
          </div>
          <div className="mt-2 border-t border-border pt-2">
            <p className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Custom</p>
            <div className="grid grid-cols-2 gap-2">
              <input
                type="date"
                value={customFrom}
                onChange={(e) => setCustomFrom(e.target.value)}
                className="h-8 rounded-md border border-input bg-background px-2 text-xs"
              />
              <input
                type="date"
                value={customTo}
                onChange={(e) => setCustomTo(e.target.value)}
                className="h-8 rounded-md border border-input bg-background px-2 text-xs"
              />
            </div>
            <button
              onClick={() => {
                if (!customFrom && !customTo) return
                onChange({
                  from: customFrom || undefined,
                  to: customTo || undefined,
                })
                setOpen(false)
              }}
              className="mt-2 w-full rounded-md border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
            >
              Apply
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
