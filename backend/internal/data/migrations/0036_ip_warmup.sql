-- 0036_ip_warmup.sql
-- IP warmup: gradually ramp a VMTA's outbound volume per receiving-domain family
-- (MBP) over a curve of daily caps to build sender reputation, then complete
-- (cap removed). The per-day cap is rendered as a KumoMTA max_message_rate on the
-- egress path for the matching (egress_source, MBP bucket).
--
-- One non-completed schedule per VMTA. `stages` is the resolved curve copy
-- ([{day_from,day_to,caps:{gmail,microsoft,yahoo,default}}, ...]) so a later
-- template change never alters a running ramp. start_date is day 1 of the ramp.
CREATE TABLE IF NOT EXISTS warmup_schedules (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vmta_id       UUID NOT NULL REFERENCES vmtas(id) ON DELETE CASCADE,
    start_date    DATE NOT NULL,
    curve         TEXT NOT NULL,
    stages        JSONB NOT NULL,
    status        TEXT NOT NULL DEFAULT 'scheduled',
    paused_reason TEXT NOT NULL DEFAULT '',
    -- Frozen ramp day while paused (0 otherwise), so a paused schedule holds its
    -- current cap exactly regardless of elapsed time.
    held_day      INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT warmup_status_chk CHECK (status IN ('scheduled', 'active', 'paused', 'completed'))
);

-- At most one in-progress warmup per VMTA (completed ones are history).
CREATE UNIQUE INDEX IF NOT EXISTS warmup_active_vmta_uniq
    ON warmup_schedules (vmta_id) WHERE status <> 'completed';
