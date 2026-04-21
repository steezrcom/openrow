CREATE TABLE steezr.password_resets (
    token       TEXT PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES steezr.users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);

CREATE INDEX password_resets_user_idx ON steezr.password_resets (user_id);
