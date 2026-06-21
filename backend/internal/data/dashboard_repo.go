package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// DashboardRepo computes dashboard summary statistics. It reads from the
// continuous-aggregate views where available and falls back to base tables.
type DashboardRepo struct {
	db *DB
}

// NewDashboardRepo constructs the repository.
func NewDashboardRepo(db *DB) *DashboardRepo { return &DashboardRepo{db: db} }

var _ biz.DashboardRepo = (*DashboardRepo)(nil)

// Summary returns the current operator dashboard summary.
func (r *DashboardRepo) Summary(ctx context.Context) (*biz.DashboardSummary, error) {
	s := &biz.DashboardSummary{ServiceState: "unknown"}

	// Total queued messages across mailclasses.
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT coalesce(sum(depth), 0) FROM mailclass_queues`).Scan(&s.QueuedMessages); err != nil {
		return nil, fmt.Errorf("queued messages: %w", err)
	}
	// Mail events in the last hour.
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM mail_records WHERE event_time >= now() - interval '1 hour'`).
		Scan(&s.RecentMailEvents); err != nil {
		return nil, fmt.Errorf("recent mail events: %w", err)
	}
	// Audit events in the last hour.
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_entries WHERE occurred_at >= now() - interval '1 hour'`).
		Scan(&s.RecentAuditEvents); err != nil {
		return nil, fmt.Errorf("recent audit events: %w", err)
	}
	// Latest service-control terminal state, if any.
	var state string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT status FROM service_control_requests
		ORDER BY requested_at DESC LIMIT 1`).Scan(&state)
	if err == nil && state != "" {
		s.ServiceState = state
	} else {
		s.ServiceState = "running"
	}
	return s, nil
}
