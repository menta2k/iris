-- 0044_bounce_action_rules.sql
-- Operator-manageable bounce classification & response rules. Each rule maps a
-- bounce signature (SMTP code + enhanced status code + provider + diagnostic
-- pattern; empty fields are wildcards) to a category and a system action:
--   retry          — informational (KumoMTA's default exponential backoff)
--   throttle       — compiled into a TSA set_config back-off for the destination
--   suspend_domain — compiled into a TSA suspend for the destination
--   suppress       — the log-stream worker adds the recipient to the suppression list
-- source = 'default' rows are the seeded starter set ("Reset to defaults");
-- 'overlay' rows are operator-authored and layer on top.
CREATE TABLE IF NOT EXISTS bounce_action_rules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    smtp_code        TEXT NOT NULL DEFAULT '',
    enhanced_code    TEXT NOT NULL DEFAULT '',
    provider         TEXT NOT NULL DEFAULT '',
    pattern          TEXT NOT NULL DEFAULT '',
    class            TEXT NOT NULL DEFAULT 'soft',
    category         TEXT NOT NULL DEFAULT '',
    action           TEXT NOT NULL,
    action_config    TEXT NOT NULL DEFAULT '',
    suggested_action TEXT NOT NULL DEFAULT '',
    priority         INTEGER NOT NULL DEFAULT 0,
    source           TEXT NOT NULL DEFAULT 'overlay',
    status           TEXT NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT bounce_action_rules_action_chk CHECK (action IN ('retry', 'throttle', 'suspend_domain', 'suppress')),
    CONSTRAINT bounce_action_rules_class_chk CHECK (class IN ('soft', 'hard')),
    CONSTRAINT bounce_action_rules_source_chk CHECK (source IN ('default', 'overlay')),
    CONSTRAINT bounce_action_rules_status_chk CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS bounce_action_rules_lookup_idx ON bounce_action_rules (status, priority DESC);
