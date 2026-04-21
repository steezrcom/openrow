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
# set OPENROW_SECRET_KEY (openssl rand -base64 32)
# set ANTHROPIC_API_KEY (optional fallback; each workspace can override in Settings)

make db-up        # Postgres via docker-compose
make dev          # air-watched Go backend + Vite frontend, both with HMR
open http://localhost:5173
```

`make dev` runs the Go server under [air](https://github.com/air-verse/air) (auto-rebuild on `.go` changes) and the Vite dev server together. Ctrl+C stops both. Use `make api` / `make web` in separate terminals if you prefer.

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
| `OPENROW_SECRET_KEY` | required | base64-encoded 32-byte key for encrypting stored API keys + connector secrets (`openssl rand -base64 32`) |
| `ANTHROPIC_API_KEY` | optional | Fallback LLM credentials when a workspace has no per-tenant LLM config. Handy for dev; in prod each workspace sets its own. |
| `HTTP_ADDR` | `:8080` | Address the server listens on |
| `APP_URL` | `http://localhost:5173` | Public URL used in email links (password reset) |
| `SECURE_COOKIES` | `false` | Set to `true` behind HTTPS in prod |
| `SPA_DIR` | unset | Path to the built SPA (e.g. `web/dist`). Serves the React app from the Go binary when set. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## LLM providers

OpenRow speaks OpenAI format and lets each workspace bring its own key. In **Settings → LLM** you'll find presets for OpenAI, Anthropic, OpenRouter, Groq, Together, DeepSeek, Google Gemini, Ollama, LM Studio, and a generic "custom" row. Pick one, paste a key, click **Fetch models** to populate the dropdown, hit **Test connection** to verify chat + tool calling.

### Using local models

Any server speaking `/v1/chat/completions` works: Ollama, LM Studio, vLLM, llama.cpp's `llama-server`, LocalAI, Jan, Open WebUI, LiteLLM.

Recommended models for the agent (tool calling):

- **Cloud:** GPT-4o, Claude Sonnet 4/4.5, Gemini 2.0 Flash, Llama 3.3 70B on Groq.
- **Local:** Llama 3.1 8B+, Qwen 2.5 7B+, Mistral Nemo, Hermes 3. Anything under 7B drops parameters and silently mis-uses tools.

Latency on CPU inference can be 10-100× slower than cloud APIs; the HTTP client waits up to 180s per turn.

### Docker networking for local models

If OpenRow runs in Docker and your local LLM server runs on the host:

- **macOS / Windows**: point the base URL at `http://host.docker.internal:11434/v1`.
- **Linux**: add `network_mode: host` to the app service in `docker-compose.app.yml`, or bind your LLM to `0.0.0.0` and use the host's LAN IP.

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
