package service

import (
	"context"
	"encoding/json"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListActionEvidence returns the mail-log event(s) behind an automatic action
// for a subject (tls_policy=<domain> or suppression=<recipient>).
func (s *Service) ListActionEvidence(ctx context.Context, req *adminv1.ListActionEvidenceRequest) (*adminv1.ListActionEvidenceReply, error) {
	if s.evidence == nil {
		return nil, notImplemented("ListActionEvidence")
	}
	items, err := s.evidence.List(ctx, req.GetSubjectType(), req.GetSubjectKey())
	if err != nil {
		return nil, s.fail(ctx, "ListActionEvidence", err)
	}
	out := &adminv1.ListActionEvidenceReply{}
	for _, e := range items {
		out.Items = append(out.Items, evidenceToProto(e))
	}
	return out, nil
}

func evidenceToProto(e *biz.ActionEvidence) *adminv1.ActionEvidence {
	eventJSON := "{}"
	if e.Event != nil {
		if b, err := json.Marshal(e.Event); err == nil {
			eventJSON = string(b)
		}
	}
	out := &adminv1.ActionEvidence{
		Id:          e.ID,
		ActionType:  e.ActionType,
		SubjectType: e.SubjectType,
		SubjectKey:  e.SubjectKey,
		MessageId:   e.MessageID,
		Reason:      e.Reason,
		EventJson:   eventJSON,
	}
	if !e.CreatedAt.IsZero() {
		out.CreatedAt = e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}
