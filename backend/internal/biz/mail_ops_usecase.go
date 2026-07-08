package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// MailOpsRepo is the persistence boundary for mail operations.
type MailOpsRepo interface {
	ListMailRecords(ctx context.Context, f MailFilter, page Page) ([]*MailRecord, error)
	ListRecordsByMessageID(ctx context.Context, messageID string) ([]*MailRecord, error)
	ListBounces(ctx context.Context, f BounceFilter, page Page) ([]*BounceRecord, error)
	ListFeedbackReports(ctx context.Context, page Page) ([]*FeedbackReport, error)
	// ListDSNMessages returns the raw DSN messages archived for a recipient,
	// newest first, bounded by limit. Used to show the notification behind a
	// dsn-type bounce.
	ListDSNMessages(ctx context.Context, recipient string, limit int) ([]*DSNMessage, error)
	CreateServiceControlRequest(ctx context.Context, rec *ServiceControlRecord) (*ServiceControlRecord, error)
	ActiveServiceControlExists(ctx context.Context) (bool, error)
	UpdateServiceControlStatus(ctx context.Context, id, status, resultSummary string) error
}

// KumoQueueAdmin controls kumod's live scheduled queues over its admin HTTP API.
type KumoQueueAdmin interface {
	QueueSummary(ctx context.Context) ([]*QueueState, error)
	SuspendQueue(ctx context.Context, domain, reason string) (string, error)
	ResumeQueue(ctx context.Context, domain string) (string, error)
	BounceQueue(ctx context.Context, domain, reason string) (string, error)
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
	queue    KumoQueueAdmin
}

// NewMailOpsUsecase constructs the use case.
func NewMailOpsUsecase(repo MailOpsRepo, producer CommandProducer, auditor *Auditor) *MailOpsUsecase {
	return &MailOpsUsecase{repo: repo, producer: producer, auditor: auditor}
}

// WithQueueAdmin wires the live kumod queue controller used by ListQueues and
// RequestQueueAction. Without it those operations report unavailable.
func (uc *MailOpsUsecase) WithQueueAdmin(q KumoQueueAdmin) *MailOpsUsecase {
	uc.queue = q
	return uc
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

// NextDeliveryAttempt estimates a deferred message's retry schedule — next
// attempt, remaining attempts, and expiry — from its full recorded lifecycle and
// the effective retry schedule. Read-only; returns Deferred=false when the
// message already reached a terminal outcome.
func (uc *MailOpsUsecase) NextDeliveryAttempt(ctx context.Context, messageID string, sched RetrySchedule) (*NextAttemptEstimate, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, Invalid("MESSAGE_ID_REQUIRED", "message_id is required")
	}
	events, err := uc.repo.ListRecordsByMessageID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	est := EstimateNextAttempt(events, sched)
	return &est, nil
}

// ListBounces returns bounce records matching the filter.
func (uc *MailOpsUsecase) ListBounces(ctx context.Context, f BounceFilter, page Page) ([]*BounceRecord, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	f, err := NormalizeBounceFilter(f)
	if err != nil {
		return nil, err
	}
	return uc.repo.ListBounces(ctx, f, page)
}

// DSNMessagesForRecipient returns the raw DSN notifications archived for a
// recipient, so the operator can read the full asynchronous bounce behind a
// dsn-type bounce row. Empty when nothing was archived for that recipient.
func (uc *MailOpsUsecase) DSNMessagesForRecipient(ctx context.Context, recipient string) ([]*DSNMessage, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if recipient == "" {
		return nil, nil
	}
	return uc.repo.ListDSNMessages(ctx, recipient, 20)
}

// ListFeedbackReports returns feedback reports.
func (uc *MailOpsUsecase) ListFeedbackReports(ctx context.Context, page Page) ([]*FeedbackReport, error) {
	if _, err := RequirePermission(ctx, PermMailRead); err != nil {
		return nil, err
	}
	return uc.repo.ListFeedbackReports(ctx, page)
}

// ListQueues returns kumod's live scheduled-queue summary (per destination
// domain), including suspended state.
func (uc *MailOpsUsecase) ListQueues(ctx context.Context) ([]*QueueState, error) {
	if _, err := RequirePermission(ctx, PermQueueRead); err != nil {
		return nil, err
	}
	if uc.queue == nil {
		return nil, Unavailable("QUEUE_ADMIN_UNCONFIGURED", "kumod queue admin is not configured")
	}
	return uc.queue.QueueSummary(ctx)
}

// Queue action verbs.
const (
	QueueActionSuspend = "suspend"
	QueueActionResume  = "resume"
	QueueActionBounce  = "bounce"
)

// QueueActionResult is returned after a queue-control action.
type QueueActionResult struct {
	Status  string
	Summary string
}

// RequestQueueAction performs a live queue-control action (suspend/resume/bounce)
// on kumod for a destination domain, synchronously, and audits it. Bounce is
// destructive and requires a confirmation id.
func (uc *MailOpsUsecase) RequestQueueAction(ctx context.Context, action, domain, reason, confirmationID string) (*QueueActionResult, error) {
	if _, err := RequirePermission(ctx, PermQueueControl); err != nil {
		uc.audit(ctx, "queue.action", "queue", domain, AuditDenied, map[string]any{"action": action})
		return nil, err
	}
	if uc.queue == nil {
		return nil, Unavailable("QUEUE_ADMIN_UNCONFIGURED", "kumod queue admin is not configured")
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, Invalid("QUEUE_DOMAIN_REQUIRED", "domain is required")
	}

	var summary string
	var err error
	switch action {
	case QueueActionSuspend:
		summary, err = uc.queue.SuspendQueue(ctx, domain, reason)
	case QueueActionResume:
		summary, err = uc.queue.ResumeQueue(ctx, domain)
	case QueueActionBounce:
		if strings.TrimSpace(confirmationID) == "" {
			return nil, Invalid("CONFIRMATION_REQUIRED", "confirmation_id is required to bounce a queue (destructive)")
		}
		summary, err = uc.queue.BounceQueue(ctx, domain, reason)
	default:
		return nil, Invalid("QUEUE_ACTION_INVALID", "action %q must be suspend, resume, or bounce", action)
	}
	if err != nil {
		uc.audit(ctx, "queue.action", "queue", domain, AuditFailure, map[string]any{"action": action})
		return nil, err
	}
	uc.audit(ctx, "queue.action", "queue", domain, AuditSuccess, map[string]any{"action": action, "reason": reason})
	return &QueueActionResult{Status: "ok", Summary: summary}, nil
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
