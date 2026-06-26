-- 0028_listener_role.sql
-- Make a listener's role explicit: 'inbound' (an MX that accepts mail for local
-- domains and relays for no one but loopback) vs 'submission' (authorized senders
-- relay outbound through it). The role is kept consistent with the relay
-- allowlist: inbound => no relay hosts, submission => at least one.

ALTER TABLE listeners ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'inbound';

-- Backfill from the existing relay allowlist so every row is self-consistent on
-- upgrade: a listener that already trusts relay hosts is a submission listener.
UPDATE listeners
SET role = CASE WHEN coalesce(array_length(relay_hosts, 1), 0) > 0
                THEN 'submission' ELSE 'inbound' END;

ALTER TABLE listeners DROP CONSTRAINT IF EXISTS listeners_role_chk;
ALTER TABLE listeners ADD CONSTRAINT listeners_role_chk
    CHECK (role IN ('inbound', 'submission'));
