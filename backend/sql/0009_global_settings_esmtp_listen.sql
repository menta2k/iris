-- Add esmtp_listen_addr to global_settings so the default
-- kumo.start_esmtp_listener block's bind is operator-editable from
-- the Global Settings page (alongside the relay_hosts field). Only
-- consulted when no Listener rows exist; per-listener entries on the
-- Listeners page already render their own blocks and override the
-- default.
--
-- Idempotent: ADD COLUMN IF NOT EXISTS lets us run on a fresh DB
-- (where ent's first schema diff already created the column) and on
-- an already-migrated 0008 DB equally.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS esmtp_listen_addr VARCHAR(128);
