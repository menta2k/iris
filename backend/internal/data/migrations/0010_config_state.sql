-- Tracks the checksum of the KumoMTA policy last successfully applied, so the UI
-- can warn when the current configuration has drifted (changes made but not yet
-- regenerated/applied). Singleton row (id = 1).
CREATE TABLE IF NOT EXISTS config_state (
    id               INTEGER PRIMARY KEY DEFAULT 1,
    applied_checksum TEXT NOT NULL DEFAULT '',
    applied_at       TIMESTAMPTZ,
    applied_by       UUID,
    CONSTRAINT config_state_singleton CHECK (id = 1)
);
INSERT INTO config_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;
