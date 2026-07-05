-- Store the full applied KumoMTA policy content (not just its checksum) so the
-- UI can diff the pending (regenerated) policy against the running one.
ALTER TABLE config_state
    ADD COLUMN IF NOT EXISTS applied_content TEXT NOT NULL DEFAULT '';
