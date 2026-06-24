package biz

import (
	"strings"
	"time"
)

// Queue states.
const (
	QueueRunning  = "running"
	QueuePaused   = "paused"
	QueueDraining = "draining"
	QueueUnknown  = "unknown"
)

// MailclassQueue is the current queue state for a mailclass.
type MailclassQueue struct {
	Mailclass               string
	State                   string
	Depth                   int64
	OldestMessageAgeSeconds int64
	LastObservedAt          *time.Time
}

// QueueState is a live KumoMTA scheduled-queue summary for one destination
// domain: how many messages are scheduled (waiting/retrying) and whether the
// queue is administratively suspended.
type QueueState struct {
	Domain        string
	Depth         int64
	Suspended     bool
	SuspendID     string
	SuspendReason string
}

// Service-control request states.
const (
	SvcRequested = "requested"
	SvcRunning   = "running"
	SvcSucceeded = "succeeded"
	SvcFailed    = "failed"
	SvcCancelled = "cancelled"
	SvcTimedOut  = "timed_out"
)

// ServiceControlRecord tracks a serialized KumoMTA service-control request.
type ServiceControlRecord struct {
	ID             string
	RequestedAt    time.Time
	RequestedBy    string
	Operation      string
	ConfirmationID string
	Status         string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	ResultSummary  string
}

// ValidateQueueActionRequest checks a queue-action request before enqueueing.
func ValidateQueueActionRequest(mailclass, action, confirmationID string) error {
	if strings.TrimSpace(mailclass) == "" {
		return Invalid("QUEUE_MAILCLASS_REQUIRED", "mailclass is required")
	}
	if !ValidQueueAction(action) {
		return Invalid("QUEUE_ACTION_INVALID", "action %q is not valid", action)
	}
	if strings.TrimSpace(confirmationID) == "" {
		return Invalid("CONFIRMATION_REQUIRED", "confirmation_id is required for queue actions")
	}
	return nil
}

// ValidateServiceControlRequest checks a service-control request. Service
// control is high-risk and always requires explicit confirmation.
func ValidateServiceControlRequest(operation, confirmationID string) error {
	if !ValidServiceOperation(operation) {
		return Invalid("SERVICE_OPERATION_INVALID", "operation %q is not valid", operation)
	}
	if strings.TrimSpace(confirmationID) == "" {
		return Invalid("CONFIRMATION_REQUIRED", "confirmation_id is required for service control")
	}
	return nil
}
