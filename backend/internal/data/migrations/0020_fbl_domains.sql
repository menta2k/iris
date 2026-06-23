-- 0020_fbl_domains.sql
-- Promote the single FBL/ARF feedback domain to a list so multiple domains can
-- be ARF candidates at once. Each stored domain is lower-cased on write by the
-- application; kumod matches inbound domains case-insensitively via the
-- rendered FBL_DOMAINS table.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS fbl_domains TEXT[] NOT NULL DEFAULT '{}';

-- Backfill: carry any existing single FBL domain into the new array exactly once.
UPDATE global_settings
SET fbl_domains = ARRAY[lower(trim(fbl_domain))]
WHERE fbl_domain <> ''
  AND fbl_domains = '{}';

ALTER TABLE global_settings
    DROP COLUMN IF EXISTS fbl_domain;
