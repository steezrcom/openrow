-- Named, persistent view configurations on entities. A user can define
-- multiple views (table, cards, kanban, gallery) per entity, each with
-- its own config. The implicit "Table" default is rendered client-side
-- and doesn't get a row here.
CREATE TABLE openrow.entity_views (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES openrow.tenants(id) ON DELETE CASCADE,
    entity_id           UUID NOT NULL REFERENCES openrow.entities(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    view_type           TEXT NOT NULL,                    -- 'table' | 'cards' | 'kanban' | 'gallery'
    config              JSONB NOT NULL DEFAULT '{}'::jsonb,
    position            INT NOT NULL DEFAULT 0,
    created_by_user_id  UUID REFERENCES openrow.users(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, name)
);

CREATE INDEX entity_views_entity_idx ON openrow.entity_views (entity_id, position);
