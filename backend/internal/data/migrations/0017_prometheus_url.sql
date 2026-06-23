-- 0017_prometheus_url.sql
-- Add the Prometheus base URL to the global settings singleton. When set, the
-- dashboard metrics endpoint (GetMetricsTimeseries) queries this Prometheus for
-- curated time-series; when empty the endpoint reports metrics as unavailable.

ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS prometheus_url TEXT NOT NULL DEFAULT '';
