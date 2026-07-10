-- Per-probe event log for the ESP-monitoring detail view: a timestamped trail of
-- what happened in each phase (send / fetch / analyze), so operators can trace a
-- probe's progress and see fetch/auth errors instead of a black box.
CREATE TABLE IF NOT EXISTS monitoring_probe_events (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    probe_id UUID NOT NULL REFERENCES monitoring_probes(id) ON DELETE CASCADE,
    at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    phase    TEXT NOT NULL,                 -- send | fetch | analyze
    level    TEXT NOT NULL DEFAULT 'info',  -- info | error
    message  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS monitoring_probe_events_probe_idx
    ON monitoring_probe_events (probe_id, at);
