-- 0004_backfill_mail_class.sql
-- Back-fill log_event.mail_class for rows persisted before the consumer
-- started extracting the X-Kumo-Mail-Class header. The full kumomta
-- log_record JSON is preserved in extra_json (text), so we cast to jsonb
-- and pull the header out from there. Idempotent: only touches rows
-- where mail_class is NULL or empty, so re-runs after the first are
-- effectively no-ops on the same hypertable.
--
-- Header value can ship in two shapes from kumomta — a bare string
-- ({"X-Kumo-Mail-Class": "tx-test"}) or a single-element array
-- ({"X-Kumo-Mail-Class": ["tx-test"]}); we COALESCE both. Truncate to
-- 64 chars to match the column's MaxLen.

DO $$
BEGIN
  -- Defensive: if 0001/ent hasn't created the column yet (e.g. migrations
  -- ran out of order on an older deploy) just no-op rather than failing
  -- the whole migration phase.
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'log_event'
      AND column_name = 'mail_class'
  ) THEN
    RAISE NOTICE '0004: log_event.mail_class column missing — skipping backfill';
    RETURN;
  END IF;

  -- Cheap pre-filter: skip the jsonb cast on rows whose extra_json doesn't
  -- start with '{' (handles NULL, empty, and any legacy non-JSON garbage).
  -- We don't anchor on '}' because kumomta's serializer often appends a
  -- trailing newline, which would break a closing-brace match.
  WITH src AS (
    SELECT id, at,
           LEFT(
             COALESCE(
               -- string shape
               extra_json::jsonb #>> '{headers,X-Kumo-Mail-Class}',
               -- array shape: pick the first element as text
               extra_json::jsonb #> '{headers,X-Kumo-Mail-Class}' ->> 0
             ),
             64
           ) AS hdr
    FROM   log_event
    WHERE  (mail_class IS NULL OR mail_class = '')
      AND  extra_json IS NOT NULL
      AND  extra_json LIKE '{%'
  )
  UPDATE log_event AS le
  SET    mail_class = src.hdr
  FROM   src
  WHERE  le.id = src.id
    AND  le.at = src.at
    AND  src.hdr IS NOT NULL
    AND  src.hdr <> '';
END
$$;
