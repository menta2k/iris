package service

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListMailRecords returns filtered mail log records (US2).
func (s *Service) ListMailRecords(ctx context.Context, req *adminv1.ListMailRecordsRequest) (*adminv1.ListMailRecordsReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListMailRecords")
	}
	page := pageFrom(req.GetPage())
	f := biz.MailFilter{
		Mailclass: req.GetMailclass(),
		Sender:    req.GetSender(),
		Recipient: req.GetRecipient(),
		VMTAID:    req.GetVmtaId(),
	}
	if req.GetFromTime() != nil {
		t := req.GetFromTime().AsTime()
		f.FromTime = &t
	}
	if req.GetToTime() != nil {
		t := req.GetToTime().AsTime()
		f.ToTime = &t
	}
	items, err := s.mailOps.ListMailRecords(ctx, f, page)
	if err != nil {
		return nil, s.fail(ctx, "ListMailRecords", err)
	}
	out := &adminv1.ListMailRecordsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, m := range items {
		out.Items = append(out.Items, &adminv1.MailRecord{
			Id: m.ID, MessageId: m.MessageID, EventTime: timestamppb.New(m.EventTime),
			Mailclass: m.Mailclass, Sender: m.Sender, Recipient: m.Recipient,
			RecipientDomain: m.RecipientDomain, VmtaId: m.VMTAID, Status: m.Status,
		})
	}
	return out, nil
}

// ListBounces returns bounce records (US2).
func (s *Service) ListBounces(ctx context.Context, req *adminv1.ListBouncesRequest) (*adminv1.ListBouncesReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListBounces")
	}
	page := pageFrom(req.GetPage())
	items, err := s.mailOps.ListBounces(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListBounces", err)
	}
	out := &adminv1.ListBouncesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, b := range items {
		out.Items = append(out.Items, &adminv1.Bounce{
			Id: b.ID, EventTime: timestamppb.New(b.EventTime), Recipient: b.Recipient,
			Mailclass: b.Mailclass, SmtpStatus: b.SMTPStatus, BounceType: b.BounceType,
			Diagnostic: b.Diagnostic, ProcessingState: b.ProcessingState, Classification: b.Classification,
		})
	}
	return out, nil
}

// ListFeedbackReports returns feedback reports (US2).
func (s *Service) ListFeedbackReports(ctx context.Context, req *adminv1.ListFeedbackReportsRequest) (*adminv1.ListFeedbackReportsReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListFeedbackReports")
	}
	page := pageFrom(req.GetPage())
	items, err := s.mailOps.ListFeedbackReports(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListFeedbackReports", err)
	}
	out := &adminv1.ListFeedbackReportsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, f := range items {
		out.Items = append(out.Items, &adminv1.FeedbackReport{
			Id: f.ID, ReceivedAt: timestamppb.New(f.ReceivedAt), Source: f.Source,
			ReportType: f.ReportType, Recipient: f.Recipient, ProcessingState: f.ProcessingState,
		})
	}
	return out, nil
}

// ListQueues returns mailclass queue snapshots (US2).
func (s *Service) ListQueues(ctx context.Context, req *adminv1.ListQueuesRequest) (*adminv1.ListQueuesReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListQueues")
	}
	page := pageFrom(req.GetPage())
	items, err := s.mailOps.ListQueues(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListQueues", err)
	}
	out := &adminv1.ListQueuesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, q := range items {
		out.Items = append(out.Items, &adminv1.Queue{
			Mailclass: q.Mailclass, State: q.State, Depth: q.Depth,
			OldestMessageAgeSeconds: q.OldestMessageAgeSeconds,
		})
	}
	return out, nil
}

// RequestQueueAction enqueues a queue-control command (US2).
func (s *Service) RequestQueueAction(ctx context.Context, req *adminv1.RequestQueueActionRequest) (*adminv1.QueueActionReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("RequestQueueAction")
	}
	res, err := s.mailOps.RequestQueueAction(ctx, req.GetMailclass(), req.GetAction(), req.GetConfirmationId())
	if err != nil {
		return nil, s.fail(ctx, "RequestQueueAction", err)
	}
	return &adminv1.QueueActionReply{RequestId: res.RequestID, Status: res.Status}, nil
}

// RequestServiceControl enqueues a serialized KumoMTA service-control command (US2).
func (s *Service) RequestServiceControl(ctx context.Context, req *adminv1.RequestServiceControlRequest) (*adminv1.ServiceControlRequest, error) {
	if s.mailOps == nil {
		return nil, notImplemented("RequestServiceControl")
	}
	rec, err := s.mailOps.RequestServiceControl(ctx, req.GetOperation(), req.GetConfirmationId())
	if err != nil {
		return nil, s.fail(ctx, "RequestServiceControl", err)
	}
	return &adminv1.ServiceControlRequest{Id: rec.ID, Operation: rec.Operation, Status: rec.Status}, nil
}
