package biz

import (
	"context"
	"strings"
	"time"
)

// ActionEvidence is the exact mail-log event that led to an automatic
// enforcement action, so an operator can see WHY the action was taken.
type ActionEvidence struct {
	ID          string
	ActionType  string         // EvidenceAction*
	SubjectType string         // EvidenceSubject*
	SubjectKey  string         // lower(domain) | lower(recipient)
	MessageID   string         // triggering message id, when known
	Reason      string         // short human reason
	Event       map[string]any // the mail-log record fields at the time
	CreatedAt   time.Time
}

// Evidence action + subject kinds.
const (
	EvidenceActionTLSAutoDisable = "tls_auto_disable"
	EvidenceActionBounceSuppress = "bounce_suppress"

	EvidenceSubjectTLSPolicy   = "tls_policy"
	EvidenceSubjectSuppression = "suppression"
)

// ActionEvidenceRepo persists and reads action evidence.
type ActionEvidenceRepo interface {
	RecordEvidence(ctx context.Context, ev *ActionEvidence) error
	// ListEvidence returns evidence for a subject, newest first, capped at limit.
	ListEvidence(ctx context.Context, subjectType, subjectKey string, limit int) ([]*ActionEvidence, error)
}

// ActionEvidenceUsecase records evidence for automatic actions (internal, no
// permission gate) and lists it for the UI (read-gated).
type ActionEvidenceUsecase struct {
	repo ActionEvidenceRepo
}

// NewActionEvidenceUsecase constructs the use case.
func NewActionEvidenceUsecase(repo ActionEvidenceRepo) *ActionEvidenceUsecase {
	return &ActionEvidenceUsecase{repo: repo}
}

// Record persists evidence for an automatic action. Called from workers; no
// permission gate. Best-effort — callers log and continue on error.
func (uc *ActionEvidenceUsecase) Record(ctx context.Context, ev *ActionEvidence) error {
	if ev == nil || strings.TrimSpace(ev.SubjectKey) == "" {
		return nil
	}
	ev.SubjectKey = strings.ToLower(strings.TrimSpace(ev.SubjectKey))
	return uc.repo.RecordEvidence(ctx, ev)
}

// List returns the evidence recorded for a subject, newest first.
func (uc *ActionEvidenceUsecase) List(ctx context.Context, subjectType, subjectKey string) ([]*ActionEvidence, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	subjectType = strings.ToLower(strings.TrimSpace(subjectType))
	subjectKey = strings.ToLower(strings.TrimSpace(subjectKey))
	if subjectType == "" || subjectKey == "" {
		return nil, Invalid("EVIDENCE_SUBJECT_REQUIRED", "subject_type and subject_key are required")
	}
	return uc.repo.ListEvidence(ctx, subjectType, subjectKey, 50)
}
