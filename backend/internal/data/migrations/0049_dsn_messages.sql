-- Raw asynchronous bounce (DSN) messages captured at the bounce domain, retained
-- so an operator can inspect the full notification behind a dsn-sourced
-- suppression. Keyed by the resolved recipient (the suppression value).
CREATE TABLE IF NOT EXISTS dsn_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient   TEXT NOT NULL,
    message_id  TEXT NOT NULL DEFAULT '',
    raw_message TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dsn_messages_recipient
    ON dsn_messages (recipient, received_at DESC);
