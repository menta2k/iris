-- 0031_inbound_route_spam_scan.sql
-- Per-route spam scanning: each inbound route can override the deployment-wide
-- rspamd mode for its own captured mail. "default" follows the global mode;
-- "off"/"tag"/"enforce" override it (honored only when an rspamd URL is set).

ALTER TABLE inbound_routes
    ADD COLUMN IF NOT EXISTS spam_scan TEXT NOT NULL DEFAULT 'default';

ALTER TABLE inbound_routes
    DROP CONSTRAINT IF EXISTS inbound_routes_spam_scan_chk;
ALTER TABLE inbound_routes
    ADD CONSTRAINT inbound_routes_spam_scan_chk
        CHECK (spam_scan IN ('default', 'off', 'tag', 'enforce'));
