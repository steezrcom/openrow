# OpenRow

> The AI-native backoffice for small teams. Describe your business in plain English. OpenRow creates real tables, forms, and dashboards you can run on.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](LICENSE)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)
![TypeScript](https://img.shields.io/badge/typescript-5.7-3178C6)

OpenRow is an open-source operations platform built for agencies, studios, and consultancies under 50 people. It's an AI-native alternative to Airtable / Notion / small ERPs: you describe the entities and reports you want, and OpenRow turns them into real Postgres tables, dashboards, and data flows. Everything in the product is addressable from a Claude-powered chat panel — entities, rows, schema edits, charts, and timers.

## Features

- **Describe to create.** Natural-language schema design. `"A customer with name, email, and an optional phone"` becomes a real typed table with indexes and constraints.
- **Chat as the control plane.** Ask for a new entity, a row edit, a dashboard, a field drop, or a report redesign. Claude calls the same tools a developer would.
- **Dashboards and reports.** KPI, bar, line, area, pie, and table widgets. Grouped and stacked series, period-over-period comparison, date-range filters, drag-to-reorder, inline edit.
- **Time tracking.** Global timer + weekly timesheet grid + default agency dashboard.
- **Multi-tenant by design.** Each workspace gets its own Postgres schema; no JSONB soup.
- **Fully self-hostable.** Go backend + React SPA + Postgres. One binary, one container.

## Architecture

- **Backend.** Go 1.25, `net/http`, `pgx/v5`, Anthropic SDK.
- **Database.** PostgreSQL 16. Metadata (entities, fields, dashboards, reports) lives in an `openrow` schema; each tenant gets its own schema for user data.
- **Frontend.** React 18 + TypeScript, Vite, TanStack Router + Query, Tailwind, Recharts, Zustand.
- **AI.** Anthropic Claude (Sonnet 4.6 default) via tool use. Tools are narrow and typed: `create_entity`, `add_row`, `update_report`, `apply_template`, etc.

## Quickstart (local dev)

```bash
git clone https://github.com/openrow/openrow
cd openrow
cp .env.example .env
# set ANTHROPIC_API_KEY in .env

make db-up        # Postgres via docker-compose
make api          # Go server on :8080
make web          # Vite dev server on :5173, proxies /api
open http://localhost:5173
```

Sign up, pick a workspace name, and install the **Agency** template on the empty home screen. Or just describe your first entity.

## Quickstart (all-in-one Docker)

```bash
git clone https://github.com/openrow/openrow
cd openrow
cp .env.example .env
# set ANTHROPIC_API_KEY in .env

docker compose -f docker-compose.yml -f docker-compose.app.yml up -d
open http://localhost:8080
```

This runs the app (Go + embedded SPA) and Postgres together. See [docs/self-host.md](docs/self-host.md) for production deployment notes.

## Configuration

Environment variables (all read from `.env`):

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `DATABASE_URL` | required | Postgres connection string |
| `ANTHROPIC_API_KEY` | optional | Needed for the chat panel and `describe-to-build` flows |
| `HTTP_ADDR` | `:8080` | Address the server listens on |
| `APP_URL` | `http://localhost:5173` | Public URL used in email links (password reset) |
| `SECURE_COOKIES` | `false` | Set to `true` behind HTTPS in prod |
| `SPA_DIR` | unset | Path to the built SPA (e.g. `web/dist`). Serves the React app from the Go binary when set. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Project layout

```
cmd/server/          Go entrypoint
internal/
  ai/                Claude agent + tools + NL entity proposer
  auth/              users, sessions, memberships, password resets
  entities/          dynamic DDL, entity metadata, row CRUD
  httpapi/           HTTP handlers (auth, entities, dashboards, chat, templates)
  mailer/            email interface + stdout impl
  reports/           query spec, executor, dashboard service
  spa/               serve the built React app from disk
  store/             pgx pool, migrations
  templates/         code-defined workspace starters (Agency)
  tenant/            tenant CRUD + per-tenant schema
web/
  src/               React app (routes, components, hooks)
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Small PRs welcome, especially: more templates (services, consulting, e-commerce), locale packs beyond Czech, connector scaffolding (Fakturoid, Stripe), and polish on the agent prompts.

## Security

For vulnerability reports, see [SECURITY.md](SECURITY.md). Please don't open public issues for security bugs.

## License

OpenRow is licensed under the GNU Affero General Public License v3.0. Source in [LICENSE](LICENSE). In plain English: you can self-host, fork, and modify freely. If you run a modified version as a service, you must release your modifications under the same license.

## Acknowledgements

- [Anthropic](https://anthropic.com) for Claude and the Go SDK
- [TanStack](https://tanstack.com) for the router and query libraries
- [Recharts](https://recharts.org) for charts
- [Lucide](https://lucide.dev) for icons
