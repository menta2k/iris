package biz

import (
	"context"
	"time"
)

// AuditOutcome categorizes the result of an audited operation.
type AuditOutcome string

const (
	AuditSuccess AuditOutcome = "success"
	AuditFailure AuditOutcome = "failure"
	AuditDenied  AuditOutcome = "denied"
)

// AuditEvent is an append-only record of a security-relevant operation. The
// SafeChangeSummary must already be redacted before it reaches the writer.
type AuditEvent struct {
	OccurredAt        time.Time
	ActorUserID       string
	Operation         string
	TargetType        string
	TargetID          string
	Outcome           AuditOutcome
	IPAddress         string
	UserAgent         string
	RequestID         string
	SafeChangeSummary map[string]any
}

// AuditWriter appends audit events. Implementations must be append-only.
type AuditWriter interface {
	Write(ctx context.Context, e AuditEvent) error
}

// Auditor wraps an AuditWriter with convenience helpers that build redacted
// audit events from the request identity in context.
type Auditor struct {
	writer AuditWriter
}

// NewAuditor constructs an Auditor over the given writer.
func NewAuditor(w AuditWriter) *Auditor {
	return &Auditor{writer: w}
}

// Record writes an audit event, deriving actor and request metadata from the
// context identity and redacting the change summary defensively.
func (a *Auditor) Record(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) error {
	id := IdentityFrom(ctx)
	e := AuditEvent{
		Operation:         op,
		TargetType:        targetType,
		TargetID:          targetID,
		Outcome:           outcome,
		SafeChangeSummary: RedactMap(summary),
	}
	if id != nil {
		e.ActorUserID = id.UserID
		e.IPAddress = id.IPAddress
		e.UserAgent = id.UserAgent
		e.RequestID = id.RequestID
	}
	return a.writer.Write(ctx, e)
}
