// DsnRepo backs service.DsnStore with ent (TimescaleDB hypertable).
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/dsnevent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// DsnRepo persists/reads parsed DSN events.
type DsnRepo struct{ client *ent.Client }

// NewDsnRepo wires the ent client.
func NewDsnRepo(c *ent.Client) *DsnRepo { return &DsnRepo{client: c} }

// List returns DSNs newest-first matching the filter. Recipient + MailClass
// use case-insensitive substring; Category / Status / StatusClass / MessageID
// use exact match (UI surfaces them as multi-select / fixed buttons).
func (r *DsnRepo) List(ctx context.Context, f service.DsnFilter, limit, offset int) ([]service.DsnRow, uint32, error) {
	q := r.client.DsnEvent.Query()
	if f.Category != "" {
		q = q.Where(dsnevent.CategoryEQ(f.Category))
	}
	if f.StatusClass != "" {
		q = q.Where(dsnevent.StatusClassEQ(f.StatusClass))
	}
	if f.Status != "" {
		q = q.Where(dsnevent.StatusEQ(f.Status))
	}
	if f.Recipient != "" {
		q = q.Where(dsnevent.FinalRecipientContainsFold(f.Recipient))
	}
	if f.MailClass != "" {
		q = q.Where(dsnevent.MailClassContainsFold(f.MailClass))
	}
	if f.MessageID != "" {
		q = q.Where(dsnevent.MessageIDRefEQ(strings.TrimSpace(f.MessageID)))
	}
	if !f.Since.IsZero() {
		q = q.Where(dsnevent.ReceivedAtGTE(f.Since))
	}
	if !f.Until.IsZero() {
		q = q.Where(dsnevent.ReceivedAtLT(f.Until))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dsn_repo: count: %w", err)
	}
	rows, err := q.
		Order(ent.Desc(dsnevent.FieldReceivedAt)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("dsn_repo: list: %w", err)
	}
	out := make([]service.DsnRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.DsnRow{
			ID:                e.ID,
			ReceivedAt:        e.ReceivedAt,
			VerpToken:         e.VerpToken,
			MessageIDRef:      e.MessageIDRef,
			OriginalRecipient: e.OriginalRecipient,
			FinalRecipient:    e.FinalRecipient,
			Action:            e.Action,
			Status:            e.Status,
			StatusClass:       e.StatusClass,
			DiagnosticCode:    e.DiagnosticCode,
			RemoteMTA:         e.RemoteMta,
			Category:          e.Category,
			MailClass:         e.MailClass,
			Tenant:            e.Tenant,
			Campaign:          e.Campaign,
			RawSize:           e.RawSize,
			ExtraJSON:         e.ExtraJSON,
		})
	}
	return out, uint32(total), nil
}

var _ service.DsnStore = (*DsnRepo)(nil)
