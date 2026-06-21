-- ACME (Let's Encrypt) certificate management.

-- Singleton operator ACME account. The account key + registration are secrets;
-- protect the DB volume accordingly.
CREATE TABLE IF NOT EXISTS acme_account (
    id                INTEGER PRIMARY KEY DEFAULT 1,
    email             TEXT NOT NULL DEFAULT '',
    server_url        TEXT NOT NULL DEFAULT '',
    registration_json TEXT NOT NULL DEFAULT '',
    private_key_pem   TEXT NOT NULL DEFAULT '',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT acme_account_singleton CHECK (id = 1)
);
INSERT INTO acme_account (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- Issued / in-flight certificates. One row per domain.
CREATE TABLE IF NOT EXISTS acme_certificate (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain           TEXT NOT NULL UNIQUE,
    alt_names        TEXT[] NOT NULL DEFAULT '{}',
    challenge_type   TEXT NOT NULL DEFAULT 'http-01',
    cert_pem         TEXT NOT NULL DEFAULT '',
    key_pem          TEXT NOT NULL DEFAULT '',
    cert_path        TEXT NOT NULL DEFAULT '',
    key_path         TEXT NOT NULL DEFAULT '',
    expires_at       TIMESTAMPTZ,
    last_renewed_at  TIMESTAMPTZ,
    status           TEXT NOT NULL DEFAULT 'pending',
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT acme_certificate_status_chk
        CHECK (status IN ('pending', 'issued', 'renewing', 'failed'))
);
CREATE INDEX IF NOT EXISTS acme_certificate_renew_idx ON acme_certificate (status, expires_at);
