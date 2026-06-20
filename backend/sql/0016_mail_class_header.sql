-- Add the per-class header_name / header_value columns to mail_classes on
-- existing databases. ent's auto-migrate adds them on fresh installs, but on
-- the TimescaleDB-converted prod DB its transactional schema diff aborts on
-- the pre-existing audit_entry TS103 quirk (same rationale as 0006/0010/0011)
-- before reaching this ALTER, so we add the columns idempotently here as the
-- reliable path. Types/nullability mirror the ent fields (String, MaxLen 128 /
-- 256, Optional -> nullable VARCHAR).
ALTER TABLE mail_classes
    ADD COLUMN IF NOT EXISTS header_name VARCHAR(128);
ALTER TABLE mail_classes
    ADD COLUMN IF NOT EXISTS header_value VARCHAR(256);
