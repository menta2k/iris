-- 0013_routing_sender_ip.sql
-- Add a sender-IP classification rule type. A routing rule with
-- match_type='sender_ip' matches the connecting client's IP (match_value is an
-- IP or CIDR) and ASSIGNS a mailclass (assign_mailclass) to mail that carries
-- no mailclass header. Such rules have no VMTA/group target — delivery follows
-- the existing mailclass rule for the assigned class.

ALTER TABLE routing_rules
    ADD COLUMN IF NOT EXISTS assign_mailclass TEXT NOT NULL DEFAULT '';

-- Allow the new match type.
ALTER TABLE routing_rules DROP CONSTRAINT IF EXISTS routing_rules_match_type_chk;
ALTER TABLE routing_rules ADD CONSTRAINT routing_rules_match_type_chk
    CHECK (match_type IN ('mailclass', 'recipient_email', 'recipient_domain', 'sender_ip'));

-- sender_ip rules have no target, so target is now optional. Keep the value
-- domain constrained but permit an empty target_type for targetless rules.
ALTER TABLE routing_rules ALTER COLUMN target_id DROP NOT NULL;
ALTER TABLE routing_rules ALTER COLUMN target_type DROP NOT NULL;
ALTER TABLE routing_rules DROP CONSTRAINT IF EXISTS routing_rules_target_type_chk;
ALTER TABLE routing_rules ADD CONSTRAINT routing_rules_target_type_chk
    CHECK (target_type IS NULL OR target_type IN ('', 'vmta', 'vmta_group'));
