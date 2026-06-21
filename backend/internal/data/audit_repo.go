package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// AuditRepo is an append-only writer and reader for audit entries backed by the
// audit_entries hypertable.
type AuditRepo struct {
	db *DB
}

// NewAuditRepo constructs an AuditRepo. It also satisfies biz.AuditWriter.
func NewAuditRepo(db *DB) *AuditRepo { return &AuditRepo{db: db} }

var _ biz.AuditWriter = (*AuditRepo)(nil)

// Write appends a single audit event. Audit entries are never updated or
// deleted through the application.
func (r *AuditRepo) Write(ctx context.Context, e biz.AuditEvent) error {
	summary, err := json.Marshal(e.SafeChangeSummary)
	if err != nil {
		summary = []byte("{}")
	}
	actor := nullableUUID(e.ActorUserID)
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO audit_entries
			(actor_user_id, operation, target_type, target_id, outcome,
			 ip_address, user_agent, request_id, safe_change_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		actor, e.Operation, e.TargetType, e.TargetID, string(e.Outcome),
		e.IPAddress, e.UserAgent, e.RequestID, string(summary),
	)
	if err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// AuditListItem is a single audit row returned to operators.
type AuditListItem struct {
	ID                string
	OccurredAt        string
	ActorUserID       string
	Operation         string
	TargetType        string
	TargetID          string
	Outcome           string
	IPAddress         string
	RequestID         string
	SafeChangeSummary map[string]any
}

// List returns audit entries most-recent first with bounded pagination.
func (r *AuditRepo) List(ctx context.Context, page biz.Page) ([]AuditListItem, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, to_char(occurred_at, 'YYYY-MM-DD"T"HH24:MI:SS.MSOF'),
		       coalesce(actor_user_id::text, ''), operation,
		       target_type, target_id, outcome, ip_address, request_id,
		       safe_change_summary
		FROM audit_entries
		ORDER BY occurred_at DESC
		LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query audit entries: %w", err)
	}
	defer rows.Close()

	var items []AuditListItem
	for rows.Next() {
		var it AuditListItem
		var summaryRaw []byte
		if err := rows.Scan(&it.ID, &it.OccurredAt, &it.ActorUserID, &it.Operation,
			&it.TargetType, &it.TargetID, &it.Outcome, &it.IPAddress, &it.RequestID,
			&summaryRaw); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		_ = json.Unmarshal(summaryRaw, &it.SafeChangeSummary)
		items = append(items, it)
	}
	return items, rows.Err()
}
