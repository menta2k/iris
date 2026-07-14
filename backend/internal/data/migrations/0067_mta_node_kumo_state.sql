-- 0067_mta_node_kumo_state.sql
-- Live kumod state per cluster node (running/degraded/unreachable/unknown),
-- refreshed by the cluster-health heartbeat worker so the Cluster page shows
-- current node health instead of apply-time snapshots.
ALTER TABLE mta_nodes ADD COLUMN IF NOT EXISTS kumo_state TEXT NOT NULL DEFAULT '';
