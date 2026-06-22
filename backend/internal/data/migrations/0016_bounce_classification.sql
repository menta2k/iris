-- 0016_bounce_classification.sql
-- KumoMTA's bounce classifier (configure_bounce_classifier) labels each bounce
-- with a category (InvalidRecipient, SpamBlock, QuotaIssue, …). Store it so the
-- Bounces UI can show it and suppression can act on it.
ALTER TABLE bounce_records
    ADD COLUMN IF NOT EXISTS classification TEXT NOT NULL DEFAULT '';
