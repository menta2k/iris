-- 0019_mail_record_from_header.sql
-- Capture the original From header on mail events. The envelope sender is
-- VERP-rewritten at reception (b+<mac>.<id>@<bounce-domain>), so `sender` no
-- longer reflects who the mail was from. from_header preserves the original
-- From for display and search in the Logs UI.

ALTER TABLE mail_records
    ADD COLUMN IF NOT EXISTS from_header TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS mail_records_from_header_idx
    ON mail_records (from_header, event_time DESC);
