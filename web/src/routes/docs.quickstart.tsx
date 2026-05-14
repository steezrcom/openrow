import { createFileRoute, Link } from '@tanstack/react-router'
import { Coffee } from 'lucide-react'
import { Callout, CodeBlock, DocPage, Section, Step, Steps } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/quickstart')({
  component: Quickstart,
})

function Quickstart() {
  return (
    <DocPage
      kicker="Get started"
      title="Quickstart"
      lede="From zero to a working invoicing pipeline in about fifteen minutes. The path below is the one we walk every new agency through."
    >
      <Section id="sign-up" title="1. Make a workspace">
        <p className="text-muted-foreground">
          Either{' '}
          <Link to="/signup" className="text-primary underline hover:no-underline">
            sign up on openrow.app
          </Link>{' '}
          (we run the boxes) or follow the{' '}
          <Link to="/docs/self-hosting" className="text-primary underline hover:no-underline">
            self-host instructions
          </Link>{' '}
          to run it locally. Same UI either way.
        </p>
        <p className="text-muted-foreground">
          Pick a workspace name — this is also the Postgres schema your data lives in (e.g. <code>tenant_acme</code>),
          so use something short and ASCII.
        </p>
      </Section>

      <Section id="llm-key" title="2. Paste an LLM key">
        <p className="text-muted-foreground">
          openrow is BYOK — every workspace plugs in its own provider. The fastest path on the hosted plan
          is to grab an Anthropic API key:
        </p>
        <Steps>
          <Step title="Go to Settings → LLM">
            Open the workspace menu, pick Settings, then the LLM tab.
          </Step>
          <Step title="Pick a provider">
            Anthropic is the default and the most reliable for tool calling. OpenAI, Groq, Gemini, OpenRouter,
            Together, DeepSeek, Ollama, LM Studio, and a generic Custom row are also there.
          </Step>
          <Step title="Paste your key and fetch models">
            Click Fetch models to populate the dropdown with whatever you have access to. Pick one (Sonnet 4.6 is
            a safe default), then click Test connection.
          </Step>
        </Steps>
        <Callout tone="primary" title="Why bring your own key?">
          Cost lands in your provider account, not ours. You pick the model that fits your tolerance for
          latency, cost, and "the model is too eager to use tools". And the key never leaves your workspace —
          it's encrypted at rest with a per-deployment AES-256-GCM root key.
        </Callout>
      </Section>

      <Section id="template" title="3. Install the Agency template">
        <p className="text-muted-foreground">
          On the home screen of a fresh workspace, click <strong>Install Agency</strong>. You get twelve
          entities and thirteen flows wired end-to-end: clients, projects, sales and received invoices, bank
          transactions, the reconciliation flow, overdue reminders, a finance dashboard. The whole bookkeeping
          loop in a single click.
        </p>
        <p className="text-muted-foreground">
          You can also start empty and describe what you need. The template is the fastest way to see what the
          app feels like when it's full of real things.
        </p>
      </Section>

      <Section id="first-chat" title="4. Ask the agent for something useful">
        <p className="text-muted-foreground">
          Open the chat panel (right side of the workspace). Try one of these:
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Prompt>Add a field <strong>delivery_address</strong> to clients.</Prompt>
          <Prompt>Make a contacts entity with name, email, IČO, and a flag for VAT payer.</Prompt>
          <Prompt>Show me overdue invoices, grouped by client, this month.</Prompt>
          <Prompt>Email me a Slack digest at 8am every Monday with received invoices waiting for approval.</Prompt>
        </div>
        <p className="text-sm text-muted-foreground">
          The agent uses real tools — <code>create_entity</code>, <code>add_row</code>, <code>create_report</code>,
          <code>create_flow</code>, plus every action of every connector you have configured.
          See <Link to="/docs/concepts" className="text-primary underline hover:no-underline">Core concepts</Link>{' '}
          for what's actually happening under the hood.
        </p>
      </Section>

      <Section id="connect" title="5. Plug in a connector">
        <p className="text-muted-foreground">
          The agency template assumes you'll wire up at least one bank and one invoicing tool. The shortest path:
        </p>
        <Steps>
          <Step title="Pick your bank">
            <CLink id="fio">Fio</CLink> is the fastest to set up (one API token, no OAuth). Czech alternatives:{' '}
            <CLink id="csas">Česká spořitelna</CLink>, <CLink id="csob">ČSOB</CLink>,{' '}
            <CLink id="revolut">Revolut Business</CLink>.
          </Step>
          <Step title="Pick your invoicing tool">
            <CLink id="fakturoid">Fakturoid</CLink>, <CLink id="uol">ÚOL</CLink>, or{' '}
            <CLink id="flexi">ABRA Flexi</CLink>.
          </Step>
          <Step title="(Optional) Pick a notifier">
            <CLink id="slack">Slack</CLink> for internal team pings,{' '}
            <CLink id="resend">Resend</CLink> for transactional email to clients.
          </Step>
        </Steps>
        <p className="text-muted-foreground">
          Every connector page walks you through the exact clicks on the provider's side.
        </p>
      </Section>

      <Section id="next" title="What now?">
        <div className="rounded-xl border border-border bg-card p-6">
          <div className="flex items-start gap-4">
            <Coffee className="mt-1 h-5 w-5 shrink-0 text-primary" />
            <div>
              <p className="font-medium">Make a coffee and watch a day's worth of flows run.</p>
              <p className="mt-2 text-sm text-muted-foreground">
                The agency template's flows default to <strong>approve</strong> mode — they queue every write for your
                review before it lands. Watch ten or so runs, sanity-check the diffs, then flip to <strong>auto</strong>{' '}
                when you trust it.
              </p>
            </div>
          </div>
        </div>

        <CodeBlock>{`# self-hosters: tail the server logs while you click around
docker compose logs -f app

# or, in the dev tree:
make dev`}</CodeBlock>
      </Section>
    </DocPage>
  )
}

function Prompt({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 font-mono text-sm">
      <span className="mr-2 text-muted-foreground">›</span>
      {children}
    </div>
  )
}

function CLink({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <Link
      to="/docs/connectors/$id"
      params={{ id }}
      className="text-primary underline hover:no-underline"
    >
      {children}
    </Link>
  )
}
