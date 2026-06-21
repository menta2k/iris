package biz

import "context"

// OutboundConfigRepo is the persistence boundary for outbound configuration.
type OutboundConfigRepo interface {
	CreateListener(ctx context.Context, l *Listener) (*Listener, error)
	UpdateListener(ctx context.Context, id string, l *Listener) (*Listener, error)
	ListListeners(ctx context.Context, page Page) ([]*Listener, error)
	ListenerExists(ctx context.Context, id string) (bool, error)

	CreateVMTA(ctx context.Context, v *VMTA) (*VMTA, error)
	UpdateVMTA(ctx context.Context, id string, v *VMTA) (*VMTA, error)
	ListVMTAs(ctx context.Context, status string, page Page) ([]*VMTA, error)
	VMTAExists(ctx context.Context, id string) (bool, error)

	CreateVMTAGroup(ctx context.Context, g *VMTAGroup) (*VMTAGroup, error)
	UpdateVMTAGroup(ctx context.Context, id string, g *VMTAGroup) (*VMTAGroup, error)
	ListVMTAGroups(ctx context.Context, page Page) ([]*VMTAGroup, error)

	CreateRoutingRule(ctx context.Context, r *RoutingRule) (*RoutingRule, error)
	UpdateRoutingRule(ctx context.Context, id string, r *RoutingRule) (*RoutingRule, error)
	ListRoutingRules(ctx context.Context, matchType, matchValue string, page Page) ([]*RoutingRule, error)
	TargetExists(ctx context.Context, targetType, id string) (bool, error)
}

// RecipientEligibilityChecker reports whether a recipient may receive outbound
// mail. It is implemented by the domain-safety use case (US4) and integrated
// here so suppression is enforced as part of send eligibility.
type RecipientEligibilityChecker interface {
	IsRecipientEligible(ctx context.Context, recipient string) (bool, error)
}

// OutboundConfigUsecase implements outbound sending configuration (US1):
// VMTAs, VMTA groups, and routing rules, with authorization and audit logging.
type OutboundConfigUsecase struct {
	repo        OutboundConfigRepo
	auditor     *Auditor
	eligibility RecipientEligibilityChecker
}

// NewOutboundConfigUsecase constructs the use case.
func NewOutboundConfigUsecase(repo OutboundConfigRepo, auditor *Auditor) *OutboundConfigUsecase {
	return &OutboundConfigUsecase{repo: repo, auditor: auditor}
}

// WithEligibilityChecker wires the suppression-based recipient eligibility
// checker (US4 integration). Returns the use case for fluent construction.
func (uc *OutboundConfigUsecase) WithEligibilityChecker(c RecipientEligibilityChecker) *OutboundConfigUsecase {
	uc.eligibility = c
	return uc
}

// EvaluateRecipient returns whether a recipient is eligible for outbound send,
// enforcing suppression entries when an eligibility checker is configured. With
// no checker wired, recipients are eligible by default.
func (uc *OutboundConfigUsecase) EvaluateRecipient(ctx context.Context, recipient string) (bool, error) {
	if uc.eligibility == nil {
		return true, nil
	}
	return uc.eligibility.IsRecipientEligible(ctx, recipient)
}

// requireListener returns a validation error if the referenced listener is
// missing.
func (uc *OutboundConfigUsecase) requireListener(ctx context.Context, id string) error {
	ok, err := uc.repo.ListenerExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return Invalid("VMTA_LISTENER_MISSING", "listener %q does not exist", id)
	}
	return nil
}

// ListListeners returns ESMTP listeners after a read authorization check.
func (uc *OutboundConfigUsecase) ListListeners(ctx context.Context, page Page) ([]*Listener, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	return uc.repo.ListListeners(ctx, page)
}

// CreateListener validates, persists, and audits a new listener.
func (uc *OutboundConfigUsecase) CreateListener(ctx context.Context, l *Listener) (*Listener, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateListener(ctx, l)
	if err != nil {
		uc.audit(ctx, "listener.create", "listener", l.Name, AuditFailure, map[string]any{"name": l.Name})
		return nil, err
	}
	uc.audit(ctx, "listener.create", "listener", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "ip_address": out.IPAddress, "port": out.Port, "hostname": out.Hostname,
	})
	return out, nil
}

// UpdateListener validates, updates, and audits an existing listener.
func (uc *OutboundConfigUsecase) UpdateListener(ctx context.Context, id string, l *Listener) (*Listener, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("LISTENER_ID_REQUIRED", "listener id is required")
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateListener(ctx, id, l)
	if err != nil {
		uc.audit(ctx, "listener.update", "listener", id, AuditFailure, map[string]any{"name": l.Name})
		return nil, err
	}
	uc.audit(ctx, "listener.update", "listener", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "ip_address": out.IPAddress, "port": out.Port, "hostname": out.Hostname, "status": out.Status,
	})
	return out, nil
}

// ListVMTAs returns VMTAs after a read authorization check.
func (uc *OutboundConfigUsecase) ListVMTAs(ctx context.Context, status string, page Page) ([]*VMTA, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	if status != "" && !ValidVMTAStatus(status) {
		return nil, Invalid("VMTA_STATUS_INVALID", "status %q is not valid", status)
	}
	return uc.repo.ListVMTAs(ctx, status, page)
}

// CreateVMTA validates, ensures the listener exists, persists, and audits a VMTA.
func (uc *OutboundConfigUsecase) CreateVMTA(ctx context.Context, v *VMTA) (*VMTA, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if err := uc.requireListener(ctx, v.ListenerID); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateVMTA(ctx, v)
	if err != nil {
		uc.audit(ctx, "vmta.create", "vmta", v.Name, AuditFailure, map[string]any{"name": v.Name})
		return nil, err
	}
	uc.audit(ctx, "vmta.create", "vmta", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "ip_address": out.IPAddress, "ehlo_name": out.EHLOName,
	})
	return out, nil
}

// UpdateVMTA validates and updates an existing VMTA, auditing the change.
func (uc *OutboundConfigUsecase) UpdateVMTA(ctx context.Context, id string, v *VMTA) (*VMTA, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("VMTA_ID_REQUIRED", "vmta id is required")
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if err := uc.requireListener(ctx, v.ListenerID); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateVMTA(ctx, id, v)
	if err != nil {
		uc.audit(ctx, "vmta.update", "vmta", id, AuditFailure, map[string]any{"name": v.Name})
		return nil, err
	}
	uc.audit(ctx, "vmta.update", "vmta", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "ip_address": out.IPAddress, "ehlo_name": out.EHLOName, "status": out.Status,
	})
	return out, nil
}

// ListVMTAGroups returns VMTA groups after a read authorization check.
func (uc *OutboundConfigUsecase) ListVMTAGroups(ctx context.Context, page Page) ([]*VMTAGroup, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	return uc.repo.ListVMTAGroups(ctx, page)
}

// CreateVMTAGroup validates members, ensures referenced VMTAs exist, persists,
// and audits a new group.
func (uc *OutboundConfigUsecase) CreateVMTAGroup(ctx context.Context, g *VMTAGroup) (*VMTAGroup, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	for _, m := range g.Members {
		ok, err := uc.repo.VMTAExists(ctx, m.VMTAID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Invalid("VMTA_GROUP_MEMBER_MISSING", "vmta %q does not exist", m.VMTAID)
		}
	}
	out, err := uc.repo.CreateVMTAGroup(ctx, g)
	if err != nil {
		uc.audit(ctx, "vmta_group.create", "vmta_group", g.Name, AuditFailure, map[string]any{"name": g.Name})
		return nil, err
	}
	uc.audit(ctx, "vmta_group.create", "vmta_group", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "member_count": len(out.Members), "total_weight": out.TotalWeight(),
	})
	return out, nil
}

// UpdateVMTAGroup validates members, ensures referenced VMTAs exist, updates,
// and audits an existing group.
func (uc *OutboundConfigUsecase) UpdateVMTAGroup(ctx context.Context, id string, g *VMTAGroup) (*VMTAGroup, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("VMTA_GROUP_ID_REQUIRED", "vmta group id is required")
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	for _, m := range g.Members {
		ok, err := uc.repo.VMTAExists(ctx, m.VMTAID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Invalid("VMTA_GROUP_MEMBER_MISSING", "vmta %q does not exist", m.VMTAID)
		}
	}
	out, err := uc.repo.UpdateVMTAGroup(ctx, id, g)
	if err != nil {
		uc.audit(ctx, "vmta_group.update", "vmta_group", id, AuditFailure, map[string]any{"name": g.Name})
		return nil, err
	}
	uc.audit(ctx, "vmta_group.update", "vmta_group", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "status": out.Status, "member_count": len(out.Members), "total_weight": out.TotalWeight(),
	})
	return out, nil
}

// ListRoutingRules returns routing rules after a read authorization check.
func (uc *OutboundConfigUsecase) ListRoutingRules(ctx context.Context, matchType, matchValue string, page Page) ([]*RoutingRule, error) {
	if _, err := RequirePermission(ctx, PermRoutingRead); err != nil {
		return nil, err
	}
	if matchType != "" && !ValidMatchType(matchType) {
		return nil, Invalid("ROUTING_MATCH_TYPE_INVALID", "match_type %q is not valid", matchType)
	}
	return uc.repo.ListRoutingRules(ctx, matchType, SanitizeFilter(matchValue), page)
}

// CreateRoutingRule validates, ensures the target exists, persists, and audits
// a new routing rule.
func (uc *OutboundConfigUsecase) CreateRoutingRule(ctx context.Context, rule *RoutingRule) (*RoutingRule, error) {
	if _, err := RequirePermission(ctx, PermRoutingWrite); err != nil {
		return nil, err
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	// sender_ip rules carry no VMTA/group target; skip the existence check.
	if rule.MatchType != MatchSenderIP {
		ok, err := uc.repo.TargetExists(ctx, rule.TargetType, rule.TargetID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Invalid("ROUTING_TARGET_MISSING", "routing target %q %q does not exist", rule.TargetType, rule.TargetID)
		}
	}
	out, err := uc.repo.CreateRoutingRule(ctx, rule)
	if err != nil {
		uc.audit(ctx, "routing_rule.create", "routing_rule", rule.Name, AuditFailure, map[string]any{"name": rule.Name})
		return nil, err
	}
	uc.audit(ctx, "routing_rule.create", "routing_rule", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "match_type": out.MatchType, "match_value": out.MatchValue,
		"priority": out.Priority, "target_type": out.TargetType, "target_id": out.TargetID,
	})
	return out, nil
}

// UpdateRoutingRule validates, ensures the target exists, updates, and audits
// an existing routing rule.
func (uc *OutboundConfigUsecase) UpdateRoutingRule(ctx context.Context, id string, rule *RoutingRule) (*RoutingRule, error) {
	if _, err := RequirePermission(ctx, PermRoutingWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("ROUTING_ID_REQUIRED", "routing rule id is required")
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	// sender_ip rules carry no VMTA/group target; skip the existence check.
	if rule.MatchType != MatchSenderIP {
		ok, err := uc.repo.TargetExists(ctx, rule.TargetType, rule.TargetID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Invalid("ROUTING_TARGET_MISSING", "routing target %q %q does not exist", rule.TargetType, rule.TargetID)
		}
	}
	out, err := uc.repo.UpdateRoutingRule(ctx, id, rule)
	if err != nil {
		uc.audit(ctx, "routing_rule.update", "routing_rule", id, AuditFailure, map[string]any{"name": rule.Name})
		return nil, err
	}
	uc.audit(ctx, "routing_rule.update", "routing_rule", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "match_type": out.MatchType, "match_value": out.MatchValue,
		"priority": out.Priority, "target_type": out.TargetType, "target_id": out.TargetID, "status": out.Status,
	})
	return out, nil
}

// audit records an audit event, logging and swallowing audit-write errors so a
// failed audit does not mask the primary operation result.
func (uc *OutboundConfigUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
