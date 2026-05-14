import { Link } from '@tanstack/react-router'
import { ArrowLeft, Clock, ExternalLink, Lock } from 'lucide-react'
import { DocPage, Section, Callout } from '@/components/DocsLayout'
import type { DocsConnector } from '@/content/docs/connectors'

export function DocsConnectorPage({ c }: { c: DocsConnector }) {
  return (
    <DocPage
      kicker={categoryLabel(c.category)}
      title={c.name}
      lede={c.intro}
    >
      <div className="-mt-6 flex flex-wrap items-center gap-2">
        <Link
          to="/docs/connectors"
          className="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-3 w-3" />
          All connectors
        </Link>
        <a
          href={c.homepage}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <ExternalLink className="h-3 w-3" />
          {c.homepage.replace(/^https?:\/\//, '')}
        </a>
        {c.status === 'coming_soon' && (
          <span className="inline-flex items-center gap-1 rounded-md border border-amber-500/40 bg-amber-500/10 px-2.5 py-1 text-xs text-amber-700 dark:text-amber-300">
            <Clock className="h-3 w-3" />
            Coming soon
          </span>
        )}
      </div>

      <Section id="setup" title="Setting it up">
        <ol className="space-y-3">
          {c.steps.map((s, i) => (
            <li key={i} className="flex gap-4 rounded-lg border border-border bg-card p-4">
              <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-medium text-primary">
                {i + 1}
              </span>
              <p className="text-sm leading-relaxed text-muted-foreground">{s}</p>
            </li>
          ))}
        </ol>
      </Section>

      {c.fields.length > 0 && (
        <Section id="credentials" title="Credentials">
          <p className="text-sm text-muted-foreground">
            Every field below is stored encrypted at rest. Secret fields are masked in the UI after save
            and never returned in plaintext over the API.
          </p>
          <div className="overflow-hidden rounded-lg border border-border">
            <table className="w-full text-sm">
              <thead className="bg-muted/30 text-xs uppercase tracking-wider text-muted-foreground">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">Field</th>
                  <th className="px-4 py-2 text-left font-medium">Required</th>
                  <th className="px-4 py-2 text-left font-medium">What to paste</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {c.fields.map((f) => (
                  <tr key={f.label} className="align-top">
                    <td className="whitespace-nowrap px-4 py-3 font-medium">
                      <div className="flex items-center gap-1.5">
                        {f.label}
                        {f.secret && <Lock className="h-3 w-3 text-muted-foreground" />}
                      </div>
                      {f.placeholder && (
                        <p className="mt-1 font-mono text-[11px] font-normal text-muted-foreground">
                          {f.placeholder}
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {f.required ? 'Required' : 'Optional'}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">{f.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Section>
      )}

      {c.actions.length > 0 && (
        <Section id="actions" title="What the agent can do">
          <p className="text-sm text-muted-foreground">
            Once configured, the agent (and any flow you write) gets these tools:
          </p>
          <div className="grid gap-3 md:grid-cols-2">
            {c.actions.map((a) => (
              <div key={a.name} className="rounded-lg border border-border bg-card p-4">
                <div className="flex items-center gap-2">
                  <p className="font-medium">{a.name}</p>
                  {a.mutates && (
                    <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-amber-700 dark:text-amber-300">
                      Writes
                    </span>
                  )}
                </div>
                <p className="mt-1 text-sm text-muted-foreground">{a.description}</p>
              </div>
            ))}
          </div>
        </Section>
      )}

      {c.webhook && (
        <Section id="webhooks" title="Webhooks">
          <Callout tone="default">{c.webhook}</Callout>
        </Section>
      )}

      {c.example && (
        <Section id="try-it" title="Try it">
          <p className="text-sm text-muted-foreground">
            Once configured, paste this into the chat panel — the agent will pick the right tool.
          </p>
          <div className="rounded-lg border border-border bg-muted/30 p-4 font-mono text-sm">
            <span className="mr-2 text-muted-foreground">›</span>
            {c.example}
          </div>
        </Section>
      )}
    </DocPage>
  )
}

function categoryLabel(c: string): string {
  switch (c) {
    case 'banking':
      return 'Banking'
    case 'billing':
      return 'Billing & invoicing'
    case 'erp':
      return 'ERP'
    case 'payments':
      return 'Payments'
    case 'chat':
      return 'Chat'
    case 'email':
      return 'Email'
    case 'docs':
      return 'Docs & knowledge'
    case 'dev':
      return 'Developer tools'
    case 'registry':
      return 'Public registry'
    case 'reference':
      return 'Reference data'
    default:
      return c
  }
}
