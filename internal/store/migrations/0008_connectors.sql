-- Per-tenant connector configuration (Fakturoid, Stripe, Revolut, etc.).
-- Credentials are stored as a single AES-256-GCM encrypted blob of the
-- field-name → value JSON map, keyed by OPENROW_SECRET_KEY.
CREATE TABLE openrow.connector_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    connector_id    TEXT NOT NULL,
    credentials     BYTEA,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, connector_id)
);

CREATE INDEX connector_configs_tenant_idx ON openrow.connector_configs (tenant_id);
