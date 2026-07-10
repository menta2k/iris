-- Per-probe fetch backoff: track how many mailbox-fetch attempts a probe has had
-- and when the next attempt is eligible, so iris backs off exponentially instead
-- of hammering a rate-limiting IMAP/POP3 server every minute (e.g. abv.bg POP3
-- resetting connections from the sending-MTA IP).
ALTER TABLE monitoring_probes
    ADD COLUMN IF NOT EXISTS fetch_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_fetch_at   TIMESTAMPTZ;
