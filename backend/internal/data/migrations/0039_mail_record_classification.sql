-- Subject classification label on mail events (optional feature; off by default).
-- Only the short (<= 2 words) label is stored here; the raw subject is never
-- persisted on mail_records — it lives solely in subject_classifications. The
-- classification worker backfills this column asynchronously by message_id.
ALTER TABLE mail_records
    ADD COLUMN IF NOT EXISTS classification TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS mail_records_classification_idx
    ON mail_records (classification, event_time DESC);
