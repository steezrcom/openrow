-- Scheduler support for cron-triggered flows.
-- next_run_at is the next time the scheduler should dispatch the flow.
-- For non-cron flows it stays NULL. Partial index keeps lookup cheap.
ALTER TABLE openrow.flows
    ADD COLUMN next_run_at TIMESTAMPTZ;

CREATE INDEX flows_cron_due_idx
    ON openrow.flows (next_run_at)
    WHERE enabled = true AND trigger_kind = 'cron' AND next_run_at IS NOT NULL;
