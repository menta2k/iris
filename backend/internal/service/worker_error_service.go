package service

import (
	"context"
	"encoding/json"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListWorkerErrorLogs returns recent worker error-log entries, filtered by
// level/worker and an optional event-time range.
func (s *Service) ListWorkerErrorLogs(ctx context.Context, req *adminv1.ListWorkerErrorLogsRequest) (*adminv1.ListWorkerErrorLogsReply, error) {
	if s.workerErrors == nil {
		return nil, notImplemented("ListWorkerErrorLogs")
	}
	filter := biz.WorkerErrorFilter{
		Level:  req.GetLevel(),
		Worker: req.GetWorker(),
	}
	if from := parseRFC3339(req.GetFrom()); !from.IsZero() {
		filter.From = &from
	}
	if to := parseRFC3339(req.GetTo()); !to.IsZero() {
		filter.To = &to
	}
	page := pageFrom(req.GetPage())
	items, err := s.workerErrors.List(ctx, filter, page)
	if err != nil {
		return nil, s.fail(ctx, "ListWorkerErrorLogs", err)
	}
	out := &adminv1.ListWorkerErrorLogsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, w := range items {
		out.Items = append(out.Items, &adminv1.WorkerErrorLog{
			Id:        w.ID,
			EventTime: w.EventTime.Format(time.RFC3339),
			Level:     w.Level,
			Worker:    w.Worker,
			Message:   w.Message,
			Detail:    marshalDetail(w.Detail),
		})
	}
	return out, nil
}

// marshalDetail renders the structured attributes as a compact JSON object
// string, returning "{}" when empty or unmarshalable.
func marshalDetail(detail map[string]any) string {
	if len(detail) == 0 {
		return "{}"
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return "{}"
	}
	return string(b)
}
