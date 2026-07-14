-- 0068_listener_node.sql
-- Node-aware listener binds: a listener may be pinned to a specific cluster
-- node so every node can accept submission on its own address from ONE
-- identical policy. NULL node_id = bind on every node (the pre-cluster
-- behavior: a single shared bind, typically 0.0.0.0 or a floating IP). A
-- node-pinned listener renders inside an `if NODE_NAME == '<node>'` guard, so
-- only that node starts it. RESTRICT (matching vmtas.node_id) keeps a node from
-- being deleted while it still owns listeners.
ALTER TABLE listeners ADD COLUMN IF NOT EXISTS node_id UUID REFERENCES mta_nodes(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS listeners_node_idx ON listeners (node_id);
