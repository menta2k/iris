package biz

import "context"

// SubjectClassificationUsecase is the operator-facing CRUD for classification
// rules (the "Subject Classifications" page). Reads require settings:read,
// mutations settings:write.
type SubjectClassificationUsecase struct {
	repo    SubjectClassificationRepo
	auditor *Auditor
}

// NewSubjectClassificationUsecase constructs the use case.
func NewSubjectClassificationUsecase(repo SubjectClassificationRepo, auditor *Auditor) *SubjectClassificationUsecase {
	return &SubjectClassificationUsecase{repo: repo, auditor: auditor}
}

// List returns all classification rules (operator-authored and AI-generated).
func (uc *SubjectClassificationUsecase) List(ctx context.Context) ([]*SubjectClassification, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	return uc.repo.List(ctx)
}

// Create adds an operator-authored rule.
func (uc *SubjectClassificationUsecase) Create(ctx context.Context, c *SubjectClassification) (*SubjectClassification, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	c.Source = ClassificationSourceManual
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.Create(ctx, c)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "classification.create", out.ID, AuditSuccess, map[string]any{"label": out.Label})
	return out, nil
}

// Update edits a rule by id.
func (uc *SubjectClassificationUsecase) Update(ctx context.Context, c *SubjectClassification) (*SubjectClassification, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if c.ID == "" {
		return nil, Invalid("CLASSIFICATION_ID_REQUIRED", "id is required")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.Update(ctx, c)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "classification.update", out.ID, AuditSuccess, map[string]any{"label": out.Label})
	return out, nil
}

// Delete removes a rule by id.
func (uc *SubjectClassificationUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if err := uc.repo.Delete(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, "classification.delete", id, AuditSuccess, nil)
	return nil
}

func (uc *SubjectClassificationUsecase) audit(ctx context.Context, op, id string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "subject_classification", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
