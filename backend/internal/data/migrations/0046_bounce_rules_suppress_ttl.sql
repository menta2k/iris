-- Per-rule suppression lifetime: when a suppress rule fires, how long the
-- recipient stays suppressed (KumoMTA duration form, e.g. "30d"). Empty uses the
-- global suppression TTL. Lets invalid-recipient suppress permanently while a
-- full-mailbox suppress lapses after a while so the address can be retried later.
ALTER TABLE bounce_action_rules
    ADD COLUMN IF NOT EXISTS suppress_ttl TEXT NOT NULL DEFAULT '';
