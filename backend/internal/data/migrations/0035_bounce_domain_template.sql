-- 0035_bounce_domain_template.sql
-- Per-sending-domain bounce (VERP return-path) domains.
--
-- The bounce domain is the envelope MAIL FROM used for outbound mail; async DSNs
-- come back to it. When it lives under a different organizational domain than the
-- From header (e.g. From @example.com, return-path @bounce.kumo.example.com), SPF can
-- never align, so DMARC rests entirely on DKIM. A single broken signature is then
-- quarantined with no SPF fallback.
--
-- bounce_domain_template, when set, derives an aligned bounce domain per sending
-- (DKIM) domain by substituting "{domain}" — e.g. "bounce.kumo.{domain}" makes
-- mail from @example.com use @bounce.kumo.example.com, which aligns under example.com and
-- gives SPF as a DMARC fallback. Empty preserves the single global bounce_domain.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS bounce_domain_template TEXT NOT NULL DEFAULT '';
