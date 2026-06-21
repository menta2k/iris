-- 0007_delivery_and_bounce_settings.sql
-- Add delivery-rate (egress retry schedule) and bounce/DSN pipeline options to
-- the global_settings singleton.

ALTER TABLE global_settings
    -- Delivery rates: the outbound retry schedule (KumoMTA duration form:
    -- "20m", "4h", "7d"). Empty leaves KumoMTA's defaults.
    ADD COLUMN IF NOT EXISTS egress_retry_interval     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS egress_max_retry_interval TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS egress_max_age            TEXT NOT NULL DEFAULT '',
    -- Bounce / DSN pipeline.
    ADD COLUMN IF NOT EXISTS bounce_domain             TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS auto_suppress_hard_bounces BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS soft_bounce_threshold     INTEGER NOT NULL DEFAULT 0,
    ADD CONSTRAINT global_settings_soft_threshold_chk CHECK (soft_bounce_threshold >= 0);

-- Per-recipient soft-bounce counters, used to auto-suppress an address once it
-- accumulates soft_bounce_threshold soft bounces.
CREATE TABLE IF NOT EXISTS recipient_bounce_counts (
    recipient   TEXT PRIMARY KEY,
    soft_count  INTEGER NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
