package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// InboundRepo persists webhook rules, webhook delivery events, and Rspamd
// filter results.
type InboundRepo struct {
	db *DB
}

// NewInboundRepo constructs the repository.
func NewInboundRepo(db *DB) *InboundRepo { return &InboundRepo{db: db} }

var _ biz.InboundRepo = (*InboundRepo)(nil)

// CreateWebhookRule inserts a webhook rule.
func (r *InboundRepo) CreateWebhookRule(ctx context.Context, w *biz.WebhookRule) (*biz.WebhookRule, error) {
	policy, _ := json.Marshal(w.RetryPolicy)
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO webhook_rules (name, match_type, match_value, destination_url, secret_ref, status, timeout_seconds, retry_policy)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, match_type, match_value, destination_url, status, timeout_seconds`,
		w.Name, w.MatchType, w.MatchValue, w.DestinationURL, w.SecretRef, w.Status, w.TimeoutSeconds, string(policy))
	out := &biz.WebhookRule{}
	if err := row.Scan(&out.ID, &out.Name, &out.MatchType, &out.MatchValue,
		&out.DestinationURL, &out.Status, &out.TimeoutSeconds); err != nil {
		return nil, mapConstraint(err, "webhook_rule")
	}
	return out, nil
}

// UpdateWebhookRule updates a webhook rule by id. An empty secret_ref preserves
// the stored secret reference.
func (r *InboundRepo) UpdateWebhookRule(ctx context.Context, id string, w *biz.WebhookRule) (*biz.WebhookRule, error) {
	policy, _ := json.Marshal(w.RetryPolicy)
	row := r.db.Pool.QueryRow(ctx, `
		UPDATE webhook_rules SET name = $2, match_type = $3, match_value = $4,
			destination_url = $5, secret_ref = COALESCE(NULLIF($6, ''), secret_ref),
			status = $7, timeout_seconds = $8, retry_policy = $9, updated_at = now()
		WHERE id = $1
		RETURNING id, name, match_type, match_value, destination_url, status, timeout_seconds`,
		id, w.Name, w.MatchType, w.MatchValue, w.DestinationURL, w.SecretRef, w.Status, w.TimeoutSeconds, string(policy))
	out := &biz.WebhookRule{}
	if err := row.Scan(&out.ID, &out.Name, &out.MatchType, &out.MatchValue,
		&out.DestinationURL, &out.Status, &out.TimeoutSeconds); err != nil {
		return nil, mapConstraint(err, "webhook_rule")
	}
	return out, nil
}

// ListWebhookRules returns webhook rules. Secret refs are not selected.
func (r *InboundRepo) ListWebhookRules(ctx context.Context, page biz.Page) ([]*biz.WebhookRule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, match_type, match_value, destination_url, status, timeout_seconds
		FROM webhook_rules ORDER BY name LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query webhook rules: %w", err)
	}
	defer rows.Close()
	var out []*biz.WebhookRule
	for rows.Next() {
		w := &biz.WebhookRule{}
		if err := rows.Scan(&w.ID, &w.Name, &w.MatchType, &w.MatchValue, &w.DestinationURL, &w.Status, &w.TimeoutSeconds); err != nil {
			return nil, fmt.Errorf("scan webhook rule: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// MatchWebhookRules returns active rules whose match applies to the recipient.
func (r *InboundRepo) MatchWebhookRules(ctx context.Context, recipient string) ([]*biz.WebhookRule, error) {
	domain := biz.RecipientDomain(recipient)
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, match_type, match_value, destination_url, secret_ref, status, timeout_seconds, retry_policy
		FROM webhook_rules
		WHERE status = 'active'
		  AND ((match_type = 'recipient_email' AND match_value = $1)
		    OR (match_type = 'recipient_domain' AND match_value = $2))`,
		biz.NormalizeSuppressionValue("email", recipient), domain)
	if err != nil {
		return nil, fmt.Errorf("match webhook rules: %w", err)
	}
	defer rows.Close()
	var out []*biz.WebhookRule
	for rows.Next() {
		w := &biz.WebhookRule{}
		var policyRaw []byte
		if err := rows.Scan(&w.ID, &w.Name, &w.MatchType, &w.MatchValue, &w.DestinationURL,
			&w.SecretRef, &w.Status, &w.TimeoutSeconds, &policyRaw); err != nil {
			return nil, fmt.Errorf("scan matched webhook rule: %w", err)
		}
		_ = json.Unmarshal(policyRaw, &w.RetryPolicy)
		out = append(out, w)
	}
	return out, rows.Err()
}

// RecordDeliveryEvent appends a webhook delivery attempt event.
func (r *InboundRepo) RecordDeliveryEvent(ctx context.Context, e *biz.WebhookDeliveryEvent) error {
	var mailID, ruleID any
	if e.WebhookRuleID != "" {
		ruleID = e.WebhookRuleID
	}
	if e.MailRecordID != "" {
		mailID = e.MailRecordID
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO webhook_delivery_events
			(webhook_rule_id, mail_record_id, attempt, status, response_code, error_summary, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ruleID, mailID, e.Attempt, e.Status, e.ResponseCode, e.ErrorSummary, e.NextRetryAt)
	if err != nil {
		return fmt.Errorf("record delivery event: %w", err)
	}
	return nil
}

// ListWebhookDeliveries returns recent delivery attempts, newest first, joined
// with the webhook rule name and (when present) the originating recipient.
func (r *InboundRepo) ListWebhookDeliveries(ctx context.Context, page biz.Page) ([]*biz.WebhookDeliveryEvent, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT d.id, to_char(d.event_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		       coalesce(d.webhook_rule_id::text, ''), coalesce(w.name, ''),
		       coalesce(d.mail_record_id::text, ''), coalesce(m.recipient, ''),
		       d.attempt, d.status, d.response_code, d.error_summary
		FROM webhook_delivery_events d
		LEFT JOIN webhook_rules w ON w.id = d.webhook_rule_id
		LEFT JOIN mail_records m ON m.id = d.mail_record_id
		ORDER BY d.event_time DESC
		LIMIT $1 OFFSET $2`, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query webhook deliveries: %w", err)
	}
	defer rows.Close()
	var out []*biz.WebhookDeliveryEvent
	for rows.Next() {
		e := &biz.WebhookDeliveryEvent{}
		var eventTime string
		if err := rows.Scan(&e.ID, &eventTime, &e.WebhookRuleID, &e.WebhookName,
			&e.MailRecordID, &e.Recipient, &e.Attempt, &e.Status, &e.ResponseCode, &e.ErrorSummary); err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}
		e.EventTime, _ = time.Parse("2006-01-02T15:04:05Z07:00", eventTime)
		out = append(out, e)
	}
	return out, rows.Err()
}

// CreateRspamdResult inserts a filter result.
func (r *InboundRepo) CreateRspamdResult(ctx context.Context, res *biz.RspamdFilterResult) error {
	symbols, _ := json.Marshal(res.Symbols)
	var mailID any
	if res.MailRecordID != "" {
		mailID = res.MailRecordID
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO rspamd_filter_results (mail_record_id, action, score, symbols, reason, raw_ref)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		mailID, res.Action, res.Score, string(symbols), res.Reason, res.RawRef)
	if err != nil {
		return fmt.Errorf("insert rspamd result: %w", err)
	}
	return nil
}

// ListRspamdResults returns filter results newest first.
func (r *InboundRepo) ListRspamdResults(ctx context.Context, page biz.Page) ([]*biz.RspamdFilterResult, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, event_time, coalesce(mail_record_id::text,''), action, score, symbols, reason
		FROM rspamd_filter_results ORDER BY event_time DESC LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query rspamd results: %w", err)
	}
	defer rows.Close()
	var out []*biz.RspamdFilterResult
	for rows.Next() {
		res := &biz.RspamdFilterResult{}
		var symbolsRaw []byte
		if err := rows.Scan(&res.ID, &res.EventTime, &res.MailRecordID, &res.Action, &res.Score, &symbolsRaw, &res.Reason); err != nil {
			return nil, fmt.Errorf("scan rspamd result: %w", err)
		}
		_ = json.Unmarshal(symbolsRaw, &res.Symbols)
		out = append(out, res)
	}
	return out, rows.Err()
}
