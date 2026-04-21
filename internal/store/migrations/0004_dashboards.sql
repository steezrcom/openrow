CREATE TABLE steezr.dashboards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES steezr.tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    description TEXT,
    position    INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX dashboards_tenant_idx ON steezr.dashboards (tenant_id);

CREATE TABLE steezr.reports (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id  UUID NOT NULL REFERENCES steezr.dashboards(id) ON DELETE CASCADE,
    title         TEXT NOT NULL,
    subtitle      TEXT,
    widget_type   TEXT NOT NULL,
    query_spec    JSONB NOT NULL,
    width         INT  NOT NULL DEFAULT 6,  -- in a 12-column grid
    position      INT  NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reports_dashboard_idx ON steezr.reports (dashboard_id);
