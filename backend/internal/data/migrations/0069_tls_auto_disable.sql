-- Global toggle: let the log processor auto-add a Disabled TLS policy for a
-- destination domain after delivery to it fails a STARTTLS handshake (e.g. a
-- DHE-only server kumod's rustls cannot negotiate). Default off (opt-in).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS tls_auto_disable boolean NOT NULL DEFAULT false;
