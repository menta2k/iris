-- Add the outbound retry/age columns to global_settings on existing
-- databases. ent's auto-migrate adds them on fresh installs, but its
-- transactional schema diff can abort on the pre-existing audit_entry
-- TS103 quirk (same rationale as 0006/0010/0011), so we add them
-- idempotently here. Types mirror the ent fields (String, MaxLen 32).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS egress_retry_interval     VARCHAR(32),
    ADD COLUMN IF NOT EXISTS egress_max_retry_interval VARCHAR(32),
    ADD COLUMN IF NOT EXISTS egress_max_age            VARCHAR(32);
