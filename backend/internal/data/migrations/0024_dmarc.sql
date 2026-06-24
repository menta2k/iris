-- 0024_dmarc.sql
-- DMARC aggregate (RFC 7489 rua) report parsing. One operator-configured report
-- address (advertised as rua= in domains' DMARC records) receives reports for
-- many domains; the sending domain is inside each report.

ALTER TABLE global_settings
    ADD COLUMN IF NOT EXISTS dmarc_report_email TEXT NOT NULL DEFAULT '';

-- One row per received aggregate report. Deduped on (org_name, report_id) since
-- providers resend.
CREATE TABLE IF NOT EXISTS dmarc_reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_name    TEXT NOT NULL DEFAULT '',
    report_id   TEXT NOT NULL DEFAULT '',
    domain      TEXT NOT NULL DEFAULT '',
    date_begin  TIMESTAMPTZ NOT NULL,
    date_end    TIMESTAMPTZ NOT NULL,
    policy_p    TEXT NOT NULL DEFAULT '',
    policy_pct  INT  NOT NULL DEFAULT 100,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT dmarc_reports_uniq UNIQUE (org_name, report_id)
);
CREATE INDEX IF NOT EXISTS dmarc_reports_domain_idx ON dmarc_reports (domain, date_begin DESC);

-- One row per <record> in a report (a source IP's result for a count of messages).
CREATE TABLE IF NOT EXISTS dmarc_records (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id   UUID NOT NULL REFERENCES dmarc_reports(id) ON DELETE CASCADE,
    source_ip   TEXT NOT NULL DEFAULT '',
    count       INT  NOT NULL DEFAULT 0,
    disposition TEXT NOT NULL DEFAULT '',
    dkim_result TEXT NOT NULL DEFAULT '',
    spf_result  TEXT NOT NULL DEFAULT '',
    header_from TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS dmarc_records_report_idx ON dmarc_records (report_id);
CREATE INDEX IF NOT EXISTS dmarc_records_from_idx ON dmarc_records (header_from);
