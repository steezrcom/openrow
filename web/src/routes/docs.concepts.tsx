import { createFileRoute, Link } from '@tanstack/react-router'
import {
  BarChart3,
  Bot,
  Database,
  Hourglass,
  LayoutGrid,
  Plug,
  Rows,
  ShieldCheck,
  Workflow,
} from 'lucide-react'
import { DocPage, Section } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/concepts')({
  component: Concepts,
})

function Concepts() {
  return (
    <DocPage
      kicker="Concepts"
      title="The pieces of openrow"
      lede="Eight ideas that, taken together, explain everything else in the product. None of them are surprising — but the way they snap together is."
    >
      <Concept
        id="workspace"
        icon={ShieldCheck}
        title="Workspace"
      >
        <p>
          A workspace (tenant) is the boundary every other concept lives inside. Each workspace gets its own
          Postgres schema (<code>tenant_acme</code>), its own LLM config, its own connector credentials, its
          own members. Two workspaces in the same database can have entities with the same name and never
          collide.
        </p>
        <p>
          Roles inside a workspace: <strong>owner</strong> (billing, delete), <strong>admin</strong> (schema
          changes, connectors), <strong>member</strong> (read &amp; write rows), <strong>viewer</strong> (read only).
          The chat agent inherits the caller's role on each call — it can't bypass your permissions.
        </p>
      </Concept>

      <Concept id="entity" icon={Database} title="Entity">
        <p>
          An entity is a typed table. <em>"A project with name, client, status and a deadline"</em> becomes
          a real Postgres table <code>tenant_acme.projects</code> with four columns, the right indexes, a
          foreign-key to <code>clients</code>, and a default sort. Both sides — the table data and the
          metadata that describes it — are kept in sync transactionally; the agent edits both at once.
        </p>
        <p>
          Entities have <strong>fields</strong> (columns) of well-known kinds: text, number, money, date, datetime,
          boolean, select (enum), reference (FK), and a few formats on top (email, url, phone, ičo, dič).
          You can ask the agent to add, rename, or drop fields any time. Drops are confirmed; renames are safe.
        </p>
      </Concept>

      <Concept id="row" icon={Rows} title="Row">
        <p>
          The unit of data. Every row carries a UUID, a created_at, an updated_at, and whatever fields the entity
          declares. You can edit rows inline in the grid, in the side drawer, or via the chat. Filters, saved
          views, and sort orders are stored per-entity per-user.
        </p>
      </Concept>

      <Concept id="reports" icon={BarChart3} title="Dashboards & reports">
        <p>
          A <strong>report</strong> is a query spec — entity + filters + group-by + measure + chart type. A{' '}
          <strong>dashboard</strong> is a named collection of reports with a shared date range and grid layout.
          Six chart types: KPI, bar, line, area, pie, table. Date-range picker on top of every dashboard scopes
          every widget. Drag to reorder, click to drill in.
        </p>
        <p>
          You'll usually ask the agent to make a report (<em>"výnosy podle klienta za tento měsíc"</em>) and
          drop it on a dashboard. Editing the chart afterwards is point-and-click.
        </p>
      </Concept>

      <Concept id="flows" icon={Workflow} title="Flows">
        <p>
          A flow is automation that doesn't need you to be online. Each flow has a <strong>trigger</strong> (cron,
          row insert/update, webhook), a list of <strong>tools</strong> it's allowed to call, and a{' '}
          <strong>goal</strong> in plain English. The agent runs the goal, calling tools as needed, then stops.
        </p>
        <p>
          Three execution modes:
        </p>
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Dry-run.</strong> The agent plans every tool call but doesn't execute writes. The log shows what
            would have happened. Start here for everything new.
          </li>
          <li>
            <strong>Approve.</strong> Reads happen automatically; writes pause and wait for your tap. Each
            approval shows a diff of what's about to change. Lives at{' '}
            <Link to="/docs/how-tos" className="text-primary underline hover:no-underline">
              /app/approvals
            </Link>
            .
          </li>
          <li>
            <strong>Auto.</strong> Writes and reads both fire without you. Flip here when you've watched enough
            clean runs to trust the flow.
          </li>
        </ul>
      </Concept>

      <Concept id="approvals" icon={Hourglass} title="Approvals">
        <p>
          The bridge between dry-run and auto. Approvals queue every pending write from every flow in a single
          inbox. You see the row, the diff, the flow that wants to make the change, and the message the agent
          left to justify it. Approve, reject, or skip. Rejecting once usually means rewriting the flow's goal
          to be more specific.
        </p>
      </Concept>

      <Concept id="agent" icon={Bot} title="Chat agent">
        <p>
          The chat panel on the right of the workspace is the control plane for everything else: entities, rows,
          dashboards, flows, connectors, settings. Same tools a developer would call via the API; the agent picks
          which ones to use based on what you ask. The agent is a stateless turn taker — every message starts a
          fresh planning loop with the current workspace snapshot.
        </p>
        <p>
          It never has direct database access. It only has the tools we hand it, every one of them tenant-scoped.
          That means a bug in the agent can't, say, leak data across workspaces — the boundary is enforced one
          layer below the LLM.
        </p>
      </Concept>

      <Concept id="connectors" icon={Plug} title="Connectors">
        <p>
          A connector is openrow's way of talking to the world outside Postgres — banks, accounting tools, Slack,
          Notion, GitHub. Each is read- or write-scoped per action and stored encrypted at rest. Once installed,
          every action of a connector becomes a tool named{' '}
          <code>connector.&lt;id&gt;.&lt;action&gt;</code> available to the agent and to your flows.
        </p>
        <p>
          See the full list at <Link to="/docs/connectors" className="text-primary underline hover:no-underline">/docs/connectors</Link>.
        </p>
      </Concept>

      <Concept id="templates" icon={LayoutGrid} title="Templates">
        <p>
          A template is a code-defined starter pack: entities, fields, default rows, sample reports, and flows.
          The <strong>Agency</strong> template is the reference — twelve entities, thirteen flows, the whole
          back-office loop. Templates can only be installed into empty workspaces (or overlaid carefully via the
          agent).
        </p>
      </Concept>
    </DocPage>
  )
}

function Concept({
  id,
  icon: Icon,
  title,
  children,
}: {
  id: string
  icon: React.ComponentType<{ className?: string }>
  title: string
  children: React.ReactNode
}) {
  return (
    <Section id={id} title={title}>
      <div className="flex flex-col gap-4 md:flex-row">
        <div className="md:w-12 md:shrink-0">
          <span className="inline-flex h-10 w-10 items-center justify-center rounded-md bg-primary/15 text-primary">
            <Icon className="h-5 w-5" />
          </span>
        </div>
        <div className="space-y-3 text-muted-foreground md:flex-1">
          {children}
        </div>
      </div>
    </Section>
  )
}

