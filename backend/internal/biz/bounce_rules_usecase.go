package biz

import "context"

// BounceRuleRepo is the persistence boundary for bounce-action rules.
type BounceRuleRepo interface {
	CreateBounceRule(ctx context.Context, a *BounceActionRule) (*BounceActionRule, error)
	UpdateBounceRule(ctx context.Context, id string, a *BounceActionRule) (*BounceActionRule, error)
	DeleteBounceRule(ctx context.Context, id string) error
	ListBounceRules(ctx context.Context) ([]*BounceActionRule, error)
	ListActiveBounceRules(ctx context.Context) ([]*BounceActionRule, error)
	CountBounceRules(ctx context.Context) (int, error)
	ReplaceDefaultBounceRules(ctx context.Context, rules []*BounceActionRule) error
}

// BounceRuleUsecase manages the bounce classification & response ruleset. It
// reuses the VMTA/sending-configuration permissions (bounce policy is part of
// outbound deliverability). Reads seed the curated defaults on first use.
type BounceRuleUsecase struct {
	repo    BounceRuleRepo
	auditor *Auditor
}

// NewBounceRuleUsecase constructs the use case.
func NewBounceRuleUsecase(repo BounceRuleRepo, auditor *Auditor) *BounceRuleUsecase {
	return &BounceRuleUsecase{repo: repo, auditor: auditor}
}

// BounceMatchResult is the outcome of testing a bounce against the ruleset.
type BounceMatchResult struct {
	Signature BounceSignature   // normalized (enhanced/provider filled in)
	Matched   *BounceActionRule // nil when no rule matched (default = retry)
}

// List returns every rule, seeding the curated defaults the first time the table
// is empty so the console is never blank on a fresh deployment.
func (uc *BounceRuleUsecase) List(ctx context.Context) ([]*BounceActionRule, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	if err := uc.seedIfEmpty(ctx); err != nil {
		return nil, err
	}
	return uc.repo.ListBounceRules(ctx)
}

// Create validates and persists an operator (overlay) rule.
func (uc *BounceRuleUsecase) Create(ctx context.Context, a *BounceActionRule) (*BounceActionRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	a.Source = BounceRuleSourceOverlay
	if err := ValidateBounceRule(a); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateBounceRule(ctx, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "bounce_rule.create", out.ID, map[string]any{"action": out.Action, "category": out.Category})
	return out, nil
}

// Update validates and persists an edit.
func (uc *BounceRuleUsecase) Update(ctx context.Context, id string, a *BounceActionRule) (*BounceActionRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := ValidateBounceRule(a); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateBounceRule(ctx, id, a)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "bounce_rule.update", id, map[string]any{"action": out.Action})
	return out, nil
}

// Delete removes a rule.
func (uc *BounceRuleUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return err
	}
	if err := uc.repo.DeleteBounceRule(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, AuditSuccess, "bounce_rule.delete", id, nil)
	return nil
}

// ResetToDefaults restores the curated default ruleset, leaving operator overlay
// rules in place.
func (uc *BounceRuleUsecase) ResetToDefaults(ctx context.Context) ([]*BounceActionRule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := uc.repo.ReplaceDefaultBounceRules(ctx, DefaultBounceRules()); err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "bounce_rule.reset_defaults", "", nil)
	return uc.repo.ListBounceRules(ctx)
}

// TestDiagnostic evaluates a bounce signature against the active ruleset and
// returns the matched rule (or nil). Read-only; used by the console's tester.
func (uc *BounceRuleUsecase) TestDiagnostic(ctx context.Context, sig BounceSignature) (*BounceMatchResult, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	if err := uc.seedIfEmpty(ctx); err != nil {
		return nil, err
	}
	rules, err := uc.repo.ListActiveBounceRules(ctx)
	if err != nil {
		return nil, err
	}
	return uc.match(rules, sig), nil
}

// ActiveRules returns the active ruleset (internal; for the worker/render).
func (uc *BounceRuleUsecase) ActiveRules(ctx context.Context) ([]*BounceActionRule, error) {
	return uc.repo.ListActiveBounceRules(ctx)
}

// match runs the pure matcher and packages the normalized signature.
func (uc *BounceRuleUsecase) match(rules []*BounceActionRule, sig BounceSignature) *BounceMatchResult {
	return &BounceMatchResult{
		Signature: sig.normalize(),
		Matched:   MatchBounceRule(rules, sig),
	}
}

func (uc *BounceRuleUsecase) seedIfEmpty(ctx context.Context) error {
	n, err := uc.repo.CountBounceRules(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return uc.repo.ReplaceDefaultBounceRules(ctx, DefaultBounceRules())
}

func (uc *BounceRuleUsecase) audit(ctx context.Context, outcome AuditOutcome, action, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "bounce_action_rule", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
