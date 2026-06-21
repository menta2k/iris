-- Feedback (FBL/ARF) domain: when set, the generated KumoMTA policy enables
-- log_arf for this listener domain so kumod parses inbound ARF reports and emits
-- Feedback log records, which the feedback consumer ingests (auto-suppression).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS fbl_domain TEXT NOT NULL DEFAULT '';
