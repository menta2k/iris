-- 0015_admin_server_settings.sql
-- Operator-configurable Iris admin server (HTTP/HTTPS) and ACME auto-renew
-- schedule, added to the global_settings singleton. The admin server fields are
-- applied at startup (a service restart picks up changes); admin_tls_cert_domain
-- selects an issued ACME certificate to serve HTTPS with.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS admin_http_addr       TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS admin_tls_enabled     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS admin_tls_cert_domain TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS acme_renew_interval   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS acme_renew_before     TEXT    NOT NULL DEFAULT '';
