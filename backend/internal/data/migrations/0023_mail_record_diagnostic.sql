-- 0023_mail_record_diagnostic.sql
-- Capture the SMTP response on each mail event so the Logs UI can show WHY a
-- message deferred (TransientFailure) or what the server said on delivery/bounce.
-- Previously only Bounce records carried a diagnostic (bounce_records), leaving
-- deferrals reasonless in the UI.
ALTER TABLE mail_records
    ADD COLUMN IF NOT EXISTS smtp_status TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS diagnostic  TEXT NOT NULL DEFAULT '';
