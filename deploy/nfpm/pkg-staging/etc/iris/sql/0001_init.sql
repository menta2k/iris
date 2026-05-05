-- 0001_init.sql
-- TimescaleDB-specific DDL applied after ent's auto-migration. Idempotent —
-- safe to re-run on every boot. Each block guards itself with IF (NOT) EXISTS
-- so a partially-applied migration recovers cleanly on the next start.
--
-- The pattern: ent declares `id` as the primary key; TimescaleDB requires the
-- partitioning column to be in every unique index, so before create_hypertable
-- we replace the PK with a composite (id, at). Subsequent ent migrations
-- will try to "fix" the PK back to id-only — that diff is rejected by
-- TimescaleDB and we ignore the error in data.go (matching the prototype's
-- approach).

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ----- audit_entry -----
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'audit_entry'
  ) AND NOT EXISTS (
    SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'audit_entry'
  ) THEN
    ALTER TABLE audit_entry DROP CONSTRAINT IF EXISTS audit_entry_pkey;
    ALTER TABLE audit_entry ADD PRIMARY KEY (id, at);
    PERFORM create_hypertable('audit_entry', 'at',
      chunk_time_interval => INTERVAL '7 days',
      if_not_exists => TRUE,
      migrate_data  => TRUE);
  END IF;
END
$$;

-- Retention: keep audit log for 1 year. Adjust per compliance requirements.
SELECT add_retention_policy('audit_entry', INTERVAL '365 days', if_not_exists => TRUE);

-- ----- log_event -----
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'log_event'
  ) AND NOT EXISTS (
    SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'log_event'
  ) THEN
    ALTER TABLE log_event DROP CONSTRAINT IF EXISTS log_event_pkey;
    ALTER TABLE log_event ADD PRIMARY KEY (id, at);
    PERFORM create_hypertable('log_event', 'at',
      chunk_time_interval => INTERVAL '1 day',
      if_not_exists => TRUE,
      migrate_data  => TRUE);
  END IF;
END
$$;

-- Compress old chunks after 7 days, drop after 90 days.
ALTER TABLE log_event SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'event_type'
);

SELECT add_compression_policy('log_event', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('log_event', INTERVAL '90 days', if_not_exists => TRUE);

-- ----- metric_snapshot -----
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'metric_snapshot'
  ) AND NOT EXISTS (
    SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'metric_snapshot'
  ) THEN
    ALTER TABLE metric_snapshot DROP CONSTRAINT IF EXISTS metric_snapshot_pkey;
    ALTER TABLE metric_snapshot ADD PRIMARY KEY (id, at);
    PERFORM create_hypertable('metric_snapshot', 'at',
      chunk_time_interval => INTERVAL '1 day',
      if_not_exists => TRUE,
      migrate_data  => TRUE);
  END IF;
END
$$;

ALTER TABLE metric_snapshot SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'queue'
);

SELECT add_compression_policy('metric_snapshot', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_retention_policy('metric_snapshot', INTERVAL '180 days', if_not_exists => TRUE);

-- ----- feedback_reports -----
-- The Redis log-stream consumer inserts here on every Feedback record. FBL
-- traffic is bursty but per-row, so a 7-day chunk interval keeps chunk count
-- bounded while still letting compression benefit from per-domain locality.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'feedback_reports'
  ) AND NOT EXISTS (
    SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'feedback_reports'
  ) THEN
    ALTER TABLE feedback_reports DROP CONSTRAINT IF EXISTS feedback_reports_pkey;
    ALTER TABLE feedback_reports ADD PRIMARY KEY (id, received_at);
    PERFORM create_hypertable('feedback_reports', 'received_at',
      chunk_time_interval => INTERVAL '7 days',
      if_not_exists => TRUE,
      migrate_data  => TRUE);
  END IF;
END
$$;

ALTER TABLE feedback_reports SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'feedback_type'
);

SELECT add_compression_policy('feedback_reports', INTERVAL '14 days', if_not_exists => TRUE);
SELECT add_retention_policy('feedback_reports', INTERVAL '365 days', if_not_exists => TRUE);

-- ----- continuous aggregate: hourly delivery rate -----
-- Used by the dashboard. Refreshed every 15 min, materialising the prior
-- 7 days. Created here because it needs TimescaleDB syntax ent can't emit.
CREATE MATERIALIZED VIEW IF NOT EXISTS metric_delivery_hourly
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 hour', at) AS bucket,
  queue,
  SUM(delivered_total) AS delivered,
  SUM(failed_total) AS failed,
  AVG(delivery_rate_per_min) AS avg_rate
FROM metric_snapshot
GROUP BY bucket, queue
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metric_delivery_hourly',
  start_offset => INTERVAL '7 days',
  end_offset   => INTERVAL '1 hour',
  schedule_interval => INTERVAL '15 minutes',
  if_not_exists => TRUE);
