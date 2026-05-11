import { createFileRoute, Link } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useState } from 'react'
import {
  AlertTriangle,
  ArrowUpRight,
  BarChart3,
  Hourglass,
  Package,
  Receipt,
  ShieldAlert,
  Sparkles,
  Table2,
  Wallet,
} from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useEntities } from '@/hooks/useEntities'
import { useDashboards } from '@/hooks/useDashboards'
import { useMe } from '@/hooks/useMe'
import { Button, Card, Pill, Textarea } from '@/components/ui'
import { toast } from '@/components/Toast'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/app/')({
  component: Dashboard,
})

function Dashboard() {
  const me = useMe()
  const entities = useEntities()
  const firstName = me.data?.user.name.split(' ')[0] ?? ''
  const isEmpty = !entities.isLoading && (entities.data ?? []).length === 0

  return (
    <div className="mx-auto max-w-5xl px-8 py-8">
      <header className="mb-6">
        <p className="text-sm text-muted-foreground">{greeting()}{firstName ? `, ${firstName}` : ''}</p>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight">
          {isEmpty ? 'Postavme něco nového.' : 'Co dnes potřebuje pozornost?'}
        </h1>
      </header>

      {isEmpty && <EmptyWorkspace />}
      {!isEmpty && <TodayPanel />}

      <section className="mt-10 grid gap-4 md:grid-cols-2">
        <EntitiesPanel />
        <DashboardsPanel />
      </section>

      <section className="mt-10">
        <AskAssistant />
      </section>

      <p className="mt-10 text-center text-xs text-muted-foreground">
        Tip: stiskni <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px]">⌘K</kbd> pro rychlé hledání.
      </p>
    </div>
  )
}

// --- Today panel ----------------------------------------------------------

function TodayPanel() {
  const entities = useEntities()
  const hasEntity = (name: string) => (entities.data ?? []).some((e) => e.name === name)

  // Pending approvals — always available.
  const approvals = useQuery({
    queryKey: ['flow-approvals'],
    queryFn: api.listFlowApprovals,
    refetchInterval: 15000,
  })

  // Overdue AR.
  const invoices = useQuery({
    queryKey: ['rows', 'invoices', 'today'],
    queryFn: () => api.listRows('invoices', { limit: 500 }).then((r) => r.rows),
    enabled: hasEntity('invoices'),
  })
  const today = new Date().toISOString().slice(0, 10)
  const overdueInvoices = (invoices.data ?? []).filter(
    (r) =>
      String(r.status ?? '').toLowerCase() !== 'paid' &&
      String(r.status ?? '').toLowerCase() !== 'draft' &&
      String(r.status ?? '').toLowerCase() !== 'cancelled' &&
      String(r.kind ?? '').toLowerCase() !== 'order_sheet' &&
      r.due_date != null &&
      String(r.due_date) < today,
  )
  const overdueTotal = overdueInvoices.reduce((s, r) => s + numf(r.total), 0)

  // AP needing review.
  const received = useQuery({
    queryKey: ['rows', 'received_invoices', 'today'],
    queryFn: () => api.listRows('received_invoices', { limit: 500 }).then((r) => r.rows),
    enabled: hasEntity('received_invoices'),
  })
  const needsReview = (received.data ?? []).filter(
    (r) => String(r.status ?? '').toLowerCase() === 'needs_review',
  )
  const needsReviewTotal = needsReview.reduce((s, r) => s + numf(r.total), 0)

  // Unmatched incoming bank transactions.
  const bankTx = useQuery({
    queryKey: ['rows', 'bank_transactions', 'today'],
    queryFn: () => api.listRows('bank_transactions', { limit: 500 }).then((r) => r.rows),
    enabled: hasEntity('bank_transactions'),
  })
  const unmatchedIn = (bankTx.data ?? []).filter(
    (r) =>
      String(r.direction ?? '').toLowerCase() === 'in' &&
      !r.matched_invoice,
  )

  const cards: StatCardProps[] = []
  cards.push({
    icon: ShieldAlert,
    tone: (approvals.data ?? []).length > 0 ? 'amber' : 'muted',
    label: 'Čeká na schválení',
    value: (approvals.data ?? []).length,
    hint:
      (approvals.data ?? []).length > 0
        ? `Otevři ${(approvals.data ?? []).length} žádost${pluralCs((approvals.data ?? []).length, '', 'i', 'í')}`
        : 'Vše schváleno',
    to: { to: '/app/approvals' as const },
  })
  if (hasEntity('invoices')) {
    cards.push({
      icon: AlertTriangle,
      tone: overdueInvoices.length > 0 ? 'destructive' : 'muted',
      label: 'Faktury po splatnosti',
      value: overdueInvoices.length,
      hint: overdueInvoices.length > 0 ? `${formatCZK(overdueTotal)} celkem` : 'Žádné po termínu',
      to: overdueInvoices.length > 0 ? { to: '/app/entities/$name' as const, params: { name: 'invoices' } } : undefined,
    })
  }
  if (hasEntity('received_invoices')) {
    cards.push({
      icon: Receipt,
      tone: needsReview.length > 0 ? 'amber' : 'muted',
      label: 'Přijaté faktury k revizi',
      value: needsReview.length,
      hint: needsReview.length > 0 ? `${formatCZK(needsReviewTotal)} čeká` : 'Vše prošlo',
      to: needsReview.length > 0 ? { to: '/app/entities/$name' as const, params: { name: 'received_invoices' } } : undefined,
    })
  }
  if (hasEntity('bank_transactions')) {
    cards.push({
      icon: Wallet,
      tone: unmatchedIn.length > 0 ? 'amber' : 'muted',
      label: 'Nepárované platby',
      value: unmatchedIn.length,
      hint: unmatchedIn.length > 0 ? 'Bez faktury podle VS' : 'Vše spárováno',
      to: unmatchedIn.length > 0 ? { to: '/app/entities/$name' as const, params: { name: 'bank_transactions' } } : undefined,
    })
  }

  const loading = approvals.isLoading || invoices.isLoading || received.isLoading || bankTx.isLoading

  return (
    <section className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {loading
          ? Array.from({ length: cards.length || 4 }).map((_, i) => (
              <div key={i} className="h-[88px] animate-pulse rounded-md bg-muted/30" />
            ))
          : cards.map((c, i) => <StatCard key={i} {...c} />)}
      </div>

      {overdueInvoices.length > 0 && (
        <AttentionList
          icon={AlertTriangle}
          title="Faktury po splatnosti"
          tone="destructive"
          items={overdueInvoices.slice(0, 5).map((r) => ({
            id: String(r.id),
            primary: String(r.number ?? '—'),
            secondary: r.due_date ? `splatno ${formatDate(String(r.due_date))}` : '',
            amount: numf(r.total),
          }))}
          allHref={{ to: '/app/entities/$name', params: { name: 'invoices' } }}
        />
      )}

      {needsReview.length > 0 && (
        <AttentionList
          icon={Receipt}
          title="Přijaté faktury k revizi"
          tone="amber"
          items={needsReview.slice(0, 5).map((r) => ({
            id: String(r.id),
            primary: String(r.supplier_name ?? r.external_id ?? '—'),
            secondary: r.due_date ? `splatno ${formatDate(String(r.due_date))}` : '',
            amount: numf(r.total),
          }))}
          allHref={{ to: '/app/entities/$name', params: { name: 'received_invoices' } }}
        />
      )}
    </section>
  )
}

// --- entities + dashboards panels ----------------------------------------

function EntitiesPanel() {
  const entities = useEntities()
  const list = entities.data ?? []
  return (
    <Card className="p-5">
      <header className="mb-3 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-medium">
          <Table2 className="h-4 w-4 text-muted-foreground" />
          Entity
        </h2>
        <span className="text-xs text-muted-foreground">{list.length}</span>
      </header>
      {entities.isLoading && (
        <div className="space-y-2">
          <div className="h-9 animate-pulse rounded-md bg-muted/30" />
          <div className="h-9 animate-pulse rounded-md bg-muted/30" />
          <div className="h-9 animate-pulse rounded-md bg-muted/30" />
        </div>
      )}
      {!entities.isLoading && list.length > 0 && (
        <ul className="space-y-1">
          {list.slice(0, 8).map((e) => (
            <li key={e.id}>
              <Link
                to="/app/entities/$name"
                params={{ name: e.name }}
                className="group flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent"
              >
                <span className="min-w-0 truncate">{e.display_name || e.name}</span>
                <Pill>{e.name}</Pill>
              </Link>
            </li>
          ))}
          {list.length > 8 && (
            <li className="px-2 pt-1 text-xs text-muted-foreground">+ {list.length - 8} dalších</li>
          )}
        </ul>
      )}
    </Card>
  )
}

function DashboardsPanel() {
  const dashboards = useDashboards()
  const list = dashboards.data ?? []
  return (
    <Card className="p-5">
      <header className="mb-3 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-medium">
          <BarChart3 className="h-4 w-4 text-muted-foreground" />
          Dashboardy
        </h2>
        <span className="text-xs text-muted-foreground">{list.length}</span>
      </header>
      {dashboards.isLoading && (
        <div className="space-y-2">
          <div className="h-9 animate-pulse rounded-md bg-muted/30" />
          <div className="h-9 animate-pulse rounded-md bg-muted/30" />
        </div>
      )}
      {!dashboards.isLoading && list.length === 0 && (
        <p className="text-sm text-muted-foreground">Zatím tu žádné nejsou.</p>
      )}
      {list.length > 0 && (
        <ul className="space-y-1">
          {list.map((d) => (
            <li key={d.id}>
              <Link
                to="/app/dashboards/$slug"
                params={{ slug: d.slug }}
                className="group flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-accent"
              >
                <span className="min-w-0 truncate">{d.name}</span>
                <ArrowUpRight className="h-3.5 w-3.5 text-muted-foreground" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

// --- attention list -------------------------------------------------------

type ListTone = 'destructive' | 'amber'

function AttentionList({
  icon: Icon,
  title,
  tone,
  items,
  allHref,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  tone: ListTone
  items: { id: string; primary: string; secondary: string; amount: number }[]
  allHref: { to: '/app/entities/$name'; params: { name: string } }
}) {
  return (
    <Card className="p-5">
      <header className="mb-3 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-medium">
          <Icon className={cn('h-4 w-4', tone === 'destructive' ? 'text-destructive' : 'text-amber-500')} />
          {title}
        </h2>
        <Link to={allHref.to} params={allHref.params} className="text-xs text-primary hover:underline">
          Zobrazit vše
        </Link>
      </header>
      <ul className="divide-y divide-border">
        {items.map((it) => (
          <li key={it.id} className="flex items-center justify-between gap-3 py-2 text-sm">
            <div className="min-w-0 flex-1">
              <p className="truncate">{it.primary}</p>
              {it.secondary && <p className="text-xs text-muted-foreground">{it.secondary}</p>}
            </div>
            <span className="font-mono text-xs">{formatCZK(it.amount)}</span>
          </li>
        ))}
      </ul>
    </Card>
  )
}

// --- stat card ------------------------------------------------------------

type Tone = 'muted' | 'amber' | 'destructive'

interface StatCardProps {
  icon: React.ComponentType<{ className?: string }>
  tone: Tone
  label: string
  value: number | string
  hint?: string
  to?:
    | { to: '/app/approvals' }
    | { to: '/app/entities/$name'; params: { name: string } }
}

function StatCard({ icon: Icon, tone, label, value, hint, to }: StatCardProps) {
  const body = (
    <Card
      className={cn(
        'group flex items-start gap-3 p-4 transition-colors',
        to && 'hover:bg-accent hover:border-primary/40',
        tone === 'destructive' && Number(value) > 0 && 'border-destructive/40',
        tone === 'amber' && Number(value) > 0 && 'border-amber-500/40',
      )}
    >
      <div
        className={cn(
          'flex h-9 w-9 shrink-0 items-center justify-center rounded-md',
          tone === 'destructive' && Number(value) > 0
            ? 'bg-destructive/10 text-destructive'
            : tone === 'amber' && Number(value) > 0
              ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
              : 'bg-muted/40 text-muted-foreground',
        )}
      >
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-[11px] uppercase tracking-wider text-muted-foreground">{label}</p>
        <p className="mt-0.5 text-2xl font-semibold leading-none">{value}</p>
        {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
      </div>
      {to && <ArrowUpRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />}
    </Card>
  )
  if (!to) return body
  if (to.to === '/app/approvals') {
    return (
      <Link to={to.to} className="block">
        {body}
      </Link>
    )
  }
  return (
    <Link to={to.to} params={to.params} className="block">
      {body}
    </Link>
  )
}

// --- empty workspace + assistant -----------------------------------------

function EmptyWorkspace() {
  return (
    <section className="space-y-4">
      <Card className="border-dashed bg-muted/10 p-6 text-sm text-muted-foreground">
        <Hourglass className="mb-2 h-5 w-5" />
        <p>Pracovní prostor je zatím prázdný. Začni šablonou, nebo popiš, co chceš v aplikaci sledovat.</p>
      </Card>
      <TemplatePicker />
    </section>
  )
}

function TemplatePicker() {
  const qc = useQueryClient()
  const templates = useQuery({ queryKey: ['templates'], queryFn: api.listTemplates })
  const [appliedId, setAppliedId] = useState<string | null>(null)

  const apply = useMutation({
    mutationFn: (id: string) => api.applyTemplate(id),
    onMutate: (id) => setAppliedId(id),
    onSuccess: async (_, id) => {
      await qc.invalidateQueries({ queryKey: ['entities'] })
      await qc.invalidateQueries({ queryKey: ['dashboards'] })
      await qc.invalidateQueries({ queryKey: ['flows'] })
      toast.success(`Šablona ${id} nainstalována.`)
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Instalace šablony selhala.'),
    onSettled: () => setAppliedId(null),
  })

  if (templates.isLoading || !templates.data || templates.data.length === 0) return null

  return (
    <div>
      <h3 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Začni šablonou
      </h3>
      <div className="grid gap-2">
        {templates.data.map((t) => (
          <Card key={t.id} className="flex items-center gap-4 p-4">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
              <Package className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h4 className="font-medium">{t.name}</h4>
                <Pill>{t.id}</Pill>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{t.description}</p>
            </div>
            <Button onClick={() => apply.mutate(t.id)} disabled={apply.isPending}>
              {appliedId === t.id ? 'Instaluji…' : 'Instalovat'}
            </Button>
          </Card>
        ))}
      </div>
    </div>
  )
}

const SUGGESTIONS = [
  'Klient s názvem, e-mailem a volitelným telefonem',
  'Faktura s částkou, splatností a odkazem na klienta',
  'Projekt s názvem, stavem a odhadem hodin',
]

function AskAssistant() {
  const qc = useQueryClient()
  const { register, handleSubmit, reset, setValue, formState: { isSubmitting } } =
    useForm<{ description: string }>()

  const propose = useMutation({
    mutationFn: (description: string) => api.proposeEntity(description),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['entities'] })
      toast.success('Entita vytvořena.')
      reset()
    },
    onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Vytvoření selhalo.'),
  })

  return (
    <Card className="overflow-hidden border-primary/30 bg-gradient-to-br from-card to-card/70">
      <form
        onSubmit={handleSubmit((v) => {
          propose.mutate(v.description)
        })}
      >
        <div className="flex items-start gap-3 p-4">
          <div className="mt-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Sparkles className="h-4 w-4" />
          </div>
          <div className="flex-1">
            <Textarea
              rows={3}
              placeholder="Popiš, co chceš sledovat — česky nebo anglicky."
              className="resize-none border-0 bg-transparent p-0 shadow-none focus-visible:ring-0"
              {...register('description', { required: true, minLength: 8 })}
            />
            <div className="mt-3 flex flex-wrap gap-1.5">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setValue('description', s, { shouldValidate: true })}
                  className="rounded-full border border-border bg-background/60 px-3 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        </div>
        <div className="flex items-center justify-between border-t border-border bg-muted/20 px-4 py-2">
          <span className="text-xs text-muted-foreground">
            Claude navrhne schéma a vytvoří skutečnou tabulku.
          </span>
          <Button type="submit" disabled={isSubmitting || propose.isPending}>
            {propose.isPending ? 'Navrhuji…' : 'Vytvořit entitu'}
          </Button>
        </div>
      </form>
    </Card>
  )
}

// --- helpers --------------------------------------------------------------

function greeting(): string {
  const h = new Date().getHours()
  if (h < 5) return 'Dobrou noc'
  if (h < 11) return 'Dobré ráno'
  if (h < 17) return 'Dobrý den'
  return 'Dobrý večer'
}

function numf(v: unknown): number {
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = parseFloat(v.replace(',', '.'))
    return isFinite(n) ? n : 0
  }
  return 0
}

function formatCZK(n: number): string {
  return new Intl.NumberFormat('cs-CZ', { style: 'currency', currency: 'CZK', maximumFractionDigits: 0 }).format(n)
}

function formatDate(s: string): string {
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleDateString('cs-CZ')
}

// pluralCs returns the noun ending for Czech counts (1, 2-4, 5+).
function pluralCs(n: number, one: string, few: string, many: string): string {
  if (n === 1) return one
  if (n >= 2 && n <= 4) return few
  return many
}

