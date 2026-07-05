package biz

import (
	"context"
	"strings"
)

// AutomationRepo is the persistence boundary for TSA automation rules.
type AutomationRepo interface {
	CreateAutomationRule(ctx context.Context, a *AutomationRule) (*AutomationRule, error)
	UpdateAutomationRule(ctx context.Context, id string, a *AutomationRule) (*AutomationRule, error)
	GetAutomationRule(ctx context.Context, id string) (*AutomationRule, error)
	ListAutomationRules(ctx context.Context) ([]*AutomationRule, error)
	ListActiveAutomationForPolicy(ctx context.Context) ([]*AutomationRule, error)
}

// AutomationUsecase manages operator-authored TSA automation rules. Reuses the
// VMTA permissions (outbound sending configuration).
type AutomationUsecase struct {
	repo    AutomationRepo
	auditor *Auditor
}

// NewAutomationUsecase constructs the use case.
func NewAutomationUsecase(repo AutomationRepo, auditor *Auditor) *AutomationUsecase {
	return &AutomationUsecase{repo: repo, auditor: auditor}
}

// List returns all automation rules.
func (uc *AutomationUsecase) List(ctx context.Context) ([]*AutomationRule, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	return uc.repo.ListAutomationRules(ctx)
}

// ActiveForPolicy returns active rules for rendering. Internal; no permission.
func (uc *AutomationUsecase) ActiveForPolicy(ctx context.Context) ([]*AutomationRule, error) {
	return uc.repo.ListActiveAutomationForPolicy(ctx)
}

// Create validates and persists a new rule.
func (uc *AutomationUsecase) Create(ctx context.Context, a *AutomationRule) (*AutomationRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateAutomationRule(ctx, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "automation.create", out.ID, map[string]any{"domain": out.Domain, "action": out.Action})
	return out, nil
}

// Update validates and persists an edit.
func (uc *AutomationUsecase) Update(ctx context.Context, id string, a *AutomationRule) (*AutomationRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateAutomationRule(ctx, id, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "automation.update", id, map[string]any{"domain": out.Domain})
	return out, nil
}

// SetStatus enables/disables a rule.
func (uc *AutomationUsecase) SetStatus(ctx context.Context, id, status string) (*AutomationRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status != AutomationActive && status != AutomationDisabled {
		return nil, Invalid("AUTOMATION_STATUS_INVALID", "status %q is not valid", status)
	}
	a, err := uc.repo.GetAutomationRule(ctx, id)
	if err != nil {
		return nil, err
	}
	a.Status = status
	out, err := uc.repo.UpdateAutomationRule(ctx, id, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "automation.set_status", id, map[string]any{"status": status})
	return out, nil
}

func (uc *AutomationUsecase) audit(ctx context.Context, outcome AuditOutcome, action, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "tsa_automation_rule", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
