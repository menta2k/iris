-- 0005_routing_match_header.sql
-- A mailclass match is a configurable header NAME + VALUE pair (e.g.
-- "X-Mail-Class: bulk"). Add the header-name column so a mailclass routing rule
-- can name the header it matches on, instead of assuming a single fixed header.
-- match_value continues to hold the value the header must equal.

ALTER TABLE routing_rules
    ADD COLUMN IF NOT EXISTS match_header TEXT NOT NULL DEFAULT '';

-- The deterministic-priority uniqueness must account for the header name too:
-- two mailclass rules can share a value+priority if they match different
-- headers. Recreate the partial unique index to include match_header.
DROP INDEX IF EXISTS routing_rules_active_priority_uniq;
CREATE UNIQUE INDEX IF NOT EXISTS routing_rules_active_priority_uniq
    ON routing_rules (match_type, match_header, match_value, priority)
    WHERE status = 'active';
