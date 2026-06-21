-- 0014_acme_dns_provider.sql
-- Singleton config for the DNS-01 challenge provider used by ACME. provider is
-- the registry key (cloudflare, route53, …); config_json is a JSON object of
-- operator-supplied credentials/tunables (provider-specific keys, described by
-- the in-code registry — no schema change needed to add a provider). Empty
-- provider means DNS-01 is not configured (issuance falls back to HTTP-01).
CREATE TABLE IF NOT EXISTS acme_dns_provider (
    id          INTEGER PRIMARY KEY DEFAULT 1,
    provider    TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  TEXT NOT NULL DEFAULT '',
    CONSTRAINT acme_dns_provider_singleton CHECK (id = 1)
);

INSERT INTO acme_dns_provider (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
