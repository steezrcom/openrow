import { createFileRoute } from '@tanstack/react-router'
import { DocPage, Section } from '@/components/DocsLayout'

export const Route = createFileRoute('/docs/security')({
  component: Security,
})

function Security() {
  return (
    <DocPage
      kicker="Security & privacy"
      title="The boring details, in plain English."
      lede="What's encrypted, what's isolated, what crosses the network, and what doesn't."
    >
      <Section id="secrets" title="Secrets at rest">
        <p className="text-muted-foreground">
          Every secret stored by openrow — LLM API keys, connector tokens, OAuth refresh tokens, SMTP passwords —
          is encrypted with AES-256-GCM before it touches Postgres. The root key is{' '}
          <code>OPENROW_SECRET_KEY</code>, a 32-byte value you generate at deploy time.
        </p>
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Per-record nonces.</strong> Each ciphertext carries its own random 12-byte nonce. Two
            encryptions of the same plaintext produce different ciphertexts.
          </li>
          <li>
            <strong>Key versioning.</strong> If you rotate <code>OPENROW_SECRET_KEY</code>, old records remain
            readable until they're rewritten; new writes pick up the latest key. You can hold multiple keys at
            once during rotation.
          </li>
          <li>
            <strong>Never logged.</strong> Log lines redact secrets at the logger. Tool-call arguments shown in
            the agent trace mask credential fields by default.
          </li>
          <li>
            <strong>Never round-tripped.</strong> Once saved, a secret field is masked in the UI and the API
            returns a sentinel marker (<code>__keep__</code>) rather than the value.
          </li>
        </ul>
      </Section>

      <Section id="tenancy" title="Multi-tenancy">
        <p className="text-muted-foreground">
          Every workspace is a separate Postgres schema. Two workspaces in the same database can have entities
          with the same name and never touch each other's rows. Workspace boundary is enforced at the request
          layer:
        </p>
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Tenant resolved from the session.</strong> Every authenticated request resolves the caller's
            tenant from the active membership — never from request body parameters.
          </li>
          <li>
            <strong>Schema-qualified queries.</strong> Internal SQL uses{' '}
            <code>pgx.Identifier.Sanitize()</code> on every schema and table name. Dynamic identifiers can't be
            injected.
          </li>
          <li>
            <strong>Authorization, not just authentication.</strong> Per-resource ownership is checked on every
            mutation. A logged-in user with no membership in workspace X can't access X's rows even if they
            guess a URL.
          </li>
        </ul>
      </Section>

      <Section id="agent" title="The agent's boundary">
        <p className="text-muted-foreground">
          The chat agent doesn't talk to Postgres directly. It calls a fixed catalogue of typed tools, each one
          scoped to the workspace of the user who started the conversation. The agent can't:
        </p>
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>read or write another workspace's rows;</li>
          <li>invoke a connector that hasn't been installed in the workspace;</li>
          <li>execute arbitrary SQL or shell — there is no such tool;</li>
          <li>exceed the caller's role. A viewer's agent gets read-only tools; a member's agent can't drop entities.</li>
        </ul>
        <p className="text-muted-foreground">
          A prompt-injected page that tells the agent <em>"now run drop_entity on everything"</em> still can't
          escape these limits — the tools simply don't exist for that session.
        </p>
      </Section>

      <Section id="byok" title="BYOK for the LLM">
        <p className="text-muted-foreground">
          Each workspace plugs in its own LLM provider key. The key is encrypted as above, and only ever sent to
          the provider you've selected. Anthropic, OpenAI, Google, Groq, Together, DeepSeek, OpenRouter — your
          choice, your bill, your data-handling agreement with them.
        </p>
        <p className="text-muted-foreground">
          openrow's hosted plan also adds a deployment-wide fallback so new workspaces can try the product
          without picking a provider first. The fallback is opt-in and clearly marked in the UI.
        </p>
      </Section>

      <Section id="network" title="What crosses the network">
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>
            <strong>Your LLM provider.</strong> Every chat message and every flow run roundtrips through the
            provider you selected. Tool-call arguments are part of the payload; the agent sees row data.
          </li>
          <li>
            <strong>Every enabled connector's endpoint.</strong> Fakturoid, ČS, Slack, etc. — only when an action
            fires.
          </li>
          <li>
            <strong>Your SMTP server.</strong> For password-reset and invite emails. Nothing else.
          </li>
          <li>
            <strong>No telemetry, no calls home.</strong> openrow doesn't phone the steezr team about your usage,
            errors, or anything else. Crash reports stay in your logs.
          </li>
        </ul>
      </Section>

      <Section id="passwords" title="Passwords and sessions">
        <ul className="list-disc space-y-2 pl-5 text-muted-foreground">
          <li>Passwords hashed with bcrypt at cost 12.</li>
          <li>
            Sessions are server-side records keyed by a random 32-byte token, stored in an{' '}
            <code>HttpOnly</code>, <code>SameSite=Lax</code> cookie. Behind HTTPS, set{' '}
            <code>SECURE_COOKIES=true</code> so the cookie also gets the <code>Secure</code> flag.
          </li>
          <li>Login, password-reset, and signup endpoints are rate-limited per IP.</li>
          <li>
            Forgotten-password links carry a single-use token with a 15-minute TTL. Used or expired tokens 410.
          </li>
        </ul>
      </Section>

      <Section id="reporting" title="Found a vulnerability?">
        <p className="text-muted-foreground">
          Don't open a public GitHub issue. Email security@openrow.app (or hello@steezr.com if you don't get a
          response) with details. We aim to acknowledge inside one business day and to ship a fix or workaround
          inside seven for confirmed issues.
        </p>
      </Section>
    </DocPage>
  )
}
