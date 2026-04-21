# Security Policy

## Supported versions

OpenRow is pre-1.0. Security fixes land in `main` and are backported only to the most recent tagged release (once releases exist). Self-hosters should track `main` until then.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security bugs.** Instead, email:

**security@openrow.app**

Include:

- A description of the vulnerability and its impact.
- Steps to reproduce, or a proof-of-concept.
- The commit SHA or release tag you tested against.
- Your name and a link (GitHub, Twitter, website) if you'd like credit.

You will get an acknowledgement within 72 hours. We aim to ship a fix within 14 days for high-severity issues (RCE, auth bypass, SQL injection, tenant isolation bugs, credential leaks) and 30 days for lower-severity ones.

## Disclosure

We follow coordinated disclosure. Once a fix is merged and a release is cut, we'll publish a GitHub Security Advisory crediting the reporter (unless you'd prefer not to be named).

## Scope

In scope:

- OpenRow's Go backend (`cmd/`, `internal/`)
- The React frontend (`web/`)
- Default configuration and deployment artifacts (`docker-compose.yml`, `Dockerfile`)
- Tenant isolation, authentication, authorization, session handling
- SQL construction, identifier quoting, input coercion

Out of scope:

- Vulnerabilities in dependencies without a clear path to exploit in OpenRow (report those upstream)
- Brute-force attacks on a specific self-hosted deployment where the operator hasn't enabled rate limiting
- Self-XSS, missing security headers on pages that return no sensitive data
- Issues requiring a compromised Anthropic API key or Postgres superuser password

## Security model (for reviewers)

- **Multi-tenancy.** Each workspace gets its own Postgres schema (`tenant_<slug>`). All tenant-scoped queries derive the schema name from the caller's active membership (validated server-side), never from client input.
- **SQL construction.** User-supplied identifiers (entity names, field names) are validated against `^[a-z][a-z0-9_]{0,62}$` and always quoted via `pgx.Identifier.Sanitize()`. Values always go through parameterized queries.
- **Sessions.** Server-side, random 32-byte tokens stored in `openrow.sessions`, 30-day rolling expiry. Cookies: `HttpOnly`, `SameSite=Lax`, `Secure` when `SECURE_COOKIES=true`.
- **Passwords.** `argon2id` with the library's default parameters. Password resets are single-use, 1-hour TTL, and revoke all existing sessions on consume.
- **Agent tools.** The AI agent operates through the same service layer as the HTTP API; it cannot issue raw SQL or reach across tenants.
