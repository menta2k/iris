-- 0032_rspamd_message_id.sql
-- Capture the KumoMTA message id with each rspamd verdict so the Rspamd Results
-- page can correlate a result to the mail log (and resolve the recipient), even
-- though scanning happens at reception before the Reception record is ingested.

ALTER TABLE rspamd_filter_results
    ADD COLUMN IF NOT EXISTS message_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS rspamd_results_message_id_idx
    ON rspamd_filter_results (message_id);
