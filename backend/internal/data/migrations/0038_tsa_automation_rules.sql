-- 0038_tsa_automation_rules.sql
-- Operator-authored KumoMTA Traffic Shaping Automation rules. iris renders the
-- active rules into iris-automation.toml as [["<domain>".automation]] blocks,
-- which the TSA daemon evaluates against live delivery events to compute
-- reactive back-off (suspend / tighten a limit) layered under the warmup ceiling.
-- (`trigger` is a reserved word in Postgres, hence trigger_spec.)
CREATE TABLE IF NOT EXISTS tsa_automation_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain       TEXT NOT NULL,              -- receiving MX pattern, or "default"
    regex        TEXT NOT NULL,              -- SMTP response pattern
    action       TEXT NOT NULL,              -- suspend | suspend_tenant | set_config
    config_name  TEXT NOT NULL DEFAULT '',   -- set_config: egress-path key
    config_value TEXT NOT NULL DEFAULT '',   -- set_config: value
    trigger_spec TEXT NOT NULL DEFAULT 'immediate', -- "immediate" or "2/hr"
    duration     TEXT NOT NULL DEFAULT '',   -- how long the action holds
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tsa_automation_action_chk CHECK (action IN ('suspend', 'suspend_tenant', 'set_config')),
    CONSTRAINT tsa_automation_status_chk CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS tsa_automation_domain_idx ON tsa_automation_rules (domain);
