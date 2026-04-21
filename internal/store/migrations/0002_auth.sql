-- Users and auth primitives for SaaS.

CREATE TABLE openrow.users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email               TEXT UNIQUE NOT NULL CHECK (email = lower(email)),
    name                TEXT NOT NULL,
    password_hash       TEXT NOT NULL,
    email_verified_at   TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE openrow.membership_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE openrow.memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES openrow.users(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    role        openrow.membership_role NOT NULL DEFAULT 'member',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);

CREATE INDEX memberships_tenant_idx ON openrow.memberships (tenant_id);
CREATE INDEX memberships_user_idx   ON openrow.memberships (user_id);

CREATE TABLE openrow.sessions (
    id                  TEXT PRIMARY KEY,
    user_id             UUID NOT NULL REFERENCES openrow.users(id) ON DELETE CASCADE,
    active_tenant_id    UUID REFERENCES openrow.tenants(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessions_user_idx ON openrow.sessions (user_id);

-- Org-level SaaS fields. Plan is free until Stripe is wired up.
ALTER TABLE openrow.tenants
    ADD COLUMN plan                   TEXT NOT NULL DEFAULT 'free',
    ADD COLUMN stripe_customer_id     TEXT,
    ADD COLUMN stripe_subscription_id TEXT,
    ADD COLUMN created_by_user_id     UUID REFERENCES openrow.users(id) ON DELETE SET NULL;
