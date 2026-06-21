-- 0001_core_config.sql
-- Core relational configuration tables for Iris. TimescaleDB hypertables are
-- created in a later migration; these are standard relational tables.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Identity ------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS iris_users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'invited',
    mfa_required    BOOLEAN NOT NULL DEFAULT true,
    mfa_enrolled_at TIMESTAMPTZ,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT iris_users_status_chk
        CHECK (status IN ('invited', 'active', 'disabled', 'locked'))
);

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    permissions TEXT[] NOT NULL DEFAULT '{}',
    builtin     BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id UUID NOT NULL REFERENCES iris_users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, role_id)
);

-- Outbound configuration ----------------------------------------------------

CREATE TABLE IF NOT EXISTS vmtas (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    ip_address  INET NOT NULL,
    ehlo_name   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vmtas_status_chk
        CHECK (status IN ('active', 'disabled', 'draining'))
);

-- Only one active VMTA may bind a given IP address.
CREATE UNIQUE INDEX IF NOT EXISTS vmtas_active_ip_uniq
    ON vmtas (ip_address) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS vmta_groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT vmta_groups_status_chk
        CHECK (status IN ('active', 'disabled'))
);

CREATE TABLE IF NOT EXISTS vmta_group_members (
    group_id   UUID NOT NULL REFERENCES vmta_groups(id) ON DELETE CASCADE,
    vmta_id    UUID NOT NULL REFERENCES vmtas(id) ON DELETE RESTRICT,
    weight     INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, vmta_id),
    CONSTRAINT vmta_group_members_weight_chk CHECK (weight > 0)
);

CREATE TABLE IF NOT EXISTS routing_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    match_type  TEXT NOT NULL,
    match_value TEXT NOT NULL,
    priority    INTEGER NOT NULL DEFAULT 100,
    target_type TEXT NOT NULL,
    target_id   UUID NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT routing_rules_match_type_chk
        CHECK (match_type IN ('mailclass', 'recipient_email', 'recipient_domain')),
    CONSTRAINT routing_rules_target_type_chk
        CHECK (target_type IN ('vmta', 'vmta_group')),
    CONSTRAINT routing_rules_status_chk
        CHECK (status IN ('active', 'disabled'))
);

-- Deterministic priority for active overlapping rules.
CREATE UNIQUE INDEX IF NOT EXISTS routing_rules_active_priority_uniq
    ON routing_rules (match_type, match_value, priority) WHERE status = 'active';

-- Domain & recipient safety -------------------------------------------------

CREATE TABLE IF NOT EXISTS dkim_domains (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain                 TEXT NOT NULL,
    selector               TEXT NOT NULL,
    public_key_fingerprint TEXT NOT NULL DEFAULT '',
    private_key_ref        TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL DEFAULT 'needs_attention',
    last_verified_at       TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT dkim_domains_uniq UNIQUE (domain, selector),
    CONSTRAINT dkim_domains_status_chk
        CHECK (status IN ('ready', 'disabled', 'needs_attention'))
);

CREATE TABLE IF NOT EXISTS suppression_entries (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type       TEXT NOT NULL,
    value      TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT 'manual',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    CONSTRAINT suppression_entries_type_chk CHECK (type IN ('email', 'domain')),
    CONSTRAINT suppression_entries_status_chk
        CHECK (status IN ('active', 'disabled', 'expired')),
    CONSTRAINT suppression_entries_uniq UNIQUE (type, value)
);

-- Inbound automation --------------------------------------------------------

CREATE TABLE IF NOT EXISTS webhook_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    match_type      TEXT NOT NULL,
    match_value     TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    secret_ref      TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active',
    timeout_seconds INTEGER NOT NULL DEFAULT 10,
    retry_policy    JSONB NOT NULL DEFAULT '{"max_attempts":5,"backoff_seconds":30}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT webhook_rules_match_type_chk
        CHECK (match_type IN ('recipient_email', 'recipient_domain')),
    CONSTRAINT webhook_rules_status_chk CHECK (status IN ('active', 'disabled')),
    CONSTRAINT webhook_rules_timeout_chk CHECK (timeout_seconds > 0)
);

-- Queue state (current snapshot per mailclass) ------------------------------

CREATE TABLE IF NOT EXISTS mailclass_queues (
    mailclass                  TEXT PRIMARY KEY,
    state                      TEXT NOT NULL DEFAULT 'unknown',
    depth                      BIGINT NOT NULL DEFAULT 0,
    oldest_message_age_seconds BIGINT NOT NULL DEFAULT 0,
    last_observed_at           TIMESTAMPTZ,
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mailclass_queues_state_chk
        CHECK (state IN ('running', 'paused', 'draining', 'unknown'))
);
