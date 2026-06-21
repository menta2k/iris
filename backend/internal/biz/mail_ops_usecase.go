package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// MailOpsRepo is the persistence boundary for mail operations.
type MailOpsRepo interface {
	ListMailRecords(ctx context.Context, f MailFilter, page Page) ([]*MailRecord, error)
	ListBounces(ctx context.Context, page Page) ([]*BounceRecord, error)
	ListFeedbackReports(ctx context.Context, page Page) ([]*FeedbackReport, error)
	ListQueues(ctx context.Context, page Page) ([]*MailclassQueue, error)
	CreateServiceControlRequest(ctx context.Context, rec *ServiceControlRecord) (*ServiceControlRecord, error)
	ActiveServiceControlExists(ctx context.Context) (bool, error)
	UpdateServiceControlStatus(ctx context.Context, id, status, resultSummary string) error
}

// CommandProducer enqueues asynchronous queue and service-control commands.
type CommandProducer interface {
	PublishQueueCommand(ctx context.Context, mailclass, action, confirmationID string) (string, error)
	PublishServiceCommand(ctx context.Context, requestID, operation string) (string, error)
}

// MailOpsUsecase implements mail-flow operations (US2): mail log queries,
// bounce/feedback inspection, queue control, and KumoMTA service control.
type MailOpsUsecase struct {
	repo     MailOpsRepo
	producer CommandProducer
	auditor  *Auditor
}

// NewMailOpsUsecase constructs the use case.
func NewMailOpsUsecase(repo MailOpsRepo, producer CommandProducer, auditor *Auditor) *MailOpsUsecase {
	return &MailOpsUsecase{repo: repo, producer: producer, auditor: auditor}
}

// ListMailRecords returns filtered mail-log records.
func (uc *MailOpsUsecase) ListMailRecords(ctx context.Context, f MailFilter, page Page) ([]*MailRecord, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	nf, err := NormalizeMailFilter(f)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListMailRecords(ctx, nf, page)
}

// ListBounces returns bounce records.
func (uc *MailOpsUsecase) ListBounces(ctx context.Context, page Page) ([]*BounceRecord, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.ListBounces(ctx, page)
}

// ListFeedbackReports returns feedback reports.
func (uc *MailOpsUsecase) ListFeedbackReports(ctx context.Context, page Page) ([]*FeedbackReport, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.ListFeedbackReports(ctx, page)
}

// ListQueues returns current per-mailclass queue snapshots.
func (uc *MailOpsUsecase) ListQueues(ctx context.Context, page Page) ([]*MailclassQueue, error) {
	if _, err := RequirePermission(ctx, PermQueueRead); err != nil {
		return nil, err
	}
	return uc.repo.ListQueues(ctx, page)
}

// QueueActionResult is returned after enqueueing a queue command.
type QueueActionResult struct {
	RequestID string
	Status    string
}

// RequestQueueAction validates and enqueues a queue-control command, auditing
// the request.
func (uc *MailOpsUsecase) RequestQueueAction(ctx context.Context, mailclass, action, confirmationID string) (*QueueActionResult, error) {
	id, err := RequirePermission(ctx, PermQueueControl)
	if err != nil {
		uc.audit(ctx, "queue.action", "queue", mailclass, AuditDenied, map[string]any{"action": action})
		return nil, err
	}
	if err := ValidateQueueActionRequest(mailclass, action, confirmationID); err != nil {
		return nil, err
	}
	reqID, err := uc.producer.PublishQueueCommand(ctx, mailclass, action, confirmationID)
	if err != nil {
		uc.audit(ctx, "queue.action", "queue", mailclass, AuditFailure, map[string]any{"action": action})
		return nil, Internal(err, "enqueue queue command")
	}
	_ = id
	uc.audit(ctx, "queue.action", "queue", mailclass, AuditSuccess, map[string]any{
		"action": action, "request_id": reqID,
	})
	return &QueueActionResult{RequestID: reqID, Status: "pending"}, nil
}

// RequestServiceControl validates, persists, and enqueues a serialized KumoMTA
// service-control request. Only one service-control operation may be active.
func (uc *MailOpsUsecase) RequestServiceControl(ctx context.Context, operation, confirmationID string) (*ServiceControlRecord, error) {
	id, err := RequirePermission(ctx, PermServiceControl)
	if err != nil {
		uc.audit(ctx, "service.control", "kumomta", operation, AuditDenied, map[string]any{"operation": operation})
		return nil, err
	}
	if err := ValidateServiceControlRequest(operation, confirmationID); err != nil {
		return nil, err
	}
	active, err := uc.repo.ActiveServiceControlExists(ctx)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, Conflict("SERVICE_CONTROL_ACTIVE", "another service-control operation is already in progress")
	}

	rec := &ServiceControlRecord{Operation: operation, ConfirmationID: confirmationID, RequestedBy: id.UserID}
	stored, err := uc.repo.CreateServiceControlRequest(ctx, rec)
	if err != nil {
		uc.audit(ctx, "service.control", "kumomta", operation, AuditFailure, map[string]any{"operation": operation})
		return nil, err
	}
	if _, err := uc.producer.PublishServiceCommand(ctx, stored.ID, operation); err != nil {
		// Roll the request back to failed so it does not block future requests.
		_ = uc.repo.UpdateServiceControlStatus(ctx, stored.ID, SvcFailed, "failed to enqueue command")
		uc.audit(ctx, "service.control", "kumomta", operation, AuditFailure, map[string]any{"operation": operation})
		return nil, Internal(err, "enqueue service command")
	}
	uc.audit(ctx, "service.control", "kumomta", stored.ID, AuditSuccess, map[string]any{
		"operation": operation, "request_id": stored.ID,
	})
	return stored, nil
}

func (uc *MailOpsUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}

// NewConfirmationID returns a random confirmation token for destructive actions.
func NewConfirmationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
