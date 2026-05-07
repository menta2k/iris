-- Extend global_settings with the admin HTTPS knobs. Idempotent —
-- ADD COLUMN IF NOT EXISTS lets us run on a fresh DB (which may have
-- the columns from ent's first schema diff already) and on an
-- already-migrated 0006 DB equally.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS https_listen        VARCHAR(64),
    ADD COLUMN IF NOT EXISTS https_cert_pem_path VARCHAR(1024),
    ADD COLUMN IF NOT EXISTS https_key_pem_path  VARCHAR(1024);
