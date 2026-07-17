-- IPv4-only outbound: when on, the generated egress path prohibits ::/0 so kumod
-- skips IPv6 MX hosts and delivers over IPv4. Default off.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS ipv4_only boolean NOT NULL DEFAULT false;
