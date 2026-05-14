import { createFileRoute, Link } from '@tanstack/react-router'
import {
  BookText,
  Boxes,
  CreditCard,
  Database,
  Github,
  Globe2,
  Landmark,
  Mail,
  MessageSquare,
  ScanSearch,
} from 'lucide-react'
import { DocPage, Section } from '@/components/DocsLayout'
import { docsConnectors, type DocsConnector } from '@/content/docs/connectors'

export const Route = createFileRoute('/docs/connectors/')({
  component: ConnectorsIndex,
})

type CategoryKey =
  | 'banking'
  | 'billing'
  | 'erp'
  | 'payments'
  | 'chat'
  | 'email'
  | 'docs'
  | 'dev'
  | 'registry'
  | 'reference'

const categories: { key: CategoryKey; label: string; icon: React.ComponentType<{ className?: string }>; blurb: string }[] = [
  {
    key: 'banking',
    label: 'Banking',
    icon: Landmark,
    blurb: 'Read your own bank accounts. Four Czech-flavoured options.',
  },
  {
    key: 'billing',
    label: 'Billing & invoicing',
    icon: BookText,
    blurb: 'Push and pull invoices to and from your accounting tool.',
  },
  {
    key: 'erp',
    label: 'ERP',
    icon: Boxes,
    blurb: 'Heavier accounting suites. AI-native bindings map their schemas.',
  },
  {
    key: 'payments',
    label: 'Payments',
    icon: CreditCard,
    blurb: 'Card payments and subscriptions.',
  },
  {
    key: 'chat',
    label: 'Chat',
    icon: MessageSquare,
    blurb: 'Post messages to Slack and Discord.',
  },
  {
    key: 'email',
    label: 'Email',
    icon: Mail,
    blurb: 'Transactional email for invoices, reminders, password resets.',
  },
  {
    key: 'docs',
    label: 'Docs & knowledge',
    icon: Database,
    blurb: 'Mirror data into shared knowledge bases.',
  },
  {
    key: 'dev',
    label: 'Developer tools',
    icon: Github,
    blurb: 'Open and update issues; receive webhooks.',
  },
  {
    key: 'registry',
    label: 'Public registries',
    icon: ScanSearch,
    blurb: 'Czech business registry, EU VAT validation. No setup needed.',
  },
  {
    key: 'reference',
    label: 'Reference data',
    icon: Globe2,
    blurb: 'ČNB exchange rates. No setup needed.',
  },
]

function ConnectorsIndex() {
  const grouped = new Map<CategoryKey, DocsConnector[]>()
  for (const c of docsConnectors) {
    const arr = grouped.get(c.category as CategoryKey) ?? []
    arr.push(c)
    grouped.set(c.category as CategoryKey, arr)
  }
  return (
    <DocPage
      kicker={`${docsConnectors.length} integrations`}
      title="Connectors"
      lede="Banks, accounting, chat, email, docs, dev tools. Every connector is read- or write-scoped per action, surfaced to the agent as a typed tool, and stored encrypted at rest."
    >
      <Section id="categories" title="By category">
        <div className="space-y-8">
          {categories.map((cat) => {
            const items = grouped.get(cat.key) ?? []
            if (items.length === 0) return null
            return (
              <div key={cat.key} className="space-y-3">
                <div className="flex items-end justify-between gap-4 border-b border-border pb-2">
                  <div className="flex items-center gap-3">
                    <span className="inline-flex h-8 w-8 items-center justify-center rounded-md bg-primary/15 text-primary">
                      <cat.icon className="h-4 w-4" />
                    </span>
                    <div>
                      <p className="text-base font-semibold">{cat.label}</p>
                      <p className="text-xs text-muted-foreground">{cat.blurb}</p>
                    </div>
                  </div>
                  <span className="text-xs text-muted-foreground">{items.length}</span>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  {items.map((c) => (
                    <Link
                      key={c.id}
                      to="/docs/connectors/$id"
                      params={{ id: c.id }}
                      className="group flex flex-col rounded-lg border border-border bg-card p-4 transition-colors hover:border-primary/40"
                    >
                      <div className="flex items-center gap-3">
                        <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-sm font-semibold text-primary">
                          {initials(c.name)}
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <p className="truncate font-medium group-hover:text-primary">{c.name}</p>
                            {c.status === 'coming_soon' && (
                              <span className="rounded border border-amber-500/40 bg-amber-500/5 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-amber-700 dark:text-amber-300">
                                Soon
                              </span>
                            )}
                          </div>
                          <p className="truncate text-xs text-muted-foreground">{c.short}</p>
                        </div>
                      </div>
                    </Link>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </Section>

      <Section id="how-they-work" title="How connectors work">
        <p className="text-muted-foreground">
          Installing a connector means three things:
        </p>
        <ol className="list-decimal space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Credentials.</strong> You paste them into Settings → Connectors → &lt;name&gt;. They're
            encrypted at rest with the workspace's AES-256-GCM root key before they hit Postgres.
          </li>
          <li>
            <strong>Actions become tools.</strong> Every action of an installed connector shows up as a tool
            named <code>connector.&lt;id&gt;.&lt;action&gt;</code>. The chat agent and flows can call it,
            scoped to the workspace.
          </li>
          <li>
            <strong>Webhooks, optional.</strong> If a connector supports webhook authentication (GitHub, Stripe),
            you can wire a flow's trigger to it. The signature is verified before the flow runs.
          </li>
        </ol>
        <p className="text-sm text-muted-foreground">
          A connector goes through two states: <strong>Available</strong> (fully wired up, ready to install) and{' '}
          <strong>Coming soon</strong> (descriptor only; the install screen is disabled). New connectors are
          accepted via PR — see <a href="https://github.com/steezrcom/openrow/blob/main/CONTRIBUTING.md" target="_blank" rel="noreferrer" className="text-primary underline hover:no-underline">CONTRIBUTING.md</a>.
        </p>
      </Section>

      <Section id="missing" title="My provider isn't here">
        <p className="text-muted-foreground">
          Three options, in increasing order of effort:
        </p>
        <ol className="list-decimal space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Webhook trigger.</strong> If the provider can POST to you, point its webhook at a flow's URL.
            You don't need a connector to receive events — you just need the URL.
          </li>
          <li>
            <strong>HTTP step in a flow.</strong> Flows have a generic HTTP step. Configure auth headers from a
            workspace secret and you're calling any REST API.
          </li>
          <li>
            <strong>Write a connector.</strong> One Go file plus a line in the catalog. The framework handles
            credential storage, encryption, the install UI, and exposing actions to the agent. See the existing
            connectors in <code>internal/connectors/catalog/</code> for reference.
          </li>
        </ol>
      </Section>
    </DocPage>
  )
}

function initials(name: string): string {
  const cleaned = name.replace(/[^\p{L}\s]/gu, '').trim()
  const parts = cleaned.split(/\s+/).filter(Boolean)
  if (parts.length === 0) return name.charAt(0).toUpperCase()
  if (parts.length === 1) return parts[0].slice(0, 1).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}
