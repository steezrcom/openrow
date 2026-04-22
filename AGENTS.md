# AGENTS.md

Short guide for AI coding agents working in this repo. Read once, apply throughout.

## Project shape

- **Backend:** Go 1.25, `net/http`, `pgx/v5`. Entry point: `cmd/server`. Packages under `internal/`.
- **Frontend:** React 18 + Vite + TypeScript, TanStack Router/Query, Tailwind, Zustand. Lives in `web/`.
- **DB:** Postgres 16. Schema-per-tenant (`tenant_<slug>`), with system tables in the `openrow` schema. Migrations embedded in `internal/store/migrations/*.sql`.
- **License:** AGPL-3.0. Don't add code under a different license.
- **Scope:** self-host focused, open-source. No billing, plans, seat counts, Stripe customer plumbing, metered limits, trial logic, or usage-based gating — not now, not as placeholders. A paid SaaS fork can add that on its own; keep upstream honest.

## Dev setup

```bash
make db-up   # starts Postgres in Docker
make seed    # creates demo@openrow.local / openrow123 in a `demo` tenant, installs the agency template, inserts demo rows
make dev     # Go server (air) + Vite together, Ctrl+C stops both
```

Required env: `DATABASE_URL`, `OPENROW_SECRET_KEY`. See `.env.example`.

## Code quality

- Simplest thing that solves the task. No speculative abstractions, no helpers without a second caller, no "future-proofing".
- Default to **no comments**. Only add one when the WHY is non-obvious (hidden constraint, subtle invariant, workaround for a specific bug). Never describe WHAT the code does, reference the current task ("added for X"), or name callers.
- Don't add error handling for scenarios that can't happen. Validate at system boundaries (HTTP handlers, external APIs, user input). Trust internal code.
- Delete dead code completely. No `// removed` markers, no `_unused` vars.
- Match scope strictly. Bug fixes don't get drive-by refactors.
- No emojis in code, commits, or UI strings unless explicitly requested.

## Security

Multi-tenancy and auth are the main pitfalls here.

- **Every app endpoint must resolve the caller's tenant from session/membership** and scope queries to that tenant. Never trust a `tenant_id` from the request body. See `internal/auth/context.go` for the pattern.
- **IDOR is the default vulnerability.** Authentication ≠ authorization. Always check the current user can act on *this specific resource* (e.g. row belongs to their tenant, entity exists in their schema).
- **Dynamic SQL identifiers must go through `pgx.Identifier{...}.Sanitize()`.** Never concat schema or table names. Parameter binding (`$1`) handles values; identifiers need `Sanitize()`.
- **Secrets** (LLM API keys, connector tokens) are encrypted at rest via `internal/secrets` (AES-256-GCM). Never log them. Never return plaintext keys in API responses.
- Never log full request/response bodies that may contain user data or tokens. Log IDs, not payloads.
- Treat every external input as hostile: HTTP bodies/headers/queries, LLM tool-call arguments, file contents, third-party API responses.
- No `.env`, credential files, `*.pem`, or `*.key` in commits. Already gitignored; don't fight it.

## Database

- **Migrations** are numbered SQL files in `internal/store/migrations/`. One change per file. Embed via `//go:embed`. Migrator runs them in order on boot.
- Prefer additive migrations: add column nullable → backfill → tighten in a later migration. Don't drop columns in the same deploy as the code change that stops using them.
- **Parameterized queries only.** Never string-concat user data into SQL.
- Wrap multi-statement writes in a transaction (`pool.BeginTx`).
- `fields.reference_entity_id` is a self-ref FK without CASCADE — be aware when deleting entity metadata (see `cmd/seed/main.go`'s `dropTenant` for the pattern).

## Entity metadata vs table data

Entities have two sides that must stay in sync:

- **Metadata:** rows in `openrow.entities` and `openrow.fields`.
- **Data:** a real table at `tenant_<slug>.<entity_name>` with columns matching the fields.

Always mutate both via `entities.Service` (`Create`, `AddField`, `DropField`, `InsertRow`, etc.) — never hand-roll DDL or metadata inserts. The service wraps both sides in one transaction.

## Frontend

- Data fetching via TanStack Query. Invalidate the right query keys after mutations (`['entities']`, `['rows', entityName]`, `['dashboards']`, `['report-exec']`).
- Forms via `react-hook-form`. Errors from `ApiError` expose `.message`.
- Tailwind utility classes directly on elements. Small shared primitives in `web/src/components/ui/`.
- Entity **display names are Czech**; code identifiers and core UI strings are English. Don't translate code symbols.
- Modals portal to `document.body` (see `components/Modal.tsx`). Avoid z-index wars.

## Verifying changes

- `go build ./...` — must pass.
- `go vet ./...` — must pass.
- `go test ./...` — must pass.
- `cd web && npx tsc --noEmit` — must pass.
- For UI changes, run `make dev` and actually click through the feature. Type-checking verifies code, not behavior. If you can't test the UI, say so explicitly rather than claiming success.

## Git

- **Conventional Commits**, lowercase, concise. Format: `type(scope): summary`. Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `revert`.
- Subject line ≤ 72 chars, imperative mood ("add X", not "added X"), no trailing period.
- One logical change per commit. If the subject needs "and" twice, split.
- Body (optional) explains *why*, not *what*. References use full URLs, not bare `#123`.
- **Never add `Co-Authored-By: Claude ...` or "Generated with Claude Code" trailers.** Commits are authored by the user.
- Never `--amend` a shared-branch commit without explicit confirmation. Never `--no-verify` or skip hooks — if a hook fails, fix the underlying issue.
- Stage files explicitly by name. Avoid `git add .` / `git add -A`.
- Don't commit generated binaries (`/bin/`, stray `server`/`seed` executables), `.DS_Store`, IDE config.

## PRs

- Title follows Conventional Commit format, same rules as the commit subject.
- Description explains the why and flags anything risky (migrations, new env vars, breaking changes).
