-- 0004_global_settings.sql
-- Singleton table of operator-editable, deployment-level policy knobs that the
-- KumoMTA config generator consumes (rspamd filtering, default egress EHLO, the
-- log stream, and listener binds). Exactly one row is enforced via CHECK
-- (id = 1); it is seeded so the service Get() never has to handle "row missing".
-- Secrets/infra (DB DSN, Redis password, session secret) stay config/env-only.

CREATE TABLE IF NOT EXISTS global_settings (
    id                    INTEGER PRIMARY KEY,
    rspamd_mode           TEXT NOT NULL DEFAULT '',
    rspamd_url            TEXT NOT NULL DEFAULT '',
    egress_ehlo_domain    TEXT NOT NULL DEFAULT '',
    log_stream_redis_url  TEXT NOT NULL DEFAULT '',
    esmtp_listen          TEXT NOT NULL DEFAULT '',
    http_listen           TEXT NOT NULL DEFAULT '',
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by            TEXT NOT NULL DEFAULT '',
    CONSTRAINT global_settings_singleton CHECK (id = 1),
    CONSTRAINT global_settings_rspamd_mode_chk
        CHECK (rspamd_mode IN ('', 'off', 'tag', 'enforce'))
);

INSERT INTO global_settings (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;
