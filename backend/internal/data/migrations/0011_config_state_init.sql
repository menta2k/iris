-- Track the init-block checksum of the last applied policy separately. Changes
-- to the init block (listeners, spool, log hook) require a KumoMTA restart, not
-- just a reload, so Apply compares this to decide reload vs restart.
ALTER TABLE config_state
    ADD COLUMN IF NOT EXISTS applied_init_checksum TEXT NOT NULL DEFAULT '';
