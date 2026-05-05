// AuditEntPersister is the production AuditPersister: it inserts batched
// audit entries into the ent-managed audit_entry table. Implemented as a
// MapCreateBulk so a single round-trip writes the whole batch.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// AuditEntPersister persists audit entries via ent.
type AuditEntPersister struct {
	client *ent.Client
}

// NewAuditEntPersister wires the ent client.
func NewAuditEntPersister(c *ent.Client) *AuditEntPersister { return &AuditEntPersister{client: c} }

// WriteBatch inserts every entry in a single bulk INSERT. A failed batch is
// surfaced to the caller; AuditWriter currently treats writes as best-effort.
func (p *AuditEntPersister) WriteBatch(ctx context.Context, entries []*audit.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	bulk := p.client.AuditEntry.MapCreateBulk(entries, func(c *ent.AuditEntryCreate, i int) {
		e := entries[i]
		c.SetAt(e.At).
			SetOperation(e.Operation).
			SetResourceType(e.ResourceType).
			SetResourceID(e.ResourceID).
			SetActorUserID(e.ActorUserID).
			SetActorUsername(e.ActorUsername).
			SetClientIP(e.ClientIP).
			SetUserAgent(e.UserAgent).
			SetRequestID(e.RequestID).
			SetStatusCode(e.StatusCode).
			SetStatusMessage(e.StatusMessage).
			SetRequestJSON(e.RequestJSON).
			SetResponseJSON(e.ResponseJSON).
			SetDurationMs(e.DurationMS)
	})
	if _, err := bulk.Save(ctx); err != nil {
		return fmt.Errorf("audit_persister: bulk insert: %w", err)
	}
	return nil
}

// Compile-time assertion.
var _ AuditPersister = (*AuditEntPersister)(nil)
