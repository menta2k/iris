package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// Diagnose reports how mail from an address is handled and the sending domain's
// DNS readiness.
func (s *Service) Diagnose(ctx context.Context, req *adminv1.DiagnoseRequest) (*adminv1.DiagnoseResult, error) {
	if s.diagnose == nil {
		return nil, notImplemented("Diagnose")
	}
	out, err := s.diagnose.Diagnose(ctx, biz.DiagnoseRequest{
		FromEmail: req.GetFromEmail(),
		Recipient: req.GetRecipient(),
		Mailclass: req.GetMailclass(),
	})
	if err != nil {
		return nil, s.fail(ctx, "Diagnose", err)
	}
	res := &adminv1.DiagnoseResult{
		FromEmail: out.FromEmail,
		Domain:    out.Domain,
		Routing: &adminv1.RoutingOutcome{
			MatchedRule: out.Routing.MatchedRule,
			EgressPool:  out.Routing.EgressPool,
			Vmtas:       out.Routing.VMTAs,
			EgressIps:   out.Routing.EgressIPs,
			Listeners:   out.Routing.Listeners,
			Note:        out.Routing.Note,
		},
	}
	for _, it := range out.Items {
		res.Items = append(res.Items, checkItemToProto(it))
	}
	return res, nil
}

// RblCheck tests the deployment IPs against DNS blocklists.
func (s *Service) RblCheck(ctx context.Context, _ *adminv1.RblCheckRequest) (*adminv1.RblCheckReply, error) {
	if s.rbl == nil {
		return nil, notImplemented("RblCheck")
	}
	rep, err := s.rbl.Check(ctx)
	if err != nil {
		return nil, s.fail(ctx, "RblCheck", err)
	}
	out := &adminv1.RblCheckReply{
		Zones:     rep.Zones,
		CheckedAt: rep.CheckedAt.Format("2006-01-02T15:04:05Z07:00"),
		Skipped:   rep.Skipped,
	}
	for _, r := range rep.Results {
		ipRes := &adminv1.RblIpResult{Ip: r.IP, Source: r.Source, Listed: r.Listed}
		for _, l := range r.Listings {
			ipRes.Listings = append(ipRes.Listings, &adminv1.RblListing{
				Zone: l.Zone, Listed: l.Listed, Reason: l.Reason,
			})
		}
		out.Results = append(out.Results, ipRes)
	}
	return out, nil
}

func checkItemToProto(it biz.CheckItem) *adminv1.DomainCheckItem {
	return &adminv1.DomainCheckItem{
		Name: it.Name, Status: it.Status, Detail: it.Detail, Records: it.Records,
	}
}
