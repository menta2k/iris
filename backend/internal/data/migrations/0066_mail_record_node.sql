-- 0066_mail_record_node.sql
-- Cluster observability (P3): record which node received/queued the message.
-- Stamped by the policy from the per-node identity prelude (iris_node.lua) and
-- carried in the log record's meta. Empty on pre-cluster rows and on synthetic
-- records from nodes without a prelude.
ALTER TABLE mail_records ADD COLUMN IF NOT EXISTS node TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS mail_records_node_idx
    ON mail_records (node, event_time DESC)
    WHERE node <> '';
