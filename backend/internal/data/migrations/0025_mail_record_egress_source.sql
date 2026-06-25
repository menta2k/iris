-- KumoMTA Delivery/Bounce/TransientFailure log records carry the egress source
-- (the sending VMTA's name) that handled the attempt; Reception records do not.
-- We denormalize it onto the event row (like recipient_domain) so the Logs UI can
-- show which VMTA sent each message without resolving the UUID FK. The existing
-- vmta_id UUID column is left for the continuous aggregates.
ALTER TABLE mail_records ADD COLUMN IF NOT EXISTS egress_source TEXT NOT NULL DEFAULT '';
