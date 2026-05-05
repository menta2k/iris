// LogRepo backs service.LogStore with ent (TimescaleDB hypertable).
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/logevent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// LogRepo persists/reads log events.
type LogRepo struct{ client *ent.Client }

// NewLogRepo wires the ent client.
func NewLogRepo(c *ent.Client) *LogRepo { return &LogRepo{client: c} }

// List returns events newest-first matching the filter. Sender / Recipient /
// MailClass use case-insensitive substring matching so the UI search box
// behaves like operators expect — typing "yahoo" matches "Bob@yahoo.com".
// EventType + Queue use exact equality (they're picked from a fixed list).
func (r *LogRepo) List(ctx context.Context, f service.LogFilter, limit, offset int) ([]service.LogRow, uint32, error) {
	q := r.client.LogEvent.Query()
	if f.EventType != "" {
		q = q.Where(logevent.EventTypeEQ(f.EventType))
	}
	if f.Queue != "" {
		q = q.Where(logevent.QueueEQ(f.Queue))
	}
	if f.Sender != "" {
		q = q.Where(logevent.SenderContainsFold(f.Sender))
	}
	if f.Recipient != "" {
		q = q.Where(logevent.RecipientContainsFold(f.Recipient))
	}
	if f.MailClass != "" {
		q = q.Where(logevent.MailClassContainsFold(f.MailClass))
	}
	if f.MessageID != "" {
		// Exact-equality on the indexed message_id column. Trim whitespace
		// because operators paste IDs from emails / log lines and an
		// accidental trailing newline would silently produce zero hits.
		q = q.Where(logevent.MessageIDEQ(strings.TrimSpace(f.MessageID)))
	}
	if !f.Since.IsZero() {
		q = q.Where(logevent.AtGTE(f.Since))
	}
	if !f.Until.IsZero() {
		q = q.Where(logevent.AtLT(f.Until))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("log_repo: count: %w", err)
	}
	rows, err := q.
		Order(ent.Desc(logevent.FieldAt)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("log_repo: list: %w", err)
	}
	out := make([]service.LogRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.LogRow{
			ID:           e.ID,
			At:           e.At,
			EventType:    e.EventType,
			Queue:        e.Queue,
			Sender:       e.Sender,
			Recipient:    e.Recipient,
			MessageID:    e.MessageID,
			ResponseCode: e.ResponseCode,
			ResponseText: e.ResponseText,
			SourceIP:     e.SourceIP,
			MailClass:    e.MailClass,
		})
	}
	return out, uint32(total), nil
}

var _ service.LogStore = (*LogRepo)(nil)
