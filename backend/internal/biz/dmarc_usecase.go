package biz

import (
	"context"
	"strings"
	"time"
)

// DMARCCount is a labeled count (e.g. per disposition).
type DMARCCount struct {
	Label string
	Count int
}

// DMARCSource is a sending source IP's rollup.
type DMARCSource struct {
	IP    string
	Total int
	Pass  int // messages that passed DMARC alignment (dkim or spf pass)
	Fail  int
}

// DMARCDomainStat is a per-domain rollup.
type DMARCDomainStat struct {
	Domain   string
	Messages int
	Pass     int
}

// DMARCReporterStat is a per-reporter (org_name) rollup — the receiving org that
// sent the aggregate reports.
type DMARCReporterStat struct {
	Reporter string
	Messages int
	Pass     int
}

// DMARCDay is a daily volume/pass point for the time series.
type DMARCDay struct {
	Date     string // YYYY-MM-DD
	Messages int
	Pass     int
}

// DMARCStats is the aggregated view shown on the DMARC page.
type DMARCStats struct {
	TotalMessages int
	DMARCPass     int
	SPFPass       int
	DKIMPass      int
	Dispositions  []DMARCCount
	TopSources    []DMARCSource
	Domains       []DMARCDomainStat
	Series        []DMARCDay
	// Reporters is the per-reporter breakdown; it ignores the Reporter filter so
	// the drill-down list stays complete even when one reporter is selected.
	Reporters []DMARCReporterStat
}

// DMARCFilter narrows the aggregation by domain, reporter (org_name), and date
// range (zero-value fields are open).
type DMARCFilter struct {
	Domain   string
	Reporter string
	From     time.Time
	To       time.Time
}

// DMARCRepo is the persistence boundary for DMARC reports.
type DMARCRepo interface {
	// InsertReport persists a report and its records idempotently (dedupe on
	// org_name+report_id); a duplicate is a no-op.
	InsertReport(ctx context.Context, report *DMARCReport, records []DMARCRecord) error
	ListDomains(ctx context.Context) ([]string, error)
	ListReports(ctx context.Context, domain string, page Page) ([]*DMARCReport, error)
	Stats(ctx context.Context, f DMARCFilter) (*DMARCStats, error)
}

// DMARCUsecase serves DMARC report ingestion (worker) and the reporting API.
type DMARCUsecase struct {
	repo    DMARCRepo
	auditor *Auditor
	events  EventEmitter
}

// NewDMARCUsecase constructs the use case.
func NewDMARCUsecase(repo DMARCRepo, auditor *Auditor) *DMARCUsecase {
	return &DMARCUsecase{repo: repo, auditor: auditor}
}

// WithEventEmitter forwards dmarc-received events to the Event Processor.
func (uc *DMARCUsecase) WithEventEmitter(e EventEmitter) *DMARCUsecase {
	uc.events = e
	return uc
}

// Ingest persists a parsed report. Called by the worker (no permission check —
// it runs on an internal context).
func (uc *DMARCUsecase) Ingest(ctx context.Context, report *DMARCReport, records []DMARCRecord) error {
	if err := uc.repo.InsertReport(ctx, report, records); err != nil {
		return err
	}
	if uc.events != nil {
		uc.events.Emit(DispatchEvent{
			Type: EventDMARCReceived, OccurredAt: report.ReceivedAt,
			Data: map[string]any{
				"domain": report.Domain, "org_name": report.OrgName,
				"report_id": report.ReportID, "records": len(records),
			},
		})
	}
	return nil
}

// Stats returns the aggregated view after an authorization check. reporter
// (org_name) optionally drills the summary/charts down to a single reporter; the
// Reporters breakdown always lists every reporter in the domain/date scope.
func (uc *DMARCUsecase) Stats(ctx context.Context, domain, reporter string, from, to time.Time) (*DMARCStats, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.Stats(ctx, DMARCFilter{
		Domain:   strings.ToLower(strings.TrimSpace(domain)),
		Reporter: strings.TrimSpace(reporter),
		From:     from,
		To:       to,
	})
}

// ListReports returns recent reports (optionally filtered by domain).
func (uc *DMARCUsecase) ListReports(ctx context.Context, domain string, page Page) ([]*DMARCReport, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.ListReports(ctx, strings.ToLower(strings.TrimSpace(domain)), page)
}

// Domains returns the distinct report domains (for the filter dropdown).
func (uc *DMARCUsecase) Domains(ctx context.Context) ([]string, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.ListDomains(ctx)
}
