import { createFileRoute } from '@tanstack/react-router'
import { Callout, CodeBlock, DocPage, Section } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/self-hosting')({
  component: SelfHosting,
})

function SelfHosting() {
  return (
    <DocPage
      kicker="Run it yourself"
      title="Self-hosting"
      lede="One Go binary plus Postgres, served from a single container. AGPL-3.0, no telemetry, no calls home."
    >
      <Section id="docker" title="Docker, all-in-one">
        <p className="text-muted-foreground">
          The fastest path. Both Postgres and the openrow app run in compose, the SPA is embedded in the same
          binary that serves the API.
        </p>
        <CodeBlock>{`git clone https://github.com/openrow/openrow
cd openrow
cp .env.example .env

# Generate a root key for encrypting API keys + connector secrets
echo "OPENROW_SECRET_KEY=$(openssl rand -base64 32)" >> .env

docker compose -f docker-compose.yml -f docker-compose.app.yml up -d
open http://localhost:8080`}</CodeBlock>
        <p className="text-muted-foreground">
          That's it. Sign up at <code>http://localhost:8080/signup</code>, create a workspace, paste an LLM key
          in Settings → LLM, and you're running.
        </p>
      </Section>

      <Section id="dev" title="Local dev setup">
        <p className="text-muted-foreground">
          If you want to hack on openrow, the dev tree gives you HMR for both backend (via{' '}
          <a href="https://github.com/air-verse/air" target="_blank" rel="noreferrer" className="text-primary underline hover:no-underline">air</a>)
          and frontend (via Vite):
        </p>
        <CodeBlock>{`make db-up      # Postgres in Docker
make seed       # demo@openrow.local / openrow123, agency template pre-installed
make dev        # Go (air) + Vite together; Ctrl+C stops both`}</CodeBlock>
      </Section>

      <Section id="env" title="Environment variables">
        <p className="text-muted-foreground">
          Required:
        </p>
        <EnvTable
          rows={[
            {
              name: 'DATABASE_URL',
              required: true,
              description: 'Postgres connection string. Example: postgres://openrow:openrow@localhost:5432/openrow?sslmode=disable',
            },
            {
              name: 'OPENROW_SECRET_KEY',
              required: true,
              description: 'Base64-encoded 32-byte key. Used to encrypt LLM keys and connector secrets at rest. Generate with openssl rand -base64 32. Lose it and stored secrets become unreadable.',
            },
          ]}
        />
        <p className="mt-6 text-muted-foreground">Optional:</p>
        <EnvTable
          rows={[
            { name: 'ANTHROPIC_API_KEY', description: 'Fallback LLM credentials for workspaces with no per-tenant config. Convenient for dev; in prod, each workspace sets its own.' },
            { name: 'HTTP_ADDR', description: 'Listen address (default: :8080).' },
            { name: 'APP_URL', description: 'Public origin used in email links (password reset, invites). Default http://localhost:5173.' },
            { name: 'SECURE_COOKIES', description: 'Set true behind HTTPS so session cookies get the Secure attribute.' },
            { name: 'SPA_DIR', description: 'Path to the built SPA (web/dist). Setting this serves the React app from the Go binary.' },
            { name: 'LOG_LEVEL', description: 'debug, info, warn, error. Default info.' },
            { name: 'SMTP_*', description: 'SMTP host, port, user, password, from. Required to actually send password-reset and invite emails. Without these, emails are printed to stdout.' },
          ]}
        />
      </Section>

      <Section id="proxy" title="Behind a reverse proxy">
        <p className="text-muted-foreground">
          Standard setup: terminate TLS at the proxy (Caddy, nginx, Traefik, Cloudflare), forward HTTP to{' '}
          <code>HTTP_ADDR</code>. Set <code>SECURE_COOKIES=true</code> and <code>APP_URL=https://yourdomain</code>{' '}
          so emails carry the right links.
        </p>
        <CodeBlock>{`# Caddyfile
yourdomain.com {
    reverse_proxy openrow:8080
}`}</CodeBlock>
        <p className="text-muted-foreground">
          openrow trusts whatever host header the proxy sets. If you're behind a Cloudflare Tunnel or similar,
          configure the upstream host so links rendered in emails match the public origin.
        </p>
      </Section>

      <Section id="postgres" title="Postgres notes">
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Version.</strong> Postgres 16 is the target. 15 should work; older may not.
          </li>
          <li>
            <strong>Schema-per-tenant.</strong> Each workspace gets its own Postgres schema (<code>tenant_acme</code>).
            System tables live in the <code>openrow</code> schema. Migrations are embedded in the binary and run on
            boot — no manual migration step.
          </li>
          <li>
            <strong>Backups.</strong> <code>pg_dump</code> on the whole database is the simplest. For per-tenant
            exports, <code>pg_dump --schema=openrow --schema=tenant_acme</code>.
          </li>
          <li>
            <strong>Connection pool.</strong> Default pool size scales with database connections; if you run
            many workspaces, raise Postgres' <code>max_connections</code> first.
          </li>
        </ul>
      </Section>

      <Section id="upgrades" title="Upgrades">
        <p className="text-muted-foreground">
          openrow rolls forward. Pull the new image, recreate the container; migrations run automatically on boot.
          Migrations are additive by policy — adding a new column nullable, backfilling, then tightening in a later
          deploy — so the previous binary keeps working while the new one rolls out.
        </p>
        <Callout tone="warn" title="Back up before upgrading.">
          The standard precaution: <code>pg_dump</code> before any non-trivial upgrade. Restore on a copy first
          if you have time, especially when crossing a minor version line.
        </Callout>
      </Section>

      <Section id="license" title="A note on AGPL-3.0">
        <p className="text-muted-foreground">
          openrow is licensed AGPL-3.0. In plain English: you can self-host, fork, modify, and redistribute. If you
          run a modified version <em>as a service</em> for other people, you must release your modifications under
          the same license. Internal use inside your own company doesn't trigger this.
        </p>
      </Section>
    </DocPage>
  )
}

function EnvTable({ rows }: { rows: { name: string; required?: boolean; description: string }[] }) {
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted/30 text-xs uppercase tracking-wider text-muted-foreground">
          <tr>
            <th className="px-4 py-2 text-left font-medium">Variable</th>
            <th className="px-4 py-2 text-left font-medium">Description</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {rows.map((r) => (
            <tr key={r.name} className="align-top">
              <td className="whitespace-nowrap px-4 py-3 font-mono text-[12.5px] font-medium">
                {r.name}
                {r.required && (
                  <span className="ml-2 rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-amber-700 dark:text-amber-300">
                    Required
                  </span>
                )}
              </td>
              <td className="px-4 py-3 text-muted-foreground">{r.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
