-- Per-dashboard default date-range preset. The dashboard page reads
-- this on first render if the user hasn't overridden the picker. Values
-- match the frontend PresetKey enum (all, 7d, 30d, 90d, mtd, qtd, ytd,
-- prev_month, last_12m). NULL or empty means "no opinion" — the frontend
-- falls back to its own default ("all").
ALTER TABLE openrow.dashboards
    ADD COLUMN default_date_range TEXT;
