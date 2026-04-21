import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
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
import { AlertTriangle, Loader2, Trash2 } from 'lucide-react'
import { api, type Report } from '@/lib/api'
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

export function ReportCard({ report }: { report: Report }) {
  const qc = useQueryClient()
  const exec = useQuery({
    queryKey: ['report-exec', report.id, report.updated_at],
    queryFn: () => api.executeReport(report.id),
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
        <button
          onClick={() => {
            if (confirm(`Delete report "${report.title}"?`)) del.mutate()
          }}
          className="invisible rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive group-hover:visible"
          title="Delete report"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </button>
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
  switch (report.widget_type) {
    case 'kpi':
      return <KPIView row={data.rows[0]} />
    case 'bar':
      return <BarView rows={data.rows} />
    case 'line':
      return <LineView rows={data.rows} />
    case 'pie':
      return <PieView rows={data.rows} />
    case 'table':
      return <TableView rows={data.rows} />
  }
  return null
}

function KPIView({ row }: { row: Record<string, unknown> }) {
  const raw = row.value
  return (
    <div className="flex h-full items-center">
      <span className="text-4xl font-semibold tracking-tight">{formatNumber(raw)}</span>
    </div>
  )
}

function BarView({ rows }: { rows: Record<string, unknown>[] }) {
  const data = rows.map((r) => ({
    label: formatLabel(r.label),
    value: asNumber(r.value),
  }))
  return (
    <ResponsiveContainer width="100%" height={220}>
      <BarChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
        <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <YAxis tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <Tooltip contentStyle={tooltipStyle} cursor={{ fill: '#ffffff08' }} />
        <Bar dataKey="value" fill="#6ee7b7" radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}

function LineView({ rows }: { rows: Record<string, unknown>[] }) {
  const data = rows.map((r) => ({
    label: formatLabel(r.label),
    value: asNumber(r.value),
  }))
  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -12 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#1f2024" />
        <XAxis dataKey="label" tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <YAxis tick={{ fill: '#8a8a8a', fontSize: 11 }} stroke="#1f2024" />
        <Tooltip contentStyle={tooltipStyle} />
        <Line type="monotone" dataKey="value" stroke="#6ee7b7" strokeWidth={2} dot={{ fill: '#6ee7b7', r: 3 }} />
      </LineChart>
    </ResponsiveContainer>
  )
}

function PieView({ rows }: { rows: Record<string, unknown>[] }) {
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
        <Tooltip contentStyle={tooltipStyle} />
        <Legend wrapperStyle={{ color: '#8a8a8a', fontSize: 11 }} />
      </PieChart>
    </ResponsiveContainer>
  )
}

function TableView({ rows }: { rows: Record<string, unknown>[] }) {
  const cols = Object.keys(rows[0]).filter((k) => k !== 'id' && k !== 'updated_at')
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
                <td key={c} className="px-2 py-1.5">{formatCell(row[c])}</td>
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

const tooltipStyle: React.CSSProperties = {
  background: '#131418',
  border: '1px solid #1f2024',
  borderRadius: 6,
  color: '#e6e6e6',
  fontSize: 12,
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

function formatNumber(v: unknown): string {
  const n = asNumber(v)
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 2,
  }).format(n)
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
