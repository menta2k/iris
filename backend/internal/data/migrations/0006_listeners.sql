-- 0006_listeners.sql
-- Introduce ESMTP Listeners. A listener owns the IP + port + EHLO/banner
-- hostname (and TLS/relay settings). A VMTA attaches to a listener and takes
-- its outbound egress source IP and EHLO from it, so the IP/EHLO move OFF the
-- VMTA. The VMTA gains a per-source max_connections.

CREATE TABLE IF NOT EXISTS listeners (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL UNIQUE,
    ip_address       INET NOT NULL,
    port             INTEGER NOT NULL DEFAULT 25,
    hostname         TEXT NOT NULL,
    tls_enabled      BOOLEAN NOT NULL DEFAULT false,
    tls_cert_path    TEXT NOT NULL DEFAULT '',
    tls_key_path     TEXT NOT NULL DEFAULT '',
    require_auth     BOOLEAN NOT NULL DEFAULT false,
    max_message_size BIGINT NOT NULL DEFAULT 0,
    relay_hosts      TEXT[] NOT NULL DEFAULT '{}',
    status           TEXT NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT listeners_status_chk CHECK (status IN ('active', 'disabled')),
    CONSTRAINT listeners_port_chk CHECK (port > 0 AND port <= 65535)
);

-- One active listener per IP:port bind.
CREATE UNIQUE INDEX IF NOT EXISTS listeners_active_bind_uniq
    ON listeners (ip_address, port) WHERE status = 'active';

-- VMTA gains a listener reference + max_connections.
ALTER TABLE vmtas
    ADD COLUMN IF NOT EXISTS listener_id     UUID REFERENCES listeners(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS max_connections INTEGER NOT NULL DEFAULT 0;

-- Migrate existing VMTAs: synthesize a listener from each VMTA's ip/ehlo and
-- link it, so no configuration is lost. Guarded so the migration is a no-op
-- when the legacy columns are already gone.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'vmtas' AND column_name = 'ip_address'
    ) THEN
        INSERT INTO listeners (name, ip_address, port, hostname)
        SELECT name || '-listener', ip_address, 25,
               coalesce(nullif(ehlo_name, ''), name || '.local')
        FROM vmtas
        ON CONFLICT (name) DO NOTHING;

        UPDATE vmtas v SET listener_id = l.id
        FROM listeners l
        WHERE l.name = v.name || '-listener' AND v.listener_id IS NULL;

        DROP INDEX IF EXISTS vmtas_active_ip_uniq;
        ALTER TABLE vmtas DROP COLUMN IF EXISTS ip_address;
        ALTER TABLE vmtas DROP COLUMN IF EXISTS ehlo_name;
    END IF;
END
$$;
