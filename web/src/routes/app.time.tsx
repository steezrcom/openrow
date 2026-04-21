import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { ChevronLeft, ChevronRight, Clock } from 'lucide-react'
import { api } from '@/lib/api'
import { Button, Card, Input } from '@/components/ui'
import { useEntities } from '@/hooks/useEntities'
import { cn } from '@/lib/utils'

const searchSchema = z.object({
  week: z.string().optional(), // YYYY-MM-DD of Monday
})

export const Route = createFileRoute('/app/time')({
  validateSearch: searchSchema,
  component: TimePage,
})

function mondayOf(d: Date): Date {
  const copy = new Date(d)
  const day = copy.getDay() // 0=Sun
  const diff = (day + 6) % 7 // days since Monday
  copy.setDate(copy.getDate() - diff)
  copy.setHours(0, 0, 0, 0)
  return copy
}

function iso(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(d.getDate() + n)
  return r
}

const DAY_LABELS = ['Po', 'Út', 'St', 'Čt', 'Pá', 'So', 'Ne']

function TimePage() {
  const navigate = useNavigate({ from: Route.fullPath })
  const search = Route.useSearch()
  const entities = useEntities()
  const hasTimeEntries = Boolean(entities.data?.some((e) => e.name === 'time_entries'))

  const weekStart = search.week ? new Date(search.week) : mondayOf(new Date())
  const start = mondayOf(weekStart)
  const days = useMemo(() => {
    const arr: Date[] = []
    for (let i = 0; i < 7; i++) arr.push(addDays(start, i))
    return arr
  }, [start])
  const weekFrom = iso(start)
  const weekTo = iso(addDays(start, 7))

  const rowsQuery = useQuery({
    queryKey: ['time-entries', weekFrom],
    enabled: hasTimeEntries,
    queryFn: async () => {
      // Fetch all entries in the week via listRows + client filter. 200 rows is
      // plenty for a week in a small agency; bigger windows get proper server
      // filtering when we need it.
      const res = await api.listRows('time_entries', {
        sort: 'date',
        dir: 'asc',
        limit: 200,
      })
      return res.rows.filter(
        (r) => typeof r.date === 'string' && r.date >= weekFrom && r.date < weekTo
      )
    },
  })

  if (!entities.isLoading && !hasTimeEntries) {
    return (
      <div className="mx-auto max-w-3xl px-8 py-10">
        <EmptyState />
      </div>
    )
  }

  const rows = rowsQuery.data ?? []

  // Pivot rows by project (+ task) × day.
  type RowKey = { projectID: string; projectLabel: string; taskID: string | null; taskLabel: string | null }
  const pivot = new Map<string, { key: RowKey; byDay: Record<string, { total: number; entries: Record<string, unknown>[] }> }>()
  for (const r of rows) {
    const projectID = String(r.project ?? '')
    const projectLabel = String(r.project__label ?? projectID.slice(0, 8))
    const taskID = r.task == null || r.task === '' ? null : String(r.task)
    const taskLabel = r.task__label == null ? null : String(r.task__label)
    const rowKey = `${projectID}::${taskID ?? ''}`
    if (!pivot.has(rowKey)) {
      pivot.set(rowKey, {
        key: { projectID, projectLabel, taskID, taskLabel },
        byDay: {},
      })
    }
    const bucket = pivot.get(rowKey)!
    const dateStr = String(r.date)
    if (!bucket.byDay[dateStr]) bucket.byDay[dateStr] = { total: 0, entries: [] }
    bucket.byDay[dateStr].total += Number(r.hours) || 0
    bucket.byDay[dateStr].entries.push(r)
  }
  const pivotRows = Array.from(pivot.values()).sort((a, b) => a.key.projectLabel.localeCompare(b.key.projectLabel))

  const totalByDay: Record<string, number> = {}
  for (const d of days) totalByDay[iso(d)] = 0
  for (const r of rows) {
    const dateStr = String(r.date)
    if (dateStr in totalByDay) totalByDay[dateStr] += Number(r.hours) || 0
  }
  const weekTotal = Object.values(totalByDay).reduce((a, b) => a + b, 0)

  function nav(delta: number) {
    const next = addDays(start, delta * 7)
    navigate({ search: () => ({ week: iso(next) }) })
  }

  return (
    <div className="px-8 py-8">
      <header className="mb-6 flex items-center justify-between gap-4">
        <div>
          <p className="text-xs text-muted-foreground">
            <Link to="/app" className="hover:text-foreground">Home</Link>
            <span className="mx-2">/</span>
            Timesheet
          </p>
          <h1 className="mt-2 flex items-center gap-3 text-2xl font-semibold tracking-tight">
            <Clock className="h-5 w-5 text-primary" />
            Week of {iso(start)}
          </h1>
          <p className="mt-1 text-xs text-muted-foreground">
            {weekTotal.toFixed(1)} h logged this week
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" onClick={() => nav(-1)}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            onClick={() => navigate({ search: () => ({ week: iso(mondayOf(new Date())) }) })}
          >
            This week
          </Button>
          <Button variant="ghost" onClick={() => nav(1)}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </header>

      <Card className="overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-muted-foreground">
              <th className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider">Project</th>
              {days.map((d, i) => (
                <th key={iso(d)} className="px-3 py-2 text-right text-xs font-medium uppercase tracking-wider">
                  <div>{DAY_LABELS[i]}</div>
                  <div className="text-[10px] font-normal normal-case">
                    {String(d.getDate()).padStart(2, '0')}.{String(d.getMonth() + 1).padStart(2, '0')}
                  </div>
                </th>
              ))}
              <th className="px-4 py-2 text-right text-xs font-medium uppercase tracking-wider">Total</th>
            </tr>
          </thead>
          <tbody>
            {pivotRows.length === 0 ? (
              <tr>
                <td
                  colSpan={9}
                  className="px-4 py-8 text-center text-sm text-muted-foreground"
                >
                  No time logged this week. Start the timer in the header or add entries by hand.
                </td>
              </tr>
            ) : (
              pivotRows.map(({ key, byDay }) => {
                const rowTotal = Object.values(byDay).reduce((a, b) => a + b.total, 0)
                return (
                  <tr key={`${key.projectID}::${key.taskID ?? ''}`} className="border-b border-border/40">
                    <td className="px-4 py-2">
                      <div className="font-medium">{key.projectLabel}</div>
                      {key.taskLabel && (
                        <div className="text-xs text-muted-foreground">{key.taskLabel}</div>
                      )}
                    </td>
                    {days.map((d) => {
                      const k = iso(d)
                      const cell = byDay[k]
                      return (
                        <td key={k} className="px-3 py-1 text-right tabular-nums">
                          <CellEditor
                            weekFrom={weekFrom}
                            date={k}
                            projectID={key.projectID}
                            taskID={key.taskID}
                            currentTotal={cell?.total ?? 0}
                            entries={cell?.entries ?? []}
                          />
                        </td>
                      )
                    })}
                    <td className="px-4 py-2 text-right font-semibold tabular-nums">
                      {rowTotal > 0 ? rowTotal.toFixed(1) : ''}
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
          <tfoot className="bg-muted/20">
            <tr>
              <td className="px-4 py-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">Totals</td>
              {days.map((d) => {
                const k = iso(d)
                const t = totalByDay[k]
                return (
                  <td key={k} className="px-3 py-2 text-right text-xs tabular-nums text-muted-foreground">
                    {t > 0 ? t.toFixed(1) : ''}
                  </td>
                )
              })}
              <td className="px-4 py-2 text-right text-xs font-semibold tabular-nums">
                {weekTotal > 0 ? weekTotal.toFixed(1) : ''}
              </td>
            </tr>
          </tfoot>
        </table>
      </Card>
    </div>
  )
}

function EmptyState() {
  return (
    <Card className="p-8 text-center">
      <Clock className="mx-auto mb-3 h-6 w-6 text-muted-foreground" />
      <h2 className="font-medium">Time tracking isn't set up yet</h2>
      <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">
        Install the Agency template on the Home screen, or ask Claude to add a <code>time_entries</code> entity.
      </p>
      <div className="mt-4">
        <Link to="/app">
          <Button variant="ghost">Go to Home</Button>
        </Link>
      </div>
    </Card>
  )
}

// CellEditor: shows total hours for the cell; clicking it opens an input to
// edit-in-place. For now, we add a new entry each save (simplest semantics:
// many logs per day allowed). Editing an existing single entry is next polish.
function CellEditor({
  weekFrom,
  date,
  projectID,
  taskID,
  currentTotal,
  entries,
}: {
  weekFrom: string
  date: string
  projectID: string
  taskID: string | null
  currentTotal: number
  entries: Record<string, unknown>[]
}) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(String(currentTotal || ''))

  const create = useMutation({
    mutationFn: async (hours: number) => {
      const values: Record<string, string> = {
        project: projectID,
        date,
        hours: String(hours),
        person: (JSON.parse(localStorage.getItem('steezr.lastPerson') || '""') as string) || 'me',
      }
      if (taskID) values.task = taskID
      return api.createRow('time_entries', values)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['time-entries', weekFrom] })
      qc.invalidateQueries({ queryKey: ['rows', 'time_entries'] })
      setEditing(false)
    },
  })

  if (!editing) {
    return (
      <button
        onClick={() => {
          setEditing(true)
          setValue('')
        }}
        className={cn(
          'block w-full rounded px-1 py-0.5 text-right font-mono text-xs',
          currentTotal > 0
            ? 'text-foreground hover:bg-accent'
            : 'text-muted-foreground/40 hover:bg-accent hover:text-foreground'
        )}
        title={entries.length ? `${entries.length} entries` : 'Add entry'}
      >
        {currentTotal > 0 ? currentTotal.toFixed(1) : '·'}
      </button>
    )
  }
  return (
    <Input
      autoFocus
      type="number"
      min={0}
      step={0.25}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => {
        const n = Number(value)
        if (isFinite(n) && n > 0) create.mutate(n)
        else setEditing(false)
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          const n = Number(value)
          if (isFinite(n) && n > 0) create.mutate(n)
          else setEditing(false)
        }
        if (e.key === 'Escape') setEditing(false)
      }}
      className="h-7 w-16 text-right font-mono text-xs"
    />
  )
}
