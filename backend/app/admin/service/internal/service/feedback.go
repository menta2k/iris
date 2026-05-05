package service

import (
	"context"
	"errors"
	"time"
)

// FeedbackRow is the data-layer view of an ARF report.
type FeedbackRow struct {
	ID                 int64
	ReceivedAt         time.Time
	FeedbackType       string
	UserAgent          string
	SourceIP           string
	OriginalRecipient  string
	OriginalSender     string
	OriginalMessageID  string
	ReportingMTA       string
	ArrivalDate        *time.Time
}

// FeedbackStore is the data-layer interface.
type FeedbackStore interface {
	List(ctx context.Context, limit, offset int) ([]FeedbackRow, uint32, error)
}

// FeedbackService is read-only — feedback is produced by the ARF parser
// pipeline (see pkg/fbl) which is wired separately.
type FeedbackService struct{ store FeedbackStore }

// NewFeedbackService constructs the service.
func NewFeedbackService(store FeedbackStore) *FeedbackService {
	return &FeedbackService{store: store}
}

// List paginates feedback reports newest-first.
func (s *FeedbackService) List(ctx context.Context, limit, offset int) ([]FeedbackRow, uint32, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.store.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, errors.Join(errors.New("feedback: list"), err)
	}
	return rows, total, nil
}
