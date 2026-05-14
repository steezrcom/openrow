import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowRight,
  Github,
  Menu,
  Sparkles,
  X,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

// Public docs layout: top nav matches MarketingHome, sticky sidebar with
// section nav, and a main content area that scrolls independently.
export function DocsLayout({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  useEffect(() => {
    setOpen(false)
  }, [pathname])

  return (
    <div className="min-h-screen bg-background text-foreground">
      <TopNav onMenu={() => setOpen((v) => !v)} mobileOpen={open} />
      <div className="mx-auto flex max-w-7xl gap-10 px-4 pb-24 pt-10 md:px-6 lg:px-8">
        <aside
          className={cn(
            'fixed inset-y-0 left-0 z-40 w-72 border-r border-border bg-background p-6 pt-16 transition-transform md:sticky md:top-16 md:z-0 md:h-[calc(100vh-4rem)] md:translate-x-0 md:border-r-0 md:bg-transparent md:p-0 md:pt-0',
            open ? 'translate-x-0' : '-translate-x-full',
          )}
        >
          <div className="md:sticky md:top-20 md:max-h-[calc(100vh-6rem)] md:overflow-y-auto md:pr-2">
            <SideNav />
          </div>
        </aside>
        <main className="min-w-0 flex-1">
          {children}
          <BottomNav />
        </main>
      </div>
      {open && (
        <button
          aria-label="Close menu"
          onClick={() => setOpen(false)}
          className="fixed inset-0 z-30 bg-background/60 backdrop-blur-sm md:hidden"
        />
      )}
    </div>
  )
}

function TopNav({ onMenu, mobileOpen }: { onMenu: () => void; mobileOpen: boolean }) {
  return (
    <header className="sticky top-0 z-50 border-b border-border bg-background/80 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-3 md:px-6 lg:px-8">
        <div className="flex items-center gap-2">
          <button
            onClick={onMenu}
            aria-label="Toggle navigation"
            className="-ml-1 inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent md:hidden"
          >
            {mobileOpen ? <X className="h-4 w-4" /> : <Menu className="h-4 w-4" />}
          </button>
          <Link to="/" className="inline-flex items-center gap-2 text-base font-semibold tracking-tight">
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-primary/15 text-primary">
              <Sparkles className="h-3.5 w-3.5" />
            </span>
            openrow<span className="text-primary">.</span>
            <span className="ml-1 hidden rounded border border-border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground sm:inline">
              docs
            </span>
          </Link>
        </div>
        <nav className="hidden items-center gap-6 text-sm text-muted-foreground md:flex">
          <Link to="/" className="hover:text-foreground" hash="features">
            Features
          </Link>
          <Link to="/" className="hover:text-foreground" hash="connectors">
            Integrations
          </Link>
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

const nav: { title: string; items: { to: string; label: string }[] }[] = [
  {
    title: 'Get started',
    items: [
      { to: '/docs', label: 'Welcome' },
      { to: '/docs/quickstart', label: 'Quickstart' },
      { to: '/docs/concepts', label: 'Core concepts' },
    ],
  },
  {
    title: 'Using openrow',
    items: [
      { to: '/docs/how-tos', label: 'How-tos' },
      { to: '/docs/llm', label: 'LLM setup' },
      { to: '/docs/connectors', label: 'Connectors' },
    ],
  },
  {
    title: 'Run it yourself',
    items: [
      { to: '/docs/self-hosting', label: 'Self-hosting' },
      { to: '/docs/security', label: 'Security & privacy' },
    ],
  },
]

function SideNav() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <nav className="space-y-6 text-sm">
      {nav.map((group) => (
        <div key={group.title}>
          <p className="mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {group.title}
          </p>
          <ul className="space-y-1">
            {group.items.map((item) => {
              const active =
                item.to === '/docs'
                  ? pathname === '/docs'
                  : pathname === item.to || pathname.startsWith(item.to + '/')
              return (
                <li key={item.to}>
                  <Link
                    to={item.to}
                    className={cn(
                      'block rounded-md px-2.5 py-1.5 transition-colors',
                      active
                        ? 'bg-primary/10 font-medium text-primary'
                        : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                    )}
                  >
                    {item.label}
                  </Link>
                </li>
              )
            })}
          </ul>
        </div>
      ))}
      <div className="rounded-lg border border-border bg-card p-4">
        <p className="text-sm font-medium">Want a hand?</p>
        <p className="mt-1 text-xs text-muted-foreground">
          Open an issue on GitHub or drop us an email.
        </p>
        <div className="mt-3 flex flex-col gap-2 text-xs">
          <a
            href="https://github.com/steezrcom/openrow/issues"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1.5 text-foreground hover:text-primary"
          >
            <Github className="h-3 w-3" />
            File an issue
          </a>
          <a href="mailto:hello@steezr.com" className="text-foreground hover:text-primary">
            hello@steezr.com
          </a>
        </div>
      </div>
    </nav>
  )
}

// BottomNav renders prev/next sibling links so a reader can sweep through
// the docs sequentially. Skips connector deep links; those live on the
// connectors index page.
function BottomNav() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const flat = nav.flatMap((g) => g.items)
  const idx = flat.findIndex((i) => i.to === pathname)
  if (idx === -1) return null
  const prev = idx > 0 ? flat[idx - 1] : null
  const next = idx < flat.length - 1 ? flat[idx + 1] : null
  if (!prev && !next) return null
  return (
    <nav className="mt-16 flex items-center justify-between gap-4 border-t border-border pt-8 text-sm">
      <div>
        {prev && (
          <Link
            to={prev.to}
            className="group inline-flex flex-col rounded-md border border-border px-4 py-2 hover:border-primary/40"
          >
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground">Previous</span>
            <span className="mt-0.5 font-medium text-foreground group-hover:text-primary">
              ← {prev.label}
            </span>
          </Link>
        )}
      </div>
      <div className="text-right">
        {next && (
          <Link
            to={next.to}
            className="group inline-flex flex-col rounded-md border border-border px-4 py-2 hover:border-primary/40"
          >
            <span className="text-[10px] uppercase tracking-wider text-muted-foreground">Next</span>
            <span className="mt-0.5 font-medium text-foreground group-hover:text-primary">
              {next.label} →
            </span>
          </Link>
        )}
      </div>
    </nav>
  )
}

// DocPage is the standard shell for a docs page. Kicker is the small
// uppercase label, title is the H1, lede is the short summary paragraph.
export function DocPage({
  kicker,
  title,
  lede,
  children,
}: {
  kicker?: string
  title: string
  lede?: string
  children: React.ReactNode
}) {
  return (
    <article className="docs-prose">
      <header className="mb-10 border-b border-border pb-6">
        {kicker && (
          <p className="text-xs font-medium uppercase tracking-wider text-primary">{kicker}</p>
        )}
        <h1 className="mt-2 text-3xl font-semibold tracking-tight md:text-4xl">{title}</h1>
        {lede && <p className="mt-4 max-w-2xl text-base text-muted-foreground md:text-lg">{lede}</p>}
      </header>
      <div className="space-y-10 text-[15px] leading-relaxed text-foreground">{children}</div>
    </article>
  )
}

export function Section({
  id,
  title,
  children,
}: {
  id?: string
  title: string
  children: React.ReactNode
}) {
  return (
    <section id={id} className="scroll-mt-20">
      <h2 className="mb-4 text-2xl font-semibold tracking-tight">{title}</h2>
      <div className="space-y-4">{children}</div>
    </section>
  )
}

export function P({ children }: { children: React.ReactNode }) {
  return <p className="text-muted-foreground">{children}</p>
}

export function Callout({
  tone = 'default',
  title,
  children,
}: {
  tone?: 'default' | 'warn' | 'primary'
  title?: string
  children: React.ReactNode
}) {
  return (
    <div
      className={cn(
        'rounded-lg border px-4 py-3 text-sm',
        tone === 'warn' && 'border-amber-500/40 bg-amber-500/5 text-amber-900 dark:text-amber-200',
        tone === 'primary' && 'border-primary/40 bg-primary/5',
        tone === 'default' && 'border-border bg-muted/30',
      )}
    >
      {title && <p className="mb-1 font-semibold">{title}</p>}
      <div className="text-muted-foreground">{children}</div>
    </div>
  )
}

export function Steps({ children }: { children: React.ReactNode }) {
  return <ol className="docs-steps space-y-4">{children}</ol>
}

export function Step({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <li className="rounded-lg border border-border bg-card p-4 marker:hidden">
      <p className="font-medium text-foreground">{title}</p>
      {children && <div className="mt-2 text-sm text-muted-foreground">{children}</div>}
    </li>
  )
}

export function CodeBlock({ children }: { children: string }) {
  return (
    <pre className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-4 font-mono text-[12.5px] leading-relaxed text-foreground">
      <code>{children}</code>
    </pre>
  )
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[11px]">
      {children}
    </kbd>
  )
}
