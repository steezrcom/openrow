-- Per-tenant LLM configuration. Users bring their own OpenAI-compatible
-- endpoint + model. API keys are AES-256-GCM encrypted with OPENROW_SECRET_KEY.
CREATE TABLE openrow.llm_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL UNIQUE REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    base_url    TEXT NOT NULL,
    api_key     BYTEA,           -- nullable: local endpoints may not need one
    model       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
