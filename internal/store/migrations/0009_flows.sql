-- Agent-authored automations. A flow is a persistent agent loop with a goal,
-- a trigger, a tool allowlist, and a permission mode.
CREATE TABLE openrow.flows (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    description          TEXT,
    goal                 TEXT NOT NULL,
    trigger_kind         TEXT NOT NULL,                 -- 'manual' | 'entity_event' | 'webhook' | 'cron'
    trigger_config       JSONB NOT NULL DEFAULT '{}'::jsonb,
    tool_allowlist       JSONB NOT NULL DEFAULT '[]'::jsonb,
    mode                 TEXT NOT NULL DEFAULT 'dry_run', -- 'dry_run' | 'approve' | 'auto'
    webhook_token_hash   BYTEA,
    webhook_secret       BYTEA,                         -- optional connector signing secret (encrypted)
    enabled              BOOLEAN NOT NULL DEFAULT true,
    created_by_user_id   UUID REFERENCES openrow.users(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX flows_tenant_idx ON openrow.flows (tenant_id);
CREATE INDEX flows_trigger_idx ON openrow.flows (tenant_id, trigger_kind) WHERE enabled = true;

-- One row per execution. message_history snapshots the full LLM message array
-- at each checkpoint so the runner can resume after an approval.
CREATE TABLE openrow.flow_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flow_id           UUID NOT NULL REFERENCES openrow.flows(id) ON DELETE CASCADE,
    tenant_id         UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    trigger_payload   JSONB NOT NULL DEFAULT '{}'::jsonb,
    status            TEXT NOT NULL,                      -- 'queued' | 'running' | 'awaiting_approval' | 'succeeded' | 'failed'
    mode              TEXT NOT NULL,                      -- snapshot of flow.mode at run time
    message_history   JSONB NOT NULL DEFAULT '[]'::jsonb,
    error             TEXT,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ
);

CREATE INDEX flow_runs_flow_idx   ON openrow.flow_runs (flow_id, started_at DESC);
CREATE INDEX flow_runs_tenant_idx ON openrow.flow_runs (tenant_id, started_at DESC);
CREATE INDEX flow_runs_status_idx ON openrow.flow_runs (status) WHERE status IN ('queued', 'running', 'awaiting_approval');

-- Append-only audit log for UI display. message_history (above) is the source
-- of truth for resumption; this table is for humans.
CREATE TABLE openrow.flow_run_steps (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flow_run_id   UUID NOT NULL REFERENCES openrow.flow_runs(id) ON DELETE CASCADE,
    position      INT NOT NULL,
    kind          TEXT NOT NULL,  -- 'agent_message' | 'tool_call' | 'tool_result' | 'mutation_blocked' | 'approval_requested'
    content       JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (flow_run_id, position)
);

CREATE INDEX flow_run_steps_run_idx ON openrow.flow_run_steps (flow_run_id, position);

-- Per-mutation approvals. A single run may accumulate multiple approvals over
-- its lifetime (one per intercepted write tool call). When status flips to
-- 'approved' or 'rejected', the runner resumes.
CREATE TABLE openrow.flow_approvals (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flow_run_id       UUID NOT NULL REFERENCES openrow.flow_runs(id) ON DELETE CASCADE,
    tenant_id         UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    tool_call_id      TEXT NOT NULL,       -- the LLM's tool_call_id, so the resumed loop can feed the result back
    tool_name         TEXT NOT NULL,
    tool_input        JSONB NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending', -- 'pending' | 'approved' | 'rejected' | 'expired'
    rejection_reason  TEXT,
    requested_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at       TIMESTAMPTZ,
    resolved_by_user_id UUID REFERENCES openrow.users(id) ON DELETE SET NULL
);

CREATE INDEX flow_approvals_tenant_idx ON openrow.flow_approvals (tenant_id, status, requested_at DESC);
CREATE INDEX flow_approvals_run_idx    ON openrow.flow_approvals (flow_run_id);
