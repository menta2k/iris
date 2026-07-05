package data

import (
	"context"
	"fmt"
	"time"

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
	// Messages deferred and still in the queue: a transient failure was logged
	// but no terminal outcome (delivery or bounce) since — so kumod is still
	// retrying them. Bounded to a recent window to keep the scan cheap.
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT message_id
			FROM mail_records
			WHERE event_time >= now() - interval '7 days'
			GROUP BY message_id
			HAVING count(*) FILTER (WHERE status = $1) > 0
			   AND count(*) FILTER (WHERE status IN ($2, $3)) = 0
		) q`,
		biz.MailDeferred, biz.MailSent, biz.MailBounced).Scan(&s.DeferredInQueue); err != nil {
		return nil, fmt.Errorf("deferred in queue: %w", err)
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

// DeliveryStats aggregates per-VMTA, per-recipient-domain delivery outcomes from
// mail_records since the given time. egress_source carries the VMTA name on
// delivery/bounce/deferral events (it is empty on Reception, which we exclude),
// and vmta_id is not populated on log rows — so we group by the source name and
// LEFT JOIN vmtas to recover the id for linking. Only delivery-attempt statuses
// are counted; rate fields are left for the usecase to derive.
func (r *DashboardRepo) DeliveryStats(ctx context.Context, since time.Time) ([]biz.WarmupDeliveryStat, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			coalesce(v.id::text, '')              AS vmta_id,
			m.egress_source                       AS vmta_name,
			m.recipient_domain                    AS recipient_domain,
			count(*) FILTER (WHERE m.status = $2) AS sent,
			count(*) FILTER (WHERE m.status = $3) AS bounced,
			count(*) FILTER (WHERE m.status = $4) AS deferred
		FROM mail_records m
		LEFT JOIN vmtas v ON v.name = m.egress_source
		WHERE m.event_time >= $1
			AND m.egress_source <> ''
			AND m.recipient_domain <> ''
			AND m.status IN ($2, $3, $4)
		GROUP BY v.id, m.egress_source, m.recipient_domain
		ORDER BY (count(*) FILTER (WHERE m.status = $2)
			+ count(*) FILTER (WHERE m.status = $3)
			+ count(*) FILTER (WHERE m.status = $4)) DESC,
			m.egress_source, m.recipient_domain
		LIMIT 500`,
		since, biz.MailSent, biz.MailBounced, biz.MailDeferred)
	if err != nil {
		return nil, fmt.Errorf("delivery stats: %w", err)
	}
	defer rows.Close()

	var out []biz.WarmupDeliveryStat
	for rows.Next() {
		var s biz.WarmupDeliveryStat
		if err := rows.Scan(&s.VMTAID, &s.VMTAName, &s.RecipientDomain,
			&s.Sent, &s.Bounced, &s.Deferred); err != nil {
			return nil, fmt.Errorf("scan delivery stat: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delivery stats rows: %w", err)
	}
	return out, nil
}

// MailClassStats aggregates mail-record volume per mailclass since the given
// time. Count is every record for the class; delivered/bounced/deferred break
// it down by terminal status (delivered == the "sent" status).
func (r *DashboardRepo) MailClassStats(ctx context.Context, since time.Time) ([]biz.MailClassStat, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			m.mailclass                           AS mailclass,
			count(*)                              AS count,
			count(*) FILTER (WHERE m.status = $2) AS delivered,
			count(*) FILTER (WHERE m.status = $3) AS bounced,
			count(*) FILTER (WHERE m.status = $4) AS deferred
		FROM mail_records m
		WHERE m.event_time >= $1
			AND m.mailclass <> ''
		GROUP BY m.mailclass
		ORDER BY count(*) DESC, m.mailclass
		LIMIT 100`,
		since, biz.MailSent, biz.MailBounced, biz.MailDeferred)
	if err != nil {
		return nil, fmt.Errorf("mailclass stats: %w", err)
	}
	defer rows.Close()

	var out []biz.MailClassStat
	for rows.Next() {
		var s biz.MailClassStat
		if err := rows.Scan(&s.Mailclass, &s.Count, &s.Delivered, &s.Bounced, &s.Deferred); err != nil {
			return nil, fmt.Errorf("scan mailclass stat: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mailclass stats rows: %w", err)
	}
	return out, nil
}

// RecipientDomainStats aggregates mail-record volume per recipient domain since
// the given time, ranked by total descending and capped at limit.
func (r *DashboardRepo) RecipientDomainStats(ctx context.Context, since time.Time, limit int) ([]biz.RecipientDomainStat, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT
			m.recipient_domain                    AS recipient_domain,
			count(*)                              AS count,
			count(*) FILTER (WHERE m.status = $2) AS delivered,
			count(*) FILTER (WHERE m.status = $3) AS bounced,
			count(*) FILTER (WHERE m.status = $4) AS deferred
		FROM mail_records m
		WHERE m.event_time >= $1
			AND m.recipient_domain <> ''
		GROUP BY m.recipient_domain
		ORDER BY count(*) DESC, m.recipient_domain
		LIMIT $5`,
		since, biz.MailSent, biz.MailBounced, biz.MailDeferred, limit)
	if err != nil {
		return nil, fmt.Errorf("recipient domain stats: %w", err)
	}
	defer rows.Close()

	var out []biz.RecipientDomainStat
	for rows.Next() {
		var s biz.RecipientDomainStat
		if err := rows.Scan(&s.RecipientDomain, &s.Count, &s.Delivered, &s.Bounced, &s.Deferred); err != nil {
			return nil, fmt.Errorf("scan recipient domain stat: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recipient domain stats rows: %w", err)
	}
	return out, nil
}
