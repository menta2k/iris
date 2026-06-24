package biz

import "context"

// FBLUsecase manages feedback-loop endpoints: the per-domain enrollments that
// decide whether inbound feedback mail is forwarded for human approval or parsed
// as an ARF report. It replaces the flat global fbl_domains list.
type FBLUsecase struct {
	repo    FBLRepo
	auditor *Auditor
}

// NewFBLUsecase constructs the use case.
func NewFBLUsecase(repo FBLRepo, auditor *Auditor) *FBLUsecase {
	return &FBLUsecase{repo: repo, auditor: auditor}
}

// List returns feedback-loop endpoints after an authorization check.
func (uc *FBLUsecase) List(ctx context.Context, page Page) ([]*FBLEndpoint, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	return uc.repo.ListFBLEndpoints(ctx, page)
}

// Create validates and persists a feedback-loop endpoint, auditing the change.
func (uc *FBLUsecase) Create(ctx context.Context, f *FBLEndpoint) (*FBLEndpoint, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateFBLEndpoint(ctx, f)
	if err != nil {
		uc.audit(ctx, "fbl.create", "fbl", f.FeedbackAddress, AuditFailure, map[string]any{"domain": f.Domain})
		return nil, err
	}
	uc.audit(ctx, "fbl.create", "fbl", out.ID, AuditSuccess, map[string]any{
		"domain": out.Domain, "feedback_address": out.FeedbackAddress, "status": out.Status,
	})
	return out, nil
}

// Update validates and updates an existing feedback-loop endpoint.
func (uc *FBLUsecase) Update(ctx context.Context, id string, f *FBLEndpoint) (*FBLEndpoint, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("FBL_ID_REQUIRED", "fbl endpoint id is required")
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateFBLEndpoint(ctx, id, f)
	if err != nil {
		uc.audit(ctx, "fbl.update", "fbl", id, AuditFailure, map[string]any{"domain": f.Domain})
		return nil, err
	}
	uc.audit(ctx, "fbl.update", "fbl", out.ID, AuditSuccess, map[string]any{
		"domain": out.Domain, "feedback_address": out.FeedbackAddress, "status": out.Status,
	})
	return out, nil
}

// Delete removes a feedback-loop endpoint.
func (uc *FBLUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if id == "" {
		return Invalid("FBL_ID_REQUIRED", "fbl endpoint id is required")
	}
	if err := uc.repo.DeleteFBLEndpoint(ctx, id); err != nil {
		uc.audit(ctx, "fbl.delete", "fbl", id, AuditFailure, nil)
		return err
	}
	uc.audit(ctx, "fbl.delete", "fbl", id, AuditSuccess, nil)
	return nil
}

func (uc *FBLUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
