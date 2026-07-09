-- Event Processor rules: forward internal events (bounce, suppression_created,
-- feedback_report, dmarc_received) to external services via a pluggable driver
-- (webhook, redis, …), filtered by event type and mailclass, one-per-event
-- (single) or accumulated (batch). driver is intentionally NOT constrained by a
-- CHECK so new drivers need no migration — validity is enforced in the app.
CREATE TABLE IF NOT EXISTS event_processors (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT NOT NULL,
    event_types    TEXT[] NOT NULL DEFAULT '{}',
    mailclasses    TEXT[] NOT NULL DEFAULT '{}',
    driver         TEXT NOT NULL,
    driver_config  JSONB NOT NULL DEFAULT '{}',
    mode           TEXT NOT NULL DEFAULT 'single',
    batch_max_size INTEGER NOT NULL DEFAULT 0,
    batch_max_wait TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT event_processors_mode_chk CHECK (mode IN ('single', 'batch')),
    CONSTRAINT event_processors_status_chk CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS event_processors_status_idx ON event_processors (status);
