package biz

import "context"

// InboundRouteRepo is the persistence boundary for inbound routes.
type InboundRouteRepo interface {
	CreateInboundRoute(ctx context.Context, r *InboundRoute) (*InboundRoute, error)
	UpdateInboundRoute(ctx context.Context, id string, r *InboundRoute) (*InboundRoute, error)
	DeleteInboundRoute(ctx context.Context, id string) error
	ListInboundRoutes(ctx context.Context, page Page) ([]*InboundRoute, error)
}

// InboundRouteUsecase implements inbound-route CRUD with authorization and
// auditing. It reuses the webhook permission scope (inbound routing is the
// generalization of inbound webhooks).
type InboundRouteUsecase struct {
	repo          InboundRouteRepo
	auditor       *Auditor
	allowInsecure bool
}

// NewInboundRouteUsecase constructs the use case. allowInsecure permits plain
// HTTP webhook destinations for local development.
func NewInboundRouteUsecase(repo InboundRouteRepo, auditor *Auditor, allowInsecure bool) *InboundRouteUsecase {
	return &InboundRouteUsecase{repo: repo, auditor: auditor, allowInsecure: allowInsecure}
}

// ListInboundRoutes returns routes after an authorization check.
func (uc *InboundRouteUsecase) ListInboundRoutes(ctx context.Context, page Page) ([]*InboundRoute, error) {
	if _, err := RequirePermission(ctx, PermWebhookRead); err != nil {
		return nil, err
	}
	return uc.repo.ListInboundRoutes(ctx, page)
}

// CreateInboundRoute validates and persists a route, auditing the change.
func (uc *InboundRouteUsecase) CreateInboundRoute(ctx context.Context, r *InboundRoute) (*InboundRoute, error) {
	if _, err := RequirePermission(ctx, PermWebhookWrite); err != nil {
		return nil, err
	}
	if err := r.Validate(uc.allowInsecure); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateInboundRoute(ctx, r)
	if err != nil {
		uc.audit(ctx, "inbound_route.create", "inbound_route", r.Name, AuditFailure, map[string]any{"name": r.Name})
		return nil, err
	}
	uc.audit(ctx, "inbound_route.create", "inbound_route", out.ID, AuditSuccess, routeSummary(out))
	return out, nil
}

// UpdateInboundRoute validates and updates an existing route.
func (uc *InboundRouteUsecase) UpdateInboundRoute(ctx context.Context, id string, r *InboundRoute) (*InboundRoute, error) {
	if _, err := RequirePermission(ctx, PermWebhookWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("INBOUND_ROUTE_ID_REQUIRED", "inbound route id is required")
	}
	if err := r.Validate(uc.allowInsecure); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateInboundRoute(ctx, id, r)
	if err != nil {
		uc.audit(ctx, "inbound_route.update", "inbound_route", id, AuditFailure, map[string]any{"name": r.Name})
		return nil, err
	}
	uc.audit(ctx, "inbound_route.update", "inbound_route", out.ID, AuditSuccess, routeSummary(out))
	return out, nil
}

// DeleteInboundRoute removes a route.
func (uc *InboundRouteUsecase) DeleteInboundRoute(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermWebhookWrite); err != nil {
		return err
	}
	if id == "" {
		return Invalid("INBOUND_ROUTE_ID_REQUIRED", "inbound route id is required")
	}
	if err := uc.repo.DeleteInboundRoute(ctx, id); err != nil {
		uc.audit(ctx, "inbound_route.delete", "inbound_route", id, AuditFailure, nil)
		return err
	}
	uc.audit(ctx, "inbound_route.delete", "inbound_route", id, AuditSuccess, nil)
	return nil
}

// routeSummary builds an audit summary, omitting the webhook secret by design.
func routeSummary(r *InboundRoute) map[string]any {
	return map[string]any{
		"name": r.Name, "action": r.Action,
		"match_type": r.MatchType, "match_value": r.MatchValue, "status": r.Status,
	}
}

func (uc *InboundRouteUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
