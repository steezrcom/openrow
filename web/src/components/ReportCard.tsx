import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { AlertTriangle, ArrowDownRight, ArrowUpRight, Loader2, Pencil, Trash2 } from 'lucide-react'
import { api, type Report, type ReportOptions } from '@/lib/api'
import { Card } from '@/components/ui'
import { cn } from '@/lib/utils'

const CHART_COLORS = [
  '#6ee7b7',
  '#60a5fa',
  '#f472b6',
  '#fbbf24',
  '#a78bfa',
  '#f87171',
  '#34d399',
  '#818cf8',
]

export function ReportCard({
  report,
  range,
  onEdit,
}: {
  report: Report
  range?: { from?: string; to?: string }
  onEdit?: () => void
}) {
  const qc = useQueryClient()
  const exec = useQuery({
    queryKey: ['report-exec', report.id, report.updated_at, range?.from ?? '', range?.to ?? ''],
    queryFn: () => api.executeReport(report.id, range),
    staleTime: 15_000,
  })

  const del = useMutation({
    mutationFn: () => api.deleteReport(report.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dashboard'] }),
  })

  return (
    <Card className={cn('group flex flex-col', widthClass(report.width))}>
      <header className="flex items-start justify-between gap-3 border-b border-border/60 px-4 py-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{report.title}</h3>
          {report.subtitle && (
            <p className="truncate text-xs text-muted-foreground">{report.subtitle}</p>
          )}
        </div>
        <div className="flex items-center gap-0.5">
          {onEdit && (
            <button
              onClick={onEdit}
              className="invisible rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground group-hover:visible"
              title="Edit report"
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
          )}
          <button
            onClick={() => {
              if (confirm(`Delete report "${report.title}"?`)) del.mutate()
            }}
            className="invisible rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive group-hover:visible"
            title="Delete report"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </header>
      <div className="flex-1 p-4 min-h-[160px]">
        {exec.isLoading && (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Running.
          </div>
        )}
        {exec.error && (
          <div className="flex h-full items-start gap-2 rounded-md bg-destructive/5 p-3 text-xs text-destructive">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span className="break-words">
              {exec.error instanceof Error ? exec.error.message : 'Failed to run'}
            </span>
          </div>
        )}
        {exec.data && <ReportBody report={report} data={exec.data} />}
      </div>
    </Card>
  )
}

function ReportBody({ report, data }: { report: Report; data: { shape: string; rows: Record<string, unknown>[] } }) {
  if (data.rows.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        No data.
      </div>
    )
  }
  const options = report.options ?? {}
  switch (report.widget_type) {
    case 'kpi':
      return <KPIView row={data.rows[0]} options={options} />
    case 'bar':
      return <BarView rows={data.rows} options={options} />
    case 'line':
      return <LineView rows={data.rows} options={options} />
    case 'area':
      return <AreaView rows={data.rows} options={options} />
    case 'pie':
      return <PieView rows={data.rows} options={options} />
    case 'table':
      return <TableView rows={data.rows} />
  }
  return null
}

function KPIView({ row, options }: { row: Record<string, unknown>; options: ReportOptions }) {
  const fmt = makeFormatter(options)
  const value = asNumber(row.value)
  const previous = row.previous_value == null ? null : asNumber(row.previous_value)
  const delta = previous != null && previous !== 0 ? (value - previous) / previous : null
  const comparePeriod = typeof row.compare_period === 'string' ? row.compare_period : ''

  return (
    <div className="flex h-full flex-col justify-center">
      <span className="text-4xl font-semibold tracking-tight">{fmt.format(value)}</span>
      {previous != null && (
        <div className="mt-2 flex items-center gap-2 text-xs">
          {delta != null && (
            <span
              className={cn(
                'inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 font-medium',
                delta >= 0
                  ? 'bg-primary/10 text-primary'
                  : 'bg-destructive/10 text-destructive'
              )}
            >
              {delta >= 0 ? <ArrowUpRight className="h-3 w-3" /> : <ArrowDownRight className="h-3 w-3" />}
              {Math.abs(delta * 100).toFixed(1)}%
            </span>
          )}
          <span className="text-muted-foreground">
            vs {comparePeriod === 'previous_year' ? 'last year' : 'previous period'} ({fmt.format(previous)})
          </span>
        </div>
      )}
    </div>
  )
}

function BarView({ rows, options }: { rows: Record<string, unknown>[]; options: ReportOptions }) {
  const fmt = makeFormatter(options)
  const hasSeries = rows.length > 0 && 'series' in rows[0]
  if (!hasSeries) {
    const data = rows.map((r) => ({ label: formatLabel(r.label), value: asNumber(r.value) }))
    return (
      <ResponsiveContainer width="100%" height={220}>
        <BarChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
          <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
          <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
          <Tooltip contentStyle={tooltipStyle} cursor={{ fill: '#ffffff08' }} formatter={(v) => fmt.format(asNumber(v))} />
          <Bar dataKey="value" fill={CHART_COLORS[0]} radius={[4, 4, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    )
  }
  const { data, seriesKeys } = pivotSeries(rows)
  const stackId = options.stacked ? 'a' : undefined
  return (
    <ResponsiveContainer width="100%" height={240}>
      <BarChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
        <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
        <Tooltip contentStyle={tooltipStyle} cursor={{ fill: '#ffffff08' }} formatter={(v) => fmt.format(asNumber(v))} />
        <Legend wrapperStyle={legendStyle} />
        {seriesKeys.map((key, i) => (
          <Bar
            key={key}
            dataKey={key}
            fill={CHART_COLORS[i % CHART_COLORS.length]}
            stackId={stackId}
            radius={options.stacked ? undefined : [4, 4, 0, 0]}
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}

function LineView({ rows, options }: { rows: Record<string, unknown>[]; options: ReportOptions }) {
  const fmt = makeFormatter(options)
  const hasSeries = rows.length > 0 && 'series' in rows[0]
  if (!hasSeries) {
    const data = rows.map((r) => ({ label: formatLabel(r.label), value: asNumber(r.value) }))
    return (
      <ResponsiveContainer width="100%" height={220}>
        <LineChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
          <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
          <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
          <Tooltip contentStyle={tooltipStyle} formatter={(v) => fmt.format(asNumber(v))} />
          <Line type="monotone" dataKey="value" stroke={CHART_COLORS[0]} strokeWidth={2} dot={{ fill: CHART_COLORS[0], r: 3 }} />
        </LineChart>
      </ResponsiveContainer>
    )
  }
  const { data, seriesKeys } = pivotSeries(rows)
  return (
    <ResponsiveContainer width="100%" height={240}>
      <LineChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
        <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
        <Tooltip contentStyle={tooltipStyle} formatter={(v) => fmt.format(asNumber(v))} />
        <Legend wrapperStyle={legendStyle} />
        {seriesKeys.map((key, i) => (
          <Line
            key={key}
            type="monotone"
            dataKey={key}
            stroke={CHART_COLORS[i % CHART_COLORS.length]}
            strokeWidth={2}
            dot={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}

function AreaView({ rows, options }: { rows: Record<string, unknown>[]; options: ReportOptions }) {
  const fmt = makeFormatter(options)
  const hasSeries = rows.length > 0 && 'series' in rows[0]
  const stackId = options.stacked ? 'a' : undefined

  if (!hasSeries) {
    const data = rows.map((r) => ({ label: formatLabel(r.label), value: asNumber(r.value) }))
    return (
      <ResponsiveContainer width="100%" height={220}>
        <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
          <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
          <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
          <Tooltip contentStyle={tooltipStyle} formatter={(v) => fmt.format(asNumber(v))} />
          <Area type="monotone" dataKey="value" stroke={CHART_COLORS[0]} fill={CHART_COLORS[0]} fillOpacity={0.2} />
        </AreaChart>
      </ResponsiveContainer>
    )
  }
  const { data, seriesKeys } = pivotSeries(rows)
  return (
    <ResponsiveContainer width="100%" height={240}>
      <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
        <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <YAxis tickFormatter={(v) => fmt.format(Number(v))} tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" width={60} />
        <Tooltip contentStyle={tooltipStyle} formatter={(v) => fmt.format(asNumber(v))} />
        <Legend wrapperStyle={legendStyle} />
        {seriesKeys.map((key, i) => (
          <Area
            key={key}
            type="monotone"
            dataKey={key}
            stroke={CHART_COLORS[i % CHART_COLORS.length]}
            fill={CHART_COLORS[i % CHART_COLORS.length]}
            fillOpacity={0.2}
            stackId={stackId}
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  )
}

function PieView({ rows, options }: { rows: Record<string, unknown>[]; options: ReportOptions }) {
  const fmt = makeFormatter(options)
  const data = rows.map((r) => ({
    label: formatLabel(r.label),
    value: asNumber(r.value),
  }))
  return (
    <ResponsiveContainer width="100%" height={220}>
      <PieChart>
        <Pie data={data} dataKey="value" nameKey="label" innerRadius={40} outerRadius={80}>
          {data.map((_, i) => (
            <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} stroke="#0b0c0e" />
          ))}
        </Pie>
        <Tooltip contentStyle={tooltipStyle} formatter={(v) => fmt.format(asNumber(v))} />
        <Legend wrapperStyle={legendStyle} />
      </PieChart>
    </ResponsiveContainer>
  )
}

function TableView({ rows }: { rows: Record<string, unknown>[] }) {
  const allKeys = Object.keys(rows[0])
  const labelKeys = new Set(
    allKeys.filter((k) => k.endsWith('__label')).map((k) => k.replace(/__label$/, ''))
  )
  const cols = allKeys.filter((k) => {
    if (k === 'id' || k === 'updated_at') return false
    if (k.endsWith('__label')) return false
    return true
  })
  const displayKey = (c: string) => (labelKeys.has(c) ? c + '__label' : c)

  return (
    <div className="overflow-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border text-muted-foreground">
            {cols.map((c) => <th key={c} className="px-2 py-1.5 text-left font-medium">{c}</th>)}
          </tr>
        </thead>
        <tbody>
          {rows.slice(0, 50).map((row, i) => (
            <tr key={i} className="border-b border-border/40">
              {cols.map((c) => (
                <td key={c} className="px-2 py-1.5">{formatCell(row[displayKey(c)])}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length > 50 && (
        <p className="mt-2 text-xs text-muted-foreground">Showing first 50 of {rows.length}</p>
      )}
    </div>
  )
}

// -------- helpers --------

const tooltipStyle: React.CSSProperties = {
  background: '#131418',
  border: '1px solid #1f2024',
  borderRadius: 6,
  color: '#e6e6e6',
  fontSize: 12,
}
const legendStyle: React.CSSProperties = {
  color: '#8a8a8a',
  fontSize: 11,
  paddingTop: 8,
}

function widthClass(width: number): string {
  const safe = Math.max(1, Math.min(12, width))
  if (safe <= 3) return 'col-span-12 md:col-span-3'
  if (safe <= 4) return 'col-span-12 md:col-span-4'
  if (safe <= 6) return 'col-span-12 md:col-span-6'
  if (safe <= 8) return 'col-span-12 md:col-span-8'
  return 'col-span-12'
}

function asNumber(v: unknown): number {
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = Number(v)
    return isFinite(n) ? n : 0
  }
  return 0
}

function makeFormatter(options: ReportOptions): Intl.NumberFormat {
  const opts: Intl.NumberFormatOptions = { maximumFractionDigits: 2 }
  switch (options.number_format) {
    case 'currency':
      opts.style = 'currency'
      opts.currency = options.currency_code || 'USD'
      opts.maximumFractionDigits = 0
      break
    case 'percent':
      opts.style = 'percent'
      opts.maximumFractionDigits = 1
      break
    case 'integer':
      opts.maximumFractionDigits = 0
      break
  }
  return new Intl.NumberFormat(options.locale || undefined, opts)
}

function formatLabel(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'string') {
    if (/^\d{4}-\d{2}-\d{2}T/.test(v)) {
      const d = new Date(v)
      if (!isNaN(d.getTime())) {
        return d.toISOString().slice(0, 10)
      }
    }
    return v
  }
  return String(v)
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(v)) {
    const d = new Date(v)
    if (!isNaN(d.getTime())) return d.toISOString().slice(0, 16).replace('T', ' ')
  }
  return String(v)
}

// Pivots rows of shape {label, series, value} into {label, <series1>: v, <series2>: v, ...}
// preserving label order and returning the list of unique series keys.
function pivotSeries(rows: Record<string, unknown>[]): {
  data: Record<string, unknown>[]
  seriesKeys: string[]
} {
  const seriesSet = new Set<string>()
  const byLabel = new Map<string, Record<string, unknown>>()
  for (const r of rows) {
    const label = formatLabel(r.label)
    const series = formatLabel(r.series)
    seriesSet.add(series)
    let row = byLabel.get(label)
    if (!row) {
      row = { label }
      byLabel.set(label, row)
    }
    row[series] = asNumber(r.value)
  }
  return {
    data: Array.from(byLabel.values()),
    seriesKeys: Array.from(seriesSet),
  }
}

// We memoize pivotSeries per rows reference; consumers can wrap with useMemo.
export function usePivotedSeries(rows: Record<string, unknown>[]) {
  return useMemo(() => pivotSeries(rows), [rows])
}
