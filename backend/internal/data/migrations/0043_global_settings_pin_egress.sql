-- Pin a message to one egress IP across retries (deterministic per-message
-- source selection) instead of KumoMTA's per-attempt weighted round-robin.
-- Default false preserves the round-robin behavior.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS pin_egress_per_message BOOLEAN NOT NULL DEFAULT false;
