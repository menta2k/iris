-- Credentials for the GreenArrow-compatible mail-injection API.
--
-- The injection listener authenticates each request's body username/password
-- against these rows (bcrypt), in addition to the optional static credential in
-- the config file. Multiple keys let each sending application have its own
-- credential; allowed_mailclasses optionally restricts a key to specific
-- mailclasses (empty = any).
CREATE TABLE IF NOT EXISTS injection_credentials (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username            TEXT NOT NULL UNIQUE,
    password_hash       TEXT NOT NULL,
    label               TEXT NOT NULL DEFAULT '',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    -- Empty array means the key may inject any mailclass.
    allowed_mailclasses TEXT[] NOT NULL DEFAULT '{}',
    last_used_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Username is the lookup key on every injection request.
CREATE INDEX IF NOT EXISTS injection_credentials_username_idx
    ON injection_credentials (username);
