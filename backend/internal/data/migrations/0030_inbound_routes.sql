-- 0030_inbound_routes.sql
-- Unified inbound routing (4.0.0): a single InboundRoute matches inbound mail by
-- recipient email/domain and dispatches it to one of three native kumod queue
-- protocols — maildir (kumod writes a Maildir), forward (relay to a pinned
-- smarthost via smtp mx_list), or webhook (POST the raw RFC822 to a URL, the
-- former webhook_rules behaviour). Existing active webhook_rules are backfilled as
-- webhook routes so path A keeps rendering unchanged. The webhook_rules table and
-- its reception-notification worker (path B) are intentionally left in place.

CREATE TABLE IF NOT EXISTS inbound_routes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    match_type      TEXT NOT NULL,
    match_value     TEXT NOT NULL,
    action          TEXT NOT NULL,
    priority        INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'active',
    -- forward action
    forward_host    TEXT NOT NULL DEFAULT '',
    forward_port    INTEGER NOT NULL DEFAULT 25,
    forward_tls     TEXT NOT NULL DEFAULT 'opportunistic',
    -- maildir action (empty path => deployment-wide base + per-user template)
    maildir_path    TEXT NOT NULL DEFAULT '',
    -- webhook action
    destination_url TEXT NOT NULL DEFAULT '',
    secret_ref      TEXT NOT NULL DEFAULT '',
    timeout_seconds INTEGER NOT NULL DEFAULT 10,
    retry_policy    JSONB NOT NULL DEFAULT '{"max_attempts":5,"backoff_seconds":30}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT inbound_routes_match_type_chk
        CHECK (match_type IN ('recipient_email', 'recipient_domain')),
    CONSTRAINT inbound_routes_action_chk
        CHECK (action IN ('maildir', 'forward', 'webhook')),
    CONSTRAINT inbound_routes_status_chk CHECK (status IN ('active', 'disabled')),
    CONSTRAINT inbound_routes_fwd_tls_chk
        CHECK (forward_tls IN ('none', 'opportunistic', 'required')),
    CONSTRAINT inbound_routes_fwd_port_chk
        CHECK (forward_port > 0 AND forward_port <= 65535),
    CONSTRAINT inbound_routes_timeout_chk CHECK (timeout_seconds > 0),
    CONSTRAINT inbound_routes_match_uniq UNIQUE (action, match_type, match_value)
);

CREATE INDEX IF NOT EXISTS inbound_routes_match_idx
    ON inbound_routes (status, match_type, match_value);

-- Backfill existing webhook rules as webhook routes. Idempotent: the NOT EXISTS
-- guard (and the unique constraint) prevent duplicates if this runs again.
INSERT INTO inbound_routes
    (name, match_type, match_value, action, status,
     destination_url, secret_ref, timeout_seconds, retry_policy, created_at, updated_at)
SELECT w.name, w.match_type, w.match_value, 'webhook', w.status,
       w.destination_url, w.secret_ref, w.timeout_seconds, w.retry_policy, w.created_at, w.updated_at
FROM webhook_rules w
WHERE NOT EXISTS (
    SELECT 1 FROM inbound_routes ir
    WHERE ir.action = 'webhook'
      AND ir.match_type = w.match_type
      AND ir.match_value = w.match_value
);

-- Deployment-wide Maildir base. Routes with an empty maildir_path land under
-- <base>/<domain>/<local-part> via kumod's maildir_path template expansion.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS inbound_maildir_base_path TEXT NOT NULL DEFAULT '/var/spool/iris/maildirs';
