package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// formatTime renders a timestamp as RFC3339 UTC, or "" when zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z07:00")
}

// ListSubjectClassifications returns all classification rules (manual + AI).
func (s *Service) ListSubjectClassifications(ctx context.Context, req *adminv1.ListSubjectClassificationsRequest) (*adminv1.ListSubjectClassificationsReply, error) {
	if s.classifications == nil {
		return nil, notImplemented("ListSubjectClassifications")
	}
	items, err := s.classifications.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListSubjectClassifications", err)
	}
	out := &adminv1.ListSubjectClassificationsReply{}
	for _, c := range items {
		out.Items = append(out.Items, classificationToProto(c))
	}
	return out, nil
}

// CreateSubjectClassification adds an operator-authored rule.
func (s *Service) CreateSubjectClassification(ctx context.Context, req *adminv1.CreateSubjectClassificationRequest) (*adminv1.SubjectClassification, error) {
	if s.classifications == nil {
		return nil, notImplemented("CreateSubjectClassification")
	}
	out, err := s.classifications.Create(ctx, &biz.SubjectClassification{
		Subject:   req.GetSubject(),
		Label:     req.GetLabel(),
		MatchType: req.GetMatchType(),
		Priority:  req.GetPriority(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateSubjectClassification", err)
	}
	return classificationToProto(out), nil
}

// UpdateSubjectClassification edits a rule by id.
func (s *Service) UpdateSubjectClassification(ctx context.Context, req *adminv1.UpdateSubjectClassificationRequest) (*adminv1.SubjectClassification, error) {
	if s.classifications == nil {
		return nil, notImplemented("UpdateSubjectClassification")
	}
	out, err := s.classifications.Update(ctx, &biz.SubjectClassification{
		ID:        req.GetId(),
		Subject:   req.GetSubject(),
		Label:     req.GetLabel(),
		MatchType: req.GetMatchType(),
		Priority:  req.GetPriority(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateSubjectClassification", err)
	}
	return classificationToProto(out), nil
}

// DeleteSubjectClassification removes a rule by id.
func (s *Service) DeleteSubjectClassification(ctx context.Context, req *adminv1.DeleteSubjectClassificationRequest) (*adminv1.DeleteSubjectClassificationReply, error) {
	if s.classifications == nil {
		return nil, notImplemented("DeleteSubjectClassification")
	}
	if err := s.classifications.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteSubjectClassification", err)
	}
	return &adminv1.DeleteSubjectClassificationReply{Ok: true}, nil
}

func classificationToProto(c *biz.SubjectClassification) *adminv1.SubjectClassification {
	return &adminv1.SubjectClassification{
		Id:                c.ID,
		Subject:           c.Subject,
		SubjectNormalized: c.SubjectNormalized,
		Label:             c.Label,
		Source:            c.Source,
		MatchType:         c.MatchType,
		Priority:          c.Priority,
		HitCount:          c.HitCount,
		CreatedAt:         formatTime(c.CreatedAt),
		UpdatedAt:         formatTime(c.UpdatedAt),
	}
}
