-- Self-monitoring: singleton settings (thresholds + alert delivery) and the
-- history of threshold transitions the worker records.
CREATE TABLE IF NOT EXISTS monitor_settings (
    id               INT PRIMARY KEY DEFAULT 1,
    enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    cpu_threshold    INT NOT NULL DEFAULT 90,
    mem_threshold    INT NOT NULL DEFAULT 90,
    disk_threshold   INT NOT NULL DEFAULT 85,
    disk_paths       TEXT[] NOT NULL DEFAULT ARRAY['/'],
    notify_emails    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    from_email       TEXT NOT NULL DEFAULT '',
    smtp_host        TEXT NOT NULL DEFAULT 'localhost:25',
    cooldown_minutes INT NOT NULL DEFAULT 30,
    sample_seconds   INT NOT NULL DEFAULT 30,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT monitor_settings_singleton CHECK (id = 1)
);

INSERT INTO monitor_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS monitor_alerts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource   TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    level      TEXT NOT NULL,
    value      DOUBLE PRECISION NOT NULL,
    threshold  INT NOT NULL,
    message    TEXT NOT NULL DEFAULT '',
    notified   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_monitor_alerts_created ON monitor_alerts (created_at DESC);
