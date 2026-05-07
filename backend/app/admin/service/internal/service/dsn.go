package service

import (
	"context"
	"errors"
	"time"
)

// DsnRow is the data-layer view of one parsed DSN. Mirrors the dsn_event
// hypertable but with friendlier types (time.Time instead of pgx tz strings).
type DsnRow struct {
	ID                int64
	ReceivedAt        time.Time
	VerpToken         string
	MessageIDRef      string
	OriginalRecipient string
	FinalRecipient    string
	Action            string
	Status            string
	StatusClass       string
	DiagnosticCode    string
	RemoteMTA         string
	Category          string
	MailClass         string
	Tenant            string
	Campaign          string
	RawSize           int32
	ExtraJSON         string
}

// DsnFilter narrows a List call. Empty fields are "don't constrain".
type DsnFilter struct {
	Category    string // exact match (single value); UI multi-select expands client-side
	StatusClass string // "4" or "5" for the coarse hard/soft split
	Status      string // exact match (e.g. "5.1.1") for power-user drill-down
	Recipient   string // case-insensitive substring on final_recipient
	MailClass   string // case-insensitive substring; matches Logs UI behaviour
	MessageID   string // exact match — click-through from Logs uses this
	Since       time.Time
	Until       time.Time
}

// DsnStore is the data-layer contract.
type DsnStore interface {
	List(ctx context.Context, f DsnFilter, limit, offset int) ([]DsnRow, uint32, error)
}

// DsnService is read-only — auto-suppression already happens at insert
// time so there's no mutating surface to expose.
type DsnService struct{ store DsnStore }

// NewDsnService constructs the service.
func NewDsnService(store DsnStore) *DsnService { return &DsnService{store: store} }

// List paginates DSNs newest-first.
func (s *DsnService) List(ctx context.Context, f DsnFilter, limit, offset int) ([]DsnRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.store.List(ctx, f, limit, offset)
	if err != nil {
		return nil, 0, errors.Join(errors.New("dsn: list"), err)
	}
	return rows, total, nil
}
