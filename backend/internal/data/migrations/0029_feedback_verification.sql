-- 0029_feedback_verification.sql
-- FBL provenance verification: record whether an ARF complaint was proven to be
-- about mail we actually sent (via kumod's X-KumoRef trace, the send log, or a
-- DKIM signature verified against our own keys), and add a deployment toggle that
-- gates auto-suppression on that proof. Default permissive: suppression behaves
-- as before until fbl_require_verification is turned on.

ALTER TABLE feedback_reports
    ADD COLUMN IF NOT EXISTS verified     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS verification TEXT    NOT NULL DEFAULT '';

ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS fbl_require_verification BOOLEAN NOT NULL DEFAULT false;
