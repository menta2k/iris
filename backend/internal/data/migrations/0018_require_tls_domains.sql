-- 0018_require_tls_domains.sql
-- Outbound require-TLS policy: destination domains that must be delivered over
-- TLS (enable_tls=Required on the egress path). When kumod cannot negotiate
-- STARTTLS to such a domain the delivery fails rather than being sent in the
-- clear. The mode selects certificate strictness.

CREATE TABLE IF NOT EXISTS require_tls_domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain     TEXT NOT NULL,
    mode       TEXT NOT NULL DEFAULT 'required',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT require_tls_domains_domain_uniq UNIQUE (domain),
    CONSTRAINT require_tls_domains_mode_chk
        CHECK (mode IN ('required', 'required_insecure')),
    CONSTRAINT require_tls_domains_status_chk
        CHECK (status IN ('active', 'disabled'))
);
