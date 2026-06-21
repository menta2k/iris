-- 0002_event_hypertables.sql
-- Time-series operational event tables converted into TimescaleDB hypertables.
-- create_hypertable is wrapped so the migration still applies on a plain
-- PostgreSQL instance (the extension call is skipped if unavailable).

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Mail events ---------------------------------------------------------------

CREATE TABLE IF NOT EXISTS mail_records (
    id                    UUID NOT NULL DEFAULT gen_random_uuid(),
    message_id            TEXT NOT NULL,
    event_time            TIMESTAMPTZ NOT NULL DEFAULT now(),
    mailclass             TEXT NOT NULL DEFAULT '',
    sender                TEXT NOT NULL DEFAULT '',
    recipient             TEXT NOT NULL DEFAULT '',
    recipient_domain      TEXT NOT NULL DEFAULT '',
    vmta_id               UUID,
    route_id              UUID,
    status                TEXT NOT NULL DEFAULT 'received',
    sensitive_preview_ref TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, event_time),
    CONSTRAINT mail_records_status_chk CHECK (status IN
        ('received', 'queued', 'sent', 'deferred', 'bounced', 'suppressed', 'failed'))
);

CREATE INDEX IF NOT EXISTS mail_records_event_time_idx ON mail_records (event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_mailclass_idx ON mail_records (mailclass, event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_sender_idx ON mail_records (sender, event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_recipient_idx ON mail_records (recipient, event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_rcpt_domain_idx ON mail_records (recipient_domain, event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_vmta_idx ON mail_records (vmta_id, event_time DESC);
CREATE INDEX IF NOT EXISTS mail_records_status_idx ON mail_records (status, event_time DESC);

-- Bounce events -------------------------------------------------------------

CREATE TABLE IF NOT EXISTS bounce_records (
    id               UUID NOT NULL DEFAULT gen_random_uuid(),
    mail_record_id   UUID,
    event_time       TIMESTAMPTZ NOT NULL DEFAULT now(),
    recipient        TEXT NOT NULL DEFAULT '',
    vmta_id          UUID,
    mailclass        TEXT NOT NULL DEFAULT '',
    smtp_status      TEXT NOT NULL DEFAULT '',
    bounce_type      TEXT NOT NULL DEFAULT '',
    diagnostic       TEXT NOT NULL DEFAULT '',
    processing_state TEXT NOT NULL DEFAULT 'new',
    PRIMARY KEY (id, event_time),
    CONSTRAINT bounce_records_state_chk
        CHECK (processing_state IN ('new', 'processed', 'ignored', 'failed'))
);
CREATE INDEX IF NOT EXISTS bounce_records_time_idx ON bounce_records (event_time DESC);
CREATE INDEX IF NOT EXISTS bounce_records_mailclass_idx ON bounce_records (mailclass, event_time DESC);

-- Feedback reports ----------------------------------------------------------

CREATE TABLE IF NOT EXISTS feedback_reports (
    id               UUID NOT NULL DEFAULT gen_random_uuid(),
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    source           TEXT NOT NULL DEFAULT '',
    report_type      TEXT NOT NULL DEFAULT '',
    recipient        TEXT NOT NULL DEFAULT '',
    mail_record_id   UUID,
    processing_state TEXT NOT NULL DEFAULT 'new',
    raw_ref          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, received_at),
    CONSTRAINT feedback_reports_state_chk
        CHECK (processing_state IN ('new', 'processed', 'ignored', 'failed'))
);
CREATE INDEX IF NOT EXISTS feedback_reports_time_idx ON feedback_reports (received_at DESC);

-- Queue snapshots -----------------------------------------------------------

CREATE TABLE IF NOT EXISTS queue_snapshots (
    id                         UUID NOT NULL DEFAULT gen_random_uuid(),
    observed_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    mailclass                  TEXT NOT NULL DEFAULT '',
    state                      TEXT NOT NULL DEFAULT 'unknown',
    depth                      BIGINT NOT NULL DEFAULT 0,
    oldest_message_age_seconds BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id, observed_at)
);
CREATE INDEX IF NOT EXISTS queue_snapshots_mailclass_idx ON queue_snapshots (mailclass, observed_at DESC);

-- Webhook delivery events ---------------------------------------------------

CREATE TABLE IF NOT EXISTS webhook_delivery_events (
    id              UUID NOT NULL DEFAULT gen_random_uuid(),
    event_time      TIMESTAMPTZ NOT NULL DEFAULT now(),
    webhook_rule_id UUID,
    mail_record_id  UUID,
    attempt         INTEGER NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'pending',
    response_code   INTEGER NOT NULL DEFAULT 0,
    error_summary   TEXT NOT NULL DEFAULT '',
    next_retry_at   TIMESTAMPTZ,
    PRIMARY KEY (id, event_time),
    CONSTRAINT webhook_delivery_events_status_chk CHECK (status IN
        ('pending', 'delivered', 'retrying', 'failed', 'cancelled'))
);
CREATE INDEX IF NOT EXISTS webhook_delivery_rule_idx ON webhook_delivery_events (webhook_rule_id, event_time DESC);

-- Rspamd filter results -----------------------------------------------------

CREATE TABLE IF NOT EXISTS rspamd_filter_results (
    id             UUID NOT NULL DEFAULT gen_random_uuid(),
    event_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
    mail_record_id UUID,
    action         TEXT NOT NULL DEFAULT '',
    score          DOUBLE PRECISION NOT NULL DEFAULT 0,
    symbols        JSONB NOT NULL DEFAULT '[]',
    reason         TEXT NOT NULL DEFAULT '',
    raw_ref        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, event_time)
);
CREATE INDEX IF NOT EXISTS rspamd_results_time_idx ON rspamd_filter_results (event_time DESC);

-- Audit entries -------------------------------------------------------------

CREATE TABLE IF NOT EXISTS audit_entries (
    id                  UUID NOT NULL DEFAULT gen_random_uuid(),
    occurred_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_user_id       UUID,
    operation           TEXT NOT NULL DEFAULT '',
    target_type         TEXT NOT NULL DEFAULT '',
    target_id           TEXT NOT NULL DEFAULT '',
    outcome             TEXT NOT NULL DEFAULT 'success',
    ip_address          TEXT NOT NULL DEFAULT '',
    user_agent          TEXT NOT NULL DEFAULT '',
    request_id          TEXT NOT NULL DEFAULT '',
    safe_change_summary JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (id, occurred_at),
    CONSTRAINT audit_entries_outcome_chk
        CHECK (outcome IN ('success', 'failure', 'denied'))
);
CREATE INDEX IF NOT EXISTS audit_entries_time_idx ON audit_entries (occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_entries_actor_idx ON audit_entries (actor_user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS audit_entries_operation_idx ON audit_entries (operation, occurred_at DESC);

-- Service-control requests --------------------------------------------------

CREATE TABLE IF NOT EXISTS service_control_requests (
    id              UUID NOT NULL DEFAULT gen_random_uuid(),
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    requested_by    UUID,
    operation       TEXT NOT NULL DEFAULT '',
    confirmation_id TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'requested',
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    result_summary  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, requested_at),
    CONSTRAINT service_control_status_chk CHECK (status IN
        ('requested', 'running', 'succeeded', 'failed', 'cancelled', 'timed_out'))
);
CREATE INDEX IF NOT EXISTS service_control_time_idx ON service_control_requests (requested_at DESC);

-- Promote event tables to hypertables when TimescaleDB is present. -----------

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('mail_records', 'event_time', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('bounce_records', 'event_time', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('feedback_reports', 'received_at', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('queue_snapshots', 'observed_at', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('webhook_delivery_events', 'event_time', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('rspamd_filter_results', 'event_time', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('audit_entries', 'occurred_at', if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM create_hypertable('service_control_requests', 'requested_at', if_not_exists => TRUE, migrate_data => TRUE);
    END IF;
END
$$;
