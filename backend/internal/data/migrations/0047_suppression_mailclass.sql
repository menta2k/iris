-- Record the message class of the event that triggered a suppression (e.g. the
-- bounce/deferral mailclass), for operator context in the suppressions list.
-- Empty for manual entries. Suppression enforcement remains global (blocks the
-- recipient regardless of class).
ALTER TABLE suppression_entries
    ADD COLUMN IF NOT EXISTS mailclass TEXT NOT NULL DEFAULT '';
