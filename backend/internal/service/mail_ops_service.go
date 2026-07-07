package service

import (
	"context"
	"time"

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
		Mailclass:  req.GetMailclass(),
		Sender:     req.GetSender(),
		From:       req.GetFrom(),
		Recipient:  req.GetRecipient(),
		VMTAID:     req.GetVmtaId(),
		Status:     req.GetStatus(),
		RecordType: req.GetRecordType(),
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
			RecipientDomain: m.RecipientDomain, VmtaId: m.VMTAID, EgressSource: m.EgressSource, Status: m.Status,
			RecordType: m.RecordType,
			FromHeader: m.FromHeader, SmtpStatus: m.SMTPStatus, Diagnostic: m.Diagnostic,
			Classification: m.Classification,
		})
	}
	return out, nil
}

// GetNextDeliveryAttempt estimates a deferred message's retry schedule.
func (s *Service) GetNextDeliveryAttempt(ctx context.Context, req *adminv1.GetNextDeliveryAttemptRequest) (*adminv1.NextDeliveryAttempt, error) {
	if s.mailOps == nil {
		return nil, notImplemented("GetNextDeliveryAttempt")
	}
	var sched biz.RetrySchedule
	if s.settings != nil {
		sched = s.settings.RetryScheduleNow(ctx)
	}
	est, err := s.mailOps.NextDeliveryAttempt(ctx, req.GetMessageId(), sched)
	if err != nil {
		return nil, s.fail(ctx, "GetNextDeliveryAttempt", err)
	}
	out := &adminv1.NextDeliveryAttempt{
		Deferred:          est.Deferred,
		Attempts:          int32(est.Attempts),
		RemainingAttempts: int32(est.RemainingAttempts),
		WillExpire:        est.WillExpire,
	}
	if est.Interval > 0 {
		out.Interval = est.Interval.String()
	}
	if !est.LastAttempt.IsZero() {
		out.LastAttempt = est.LastAttempt.UTC().Format(time.RFC3339)
	}
	if !est.NextAttempt.IsZero() {
		out.NextAttempt = est.NextAttempt.UTC().Format(time.RFC3339)
	}
	if !est.FinalAttempt.IsZero() {
		out.FinalAttempt = est.FinalAttempt.UTC().Format(time.RFC3339)
	}
	if !est.ExpiresAt.IsZero() {
		out.ExpiresAt = est.ExpiresAt.UTC().Format(time.RFC3339)
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

// ListDsnMessages returns the raw DSN notifications archived for a recipient,
// so the operator can read the full asynchronous bounce behind a dsn-type bounce.
func (s *Service) ListDsnMessages(ctx context.Context, req *adminv1.ListDsnMessagesRequest) (*adminv1.ListDsnMessagesReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListDsnMessages")
	}
	msgs, err := s.mailOps.DSNMessagesForRecipient(ctx, req.GetRecipient())
	if err != nil {
		return nil, s.fail(ctx, "ListDsnMessages", err)
	}
	out := &adminv1.ListDsnMessagesReply{}
	for _, m := range msgs {
		out.Items = append(out.Items, &adminv1.DsnMessage{
			Id:         m.ID,
			MessageId:  m.MessageID,
			RawMessage: m.RawMessage,
			ReceivedAt: m.ReceivedAt.UTC().Format(time.RFC3339),
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

// ListQueues returns kumod's live scheduled-queue summary (US2).
func (s *Service) ListQueues(ctx context.Context, _ *adminv1.ListQueuesRequest) (*adminv1.ListQueuesReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("ListQueues")
	}
	items, err := s.mailOps.ListQueues(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListQueues", err)
	}
	out := &adminv1.ListQueuesReply{Page: &adminv1.PageReply{}}
	for _, q := range items {
		out.Items = append(out.Items, &adminv1.Queue{
			Domain: q.Domain, Depth: q.Depth, Suspended: q.Suspended, SuspendReason: q.SuspendReason,
		})
	}
	return out, nil
}

// RequestQueueAction performs a live queue-control action on kumod (US2).
func (s *Service) RequestQueueAction(ctx context.Context, req *adminv1.RequestQueueActionRequest) (*adminv1.QueueActionReply, error) {
	if s.mailOps == nil {
		return nil, notImplemented("RequestQueueAction")
	}
	res, err := s.mailOps.RequestQueueAction(ctx, req.GetAction(), req.GetDomain(), req.GetReason(), req.GetConfirmationId())
	if err != nil {
		return nil, s.fail(ctx, "RequestQueueAction", err)
	}
	return &adminv1.QueueActionReply{Status: res.Status, Summary: res.Summary}, nil
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
