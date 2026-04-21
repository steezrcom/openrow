# Contributing to OpenRow

Thanks for considering a contribution. This file covers the practical stuff.

## Ground rules

- Read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
- Small, focused PRs beat large ones. If your change is bigger than ~300 lines, open an issue first so we can align on approach.
- All contributions are licensed under AGPL-3.0 (same as the project). By opening a PR you confirm you have the right to submit the code under that license.

## Dev setup

See [README.md](README.md#quickstart-local-dev). You need:

- Go 1.25+
- Node 20+ and npm
- Docker (for Postgres) or a local Postgres 16

```bash
cp .env.example .env   # set ANTHROPIC_API_KEY
make db-up
make api               # terminal 1
make web               # terminal 2
```

Optional: `make build` compiles the SPA and runs the Go server with `SPA_DIR=web/dist`.

## Codebase tour

Short version in [README.md](README.md#project-layout). A few pointers:

- **Adding an entity type to the agency template?** Edit `internal/templates/agency.go` and add to the `specs` list in `installAgency`. Order matters: referenced entities must be created before their references.
- **Adding a new chart widget type?** Extend `reports.WidgetType` in `internal/reports/spec.go`, the executor shapes in `executor.go`, and the renderer in `web/src/components/ReportCard.tsx`.
- **Adding a Claude tool?** Register it in `internal/ai/tools.go` with `add(tool{...})`. Update the agent system prompt in `internal/ai/agent.go` if the tool changes how Claude should behave.
- **Adding an HTTP endpoint?** Handlers live in `internal/httpapi/`. Wire the route in `server.go`.

## Style

### Go

- `gofmt` on save. CI will reject unformatted code.
- Prefer standard library over third-party packages where reasonable.
- Errors get wrapped with `%w` and enough context to identify where they came from.
- No global state except for registries explicitly scoped (e.g. `templates.Register`).
- Parameterized SQL always. Identifiers go through `pgx.Identifier{...}.Sanitize()` after validating against the ident regex.
- Don't add comments that describe what the code does. Comments explain *why* something non-obvious happens.

### TypeScript / React

- `npm run typecheck` and `npm run build` must pass.
- Prefer TanStack Query for server state; no Redux.
- No inline styles beyond trivial cases — Tailwind classes.
- Keep components colocated with their routes until a component has 3+ callers.

### Commit messages

Conventional Commits, lowercase, concise:

```
feat(reports): add stacked area widget
fix(agent): don't invent filter values on categorical fields
chore: bump recharts to 2.16
```

Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `style`, `revert`.

Body is optional. When present it explains *why*, not *what*.

## Running tests

There aren't many yet. `go test ./...` runs what exists. Contributions that add tests for the executor, entity service, or agent tool handlers are very welcome.

## Database migrations

Migrations live in `internal/store/migrations/` as numbered `.sql` files. Add one named `000N_<slug>.sql`. The migrator runs every file that hasn't been applied yet, in order, inside a transaction each. Never edit a migration that's already been applied somewhere.

## Security-sensitive changes

If your PR touches auth, session handling, password hashing, cookie flags, SQL construction, or any code that handles user input as identifiers: open the PR, then ping `SECURITY.md` contact. We'll review with extra care.

## Questions

Open a discussion or a draft PR. Better to ask than to build the wrong thing.
