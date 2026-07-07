package biz

import (
	"context"
	"time"
)

// EventProcessorRepo is the persistence boundary for Event Processor rules.
type EventProcessorRepo interface {
	CreateEventProcessor(ctx context.Context, p *EventProcessor) (*EventProcessor, error)
	UpdateEventProcessor(ctx context.Context, id string, p *EventProcessor) (*EventProcessor, error)
	DeleteEventProcessor(ctx context.Context, id string) error
	ListEventProcessors(ctx context.Context) ([]*EventProcessor, error)
	ActiveEventProcessors(ctx context.Context) ([]*EventProcessor, error)
}

// EventProcessorUsecase manages Event Processor rules and can send a test event
// through a processor's configured driver to validate the integration.
type EventProcessorUsecase struct {
	repo     EventProcessorRepo
	registry *EventDriverRegistry
	auditor  *Auditor
}

// NewEventProcessorUsecase constructs the use case. registry may be nil (Test
// then returns unavailable).
func NewEventProcessorUsecase(repo EventProcessorRepo, registry *EventDriverRegistry, auditor *Auditor) *EventProcessorUsecase {
	return &EventProcessorUsecase{repo: repo, registry: registry, auditor: auditor}
}

// List returns all processors.
func (uc *EventProcessorUsecase) List(ctx context.Context) ([]*EventProcessor, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	return uc.repo.ListEventProcessors(ctx)
}

// Create validates and persists a processor.
func (uc *EventProcessorUsecase) Create(ctx context.Context, p *EventProcessor) (*EventProcessor, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if err := ValidateEventProcessor(p); err != nil {
		return nil, err
	}
	if uc.registry != nil && !uc.registry.Has(p.Driver) {
		return nil, Invalid("EVENT_DRIVER_UNKNOWN", "no delivery driver %q is registered", p.Driver)
	}
	out, err := uc.repo.CreateEventProcessor(ctx, p)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "event_processor.create", out.ID, map[string]any{"driver": out.Driver, "mode": out.Mode})
	return out, nil
}

// Update validates and persists an edit.
func (uc *EventProcessorUsecase) Update(ctx context.Context, id string, p *EventProcessor) (*EventProcessor, error) {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return nil, err
	}
	if err := ValidateEventProcessor(p); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdateEventProcessor(ctx, id, p)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "event_processor.update", id, map[string]any{"driver": out.Driver})
	return out, nil
}

// Delete removes a processor.
func (uc *EventProcessorUsecase) Delete(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if err := uc.repo.DeleteEventProcessor(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, AuditSuccess, "event_processor.delete", id, nil)
	return nil
}

// Test sends a synthetic event through the processor's driver to verify the
// integration end-to-end. Returns the delivery error (nil = success).
func (uc *EventProcessorUsecase) Test(ctx context.Context, p *EventProcessor) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if err := ValidateEventProcessor(p); err != nil {
		return err
	}
	if uc.registry == nil {
		return Unavailable("EVENT_REGISTRY_UNAVAILABLE", "no driver registry configured")
	}
	driver, err := uc.registry.Build(p)
	if err != nil {
		return err
	}
	eventType := EventBounce
	if len(p.EventTypes) > 0 {
		eventType = p.EventTypes[0]
	}
	mailclass := ""
	if len(p.Mailclasses) > 0 {
		mailclass = p.Mailclasses[0]
	}
	sample := DispatchEvent{
		Type:       eventType,
		OccurredAt: time.Now().UTC(),
		Mailclass:  mailclass,
		Data:       map[string]any{"test": true, "message": "iris event processor test event"},
	}
	if err := driver.Deliver(ctx, []DispatchEvent{sample}); err != nil {
		return Unavailable("EVENT_TEST_DELIVERY_FAILED", "test delivery failed: %v", err)
	}
	return nil
}

func (uc *EventProcessorUsecase) audit(ctx context.Context, outcome AuditOutcome, action, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "event_processor", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
