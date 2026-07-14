-- 0064_mta_nodes.sql
-- KumoMTA cluster node registry (P1 of cluster support, see
-- docs/kumomta-cluster-architecture.md).
--
-- A row describes one KumoMTA host iris controls. A node without an agent_url
-- is the legacy co-located node managed through the local file/reload
-- transport; a node with an agent_url is managed remotely through the
-- mTLS-authenticated iris-agent. VMTAs gain node ownership in a later phase.
CREATE TABLE IF NOT EXISTS mta_nodes (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    -- Base URL of the node's iris-agent (https://host:port). Empty = local
    -- transport (config written to the filesystem of the iris host itself).
    agent_url        TEXT NOT NULL DEFAULT '',
    -- kumo-proxy endpoint on the node's private cluster network; used when
    -- rendering egress sources whose VMTA lives on this node. Empty until the
    -- node exposes egress IPs.
    proxy_host       TEXT NOT NULL DEFAULT '',
    proxy_port       INTEGER NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'active',
    -- SHA-256 fingerprint of the agent's enrolled client certificate; pins the
    -- identity iris accepts for this node. Empty until enrollment completes.
    cert_fingerprint TEXT NOT NULL DEFAULT '',
    -- Reported by the agent on each heartbeat/apply.
    version           TEXT NOT NULL DEFAULT '',
    applied_checksum  TEXT NOT NULL DEFAULT '',
    last_seen_at      TIMESTAMPTZ,
    notes             TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mta_nodes_status_chk
        CHECK (status IN ('active', 'disabled', 'draining')),
    CONSTRAINT mta_nodes_proxy_port_chk
        CHECK (proxy_port >= 0 AND proxy_port <= 65535)
);

-- One-time enrollment tokens for bootstrapping a node's agent certificate.
-- The token value is stored bcrypt-hashed; the plaintext is shown once at
-- issuance. A token is bound to a node row and single-use.
CREATE TABLE IF NOT EXISTS mta_node_enroll_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id     UUID NOT NULL REFERENCES mta_nodes(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS mta_node_enroll_tokens_node_idx
    ON mta_node_enroll_tokens (node_id);
