-- 0026_worker_error_logs.sql
-- A generic operational error log: every Warn/Error a background worker emits is
-- mirrored here (via the errlog slog handler) so failures that previously only
-- reached stdout — e.g. an unparseable DMARC report dropped by the dmarc worker —
-- become visible and queryable in the UI.
--
-- Also: denormalize the raw KumoMTA log record type onto mail_records. The log
-- hook tracks 7 record types but several collapse to status='bounced'
-- (Bounce/AdminBounce/Expiration); storing the original type makes all 7
-- distinguishable for coverage queries and a precise Logs filter.

CREATE TABLE IF NOT EXISTS worker_error_logs (
    id          UUID NOT NULL DEFAULT gen_random_uuid(),
    event_time  TIMESTAMPTZ NOT NULL DEFAULT now(),
    level       TEXT NOT NULL DEFAULT 'error',
    worker      TEXT NOT NULL DEFAULT '',
    message     TEXT NOT NULL DEFAULT '',
    detail      JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, event_time),
    CONSTRAINT worker_error_logs_level_chk CHECK (level IN ('warn', 'error'))
);

CREATE INDEX IF NOT EXISTS worker_error_logs_time_idx
    ON worker_error_logs (event_time DESC);
CREATE INDEX IF NOT EXISTS worker_error_logs_worker_idx
    ON worker_error_logs (worker, event_time DESC);
CREATE INDEX IF NOT EXISTS worker_error_logs_level_idx
    ON worker_error_logs (level, event_time DESC);

-- Promote to a TimescaleDB hypertable when the extension is present (the call is
-- skipped on a plain PostgreSQL instance so the migration still applies).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('worker_error_logs', 'event_time',
            if_not_exists => TRUE, migrate_data => TRUE);
    END IF;
END
$$;

-- Raw KumoMTA log record type (Reception/Delivery/Bounce/TransientFailure/
-- AdminBounce/Expiration/Feedback). Empty for pre-existing rows.
ALTER TABLE mail_records ADD COLUMN IF NOT EXISTS record_type TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS mail_records_record_type_idx
    ON mail_records (record_type, event_time DESC);
