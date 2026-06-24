package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// GetDmarcStats returns aggregated DMARC statistics.
func (s *Service) GetDmarcStats(ctx context.Context, req *adminv1.GetDmarcStatsRequest) (*adminv1.DmarcStats, error) {
	if s.dmarc == nil {
		return nil, notImplemented("GetDmarcStats")
	}
	from := parseRFC3339(req.GetFrom())
	to := parseRFC3339(req.GetTo())
	st, err := s.dmarc.Stats(ctx, req.GetDomain(), from, to)
	if err != nil {
		return nil, s.fail(ctx, "GetDmarcStats", err)
	}
	out := &adminv1.DmarcStats{
		TotalMessages: int32(st.TotalMessages),
		DmarcPass:     int32(st.DMARCPass),
		SpfPass:       int32(st.SPFPass),
		DkimPass:      int32(st.DKIMPass),
	}
	for _, c := range st.Dispositions {
		out.Dispositions = append(out.Dispositions, &adminv1.DmarcCount{Label: c.Label, Count: int32(c.Count)})
	}
	for _, sc := range st.TopSources {
		out.TopSources = append(out.TopSources, &adminv1.DmarcSource{
			Ip: sc.IP, Total: int32(sc.Total), Pass: int32(sc.Pass), Fail: int32(sc.Fail),
		})
	}
	for _, d := range st.Domains {
		out.Domains = append(out.Domains, &adminv1.DmarcDomainStat{
			Domain: d.Domain, Messages: int32(d.Messages), Pass: int32(d.Pass),
		})
	}
	for _, d := range st.Series {
		out.Series = append(out.Series, &adminv1.DmarcDay{
			Date: d.Date, Messages: int32(d.Messages), Pass: int32(d.Pass),
		})
	}
	return out, nil
}

// ListDmarcReports returns recent reports, optionally filtered by domain.
func (s *Service) ListDmarcReports(ctx context.Context, req *adminv1.ListDmarcReportsRequest) (*adminv1.ListDmarcReportsReply, error) {
	if s.dmarc == nil {
		return nil, notImplemented("ListDmarcReports")
	}
	page := pageFrom(req.GetPage())
	items, err := s.dmarc.ListReports(ctx, req.GetDomain(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListDmarcReports", err)
	}
	out := &adminv1.ListDmarcReportsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, r := range items {
		out.Items = append(out.Items, &adminv1.DmarcReport{
			OrgName:    r.OrgName,
			ReportId:   r.ReportID,
			Domain:     r.Domain,
			DateBegin:  r.DateBegin.Format(time.RFC3339),
			DateEnd:    r.DateEnd.Format(time.RFC3339),
			PolicyP:    r.PolicyP,
			PolicyPct:  int32(r.PolicyPct),
			ReceivedAt: r.ReceivedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// ListDmarcDomains returns the distinct report domains (for the filter dropdown).
func (s *Service) ListDmarcDomains(ctx context.Context, _ *adminv1.ListDmarcDomainsRequest) (*adminv1.ListDmarcDomainsReply, error) {
	if s.dmarc == nil {
		return nil, notImplemented("ListDmarcDomains")
	}
	domains, err := s.dmarc.Domains(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListDmarcDomains", err)
	}
	return &adminv1.ListDmarcDomainsReply{Domains: domains}, nil
}

// parseRFC3339 parses an optional RFC3339 timestamp; blank/invalid → zero time.
func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
