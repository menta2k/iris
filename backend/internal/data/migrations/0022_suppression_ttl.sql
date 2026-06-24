-- 0022_suppression_ttl.sql
-- Move suppression enforcement out of the rendered KumoMTA config (which grew
-- one Lua line per entry) and into Redis, keyed per address with a TTL so the
-- list self-ages. Postgres remains the source of truth / audit / UI list.
--
-- expires_at records when an entry should stop blocking (NULL = permanent). The
-- live policy lookup uses Redis key TTLs; this column keeps the DB list and the
-- DB-side IsSuppressed check consistent with that aging.
ALTER TABLE suppression_entries
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS suppression_entries_expires_idx
    ON suppression_entries (expires_at)
    WHERE expires_at IS NOT NULL;

-- suppression_ttl is the operator-configured lifetime applied to suppression
-- records (Go/KumoMTA duration form, e.g. "720h", "30d"). Empty = permanent.
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS suppression_ttl TEXT NOT NULL DEFAULT '';
