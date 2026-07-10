-- Per-VMTA outbound TLS policy: force STARTTLS (or relax/disable it) for any
-- egress path that sends from this VMTA, regardless of destination. Empty means
-- "no override" — the per-domain TLS Policy or the opportunistic default applies.
-- A per-domain TLS Policy still takes precedence over the VMTA setting.
ALTER TABLE vmtas
    ADD COLUMN IF NOT EXISTS tls_mode TEXT NOT NULL DEFAULT '';

ALTER TABLE vmtas DROP CONSTRAINT IF EXISTS vmtas_tls_mode_chk;
ALTER TABLE vmtas
    ADD CONSTRAINT vmtas_tls_mode_chk
    CHECK (tls_mode IN ('', 'required', 'required_insecure', 'opportunistic_insecure', 'disabled'));
