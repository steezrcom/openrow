CREATE SCHEMA IF NOT EXISTS openrow;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE openrow.tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    pg_schema   TEXT UNIQUE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE openrow.entities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    description     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX entities_tenant_idx ON openrow.entities (tenant_id);

CREATE TABLE openrow.fields (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id               UUID NOT NULL REFERENCES openrow.entities(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    display_name            TEXT NOT NULL,
    data_type               TEXT NOT NULL,
    is_required             BOOLEAN NOT NULL DEFAULT false,
    is_unique               BOOLEAN NOT NULL DEFAULT false,
    reference_entity_id     UUID REFERENCES openrow.entities(id),
    position                INT NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, name)
);

CREATE INDEX fields_entity_idx ON openrow.fields (entity_id);
