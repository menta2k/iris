-- Mail provider (inbox-placement) monitoring.
--
-- monitoring_accounts are mailboxes (IMAP/POP3) at a provider that iris sends
-- probe mail to and later inspects. The mailbox password is stored reversibly
-- encrypted (AES-GCM, key from IRIS_MONITORING_KEY) because the fetch worker
-- must present it to the IMAP/POP3 server.
CREATE TABLE IF NOT EXISTS monitoring_accounts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label             TEXT NOT NULL,
    provider          TEXT NOT NULL DEFAULT 'custom',   -- gmail|outlook|yahoo|custom
    email             TEXT NOT NULL,                    -- the mailbox address (probe recipient)
    protocol          TEXT NOT NULL DEFAULT 'imap',     -- imap|pop3
    host              TEXT NOT NULL DEFAULT '',
    port              INTEGER NOT NULL DEFAULT 993,
    tls               BOOLEAN NOT NULL DEFAULT true,
    username          TEXT NOT NULL DEFAULT '',         -- often = email
    password_enc      TEXT NOT NULL DEFAULT '',         -- AES-GCM ciphertext, base64
    check_folders     TEXT[] NOT NULL DEFAULT ARRAY['INBOX'], -- folders searched in phase 2
    from_address      TEXT NOT NULL DEFAULT '',         -- envelope/From used for probes
    -- recurring schedule (probe sent automatically every interval)
    schedule_enabled  BOOLEAN NOT NULL DEFAULT false,
    schedule_interval TEXT NOT NULL DEFAULT '',         -- duration form, e.g. "6h"
    fetch_delay       TEXT NOT NULL DEFAULT '10m',      -- wait before the mailbox fetch (phase 2)
    enabled           BOOLEAN NOT NULL DEFAULT true,
    last_probe_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT monitoring_accounts_protocol_chk CHECK (protocol IN ('imap', 'pop3'))
);

-- One probe = one test message sent to a monitored account, carrying a unique
-- identifier (probe_uid) so it can be located in the mailbox later.
CREATE TABLE IF NOT EXISTS monitoring_probes (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id     UUID NOT NULL REFERENCES monitoring_accounts(id) ON DELETE CASCADE,
    probe_uid      TEXT NOT NULL UNIQUE,               -- embedded in X-Iris-Probe-Id + subject
    message_id     TEXT NOT NULL DEFAULT '',           -- KumoMTA message id (for send correlation)
    subject        TEXT NOT NULL DEFAULT '',
    from_addr      TEXT NOT NULL DEFAULT '',
    recipient      TEXT NOT NULL DEFAULT '',
    sent_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- phase 1: KumoMTA send outcome.
    send_status    TEXT NOT NULL DEFAULT 'queued',      -- queued|sent|deferred|bounced|error
    -- phase 2: mailbox fetch outcome.
    mailbox_status TEXT NOT NULL DEFAULT 'pending',     -- pending|found|not_found|timeout|skipped
    placement      TEXT NOT NULL DEFAULT '',            -- inbox|spam|missing|unknown
    found_at       TIMESTAMPTZ,
    latency_ms     BIGINT,
    -- phase 3: header analysis.
    analysis       JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_headers    TEXT NOT NULL DEFAULT '',
    error          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS monitoring_probes_account_idx ON monitoring_probes (account_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS monitoring_probes_message_idx ON monitoring_probes (message_id);
