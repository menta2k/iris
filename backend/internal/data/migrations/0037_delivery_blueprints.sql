-- 0037_delivery_blueprints.sql
-- Delivery Blueprints: operator-managed base traffic-shaping rules per receiving
-- MX pattern. They are the default egress-path limits a new/unknown IP starts
-- from and falls back to, rendered into the base shaping file (iris-base.toml).
-- The warmup engine and TSA layer per-IP overrides on top.
CREATE TABLE IF NOT EXISTS delivery_blueprints (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider            TEXT NOT NULL,              -- display/rollup group, e.g. "Gmail"
    mx_pattern          TEXT NOT NULL,              -- receiving domain, e.g. "google.com"
    conn_rate           TEXT NOT NULL DEFAULT '',   -- max_connection_rate, e.g. "5/min"
    deliveries_per_conn INTEGER NOT NULL DEFAULT 0, -- max_deliveries_per_connection
    conn_limit          INTEGER NOT NULL DEFAULT 0, -- connection_limit (default)
    daily_cap           INTEGER NOT NULL DEFAULT 0, -- base max_message_rate, messages/day
    status              TEXT NOT NULL DEFAULT 'active',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT delivery_blueprints_mx_uniq UNIQUE (mx_pattern),
    CONSTRAINT delivery_blueprints_status_chk CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS delivery_blueprints_provider_idx ON delivery_blueprints (provider);
