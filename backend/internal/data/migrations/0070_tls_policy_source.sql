-- Provenance for require-TLS policies: 'manual' (operator-added, the default and
-- backfill for existing rows) or 'auto' (added by the log processor when a domain
-- failed a STARTTLS handshake), so the UI can flag auto-added entries.
ALTER TABLE require_tls_domains
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
