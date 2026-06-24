package data

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// DMARCRepo persists parsed DMARC aggregate reports and serves aggregations.
type DMARCRepo struct {
	db *DB
}

// NewDMARCRepo constructs the repository.
func NewDMARCRepo(db *DB) *DMARCRepo { return &DMARCRepo{db: db} }

var _ biz.DMARCRepo = (*DMARCRepo)(nil)

// dmarcAligned is the SQL predicate for a DMARC-passing record (aligned DKIM or
// SPF). Reused across the aggregation queries.
const dmarcAligned = "(r.dkim_result = 'pass' OR r.spf_result = 'pass')"

// InsertReport persists a report + its records in a transaction, idempotent on
// (org_name, report_id): a resend is a no-op.
func (r *DMARCRepo) InsertReport(ctx context.Context, report *biz.DMARCReport, records []biz.DMARCRecord) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dmarc tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort on the non-commit path

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO dmarc_reports (org_name, report_id, domain, date_begin, date_end, policy_p, policy_pct, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (org_name, report_id) DO NOTHING
		RETURNING id`,
		report.OrgName, report.ReportID, report.Domain, report.DateBegin, report.DateEnd,
		report.PolicyP, report.PolicyPct, report.ReceivedAt).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil // duplicate report; nothing to do
	}
	if err != nil {
		return fmt.Errorf("insert dmarc report: %w", err)
	}
	for _, rec := range records {
		if _, err := tx.Exec(ctx, `
			INSERT INTO dmarc_records (report_id, source_ip, count, disposition, dkim_result, spf_result, header_from)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, rec.SourceIP, rec.Count, rec.Disposition, rec.DKIMResult, rec.SPFResult, rec.HeaderFrom); err != nil {
			return fmt.Errorf("insert dmarc record: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit dmarc tx: %w", err)
	}
	return nil
}

// ListDomains returns the distinct report domains, most-recent first.
func (r *DMARCRepo) ListDomains(ctx context.Context) ([]string, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT domain FROM dmarc_reports WHERE domain <> ''
		GROUP BY domain ORDER BY max(date_begin) DESC`)
	if err != nil {
		return nil, fmt.Errorf("query dmarc domains: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan dmarc domain: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListReports returns recent reports, optionally filtered by domain.
func (r *DMARCRepo) ListReports(ctx context.Context, domain string, page biz.Page) ([]*biz.DMARCReport, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT org_name, report_id, domain, date_begin, date_end, policy_p, policy_pct, received_at
		FROM dmarc_reports
		WHERE ($1 = '' OR domain = $1)
		ORDER BY date_begin DESC LIMIT $2 OFFSET $3`,
		domain, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query dmarc reports: %w", err)
	}
	defer rows.Close()
	var out []*biz.DMARCReport
	for rows.Next() {
		rep := &biz.DMARCReport{}
		if err := rows.Scan(&rep.OrgName, &rep.ReportID, &rep.Domain, &rep.DateBegin, &rep.DateEnd,
			&rep.PolicyP, &rep.PolicyPct, &rep.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan dmarc report: %w", err)
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

// Stats computes the aggregated view. All rollups share the same join/filter:
// dmarc_records joined to dmarc_reports, optionally by domain and date range.
func (r *DMARCRepo) Stats(ctx context.Context, f biz.DMARCFilter) (*biz.DMARCStats, error) {
	// $1 domain ('' = all), $2/$3 date range (zero time = open).
	var from, to any
	if !f.From.IsZero() {
		from = f.From
	}
	if !f.To.IsZero() {
		to = f.To
	}
	where := `JOIN dmarc_reports rep ON rep.id = r.report_id
		WHERE ($1 = '' OR rep.domain = $1)
		  AND ($2::timestamptz IS NULL OR rep.date_begin >= $2)
		  AND ($3::timestamptz IS NULL OR rep.date_begin <= $3)`
	args := []any{f.Domain, from, to}
	out := &biz.DMARCStats{}

	// Totals.
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(r.count),0),
		       COALESCE(SUM(r.count) FILTER (WHERE `+dmarcAligned+`),0),
		       COALESCE(SUM(r.count) FILTER (WHERE r.spf_result = 'pass'),0),
		       COALESCE(SUM(r.count) FILTER (WHERE r.dkim_result = 'pass'),0)
		FROM dmarc_records r `+where, args...).
		Scan(&out.TotalMessages, &out.DMARCPass, &out.SPFPass, &out.DKIMPass); err != nil {
		return nil, fmt.Errorf("dmarc totals: %w", err)
	}

	// Disposition split.
	disp, err := r.groupCount(ctx, `
		SELECT COALESCE(NULLIF(r.disposition,''),'none'), COALESCE(SUM(r.count),0)
		FROM dmarc_records r `+where+` GROUP BY 1 ORDER BY 2 DESC`, args)
	if err != nil {
		return nil, err
	}
	out.Dispositions = disp

	// Top source IPs.
	srcRows, err := r.db.Pool.Query(ctx, `
		SELECT r.source_ip, COALESCE(SUM(r.count),0),
		       COALESCE(SUM(r.count) FILTER (WHERE `+dmarcAligned+`),0)
		FROM dmarc_records r `+where+`
		GROUP BY r.source_ip ORDER BY 2 DESC LIMIT 25`, args...)
	if err != nil {
		return nil, fmt.Errorf("dmarc sources: %w", err)
	}
	defer srcRows.Close()
	for srcRows.Next() {
		var s biz.DMARCSource
		if err := srcRows.Scan(&s.IP, &s.Total, &s.Pass); err != nil {
			return nil, fmt.Errorf("scan dmarc source: %w", err)
		}
		s.Fail = s.Total - s.Pass
		out.TopSources = append(out.TopSources, s)
	}
	if err := srcRows.Err(); err != nil {
		return nil, err
	}

	// Per-domain breakdown.
	domRows, err := r.db.Pool.Query(ctx, `
		SELECT rep.domain, COALESCE(SUM(r.count),0),
		       COALESCE(SUM(r.count) FILTER (WHERE `+dmarcAligned+`),0)
		FROM dmarc_records r `+where+`
		GROUP BY rep.domain ORDER BY 2 DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("dmarc domains breakdown: %w", err)
	}
	defer domRows.Close()
	for domRows.Next() {
		var d biz.DMARCDomainStat
		if err := domRows.Scan(&d.Domain, &d.Messages, &d.Pass); err != nil {
			return nil, fmt.Errorf("scan dmarc domain stat: %w", err)
		}
		out.Domains = append(out.Domains, d)
	}
	if err := domRows.Err(); err != nil {
		return nil, err
	}

	// Daily time series.
	dayRows, err := r.db.Pool.Query(ctx, `
		SELECT to_char(date_trunc('day', rep.date_begin), 'YYYY-MM-DD'),
		       COALESCE(SUM(r.count),0),
		       COALESCE(SUM(r.count) FILTER (WHERE `+dmarcAligned+`),0)
		FROM dmarc_records r `+where+`
		GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, fmt.Errorf("dmarc series: %w", err)
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var d biz.DMARCDay
		if err := dayRows.Scan(&d.Date, &d.Messages, &d.Pass); err != nil {
			return nil, fmt.Errorf("scan dmarc day: %w", err)
		}
		out.Series = append(out.Series, d)
	}
	return out, dayRows.Err()
}

func (r *DMARCRepo) groupCount(ctx context.Context, query string, args []any) ([]biz.DMARCCount, error) {
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dmarc group count: %w", err)
	}
	defer rows.Close()
	var out []biz.DMARCCount
	for rows.Next() {
		var c biz.DMARCCount
		if err := rows.Scan(&c.Label, &c.Count); err != nil {
			return nil, fmt.Errorf("scan dmarc count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
