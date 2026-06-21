package service

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListWebhookRules returns inbound webhook rules (US5).
func (s *Service) ListWebhookRules(ctx context.Context, req *adminv1.ListWebhookRulesRequest) (*adminv1.ListWebhookRulesReply, error) {
	if s.inbound == nil {
		return nil, notImplemented("ListWebhookRules")
	}
	page := pageFrom(req.GetPage())
	items, err := s.inbound.ListWebhookRules(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListWebhookRules", err)
	}
	out := &adminv1.ListWebhookRulesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, w := range items {
		out.Items = append(out.Items, webhookToProto(w))
	}
	return out, nil
}

// ListWebhookDeliveries returns recent webhook delivery attempts (US5).
func (s *Service) ListWebhookDeliveries(ctx context.Context, req *adminv1.ListWebhookDeliveriesRequest) (*adminv1.ListWebhookDeliveriesReply, error) {
	if s.inbound == nil {
		return nil, notImplemented("ListWebhookDeliveries")
	}
	page := pageFrom(req.GetPage())
	items, err := s.inbound.ListWebhookDeliveries(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListWebhookDeliveries", err)
	}
	out := &adminv1.ListWebhookDeliveriesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, e := range items {
		out.Items = append(out.Items, &adminv1.WebhookDeliveryEvent{
			Id:            e.ID,
			EventTime:     e.EventTime.UTC().Format("2006-01-02T15:04:05Z07:00"),
			WebhookRuleId: e.WebhookRuleID,
			WebhookName:   e.WebhookName,
			MailRecordId:  e.MailRecordID,
			Recipient:     e.Recipient,
			Attempt:       int32(e.Attempt),
			Status:        e.Status,
			ResponseCode:  int32(e.ResponseCode),
			ErrorSummary:  e.ErrorSummary,
		})
	}
	return out, nil
}

// CreateWebhookRule creates an inbound webhook rule (US5).
func (s *Service) CreateWebhookRule(ctx context.Context, req *adminv1.CreateWebhookRuleRequest) (*adminv1.WebhookRule, error) {
	if s.inbound == nil {
		return nil, notImplemented("CreateWebhookRule")
	}
	out, err := s.inbound.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name:           req.GetName(),
		MatchType:      req.GetMatchType(),
		MatchValue:     req.GetMatchValue(),
		DestinationURL: req.GetDestinationUrl(),
		SecretRef:      req.GetSecretRef(),
		TimeoutSeconds: int(req.GetTimeoutSeconds()),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateWebhookRule", err)
	}
	return webhookToProto(out), nil
}

// UpdateWebhookRule updates an existing inbound webhook rule (US5).
func (s *Service) UpdateWebhookRule(ctx context.Context, req *adminv1.UpdateWebhookRuleRequest) (*adminv1.WebhookRule, error) {
	if s.inbound == nil {
		return nil, notImplemented("UpdateWebhookRule")
	}
	out, err := s.inbound.UpdateWebhookRule(ctx, req.GetId(), &biz.WebhookRule{
		Name:           req.GetName(),
		MatchType:      req.GetMatchType(),
		MatchValue:     req.GetMatchValue(),
		DestinationURL: req.GetDestinationUrl(),
		SecretRef:      req.GetSecretRef(),
		TimeoutSeconds: int(req.GetTimeoutSeconds()),
		Status:         req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateWebhookRule", err)
	}
	return webhookToProto(out), nil
}

// ListRspamdResults returns Rspamd filter results (US5).
func (s *Service) ListRspamdResults(ctx context.Context, req *adminv1.ListRspamdResultsRequest) (*adminv1.ListRspamdResultsReply, error) {
	if s.inbound == nil {
		return nil, notImplemented("ListRspamdResults")
	}
	page := pageFrom(req.GetPage())
	items, err := s.inbound.ListRspamdResults(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListRspamdResults", err)
	}
	out := &adminv1.ListRspamdResultsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, r := range items {
		out.Items = append(out.Items, &adminv1.RspamdResult{
			Id: r.ID, EventTime: timestamppb.New(r.EventTime), MailRecordId: r.MailRecordID,
			Action: r.Action, Score: r.Score, Symbols: r.Symbols, Reason: r.Reason,
		})
	}
	return out, nil
}

func webhookToProto(w *biz.WebhookRule) *adminv1.WebhookRule {
	return &adminv1.WebhookRule{
		Id: w.ID, Name: w.Name, MatchType: w.MatchType, MatchValue: w.MatchValue,
		DestinationUrl: w.DestinationURL, Status: w.Status, TimeoutSeconds: int32(w.TimeoutSeconds),
	}
}
