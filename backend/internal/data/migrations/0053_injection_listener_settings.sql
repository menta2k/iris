-- Operator-configurable injection listener (GreenArrow-compatible API), added
-- to the global_settings singleton. Applied at startup — a service restart picks
-- up changes; injection_tls_cert_domain selects an issued ACME certificate to
-- serve HTTPS with. Credentials are managed separately (injection_credentials).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS injection_enabled         BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS injection_listen_addr     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS injection_path            TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS injection_tls_enabled     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS injection_tls_cert_domain TEXT    NOT NULL DEFAULT '';
