// AuditReader returns audit-log entries for the management UI.
//
// The read path is independent of AuditWriter — the writer is the producer
// (called by the audit middleware on mutating gRPC calls), the reader is the
// consumer (called by GET /v1/audit). They share a single ent table.
package data

import (
	"context"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/auditentry"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// AuditReader reads from the audit_entry table.
type AuditReader struct {
	client *ent.Client
}

// NewAuditReader wires the ent client.
func NewAuditReader(c *ent.Client) *AuditReader { return &AuditReader{client: c} }

// List returns the most recent entries first, with simple filters.
func (r *AuditReader) List(ctx context.Context, in service.AuditListInput) ([]service.AuditRow, uint32, error) {
	q := r.client.AuditEntry.Query()
	if in.Operation != "" {
		q = q.Where(auditentry.OperationEQ(in.Operation))
	}
	if in.ActorUserID != 0 {
		q = q.Where(auditentry.ActorUserIDEQ(in.ActorUserID))
	}
	if !in.Since.IsZero() {
		q = q.Where(auditentry.AtGTE(in.Since))
	}
	if !in.Until.IsZero() {
		q = q.Where(auditentry.AtLTE(in.Until))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_reader: count: %w", err)
	}
	rows, err := q.
		Order(ent.Desc(auditentry.FieldAt)).
		Limit(in.Limit).Offset(in.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_reader: list: %w", err)
	}
	out := make([]service.AuditRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.AuditRow{
			ID:            e.ID,
			At:            e.At,
			Operation:     e.Operation,
			ResourceType:  e.ResourceType,
			ResourceID:    e.ResourceID,
			ActorUserID:   e.ActorUserID,
			ActorUsername: e.ActorUsername,
			ClientIP:      e.ClientIP,
			UserAgent:     e.UserAgent,
			RequestID:     e.RequestID,
			StatusCode:    e.StatusCode,
			StatusMessage: e.StatusMessage,
			DurationMS:    e.DurationMs,
		})
	}
	return out, uint32(total), nil
}

// Compile-time assertion that AuditReader satisfies the service contract.
var _ service.AuditStore = (*AuditReader)(nil)

var _ = time.Time{}
