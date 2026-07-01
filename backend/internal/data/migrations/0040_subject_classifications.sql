-- Subject-based email classification lookup/cache (optional feature).
--
-- pg_trgm provides in-database trigram similarity so an incoming subject can be
-- matched against previously-labeled subjects without any external dependency.
-- pg_trgm is a "trusted" extension in PostgreSQL 13+, so the database owner can
-- create it without superuser.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS subject_classifications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- A representative subject: an operator-entered phrase (source='manual') or
    -- the raw subject that first produced an AI label (source='ai'). This is the
    -- ONLY place a raw subject is stored.
    subject            TEXT NOT NULL,
    -- Normalized matching key (lowercased, prefixes/digits stripped). Unique so
    -- the same template collapses to one row that accumulates hits.
    subject_normalized TEXT NOT NULL UNIQUE,
    -- Classification label, at most ~2 words. '' means "pending" (enqueued but
    -- not yet labeled by the worker).
    label              TEXT NOT NULL DEFAULT '',
    source             TEXT NOT NULL DEFAULT 'manual',
    hit_count          BIGINT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT subject_classifications_source_chk CHECK (source IN ('manual', 'ai'))
);

-- Trigram GIN index for fast similarity() / % lookups on the normalized key.
CREATE INDEX IF NOT EXISTS subject_classifications_norm_trgm_idx
    ON subject_classifications USING gin (subject_normalized gin_trgm_ops);
