import { createFileRoute, Link } from '@tanstack/react-router'
import { Callout, DocPage, Section, Step, Steps } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/how-tos')({
  component: HowTos,
})

function HowTos() {
  return (
    <DocPage
      kicker="Doing things"
      title="How-tos"
      lede="Short, opinionated walkthroughs for the things people ask us about most. Every one of them can also be done by asking the chat agent in plain English — these are the click paths if you prefer."
    >
      <Section id="entity" title="Create an entity">
        <p className="text-muted-foreground">
          The fastest path is the chat panel. Type{' '}
          <em>"a contact with name, email, IČO, and a flag for VAT payer"</em>. The agent shows you the proposed
          schema, you click <strong>Create</strong>, and the table, the form, and the list view all exist.
        </p>
        <p className="text-muted-foreground">
          To do it by hand: open <code>/app</code>, click <strong>+ New entity</strong> in the sidebar, type a name,
          then add fields one at a time. Each field has a kind (text, number, date, money, boolean, select,
          reference) and an optional format constraint (email, IČO, URL).
        </p>
        <Callout tone="primary" title="Display names vs. code names">
          Display names are Czech by default (e.g. <em>Faktury</em>). The code identifier — used in URLs and SQL —
          stays English (<code>invoices</code>). You can't change a code name after creation; the chat agent
          warns you before it picks one.
        </Callout>
      </Section>

      <Section id="dashboard" title="Build a dashboard">
        <Steps>
          <Step title="Open /app/dashboards/<slug> or click + Dashboard">
            Every workspace has a default <code>finance</code> dashboard if you installed the Agency template.
          </Step>
          <Step title="Add a report">
            Click + Report. Pick the entity, pick a measure (count, sum, average), optionally a group-by
            field and a stack. Then pick a chart type: KPI for a single number, bar for grouped totals,
            line for time series.
          </Step>
          <Step title="Filter">
            Date-range picker at the top of the dashboard scopes every widget at once. Per-report filters
            (status = 'overdue', client = …) chain on top.
          </Step>
          <Step title="Reorder">
            Drag tiles to rearrange. Click a tile's ⋯ to resize, duplicate, or delete it.
          </Step>
        </Steps>
        <p className="text-muted-foreground">
          For period-over-period deltas (<em>"this month vs. last month"</em>), set a comparison range on the
          report — the KPI card shows the delta automatically.
        </p>
      </Section>

      <Section id="flow" title="Write a flow">
        <p className="text-muted-foreground">
          A flow is an automation with a goal, a trigger, and a list of tools. The fastest way to draft one is to
          tell the chat what should happen in plain English. The agent picks the trigger and a reasonable tool
          set, you approve.
        </p>
        <Steps>
          <Step title="Pick the trigger">
            Cron (e.g. every weekday at 06:00), row event (insert / update on entity X), or webhook (with an
            optional signing secret from a connector like GitHub or Stripe).
          </Step>
          <Step title="Write the goal">
            One paragraph. Be specific about success ("send exactly one Slack message per invoice" beats "notify
            about invoices"). The agent uses this to plan tool calls.
          </Step>
          <Step title="Pick the tools">
            Default is "all entity tools + every action of every enabled connector". Narrow this for safety:
            a flow that emails clients doesn't need <code>drop_entity</code>.
          </Step>
          <Step title="Run a dry-run">
            Click Run. The agent plans and reports — no writes happen. The run log shows every tool call it would
            have made, with arguments.
          </Step>
          <Step title="Promote to approve, then auto">
            Approve mode pauses on every write. Watch a few real runs, sanity-check, then flip to auto.
          </Step>
        </Steps>
        <Callout tone="warn" title="Tool calls cost LLM tokens">
          Every flow run is a fresh LLM session. If a flow runs every five minutes and the goal is rich, the bill
          adds up. Use specific tool allowlists and tight goals to keep token counts down.
        </Callout>
      </Section>

      <Section id="agent" title="Talk to the agent productively">
        <p className="text-muted-foreground">
          The agent is good at structured tasks (schema, data edits, report creation) and less reliable at open-ended
          analysis. Some patterns that work:
        </p>
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Be concrete about names.</strong> "Add a field <code>delivery_address</code> of kind text to{' '}
            <code>clients</code>" beats "let clients have an address".
          </li>
          <li>
            <strong>Reference entities by their code name.</strong> If the chat keeps grabbing the wrong
            table, use the snake_case name (<code>invoice_items</code>) instead of the Czech display name.
          </li>
          <li>
            <strong>Confirm before destructive actions.</strong> If the agent says it'll drop a column, ask it
            to dry-run first or to back up the column to a new one before dropping.
          </li>
          <li>
            <strong>Ask for a flow's plan before running.</strong> "What would you do?" returns a tool-call plan
            without execution, even outside dry-run.
          </li>
        </ul>
      </Section>

      <Section id="invite" title="Invite teammates">
        <p className="text-muted-foreground">
          Open Settings → Members. Add an email and a role:
        </p>
        <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
          <li><strong>Owner</strong> — full control, including billing and workspace deletion.</li>
          <li><strong>Admin</strong> — schema changes, connector config, templates.</li>
          <li><strong>Member</strong> — read &amp; write rows; run flows; talk to the agent.</li>
          <li><strong>Viewer</strong> — read-only.</li>
        </ul>
        <p className="text-muted-foreground">
          The invitation email lands via the configured mailer (SMTP) — or, in self-host dev, gets printed to
          stdout if no SMTP is wired up.
        </p>
      </Section>

      <Section id="entity-views" title="Save filtered views of an entity">
        <p className="text-muted-foreground">
          On any entity grid, set a filter (status = overdue) and a sort. Click <strong>Save view</strong>, give
          it a name. Views live in the left rail and are per-user; pin one to make it the default for everyone
          via Settings → Entity views.
        </p>
      </Section>

      <Section id="export" title="Export your data">
        <p className="text-muted-foreground">
          openrow doesn't lock data in. Every entity has an <strong>Export CSV</strong> button. For Postgres-level
          access (self-host or hosted with bring-your-own-database), every workspace is its own schema —{' '}
          <code>pg_dump --schema=tenant_acme</code> gets you everything.
        </p>
      </Section>

      <Section id="more" title="More">
        <p className="text-muted-foreground">
          Stuck? <Link to="/docs" className="text-primary underline hover:no-underline">Back to the docs index</Link>{' '}
          for the rest of the topics, or open an issue on GitHub.
        </p>
      </Section>
    </DocPage>
  )
}
