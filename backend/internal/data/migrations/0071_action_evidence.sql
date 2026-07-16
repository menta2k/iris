-- Evidence for automatic enforcement actions: the exact mail-log event that led
-- to an auto-action (e.g. the deferral that auto-disabled TLS for a domain, or
-- the bounce that auto-suppressed a recipient). Kept in a separate table so the
-- UI can show "why was this done" per subject.
CREATE TABLE IF NOT EXISTS action_evidence (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type  TEXT NOT NULL,             -- tls_auto_disable | bounce_suppress
    subject_type TEXT NOT NULL,             -- tls_policy | suppression
    subject_key  TEXT NOT NULL,             -- lower(domain) | lower(recipient)
    message_id   TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT '',
    event        JSONB NOT NULL DEFAULT '{}'::jsonb,  -- the exact mail-log record
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS action_evidence_subject_idx
    ON action_evidence (subject_type, subject_key, created_at DESC);
