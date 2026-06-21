-- 0003_continuous_aggregates.sql
-- Dashboard rollups. These are defined as portable SQL views using date_trunc
-- so the migration applies inside a transaction on both TimescaleDB and plain
-- PostgreSQL. On a TimescaleDB deployment these view bodies can be promoted to
-- materialized continuous aggregates (CREATE MATERIALIZED VIEW ... WITH
-- (timescaledb.continuous)) using the same SELECT; they intentionally use a
-- 1-minute / 1-hour bucket aligned to data-model.md.

CREATE OR REPLACE VIEW mail_stats_1m AS
SELECT date_trunc('minute', event_time) AS bucket,
       mailclass,
       vmta_id,
       recipient_domain,
       status,
       count(*) AS event_count
FROM mail_records
GROUP BY 1, 2, 3, 4, 5;

CREATE OR REPLACE VIEW bounce_stats_1h AS
SELECT date_trunc('hour', event_time) AS bucket,
       mailclass,
       vmta_id,
       smtp_status,
       bounce_type,
       count(*) AS bounce_count
FROM bounce_records
GROUP BY 1, 2, 3, 4, 5;

CREATE OR REPLACE VIEW feedback_stats_1h AS
SELECT date_trunc('hour', received_at) AS bucket,
       source,
       report_type,
       count(*) AS report_count
FROM feedback_reports
GROUP BY 1, 2, 3;

CREATE OR REPLACE VIEW queue_stats_1m AS
SELECT date_trunc('minute', observed_at) AS bucket,
       mailclass,
       max(depth) AS max_depth,
       max(oldest_message_age_seconds) AS max_oldest_age_seconds
FROM queue_snapshots
GROUP BY 1, 2;

CREATE OR REPLACE VIEW webhook_stats_1h AS
SELECT date_trunc('hour', event_time) AS bucket,
       webhook_rule_id,
       count(*) FILTER (WHERE status = 'delivered') AS delivered_count,
       count(*) FILTER (WHERE status = 'failed') AS failed_count,
       count(*) AS total_count
FROM webhook_delivery_events
GROUP BY 1, 2;

CREATE OR REPLACE VIEW audit_stats_1h AS
SELECT date_trunc('hour', occurred_at) AS bucket,
       operation,
       outcome,
       actor_user_id,
       count(*) AS audit_count
FROM audit_entries
GROUP BY 1, 2, 3, 4;
