-- 0034_retention.sql
-- UI-configurable retention & cleanup for the event hypertables. Cleanup is done
-- by dropping whole TimescaleDB chunks (instant OS-level disk reclaim — no
-- VACUUM FULL/OPTIMIZE TABLE), optionally compressing older chunks first. This
-- migration only stores the per-table configuration and history and tightens the
-- chunk interval; the version-sensitive compression/drop operations live in Go
-- (data/retention_repo.go) so they can be guarded and error-handled per
-- TimescaleDB version.

-- Per-table policy (one row per managed event hypertable). retention_days = 0
-- means keep forever; compress_after_days = 0 means no compression.
CREATE TABLE IF NOT EXISTS retention_policies (
    table_name          TEXT PRIMARY KEY,
    retention_days      INTEGER NOT NULL DEFAULT 0,
    compress_after_days INTEGER NOT NULL DEFAULT 0,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          TEXT NOT NULL DEFAULT '',
    CONSTRAINT retention_days_nonneg CHECK (retention_days >= 0),
    CONSTRAINT compress_days_nonneg CHECK (compress_after_days >= 0)
);

-- History of cleanup runs, for the UI to show what was reclaimed.
CREATE TABLE IF NOT EXISTS retention_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    table_name        TEXT NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ,
    chunks_compressed INTEGER NOT NULL DEFAULT 0,
    chunks_dropped    INTEGER NOT NULL DEFAULT 0,
    bytes_before      BIGINT NOT NULL DEFAULT 0,
    bytes_after       BIGINT NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS retention_runs_table_time_idx
    ON retention_runs (table_name, started_at DESC);

-- Default policies. audit_entries defaults to keep-forever so a compliance setup
-- is never surprised by automatic deletion; an operator can opt in via the UI.
INSERT INTO retention_policies (table_name, retention_days, compress_after_days) VALUES
    ('mail_records',             90,  7),
    ('bounce_records',           90,  7),
    ('feedback_reports',        180, 14),
    ('rspamd_filter_results',    30,  7),
    ('queue_snapshots',          14,  0),
    ('service_control_requests', 90,  0),
    ('audit_entries',             0,  0)
ON CONFLICT (table_name) DO NOTHING;

-- Tighten the chunk interval to 1 day on the event hypertables (only affects new
-- chunks) so retention is granular and chunks stay a manageable size at volume.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM set_chunk_time_interval('mail_records', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('bounce_records', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('feedback_reports', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('rspamd_filter_results', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('queue_snapshots', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('service_control_requests', INTERVAL '1 day');
        PERFORM set_chunk_time_interval('audit_entries', INTERVAL '1 day');
    END IF;
END
$$;
