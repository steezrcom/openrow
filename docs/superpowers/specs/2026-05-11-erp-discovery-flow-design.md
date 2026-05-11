# AI-native ERP discovery flow

Design for OpenRow's first AI-native ERP connector. The runtime model and the discovery flow are designed so a single pattern absorbs many vendors instead of one connector per ERP.

- **Status:** design, awaiting implementation plan
- **Date:** 2026-05-11
- **First target:** ABRA Flexi
- **Owner:** Johnny

## Motivation

The Czech ERP market has 30+ systems on lists like https://symmy.com/cs/systemy-integrace/. Hand-coding a connector per vendor multiplies effort, and each vendor's schema is per-client (custom fields, customer extensions, untranslated identifiers). The interesting work is not a connector per name. It is a discovery layer that scans a target, infers what its tables and columns mean, and produces a mapping the agent can use.

ABRA Flexi is the first target because it has self-describing metadata endpoints (`evidence-list`, `<evidence>/properties`), strong installed base in Czech SMB, and is uncomplicated enough that the discovery pipeline can be validated without also wrestling raw MSSQL introspection. Subsequent SQL targets (Helios Orange, Money S5) replace the input stages of the same pipeline.

## Runtime model

Hybrid:

- **Mirror** slow-changing dimensions (customers, products, accounts, projects, cost centers) into the tenant's Postgres schema, cursor-based, on a schedule.
- **Read-through** transactional and live state (open invoices, today's stock movements, due payments) via an agent tool that queries Flexi live.

The same mapping artifact serves both. Mirrored evidences can be promoted to native OpenRow entities so they appear in dashboards and entity tools naturally.

## Data-model placement

Mapping lives in new metadata tables:

- `openrow.external_bindings` — one row per (tenant, connector) connection. Holds encrypted config and the mapping artifact JSON. State: `discovering`, `proposed`, `active`, `error`.
- `openrow.external_binding_review_items` — review queue (evidences and fields that need human approval before activation).
- `openrow.external_binding_cursors` — per-evidence sync cursors for mirror.

Mirrored data lands in the tenant schema. If an evidence is "promoted", its target table is a normal OpenRow entity table written through `entities.Service`. If not promoted, it lives in a sandboxed namespace `tenant_<slug>.erp_<binding_id>_<evidence>`.

## Trust model

Tiered, by confidence:

- `confidence >= 0.95` → auto-apply.
- `0.7 <= confidence < 0.95` → auto-apply with a "low confidence" flag visible in UI, can be reviewed later.
- `confidence < 0.7` → review queue, no auto-apply for that evidence/field.
- Customer-defined custom fields (Flexi's `userField*@FX*` and `stitky`-driven semantics) always land in the review queue, regardless of confidence, on first sight.

Activation of a binding requires resolving the review queue or explicitly accepting "import only the auto-applied subset".

## Discovery pipeline

Stage-based, deterministic skeleton with LLM calls only where heuristics fall short. The boundary between input stages (1-3) and downstream stages (4-8) is the seam that lets a later SQL connector reuse most of the pipeline.

### Stage 1 — Connect

Validate credentials by calling `GET /c/<company>/evidence-list.json`. Persist company token, server version, Flexi version. Fail fast on auth or network errors with structured error codes.

### Stage 2 — Enumerate

Fetch full evidence list. Persist raw response as discovery input. This is the artifact discovery operates on. Re-running discovery starts here unless a manual full reset is requested.

### Stage 3 — Introspect per evidence

For each evidence:

- `GET /c/<company>/<evidence>/properties.json` — machine-readable schema (types, relations, code-lists).
- `GET /c/<company>/<evidence>.json?limit=10&detail=full` — sample rows.
- `GET /c/<company>/<evidence>/$count` — row count.

Rate-limit (parallel-bounded, e.g. 4 concurrent). Persist raw responses. Stage 3 is the most network-intensive; it produces the structured input for classification.

### Stage 4 — Heuristic classify

A static map ships with the connector: known Flexi evidence names → canonical semantic roles. Examples:

| Flexi evidence | Semantic role |
|----------------|---------------|
| `adresar` | `customer` (also `supplier`, disambiguated by `typVztahuK` field) |
| `faktura-vydana` | `invoice_outgoing` |
| `faktura-prijata` | `invoice_incoming` |
| `cenik` | `product` |
| `skladovy-pohyb` | `stock_movement` |
| `objednavka-prijata` | `order_incoming` |
| `objednavka-vydana` | `order_outgoing` |
| `zakazka` | `project` |
| `stredisko` | `cost_center` |
| `ucetni-obdobi` | `accounting_period` |
| `bankovni-ucet` | `bank_account` |
| `banka` | `bank_movement` |

Same table for well-known field names (`nazev` → `display_name`, `ic` → `registration_id_cz`, `dic` → `vat_id_cz`, `email`, `telefon` → `phone`, ...). Confidence 1.0 on exact match.

Heuristic table lives in `internal/connectors/catalog/flexi/heuristics.go` and is curated, not LLM-generated. Curation is cheap because Flexi's stock evidence set is finite and public.

### Stage 5 — LLM classify

Only for:

- Evidences not in the heuristic map.
- Fields not in the heuristic map (including Flexi's `userField*@FX*` custom fields).
- Evidences that are in the map but have user-extended schemas (extra fields beyond the published Flexi spec).

Prompt per evidence (or per field batch):

- Evidence/field name in Czech.
- Properties JSON snippet.
- 3-5 sample values.
- Closed enum of canonical roles (the same list used by the heuristic table, plus `unknown`).
- A short instruction.

LLM returns structured JSON:

```jsonc
{
  "role": "loyalty_tier",  // or "unknown"
  "confidence": 0.62,
  "reasoning": "sample values are short capitalised strings matching common tier names",
  "field_map": {           // only for evidence-level prompts
    "userField1@FX001": "loyalty_tier",
    "userField2@FX001": "unknown"
  }
}
```

LLM is the workspace-configured one (per existing OpenRow LLM provider system) with a model floor: discovery refuses to run with a model below the documented minimum (tool-calling-capable, 7B+). The connector's per-binding settings can override the workspace default to a stronger model for discovery only.

Discovery batches custom fields per evidence to amortize prompt overhead. One LLM call per evidence is the target ceiling, not per field.

### Stage 6 — Tier

Apply the trust model from above. Output a per-evidence and per-field tier label (`auto`, `auto_low_confidence`, `needs_review`). Review items get queued into `external_binding_review_items`.

### Stage 7 — Persist and propose entities

Write the mapping artifact (see schema below) to `external_bindings.mapping`. For evidences with a canonical role in the "promotable" set (customer, product, project, account, cost_center, bank_account), generate an OpenRow entity proposal containing:

- Proposed entity name and Czech display name.
- Proposed field list (mapped from the Flexi field map, English code identifiers, Czech display names).
- A "promote on activate" flag (default true for high-confidence roles).

Entity proposals do not call `entities.Service` yet. They are surfaced in the review UI alongside the review queue.

### Stage 8 — Activate

User reviews the queue (or accepts the auto-only subset) and clicks Activate. Then:

- Promoted entities are created via `entities.Service` in a single transaction.
- Binding state → `active`.
- Mirror worker picks up active binding on next tick.
- Read-through tool becomes available to the agent for this binding.

## Mapping artifact

Single JSON document persisted on the binding row. Versioned. Sample:

```jsonc
{
  "version": 1,
  "connector": "abra-flexi",
  "vendor_version": "2024.5.3",
  "discovered_at": "2026-05-11T12:00:00Z",
  "evidences": {
    "adresar": {
      "role": "customer",
      "confidence": 1.0,
      "source": "heuristic",
      "mirror": true,
      "review": "auto",
      "row_count": 1284,
      "fields": {
        "nazev": {"role": "display_name", "type": "string", "confidence": 1.0, "source": "heuristic"},
        "ic":    {"role": "registration_id_cz", "type": "string", "confidence": 1.0, "source": "heuristic"},
        "dic":   {"role": "vat_id_cz", "type": "string", "confidence": 1.0, "source": "heuristic"},
        "stitky": {"role": "tags", "type": "ref_list", "confidence": 1.0, "source": "heuristic"},
        "userField1@FX001": {
          "role": "unknown",
          "type": "string",
          "confidence": 0.4,
          "source": "llm",
          "reasoning": "sample values look like loyalty tier names; no clear canonical role",
          "review": "needs_review"
        }
      },
      "openrow_entity": {
        "name": "customer",
        "display_name": "Zákazník",
        "promoted": false
      }
    }
  }
}
```

User edits in the review UI mutate this document (with an audit log). Re-discovery merges new evidences/fields without clobbering human edits, by treating `source: "user"` entries as authoritative.

## Mirror worker

Loop:

1. For each `external_bindings` with state `active` and at least one evidence with `mirror: true`.
2. For each mirrored evidence, call `GET /c/<company>/changes.json?since=<cursor>&evidence-type=<evidence>`. Flexi's changes API returns deltas with the change kind (`create`, `update`, `delete`) and the cursor.
3. Upsert rows into the target table (promoted entity table, or sandbox table) using the field map. Czech column names map to English code identifiers in the entity field set.
4. Advance the cursor in `external_binding_cursors`.
5. Drift check: if the response includes new fields not in the mapping, enqueue a per-evidence re-discovery (stages 3-8) for that evidence only.

Sync schedule: configurable per binding, default 15 minutes. Manual "Sync now" available.

Deletes: soft-delete in the mirrored table by default (a `_deleted_at` column on sandbox tables, a `state = archived` convention on promoted entities). Hard-delete is opt-in per evidence.

## Read-through tool

Agent tool registered alongside existing OpenRow tools:

```
query_external_binding(
  binding_id: string,
  role: enum<canonical_role>,
  filter: object,
  fields: array<string>?,
  limit: int = 100
) -> rows
```

The tool:

1. Resolves `role` to the Flexi evidence name via the mapping. Errors if no evidence has that role on this binding.
2. Translates `filter` into Flexi's query language. v1 supports a fixed grammar: equality on canonical-role fields, comparison on date/number fields, `is_null`, `in_list`. Free-form text search and LLM-translated filters are out of scope for v1.
3. Selects `fields` (mapped to Flexi column names). If `fields` is null, returns all auto-mapped fields with their canonical roles as keys.
4. Calls Flexi, caches the response for 60 seconds keyed on the full query signature, returns rows with English canonical-role keys (not Czech evidence column names).

Errors from Flexi surface to the agent with structured codes so it can recover (e.g. retry on `503`, give up on `403`).

## Schema migration

```sql
-- new tables in the openrow schema

CREATE TABLE openrow.external_bindings (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
  connector       text NOT NULL,                  -- 'abra-flexi'
  config_enc      bytea NOT NULL,                 -- encrypted connection config
  state           text NOT NULL,                  -- 'discovering' | 'proposed' | 'active' | 'error'
  mapping         jsonb,                          -- the artifact above
  last_error      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON openrow.external_bindings (tenant_id);

CREATE TABLE openrow.external_binding_review_items (
  id              uuid PRIMARY KEY,
  binding_id      uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
  evidence        text NOT NULL,
  field           text,                           -- null = evidence-level item
  proposed        jsonb NOT NULL,                 -- proposed mapping snippet
  status          text NOT NULL,                  -- 'pending' | 'accepted' | 'edited' | 'rejected'
  resolved_by     uuid,
  resolved_at     timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ON openrow.external_binding_review_items (binding_id, status);

CREATE TABLE openrow.external_binding_cursors (
  binding_id      uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
  evidence        text NOT NULL,
  cursor          text NOT NULL,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (binding_id, evidence)
);
```

Tenant schema sandbox tables (`erp_<binding_id>_<evidence>`) are created on activation by the same `entities.Service`-style DDL path, with an `_deleted_at` soft-delete column.

## API surface

Under `internal/httpapi`:

- `POST   /api/connectors/external-bindings` — create binding, kicks off discovery (state: `discovering`).
- `GET    /api/connectors/external-bindings/:id` — current state, progress, summary counts.
- `GET    /api/connectors/external-bindings/:id/mapping` — full artifact.
- `PATCH  /api/connectors/external-bindings/:id/mapping` — edit a specific evidence/field mapping (user override, marks `source: "user"`).
- `GET    /api/connectors/external-bindings/:id/review-items` — paged.
- `POST   /api/connectors/external-bindings/:id/review-items/:item_id/resolve` — accept | edit | reject.
- `POST   /api/connectors/external-bindings/:id/activate` — promote entities, switch to `active`.
- `POST   /api/connectors/external-bindings/:id/rediscover` — re-run pipeline, optional `evidence` query parameter to scope to one.
- `POST   /api/connectors/external-bindings/:id/sync` — manual mirror tick.

All endpoints scope by `tenant_id` derived from session via the existing `internal/auth/context.go` pattern.

## Agent tool

New tool registered in `internal/ai` next to the existing entity/dashboard tools:

- `list_external_bindings()` — bindings available in this tenant with their summary (connector, active evidences, available roles).
- `query_external_binding(binding_id, role, filter, fields?, limit?)` — see above.

The agent also continues to see promoted entities via the existing entity tools. The read-through tool is for evidences that are not mirrored or for explicit "live data" queries.

## UI

New section under Settings → Connectors → ERP. Three screens:

1. **Connect.** Form for Flexi URL, company token, credentials. On submit → server creates binding, starts discovery, redirects to progress screen.
2. **Discovery progress.** Live progress (`x of y evidences scanned`), preview of high-confidence mappings, current LLM activity. On completion → review screen.
3. **Review.** Tabs: Auto-mapped (read-only summary), Low confidence (one-click confirm), Needs review (custom fields, ambiguous evidences with samples and proposed roles, edit/accept/reject). Bottom action: Activate (disabled until queue is empty or "Activate auto-only" is chosen).

After activation, the binding has a settings page: sync frequency, per-evidence mirror toggle, force re-discover.

## Promoted entity editability

Promoted entities are read-only in the OpenRow UI by default. A per-entity `allow_local_edits` flag exists for advanced users; even when set, mirror sync remains last-writer-wins from Flexi's side. Two-way sync (push local edits back to Flexi) is explicitly out of scope.

## Failure modes

- **Discovery fails mid-flight.** State stays `discovering`, `last_error` is set, partial mapping is persisted so the user can re-run with progress preserved.
- **LLM rate limit.** Discovery pauses on the offending evidence, retries with backoff. Heuristic mappings already produced are unaffected.
- **Mirror sync conflict (row edited in Flexi and locally after promotion).** Last-writer-wins, with a soft-conflict log entry. Two-way sync is explicitly out of scope.
- **Schema drift mid-sync.** New evidence appears: queued for discovery, not auto-mirrored. New field appears in mirrored evidence: per-evidence re-discovery triggered, mirror continues with known fields until re-discovery resolves.
- **Flexi unavailable.** Mirror logs and retries on next tick. Read-through tool returns a structured `binding_unavailable` error the agent can communicate to the user.

## Security

- Flexi credentials encrypted at rest via `internal/secrets` (existing AES-256-GCM helper).
- All endpoints resolve `tenant_id` from the session; binding IDs are scoped to that tenant.
- LLM calls never receive raw secrets (only sample row values and column names).
- Sample row payloads in LLM prompts are capped (e.g. truncate string values >200 chars, mask values that look like PII tokens). The cap is per-field, not aggregate, so the LLM still sees enough to classify.
- Mirror runs as a tenant-scoped worker; no cross-tenant queries.

## Testing

- Heuristic table: unit tests for known Flexi evidences and fields.
- Discovery pipeline: integration tests with a recorded Flexi response fixture (`testdata/flexi/`). Each stage tested in isolation against fixtures.
- LLM classify: tested with a deterministic mock returning fixed JSON; live LLM tests run only with `OPENROW_LIVE_LLM_TESTS=1`.
- Mirror: integration test against the Flexi fixture, asserts cursor advance and upserts.
- Read-through tool: tested via the same fixture.

## Out of scope for v1

- Live SQL targets (Helios, Money). Pipeline boundary keeps them additive.
- Webhook-based mirror. Cursor polling is fine for v1.
- Two-way sync.
- Cross-binding entity unification (one customer across Flexi + Fakturoid).
- LLM-translated free-form filters in read-through.
- Bulk historical backfill scheduling (initial mirror does a full pull then switches to cursor).

## Open questions

None at design freeze. Implementation plan will be drafted next via `superpowers:writing-plans`.
