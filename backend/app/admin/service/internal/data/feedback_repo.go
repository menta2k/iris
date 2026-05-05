// FeedbackRepo backs service.FeedbackStore with ent.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/feedbackreport"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// FeedbackRepo persists/reads ARF reports.
type FeedbackRepo struct{ client *ent.Client }

// NewFeedbackRepo wires the ent client.
func NewFeedbackRepo(c *ent.Client) *FeedbackRepo { return &FeedbackRepo{client: c} }

// List returns the most recent reports.
func (r *FeedbackRepo) List(ctx context.Context, limit, offset int) ([]service.FeedbackRow, uint32, error) {
	total, err := r.client.FeedbackReport.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("feedback_repo: count: %w", err)
	}
	rows, err := r.client.FeedbackReport.Query().
		Order(ent.Desc(feedbackreport.FieldReceivedAt)).
		Limit(limit).Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("feedback_repo: list: %w", err)
	}
	out := make([]service.FeedbackRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.FeedbackRow{
			ID:                e.ID,
			ReceivedAt:        e.ReceivedAt,
			FeedbackType:      e.FeedbackType,
			UserAgent:         e.UserAgent,
			SourceIP:          e.SourceIP,
			OriginalRecipient: e.OriginalRecipient,
			OriginalSender:    e.OriginalSender,
			OriginalMessageID: e.OriginalMessageID,
			ReportingMTA:      e.ReportingMta,
			ArrivalDate:       e.ArrivalDate,
		})
	}
	return out, uint32(total), nil
}

var _ service.FeedbackStore = (*FeedbackRepo)(nil)
