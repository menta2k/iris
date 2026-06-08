-- Add rspamd inbound spam-filtering config to global_settings on existing
-- databases. ent's auto-migrate adds them on fresh installs, but its
-- transactional diff can abort on the pre-existing audit_entry TS103 quirk
-- (same rationale as 0006/0010-0014), so we add them idempotently here.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS rspamd_mode VARCHAR(16),
    ADD COLUMN IF NOT EXISTS rspamd_url  VARCHAR(512);
