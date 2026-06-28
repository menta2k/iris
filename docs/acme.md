# ACME / TLS certificates

Iris can obtain and renew TLS certificates from an ACME provider (Let's Encrypt)
for your [listener](listeners.md) hostnames and for the Iris admin UI itself.

UI: **KumoMTA → TLS Certificates** (`service:control`).

## Challenge types

- **HTTP-01** — Iris answers the challenge on an HTTP bind. Enable it with
  `IRIS_ACME_HTTP_BIND` (set to a bind like `:80`; `off` disables the responder).
  The `acme-challenge` worker serves the token.
- **DNS-01** — for wildcard certificates and hosts you cannot expose on `:80`,
  via a DNS provider registry (provider credentials configured per the ACME
  setup). DNS-01 supports wildcard (`*.example.com`) domains.

## Issuance and storage

Issue a certificate for a domain from the UI. Issued PEMs are mirrored to
`IRIS_ACME_CERT_DIR` (default `/opt/kumomta/etc/tls`), which listener TLS paths
and the admin server reference. Wildcard `*` is sanitized in cert directory
names.

## Auto-renewal

The `acme-renewer` worker renews certificates on a schedule:

- `acme_renew_interval` (global setting) / `IRIS_ACME_RENEW_INTERVAL` — how often
  to check (default 12h).
- `acme_renew_before` / `IRIS_ACME_RENEW_BEFORE` — renew this far before expiry
  (default 30 days).

## Using a cert for the Iris admin UI

The Iris admin server can serve HTTPS using an issued certificate: set
`admin_tls_enabled` and `admin_tls_cert_domain` (global settings). If the cert
cannot be loaded at startup, Iris logs the problem and **falls back to plain
HTTP** rather than failing to boot. Admin server changes apply on restart.

## Related

- [Listeners](listeners.md) — inbound TLS
- [Require TLS](tls-policies.md) — outbound TLS to remote domains
- [Configuration](configuration.md)
