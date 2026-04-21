-- Per-report display options (stacked bars, number format, currency, locale, etc.)
ALTER TABLE openrow.reports
    ADD COLUMN options JSONB NOT NULL DEFAULT '{}';
