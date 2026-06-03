-- Add the egress_ehlo_domain column to global_settings on existing
-- databases. ent's auto-migrate adds it on fresh installs, but its
-- transactional schema diff can abort on the pre-existing audit_entry
-- TS103 quirk (same rationale as 0006/0010), so we add the column
-- idempotently here as a backstop. Type mirrors the ent field
-- (String, MaxLen 253 -> VARCHAR(253)).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS egress_ehlo_domain VARCHAR(253);
