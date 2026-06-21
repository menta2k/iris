package biz

import "context"

// InboundRepo is the persistence boundary for inbound automation.
type InboundRepo interface {
	CreateWebhookRule(ctx context.Context, w *WebhookRule) (*WebhookRule, error)
	UpdateWebhookRule(ctx context.Context, id string, w *WebhookRule) (*WebhookRule, error)
	ListWebhookRules(ctx context.Context, page Page) ([]*WebhookRule, error)
	MatchWebhookRules(ctx context.Context, recipient string) ([]*WebhookRule, error)
	RecordDeliveryEvent(ctx context.Context, e *WebhookDeliveryEvent) error
	ListWebhookDeliveries(ctx context.Context, page Page) ([]*WebhookDeliveryEvent, error)
	CreateRspamdResult(ctx context.Context, res *RspamdFilterResult) error
	ListRspamdResults(ctx context.Context, page Page) ([]*RspamdFilterResult, error)
}

// InboundUsecase implements inbound webhook and Rspamd handling (US5).
type InboundUsecase struct {
	repo          InboundRepo
	auditor       *Auditor
	allowInsecure bool
}

// NewInboundUsecase constructs the use case. allowInsecure permits plain-HTTP
// webhook destinations for local development.
func NewInboundUsecase(repo InboundRepo, auditor *Auditor, allowInsecure bool) *InboundUsecase {
	return &InboundUsecase{repo: repo, auditor: auditor, allowInsecure: allowInsecure}
}

// ListWebhookRules returns webhook rules after an authorization check.
func (uc *InboundUsecase) ListWebhookRules(ctx context.Context, page Page) ([]*WebhookRule, error) {
	if _, err := RequirePermission(ctx, PermWebhookRead); err != nil {
		return nil, err
	}
	return uc.repo.ListWebhookRules(ctx, page)
}

// ListWebhookDeliveries returns recent webhook delivery attempts, newest first.
func (uc *InboundUsecase) ListWebhookDeliveries(ctx context.Context, page Page) ([]*WebhookDeliveryEvent, error) {
	if _, err := RequirePermission(ctx, PermWebhookRead); err != nil {
		return nil, err
	}
	return uc.repo.ListWebhookDeliveries(ctx, page)
}

// CreateWebhookRule validates and persists a webhook rule, auditing the change.
func (uc *InboundUsecase) CreateWebhookRule(ctx context.Context, w *WebhookRule) (*WebhookRule, error) {
	if _, err := RequirePermission(ctx, PermWebhookWrite); err != nil {
		return nil, err
	}
	if err := w.Validate(uc.allowInsecure); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateWebhookRule(ctx, w)
	if err != nil {
		uc.audit(ctx, "webhook.create", "webhook", w.Name, AuditFailure, map[string]any{"name": w.Name})
		return nil, err
	}
	// The audit summary omits the webhook secret_ref by design.
	uc.audit(ctx, "webhook.create", "webhook", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "match_type": out.MatchType, "match_value": out.MatchValue,
		"destination_url": out.DestinationURL,
	})
	return out, nil
}

// UpdateWebhookRule validates and updates an existing webhook rule.
func (uc *InboundUsecase) UpdateWebhookRule(ctx context.Context, id string, w *WebhookRule) (*WebhookRule, error) {
	if _, err := RequirePermission(ctx, PermWebhookWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("WEBHOOK_ID_REQUIRED", "webhook id is required")
	}
	if w.Status == "" {
		w.Status = WebhookActive
	}
	if w.Status != WebhookActive && w.Status != WebhookDisabled {
		return nil, Invalid("WEBHOOK_STATUS_INVALID", "status %q is not valid", w.Status)
	}
	if err := w.Validate(uc.allowInsecure); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateWebhookRule(ctx, id, w)
	if err != nil {
		uc.audit(ctx, "webhook.update", "webhook", id, AuditFailure, map[string]any{"name": w.Name})
		return nil, err
	}
	uc.audit(ctx, "webhook.update", "webhook", out.ID, AuditSuccess, map[string]any{
		"name": out.Name, "match_type": out.MatchType, "match_value": out.MatchValue,
		"destination_url": out.DestinationURL, "status": out.Status,
	})
	return out, nil
}

// MatchWebhookRules returns the active webhook rules matching a recipient. Used
// by the webhook delivery worker when inbound mail arrives.
func (uc *InboundUsecase) MatchWebhookRules(ctx context.Context, recipient string) ([]*WebhookRule, error) {
	return uc.repo.MatchWebhookRules(ctx, recipient)
}

// RecordDelivery persists a webhook delivery attempt.
func (uc *InboundUsecase) RecordDelivery(ctx context.Context, e *WebhookDeliveryEvent) error {
	return uc.repo.RecordDeliveryEvent(ctx, e)
}

// ListRspamdResults returns filter results after an authorization check.
func (uc *InboundUsecase) ListRspamdResults(ctx context.Context, page Page) ([]*RspamdFilterResult, error) {
	if _, err := RequirePermission(ctx, PermRspamdRead); err != nil {
		return nil, err
	}
	return uc.repo.ListRspamdResults(ctx, page)
}

// IngestRspamdResult persists a filter result coming from the ingestion worker.
func (uc *InboundUsecase) IngestRspamdResult(ctx context.Context, res *RspamdFilterResult) error {
	return uc.repo.CreateRspamdResult(ctx, res)
}

func (uc *InboundUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
