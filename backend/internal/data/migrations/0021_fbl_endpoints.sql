-- 0021_fbl_endpoints.sql
-- Promote the flat global_settings.fbl_domains list to a per-entry feedback-loop
-- resource. Each row enrolls one mailbox-provider feedback address at a domain
-- and carries a workflow status: while 'awaiting_approval' inbound mail to the
-- feedback address is relayed and forwarded to a human (so the provider's
-- confirmation email is read); once 'approved' the domain enables the built-in
-- ARF parser (log_arf), the prior behavior of fbl_domains.
CREATE TABLE IF NOT EXISTS fbl_endpoints (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain           TEXT NOT NULL,
    feedback_address TEXT NOT NULL,
    forward_address  TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'awaiting_approval',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fbl_endpoints_status_chk
        CHECK (status IN ('awaiting_approval', 'approved')),
    CONSTRAINT fbl_endpoints_feedback_uniq UNIQUE (feedback_address)
);

CREATE INDEX IF NOT EXISTS fbl_endpoints_domain_idx ON fbl_endpoints (domain);

-- Backfill: every existing global FBL domain was already ARF-parsing, so it maps
-- to an 'approved' entry. The feedback address is unknown for legacy domains, so
-- synthesize a per-domain placeholder (approved entries match by domain for ARF,
-- so the feedback_address is informational on these rows).
INSERT INTO fbl_endpoints (domain, feedback_address, forward_address, status)
SELECT lower(trim(d)), 'fbl@' || lower(trim(d)), '', 'approved'
FROM global_settings, unnest(global_settings.fbl_domains) AS d
WHERE trim(d) <> ''
ON CONFLICT (feedback_address) DO NOTHING;

-- The list now lives in fbl_endpoints; drop the global column.
ALTER TABLE global_settings DROP COLUMN IF EXISTS fbl_domains;
