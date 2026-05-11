-- 0015_external_bindings.sql
-- Tables backing AI-native ERP discovery (see spec at
-- docs/superpowers/specs/2026-05-11-erp-discovery-flow-design.md).

CREATE TABLE openrow.external_bindings (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    connector_id         text NOT NULL,
    state                text NOT NULL CHECK (state IN ('discovering','proposed','active','error')),
    mapping              jsonb,
    llm_classification   boolean NOT NULL DEFAULT true,
    last_error           text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, connector_id)
);

CREATE TABLE openrow.external_binding_review_items (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    binding_id          uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
    evidence            text NOT NULL,
    field               text,
    proposed            jsonb NOT NULL,
    status              text NOT NULL CHECK (status IN ('pending','accepted','edited','rejected')) DEFAULT 'pending',
    resolved_by_user_id uuid REFERENCES openrow.users(id) ON DELETE SET NULL,
    resolved_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX external_binding_review_items_binding_idx
    ON openrow.external_binding_review_items (binding_id, status);

CREATE TABLE openrow.external_binding_cursors (
    binding_id      uuid NOT NULL REFERENCES openrow.external_bindings(id) ON DELETE CASCADE,
    evidence        text NOT NULL,
    cursor_value    text NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (binding_id, evidence)
);
