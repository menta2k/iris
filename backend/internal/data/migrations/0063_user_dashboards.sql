-- Per-user custom dashboards. Each user builds any number of dashboards; the
-- widget layout + config is an opaque JSONB array owned by the frontend (the
-- backend validates only that it is a JSON array within a size cap). Widgets
-- reference the metric widget catalog or carry raw PromQL.
CREATE TABLE IF NOT EXISTS user_dashboards (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES iris_users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    is_default  BOOLEAN NOT NULL DEFAULT false,
    widgets     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_dashboards_user_idx
    ON user_dashboards (user_id);

-- At most one default dashboard per user.
CREATE UNIQUE INDEX IF NOT EXISTS user_dashboards_one_default
    ON user_dashboards (user_id) WHERE is_default;
