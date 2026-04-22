-- OpenRow is a self-host-focused open-source project. The billing
-- placeholder columns added in 0002 were speculative and never wired
-- up anywhere in Go or the frontend. Drop them to keep the schema
-- honest; any future paid/SaaS fork can add its own columns without
-- conflicting with upstream.
ALTER TABLE openrow.tenants
    DROP COLUMN IF EXISTS plan,
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS stripe_subscription_id;
