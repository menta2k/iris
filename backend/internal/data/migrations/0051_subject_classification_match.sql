-- Subject-classification rules gain a match type and a priority.
--
-- match_type: 'similarity' (trigram against subject_normalized, the existing
-- self-populating behaviour) or 'regex' (subject holds a regular expression
-- matched against the raw subject). priority orders evaluation across all
-- rules: higher priority is matched first and the first match wins.
ALTER TABLE subject_classifications
    ADD COLUMN IF NOT EXISTS match_type TEXT NOT NULL DEFAULT 'similarity',
    ADD COLUMN IF NOT EXISTS priority   INTEGER NOT NULL DEFAULT 0;

ALTER TABLE subject_classifications
    DROP CONSTRAINT IF EXISTS subject_classifications_match_type_chk;
ALTER TABLE subject_classifications
    ADD CONSTRAINT subject_classifications_match_type_chk
    CHECK (match_type IN ('similarity', 'regex'));

-- Regex rules carry no normalized similarity key (it is stored as ''), so the
-- original column-level UNIQUE on subject_normalized would collide across every
-- regex row. Replace it with a partial unique index that applies only to
-- similarity rules, where the key still identifies a single collapsed template.
ALTER TABLE subject_classifications
    DROP CONSTRAINT IF EXISTS subject_classifications_subject_normalized_key;
CREATE UNIQUE INDEX IF NOT EXISTS subject_classifications_norm_similarity_key
    ON subject_classifications (subject_normalized)
    WHERE match_type = 'similarity';

-- Priority-ordered scan for the matcher (highest priority first).
CREATE INDEX IF NOT EXISTS subject_classifications_priority_idx
    ON subject_classifications (priority DESC);
