package biz

import (
	"context"
	"strings"
)

// BlueprintRepo is the persistence boundary for delivery blueprints.
type BlueprintRepo interface {
	CreateBlueprint(ctx context.Context, b *DeliveryBlueprint) (*DeliveryBlueprint, error)
	UpdateBlueprint(ctx context.Context, id string, b *DeliveryBlueprint) (*DeliveryBlueprint, error)
	GetBlueprint(ctx context.Context, id string) (*DeliveryBlueprint, error)
	ListBlueprints(ctx context.Context) ([]*DeliveryBlueprint, error)
	ListActiveBlueprintsForPolicy(ctx context.Context) ([]*DeliveryBlueprint, error)
	SeedDefaults(ctx context.Context, defaults []*DeliveryBlueprint) (int, error)
}

// BlueprintUsecase manages the base traffic-shaping blueprints that seed new IPs
// and act as the fallback for unknown sending domains. Reuses the VMTA
// permissions (outbound sending configuration).
type BlueprintUsecase struct {
	repo    BlueprintRepo
	auditor *Auditor
}

// NewBlueprintUsecase constructs the use case.
func NewBlueprintUsecase(repo BlueprintRepo, auditor *Auditor) *BlueprintUsecase {
	return &BlueprintUsecase{repo: repo, auditor: auditor}
}

// List returns all blueprints (provider then MX pattern).
func (uc *BlueprintUsecase) List(ctx context.Context) ([]*DeliveryBlueprint, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	return uc.repo.ListBlueprints(ctx)
}

// ActiveForPolicy returns active blueprints for rendering. Internal; no
// permission check.
func (uc *BlueprintUsecase) ActiveForPolicy(ctx context.Context) ([]*DeliveryBlueprint, error) {
	return uc.repo.ListActiveBlueprintsForPolicy(ctx)
}

// Create validates and persists a new blueprint.
func (uc *BlueprintUsecase) Create(ctx context.Context, b *DeliveryBlueprint) (*DeliveryBlueprint, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateBlueprint(ctx, b)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "blueprint.create", out.ID, map[string]any{"mx": out.MXPattern})
	return out, nil
}

// Update validates and persists an edit.
func (uc *BlueprintUsecase) Update(ctx context.Context, id string, b *DeliveryBlueprint) (*DeliveryBlueprint, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateBlueprint(ctx, id, b)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "blueprint.update", id, map[string]any{"mx": out.MXPattern})
	return out, nil
}

// SetStatus toggles a blueprint active/disabled (the power button in the UI).
func (uc *BlueprintUsecase) SetStatus(ctx context.Context, id, status string) (*DeliveryBlueprint, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status != BlueprintActive && status != BlueprintDisabled {
		return nil, Invalid("BLUEPRINT_STATUS_INVALID", "status %q is not valid", status)
	}
	b, err := uc.repo.GetBlueprint(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Status = status
	out, err := uc.repo.UpdateBlueprint(ctx, id, b)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "blueprint.set_status", id, map[string]any{"status": status})
	return out, nil
}

// SeedDefaults imports the built-in provider blueprints, skipping existing MX
// patterns. Returns the number inserted.
func (uc *BlueprintUsecase) SeedDefaults(ctx context.Context) (int, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return 0, err
	}
	defaults := DefaultBlueprints()
	ptrs := make([]*DeliveryBlueprint, len(defaults))
	for i := range defaults {
		d := defaults[i]
		ptrs[i] = &d
	}
	n, err := uc.repo.SeedDefaults(ctx, ptrs)
	if err != nil {
		return 0, err
	}
	uc.audit(ctx, AuditSuccess, "blueprint.seed_defaults", "", map[string]any{"inserted": n})
	return n, nil
}

func (uc *BlueprintUsecase) audit(ctx context.Context, outcome AuditOutcome, action, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "delivery_blueprint", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
