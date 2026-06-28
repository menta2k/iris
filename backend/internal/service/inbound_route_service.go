package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListInboundRoutes returns inbound routes (maildir / forward / webhook).
func (s *Service) ListInboundRoutes(ctx context.Context, req *adminv1.ListInboundRoutesRequest) (*adminv1.ListInboundRoutesReply, error) {
	if s.inboundRoutes == nil {
		return nil, notImplemented("ListInboundRoutes")
	}
	page := pageFrom(req.GetPage())
	items, err := s.inboundRoutes.ListInboundRoutes(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListInboundRoutes", err)
	}
	out := &adminv1.ListInboundRoutesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, r := range items {
		out.Items = append(out.Items, inboundRouteToProto(r))
	}
	return out, nil
}

// CreateInboundRoute creates an inbound route.
func (s *Service) CreateInboundRoute(ctx context.Context, req *adminv1.CreateInboundRouteRequest) (*adminv1.InboundRoute, error) {
	if s.inboundRoutes == nil {
		return nil, notImplemented("CreateInboundRoute")
	}
	out, err := s.inboundRoutes.CreateInboundRoute(ctx, &biz.InboundRoute{
		Name:           req.GetName(),
		MatchType:      req.GetMatchType(),
		MatchValue:     req.GetMatchValue(),
		Action:         req.GetAction(),
		Priority:       int(req.GetPriority()),
		Status:         req.GetStatus(),
		SpamScan:       req.GetSpamScan(),
		ForwardHost:    req.GetForwardHost(),
		ForwardPort:    int(req.GetForwardPort()),
		ForwardTLS:     req.GetForwardTls(),
		MaildirPath:    req.GetMaildirPath(),
		DestinationURL: req.GetDestinationUrl(),
		TimeoutSeconds: int(req.GetTimeoutSeconds()),
		SecretRef:      req.GetSecretRef(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateInboundRoute", err)
	}
	return inboundRouteToProto(out), nil
}

// UpdateInboundRoute updates an existing inbound route.
func (s *Service) UpdateInboundRoute(ctx context.Context, req *adminv1.UpdateInboundRouteRequest) (*adminv1.InboundRoute, error) {
	if s.inboundRoutes == nil {
		return nil, notImplemented("UpdateInboundRoute")
	}
	out, err := s.inboundRoutes.UpdateInboundRoute(ctx, req.GetId(), &biz.InboundRoute{
		Name:           req.GetName(),
		MatchType:      req.GetMatchType(),
		MatchValue:     req.GetMatchValue(),
		Action:         req.GetAction(),
		Priority:       int(req.GetPriority()),
		Status:         req.GetStatus(),
		SpamScan:       req.GetSpamScan(),
		ForwardHost:    req.GetForwardHost(),
		ForwardPort:    int(req.GetForwardPort()),
		ForwardTLS:     req.GetForwardTls(),
		MaildirPath:    req.GetMaildirPath(),
		DestinationURL: req.GetDestinationUrl(),
		TimeoutSeconds: int(req.GetTimeoutSeconds()),
		SecretRef:      req.GetSecretRef(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateInboundRoute", err)
	}
	return inboundRouteToProto(out), nil
}

// DeleteInboundRoute removes an inbound route.
func (s *Service) DeleteInboundRoute(ctx context.Context, req *adminv1.DeleteInboundRouteRequest) (*adminv1.DeleteInboundRouteReply, error) {
	if s.inboundRoutes == nil {
		return nil, notImplemented("DeleteInboundRoute")
	}
	if err := s.inboundRoutes.DeleteInboundRoute(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteInboundRoute", err)
	}
	return &adminv1.DeleteInboundRouteReply{Ok: true}, nil
}

// inboundRouteToProto maps a route to its proto form. The webhook secret is
// write-only and never returned.
func inboundRouteToProto(r *biz.InboundRoute) *adminv1.InboundRoute {
	return &adminv1.InboundRoute{
		Id: r.ID, Name: r.Name, MatchType: r.MatchType, MatchValue: r.MatchValue,
		Action: r.Action, Priority: int32(r.Priority), Status: r.Status, SpamScan: r.SpamScan,
		ForwardHost: r.ForwardHost, ForwardPort: int32(r.ForwardPort), ForwardTls: r.ForwardTLS,
		MaildirPath: r.MaildirPath, DestinationUrl: r.DestinationURL, TimeoutSeconds: int32(r.TimeoutSeconds),
	}
}
