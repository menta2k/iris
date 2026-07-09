-- Move inbox-monitoring policy knobs from env vars to UI-managed global settings.
-- Empty means "use the built-in default" (mirrors the other duration settings).
ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS monitoring_from               TEXT NOT NULL DEFAULT '', -- fallback probe sender
    ADD COLUMN IF NOT EXISTS monitoring_reconcile_lookback TEXT NOT NULL DEFAULT '', -- duration, e.g. "1h"
    ADD COLUMN IF NOT EXISTS monitoring_fetch_timeout      TEXT NOT NULL DEFAULT '', -- duration, e.g. "30s"
    ADD COLUMN IF NOT EXISTS monitoring_fetch_giveup       TEXT NOT NULL DEFAULT ''; -- duration, e.g. "2h"
