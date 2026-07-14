-- 0065_vmta_node.sql
-- VMTA node ownership (P2 of cluster support): a VMTA's egress IP is bound on
-- exactly one cluster node. NULL = the local/legacy co-located node. RESTRICT
-- keeps a node from being deleted while VMTAs still live on it (reassign or
-- delete the VMTAs first).
ALTER TABLE vmtas ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES mta_nodes(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS vmtas_node_idx ON vmtas (node_id);
