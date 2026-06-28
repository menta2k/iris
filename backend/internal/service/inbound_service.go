package service

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

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
			MessageId: r.MessageID, Recipient: r.Recipient,
			Action: r.Action, Score: r.Score, Symbols: r.Symbols, Reason: r.Reason,
		})
	}
	return out, nil
}
