import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BarChart3,
  Bot,
  CheckCircle2,
  Code2,
  Database,
  Github,
  KeyRound,
  Landmark,
  Lock,
  Receipt,
  Server,
  ShieldCheck,
  Sparkles,
  Wallet,
  Workflow,
  Zap,
} from 'lucide-react'

// MarketingHome is the public landing page served at "/" to anonymous
// visitors. Logged-in users are redirected from the route file before
// this component renders.
export function MarketingHome() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <TopNav />
      <Hero />
      <FeatureGrid />
      <AgencyStory />
      <ConnectorWall />
      <PricingStrip />
      <Footer />
    </div>
  )
}

// --- nav ------------------------------------------------------------------

function TopNav() {
  return (
    <header className="sticky top-0 z-30 border-b border-border bg-background/80 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <Link to="/" className="inline-flex items-center gap-2 text-base font-semibold tracking-tight">
          <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Sparkles className="h-3.5 w-3.5" />
          </span>
          openrow<span className="text-primary">.</span>
        </Link>
        <nav className="hidden items-center gap-6 text-sm text-muted-foreground md:flex">
          <a href="#features" className="hover:text-foreground">Features</a>
          <a href="#agency" className="hover:text-foreground">For agencies</a>
          <a href="#connectors" className="hover:text-foreground">Connectors</a>
          <Link to="/docs" className="hover:text-foreground">Docs</Link>
          <a href="#self-host" className="hover:text-foreground">Self-host</a>
          <a
            href="https://github.com/steezrcom/openrow"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 hover:text-foreground"
          >
            <Github className="h-3.5 w-3.5" />
            GitHub
          </a>
        </nav>
        <div className="flex items-center gap-2">
          <Link
            to="/login"
            className="rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground"
          >
            Sign in
          </Link>
          <Link
            to="/signup"
            className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90"
          >
            Try it free
            <ArrowRight className="h-3.5 w-3.5" />
          </Link>
        </div>
      </div>
    </header>
  )
}

// --- hero -----------------------------------------------------------------

function Hero() {
  return (
    <section className="relative overflow-hidden">
      {/* soft gradient glow */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 -top-32 mx-auto h-[400px] max-w-3xl rounded-full bg-primary/10 blur-3xl"
      />
      <div className="relative mx-auto grid max-w-6xl gap-12 px-6 py-20 md:grid-cols-[1.05fr,1fr] md:py-28 md:gap-10">
        <div>
          <h1 className="text-4xl font-semibold leading-[1.05] tracking-tight md:text-6xl">
            Describe your back office.
            <br />
            <span className="text-primary">openrow</span> builds it.
          </h1>
          <p className="mt-6 max-w-xl text-lg leading-relaxed text-muted-foreground">
            Tell openrow{' '}
            <span className="italic">"I need a table of received invoices with status, supplier, amount and due date"</span>
            {' '}and it creates the real Postgres table, the form, the dashboard, and the automation around it. In one chat. In about a minute.
          </p>
          <div className="mt-8 flex flex-wrap items-center gap-3">
            <Link
              to="/signup"
              className="inline-flex items-center gap-2 rounded-md bg-primary px-5 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Try it free
              <ArrowRight className="h-4 w-4" />
            </Link>
            <a
              href="https://github.com/steezrcom/openrow"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-md border border-border px-5 py-3 text-sm font-medium hover:bg-accent"
            >
              <Github className="h-4 w-4" />
              View source
            </a>
          </div>
          <p className="mt-6 text-xs text-muted-foreground">
            AGPL-3.0. Self-host the binary or run on openrow.app.
          </p>
        </div>

        <ChatPreview />
      </div>
    </section>
  )
}

// ChatPreview is a stylised, non-interactive snippet that mimics the
// real chat panel. Just enough motion to feel alive — no actual data
// fetch, no API calls.
function ChatPreview() {
  return (
    <div className="relative">
      <div className="absolute -inset-4 rounded-2xl bg-primary/5 blur-2xl" aria-hidden />
      <div className="relative rounded-xl border border-border bg-card p-4 shadow-2xl">
        <div className="mb-3 flex items-center gap-2 border-b border-border pb-3">
          <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Sparkles className="h-3.5 w-3.5" />
          </span>
          <span className="text-xs font-medium">Chat</span>
          <span className="ml-auto text-[10px] uppercase tracking-wider text-muted-foreground">agent</span>
        </div>
        <div className="space-y-3 text-sm">
          <div className="rounded-md bg-muted/40 px-3 py-2 leading-relaxed">
            A contact with name, e-mail, IČO and a flag for whether they're a VAT payer.
          </div>
          <div className="rounded-md border border-border px-3 py-2 leading-relaxed">
            <p className="text-xs text-muted-foreground">openrow</p>
            <p className="mt-1">Creating entity <code className="rounded bg-emerald-500/10 px-1 py-0.5 font-mono text-[12px] text-emerald-700 dark:text-emerald-300">contacts</code> with 4 fields…</p>
            <p className="mt-2 font-mono text-[11px] text-muted-foreground">
              ✓ name · TEXT NOT NULL<br />
              ✓ email · TEXT<br />
              ✓ ico · TEXT<br />
              ✓ vat_payer · BOOLEAN
            </p>
            <p className="mt-2 text-xs text-muted-foreground">Table, form, and list ready at <code className="rounded bg-muted px-1 font-mono text-[11px]">/app/entities/contacts</code>.</p>
          </div>
          <div className="rounded-md bg-muted/40 px-3 py-2 leading-relaxed">
            Now add a flow that emails me when a new contact is added.
          </div>
          <div className="rounded-md border border-border px-3 py-2 leading-relaxed">
            <p className="text-xs text-muted-foreground">openrow</p>
            <p className="mt-1">Drafted a flow with trigger <code className="rounded bg-violet-500/10 px-1 py-0.5 font-mono text-[12px] text-violet-700 dark:text-violet-300">contacts.insert</code> and one tool: <code className="rounded bg-emerald-500/10 px-1 py-0.5 font-mono text-[12px] text-emerald-700 dark:text-emerald-300">connector_resend_send_email</code>.</p>
            <p className="mt-2 text-xs text-muted-foreground">In <span className="font-medium">approve</span> mode by default. Promote to auto when you trust it.</p>
          </div>
        </div>
      </div>
    </div>
  )
}

// --- features -------------------------------------------------------------

function FeatureGrid() {
  return (
    <section id="features" className="border-t border-border bg-muted/10">
      <div className="mx-auto max-w-6xl px-6 py-20">
        <div className="mb-12 max-w-2xl">
          <p className="text-xs font-medium uppercase tracking-wider text-primary">What's inside</p>
          <h2 className="mt-2 text-3xl font-semibold tracking-tight md:text-4xl">
            The pieces of a back office, in one place.
          </h2>
          <p className="mt-4 text-muted-foreground">
            Tables, dashboards, automations, and the connectors to plug them into the real world. Each one solid enough to run a small company on. Together because copying data between Notion, Google Sheets, and your accounting tool is the worst part of every founder's week.
          </p>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Feature
            icon={Database}
            title="Tables you describe in words"
            kicker="Entities"
          >
            <p>
              Type <em>"a project with name, client, status and a deadline"</em>. The form, the list view, the kanban, the foreign-key dropdown to <code className="rounded bg-muted px-1 font-mono text-[11px]">clients</code>: all there before you finish your coffee. Behind the scenes it's a real Postgres table that your engineers can query directly.
            </p>
          </Feature>

          <Feature icon={Workflow} title="Automations with brakes on" kicker="Flows">
            <p>
              Every flow starts in <span className="font-medium">dry-run</span>. The log shows exactly what it would have done. When you trust it, flip to <span className="font-medium">approve</span> and the agent waits for you on every write. When that's boring, flip to <span className="font-medium">auto</span> and it disappears into the cron. The "AI goes rogue" risk is solved by design.
            </p>
          </Feature>

          <Feature icon={Landmark} title="Banks and accounting without a licence" kicker="Connectors">
            <p>
              Read your own Česká spořitelna, ČSOB, Fio and Revolut Business. Push and pull invoices in UOL or Fakturoid. Mirror to a Notion finanční databáze. The bank connectors use refresh-token OAuth. Paste once, working until you rotate.
            </p>
          </Feature>

          <Feature icon={BarChart3} title="Dashboards a non-finance person reads" kicker="Reports">
            <p>
              <span className="font-medium">Příjmy</span> this month, with month-over-month delta. Overdue invoices count plus the actual amount in koruna. Cash position across four bank accounts. Cost categories as a pie. Each dashboard has a date picker that scopes every widget.
            </p>
          </Feature>

          <Feature icon={KeyRound} title="Your LLM, your bill" kicker="BYOK">
            <p>
              Bring your own key for Anthropic, OpenAI, Groq, Together, Gemini, DeepSeek, or a local Ollama box. The key is encrypted with a per-deployment AES-256-GCM root key. Costs land in your provider account. Per-company isolation, per-company choice.
            </p>
          </Feature>

          <Feature icon={Server} title="Open source. Self-host or use ours." kicker="AGPL-3.0">
            <p>
              Single Go binary plus the React SPA, served from one container. Postgres for state. A 40-line Dockerfile and a Dokku/Heroku push gets you to production in an afternoon. Or sign up on openrow.app and we'll run the boxes.
            </p>
          </Feature>
        </div>
      </div>
    </section>
  )
}

function Feature({
  icon: Icon,
  title,
  kicker,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  kicker: string
  children: React.ReactNode
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-6 transition-colors hover:border-primary/40">
      <div className="mb-4 flex items-center gap-3">
        <span className="inline-flex h-9 w-9 items-center justify-center rounded-md bg-primary/15 text-primary">
          <Icon className="h-4 w-4" />
        </span>
        <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{kicker}</span>
      </div>
      <h3 className="text-lg font-semibold tracking-tight">{title}</h3>
      <div className="mt-2 text-sm leading-relaxed text-muted-foreground">{children}</div>
    </div>
  )
}

// --- agency story ---------------------------------------------------------

function AgencyStory() {
  return (
    <section id="agency" className="border-t border-border">
      <div className="mx-auto max-w-6xl px-6 py-20">
        <div className="grid gap-12 md:grid-cols-[1fr,1.05fr] md:gap-16">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-primary">For agencies and studios</p>
            <h2 className="mt-2 text-3xl font-semibold tracking-tight md:text-4xl">
              What it looks like at a real agency in Prague.
            </h2>
            <p className="mt-6 text-muted-foreground">
              Michaela runs an ad agency. Six people, four bank accounts (ČS, ČSOB, Fio, Revolut), invoicing in UOL, project management in Notion, and a Slack channel called <code className="rounded bg-muted px-1 font-mono text-[11px]">#faktury</code> nobody read.
            </p>
            <p className="mt-4 text-muted-foreground">
              The agency template ships with twelve entities and thirteen flows wired end-to-end. From the day she signs in, here's the loop running for her:
            </p>
            <ol className="mt-6 space-y-3 text-sm text-muted-foreground">
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">1</span>
                <span><span className="text-foreground">06:00.</span> Yesterday's transactions stream in from every bank.</span>
              </li>
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">2</span>
                <span><span className="text-foreground">06:15.</span> Incoming payments match unpaid invoices by VS. Outgoing payments match received invoices the same way. Invoices flip to paid automatically.</span>
              </li>
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">3</span>
                <span><span className="text-foreground">06:30.</span> Uncategorised outflows get classified by the agent into personal / production / overhead / other, with anything ambiguous flagged for review.</span>
              </li>
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">4</span>
                <span><span className="text-foreground">08:00, Mon-Fri.</span> Daniel gets a Slack digest of received invoices waiting for his approval to pay.</span>
              </li>
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">5</span>
                <span><span className="text-foreground">Every 15 min.</span> New draft invoices in openrow get issued in UOL with the proper number, and emailed to the client. The UOL <code className="rounded bg-muted px-1 font-mono text-[10px]">public_id</code> comes back into the row.</span>
              </li>
              <li className="flex gap-3">
                <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-medium text-primary">6</span>
                <span><span className="text-foreground">Monday, 09:00.</span> Overdue invoices send a polite reminder. Two reminders without a payment, Slack pings Daniel.</span>
              </li>
            </ol>
            <p className="mt-6 text-sm text-muted-foreground">
              Approval mode means Michaela sees every automatic mutation before it lands. Three weeks of clean runs, she flips them to auto. Accounting time per week falls under two hours.
            </p>
          </div>

          <DashboardPreview />
        </div>
      </div>
    </section>
  )
}

function DashboardPreview() {
  return (
    <div className="relative">
      <div className="absolute -inset-6 rounded-2xl bg-primary/5 blur-2xl" aria-hidden />
      <div className="relative space-y-3 rounded-xl border border-border bg-card p-5 shadow-xl">
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold">Finance</p>
          <span className="rounded border border-border px-2 py-0.5 font-mono text-[10px] text-muted-foreground">finance</span>
        </div>

        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Tile label="Příjmy" value="184 230 Kč" tone="primary" />
          <Tile label="Výdaje" value="-62 410 Kč" />
          <Tile label="Cash" value="612 800 Kč" />
          <Tile label="Nepárované" value="2" tone="amber" />
        </div>

        <div className="rounded-md border border-border p-3">
          <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Příjmy vs. výdaje</p>
          <BarChartPreview />
        </div>

        <div className="grid grid-cols-2 gap-2">
          <div className="rounded-md border border-border p-3">
            <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Kategorie nákladů</p>
            <PiePreview />
          </div>
          <div className="rounded-md border border-border p-3">
            <p className="text-[10px] uppercase tracking-wider text-muted-foreground">Plán vs. skutečnost</p>
            <SmallTable />
          </div>
        </div>
      </div>
    </div>
  )
}

function Tile({ label, value, tone }: { label: string; value: string; tone?: 'primary' | 'amber' }) {
  return (
    <div className="rounded-md border border-border p-3">
      <p className="text-[10px] uppercase tracking-wider text-muted-foreground">{label}</p>
      <p
        className={
          tone === 'primary'
            ? 'mt-1 text-base font-semibold text-primary'
            : tone === 'amber'
              ? 'mt-1 text-base font-semibold text-amber-600 dark:text-amber-400'
              : 'mt-1 text-base font-semibold'
        }
      >
        {value}
      </p>
    </div>
  )
}

function BarChartPreview() {
  // Two-series mock bars (income vs. expense) over six months.
  const data = [
    { m: 'Pro', i: 64, e: 22 },
    { m: 'Led', i: 58, e: 18 },
    { m: 'Úno', i: 71, e: 26 },
    { m: 'Bře', i: 82, e: 21 },
    { m: 'Dub', i: 78, e: 25 },
    { m: 'Kvě', i: 92, e: 30 },
  ]
  return (
    <div className="mt-2 flex h-24 items-end gap-2">
      {data.map((d) => (
        <div key={d.m} className="flex flex-1 flex-col items-stretch gap-0.5">
          <div className="flex h-full items-end gap-0.5">
            <div
              className="flex-1 rounded-t bg-primary/70"
              style={{ height: `${d.i}%` }}
              aria-label={`income ${d.i}`}
            />
            <div
              className="flex-1 rounded-t bg-muted-foreground/40"
              style={{ height: `${d.e}%` }}
              aria-label={`expense ${d.e}`}
            />
          </div>
          <span className="text-center text-[9px] text-muted-foreground">{d.m}</span>
        </div>
      ))}
    </div>
  )
}

function PiePreview() {
  // CSS conic-gradient pie. Four slices, primary + three muted shades.
  const slices = 'conic-gradient(hsl(var(--primary)) 0 35%, hsl(var(--primary)/0.6) 35% 58%, hsl(var(--muted-foreground)/0.4) 58% 80%, hsl(var(--muted-foreground)/0.2) 80% 100%)'
  return (
    <div className="mt-2 flex items-center gap-3">
      <div
        className="h-16 w-16 rounded-full"
        style={{ background: slices }}
        aria-hidden
      />
      <ul className="space-y-1 text-[10px] text-muted-foreground">
        <li><span className="mr-1 inline-block h-2 w-2 rounded-full bg-primary" />Personální</li>
        <li><span className="mr-1 inline-block h-2 w-2 rounded-full bg-primary/60" />Produkční</li>
        <li><span className="mr-1 inline-block h-2 w-2 rounded-full bg-muted-foreground/40" />Režijní</li>
        <li><span className="mr-1 inline-block h-2 w-2 rounded-full bg-muted-foreground/20" />Ostatní</li>
      </ul>
    </div>
  )
}

function SmallTable() {
  return (
    <table className="mt-2 w-full text-[10px]">
      <thead>
        <tr className="text-left text-muted-foreground">
          <th className="font-medium">Měsíc</th>
          <th className="text-right font-medium">Plán</th>
          <th className="text-right font-medium">Real.</th>
        </tr>
      </thead>
      <tbody>
        <tr><td>Led</td><td className="text-right">180k</td><td className="text-right text-primary">192k</td></tr>
        <tr><td>Úno</td><td className="text-right">180k</td><td className="text-right text-primary">184k</td></tr>
        <tr><td>Bře</td><td className="text-right">200k</td><td className="text-right text-destructive">178k</td></tr>
      </tbody>
    </table>
  )
}

// --- connectors wall ------------------------------------------------------

function ConnectorWall() {
  const connectors = [
    'Česká spořitelna',
    'ČSOB',
    'Fio banka',
    'Revolut Business',
    'ÚOL',
    'Fakturoid',
    'Notion',
    'Slack',
    'Discord',
    'Resend',
    'GitHub',
    'Linear',
    'ARES',
    'ČNB',
    'VIES',
    'Stripe',
  ]
  return (
    <section id="connectors" className="border-t border-border bg-muted/10">
      <div className="mx-auto max-w-6xl px-6 py-20">
        <div className="mb-10 max-w-2xl">
          <p className="text-xs font-medium uppercase tracking-wider text-primary">Connectors</p>
          <h2 className="mt-2 text-3xl font-semibold tracking-tight md:text-4xl">
            Sixteen integrations and counting. Mostly Czech-flavoured.
          </h2>
          <p className="mt-4 text-muted-foreground">
            Every connector is read- or write-scoped per action, surfaced to the agent as a typed tool, and stored encrypted at rest. Adding one is a single Go file plus a line in the catalog.
          </p>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
          {connectors.map((name) => (
            <div
              key={name}
              className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-3 text-sm transition-colors hover:border-primary/40"
            >
              <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-xs font-semibold text-primary">
                {name.charAt(0)}
              </span>
              <span className="truncate">{name}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

// --- pricing / self-host strip --------------------------------------------

function PricingStrip() {
  return (
    <section id="self-host" className="border-t border-border">
      <div className="mx-auto max-w-6xl px-6 py-20">
        <div className="grid gap-6 md:grid-cols-2">
          <div className="rounded-xl border border-border bg-card p-6">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Self-host</p>
            <p className="mt-2 text-2xl font-semibold tracking-tight">Free, AGPL-3.0.</p>
            <p className="mt-3 text-sm text-muted-foreground">
              One Go binary, one Postgres database, one Dockerfile. Clone, build, push. Your data on your boxes. No seat limits, no telemetry, no calls home.
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Pill icon={Lock}>Encrypted secrets at rest</Pill>
              <Pill icon={ShieldCheck}>Per-tenant isolation</Pill>
              <Pill icon={Code2}>Go + React + Postgres</Pill>
            </div>
            <a
              href="https://github.com/steezrcom/openrow"
              target="_blank"
              rel="noreferrer"
              className="mt-6 inline-flex items-center gap-2 rounded-md border border-border px-4 py-2 text-sm font-medium hover:bg-accent"
            >
              <Github className="h-4 w-4" />
              github.com/steezrcom/openrow
            </a>
          </div>
          <div className="rounded-xl border border-primary/40 bg-primary/5 p-6">
            <p className="text-xs font-medium uppercase tracking-wider text-primary">Hosted</p>
            <p className="mt-2 text-2xl font-semibold tracking-tight">openrow.app</p>
            <p className="mt-3 text-sm text-muted-foreground">
              The same software, run by us. Updates, backups and SSL handled. Bring your own LLM key so the API costs stay in your account. Per-tenant data, per-tenant choice.
            </p>
            <div className="mt-5 space-y-2 text-sm">
              <Check>Daily Postgres backups, 14-day retention</Check>
              <Check>Let's Encrypt SSL on your custom domain</Check>
              <Check>Migration to self-host any time, your data is yours</Check>
            </div>
            <Link
              to="/signup"
              className="mt-6 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Start a workspace
              <ArrowRight className="h-4 w-4" />
            </Link>
          </div>
        </div>
      </div>
    </section>
  )
}

function Pill({ icon: Icon, children }: { icon: React.ComponentType<{ className?: string }>; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1">
      <Icon className="h-3 w-3" />
      {children}
    </span>
  )
}

function Check({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2 text-muted-foreground">
      <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />
      <span>{children}</span>
    </div>
  )
}

// --- bottom CTA + footer --------------------------------------------------

function Footer() {
  return (
    <section className="border-t border-border bg-muted/10">
      <div className="mx-auto max-w-6xl px-6 py-16">
        <div className="rounded-2xl border border-border bg-card p-8 md:p-12">
          <div className="grid items-center gap-6 md:grid-cols-[1.4fr,1fr]">
            <div>
              <h2 className="text-3xl font-semibold tracking-tight">
                Want to see if your back office fits?
              </h2>
              <p className="mt-3 text-muted-foreground">
                Spin up a workspace, install the agency template, paste your LLM key, and have a working invoicing pipeline before lunch. Or download the binary and run it on your own box.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-3 md:justify-end">
              <Link
                to="/signup"
                className="inline-flex items-center gap-2 rounded-md bg-primary px-5 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90"
              >
                <Zap className="h-4 w-4" />
                Start free
              </Link>
              <a
                href="mailto:hello@steezr.com"
                className="inline-flex items-center gap-2 rounded-md border border-border px-5 py-3 text-sm font-medium hover:bg-accent"
              >
                Talk to us
              </a>
            </div>
          </div>
        </div>

        <div className="mt-12 flex flex-col gap-6 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
          <div className="flex items-center gap-2">
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary/15 text-primary">
              <Sparkles className="h-3.5 w-3.5" />
            </span>
            <span>openrow</span>
            <span className="text-muted-foreground/60">·</span>
            <span>AGPL-3.0</span>
          </div>
          <div className="flex flex-wrap items-center gap-5">
            <Link to="/docs" className="hover:text-foreground">Docs</Link>
            <a href="https://github.com/steezrcom/openrow" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:text-foreground">
              <Github className="h-3.5 w-3.5" />
              GitHub
            </a>
            <Link to="/login" className="hover:text-foreground">Sign in</Link>
            <a href="mailto:hello@steezr.com" className="hover:text-foreground">hello@steezr.com</a>
          </div>
        </div>
      </div>

      {/* Subtle bottom badge */}
      <div className="border-t border-border bg-background py-4 text-center text-[11px] text-muted-foreground">
        Made by{' '}
        <a href="https://steezr.com" target="_blank" rel="noreferrer" className="hover:text-foreground">
          steezr
        </a>
        {' '}in Prague.{' '}
        <Bot className="ml-1 inline h-3 w-3 align-text-bottom" />{' '}
        Powered by whichever LLM you bring to the workspace.
        <span className="ml-2"><Receipt className="ml-1 inline h-3 w-3 align-text-bottom" /></span>
        <span className="ml-2"><Wallet className="ml-1 inline h-3 w-3 align-text-bottom" /></span>
      </div>
    </section>
  )
}
