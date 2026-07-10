-- Allow the relaxing TLS modes (opportunistic_insecure, disabled) on the
-- per-domain TLS policy, alongside the require modes. Disabled delivers in
-- cleartext to receivers whose broken/legacy certificate kumod cannot negotiate
-- (UnsupportedCertVersion / BadSignature), instead of deferring then bouncing.
ALTER TABLE require_tls_domains DROP CONSTRAINT IF EXISTS require_tls_domains_mode_chk;
ALTER TABLE require_tls_domains
    ADD CONSTRAINT require_tls_domains_mode_chk
    CHECK (mode IN ('required', 'required_insecure', 'opportunistic_insecure', 'disabled'));
