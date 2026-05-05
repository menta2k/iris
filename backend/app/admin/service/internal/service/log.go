package service

import (
	"context"
	"errors"
	"time"
)

// LogRow is the data-layer view of a kumomta log event.
type LogRow struct {
	ID           int64
	At           time.Time
	EventType    string
	Queue        string
	Sender       string
	Recipient    string
	MessageID    string
	ResponseCode int32
	ResponseText string
	SourceIP     string
	MailClass    string
}

// LogFilter narrows a List call. All fields are optional; the empty value
// of each is treated as "don't constrain on that dimension".
type LogFilter struct {
	EventType string
	Queue     string
	Sender    string
	Recipient string
	MailClass string
	// MessageID ties together every event for a single SMTP submission —
	// Reception, retries (TransientFailure), and the eventual Delivery /
	// Bounce all share the same id. Exact-match (not ContainsFold) because
	// kumomta's IDs are 32-char hex; substring-matching is noise.
	MessageID string
	Since     time.Time
	Until     time.Time
}

// LogStore is the data-layer interface.
type LogStore interface {
	List(ctx context.Context, f LogFilter, limit, offset int) ([]LogRow, uint32, error)
}

// LogService is read-only.
type LogService struct{ store LogStore }

// NewLogService constructs the service.
func NewLogService(store LogStore) *LogService { return &LogService{store: store} }

// List paginates events newest-first.
func (s *LogService) List(ctx context.Context, f LogFilter, limit, offset int) ([]LogRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.store.List(ctx, f, limit, offset)
	if err != nil {
		return nil, 0, errors.Join(errors.New("log: list"), err)
	}
	return rows, total, nil
}
