package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// InboundRepo persists Rspamd filter results.
type InboundRepo struct {
	db *DB
}

// NewInboundRepo constructs the repository.
func NewInboundRepo(db *DB) *InboundRepo { return &InboundRepo{db: db} }

var _ biz.InboundRepo = (*InboundRepo)(nil)

// CreateRspamdResult inserts a filter result.
func (r *InboundRepo) CreateRspamdResult(ctx context.Context, res *biz.RspamdFilterResult) error {
	symbols, _ := json.Marshal(res.Symbols)
	var mailID any
	if res.MailRecordID != "" {
		mailID = res.MailRecordID
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO rspamd_filter_results (mail_record_id, message_id, action, score, symbols, reason, raw_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		mailID, res.MessageID, res.Action, res.Score, string(symbols), res.Reason, res.RawRef)
	if err != nil {
		return fmt.Errorf("insert rspamd result: %w", err)
	}
	return nil
}

// ListRspamdResults returns filter results newest first. The recipient is
// resolved from the mail log by message id when available.
func (r *InboundRepo) ListRspamdResults(ctx context.Context, page biz.Page) ([]*biz.RspamdFilterResult, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT f.id, f.event_time, coalesce(f.mail_record_id::text, ''), coalesce(f.message_id, ''),
		       coalesce(max(m.recipient), ''), f.action, f.score, f.symbols, f.reason
		FROM rspamd_filter_results f
		LEFT JOIN mail_records m ON m.message_id = f.message_id AND f.message_id <> ''
		GROUP BY f.id, f.event_time, f.mail_record_id, f.message_id, f.action, f.score, f.symbols, f.reason
		ORDER BY f.event_time DESC LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query rspamd results: %w", err)
	}
	defer rows.Close()
	var out []*biz.RspamdFilterResult
	for rows.Next() {
		res := &biz.RspamdFilterResult{}
		var symbolsRaw []byte
		if err := rows.Scan(&res.ID, &res.EventTime, &res.MailRecordID, &res.MessageID,
			&res.Recipient, &res.Action, &res.Score, &symbolsRaw, &res.Reason); err != nil {
			return nil, fmt.Errorf("scan rspamd result: %w", err)
		}
		_ = json.Unmarshal(symbolsRaw, &res.Symbols)
		out = append(out, res)
	}
	return out, rows.Err()
}
