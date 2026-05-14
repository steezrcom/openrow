import { createFileRoute, Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpenText,
  Bot,
  Compass,
  KeyRound,
  Plug,
  ServerCog,
  ShieldCheck,
  Wrench,
} from 'lucide-react'
import { DocPage } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/')({
  component: DocsIndex,
})

const cards = [
  {
    to: '/docs/quickstart',
    icon: Compass,
    title: 'Quickstart',
    text: 'Sign up, install the Agency template, paste an LLM key, and have a working invoicing pipeline before lunch.',
  },
  {
    to: '/docs/concepts',
    icon: BookOpenText,
    title: 'Core concepts',
    text: 'Workspaces, entities, rows, dashboards, flows, approvals, and the agent. Once these click, everything else is a five-minute task.',
  },
  {
    to: '/docs/how-tos',
    icon: Wrench,
    title: 'How-tos',
    text: 'Create your first entity, build a dashboard, write a flow, share the workspace with teammates.',
  },
  {
    to: '/docs/llm',
    icon: KeyRound,
    title: 'LLM setup',
    text: 'Anthropic, OpenAI, Groq, Gemini, OpenRouter, Ollama, LM Studio. Pick a provider, paste a key, fetch models.',
  },
  {
    to: '/docs/connectors',
    icon: Plug,
    title: 'Connectors',
    text: 'Sixteen connectors: ČS, ČSOB, Fio, Revolut, Fakturoid, UOL, Flexi, Slack, Notion, GitHub, and more. Step-by-step credential recipes.',
  },
  {
    to: '/docs/self-hosting',
    icon: ServerCog,
    title: 'Self-hosting',
    text: 'One Go binary plus Postgres, one Docker compose file. Your data on your boxes.',
  },
  {
    to: '/docs/security',
    icon: ShieldCheck,
    title: 'Security & privacy',
    text: 'AES-256-GCM secret encryption, schema-per-tenant isolation, BYOK for the LLM. The boring details, in plain English.',
  },
]

function DocsIndex() {
  return (
    <DocPage
      kicker="Docs"
      title="Run a small back office without copy-pasting between five apps."
      lede="openrow turns plain-English descriptions into real Postgres tables, dashboards, and automations. These docs walk you from sign-up to a running invoicing pipeline."
    >
      <div className="grid gap-3 sm:grid-cols-2">
        {cards.map((c) => (
          <Link
            key={c.to}
            to={c.to}
            className="group flex flex-col rounded-xl border border-border bg-card p-5 transition-colors hover:border-primary/40"
          >
            <div className="mb-3 flex items-center gap-3">
              <span className="inline-flex h-9 w-9 items-center justify-center rounded-md bg-primary/15 text-primary">
                <c.icon className="h-4 w-4" />
              </span>
              <p className="font-semibold tracking-tight">{c.title}</p>
            </div>
            <p className="text-sm leading-relaxed text-muted-foreground">{c.text}</p>
            <span className="mt-4 inline-flex items-center gap-1 text-xs font-medium text-primary opacity-0 transition-opacity group-hover:opacity-100">
              Read
              <ArrowRight className="h-3 w-3" />
            </span>
          </Link>
        ))}
      </div>

      <div className="rounded-2xl border border-primary/30 bg-primary/5 p-6 md:p-8">
        <div className="flex flex-col gap-6 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-primary">Try it</p>
            <p className="mt-2 max-w-xl text-lg font-semibold leading-snug">
              Spin up a workspace, install the Agency template, and ask the chat to build something for you.
            </p>
            <p className="mt-2 max-w-xl text-sm text-muted-foreground">
              Free on openrow.app, or clone the repo and run a single binary on your own box.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Link
              to="/signup"
              className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Start a workspace
              <ArrowRight className="h-4 w-4" />
            </Link>
            <a
              href="https://github.com/steezrcom/openrow"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium hover:bg-accent"
            >
              <Bot className="h-4 w-4" />
              GitHub
            </a>
          </div>
        </div>
      </div>
    </DocPage>
  )
}
