package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// MailOpsRepo reads mail/bounce/feedback/queue data and persists service-control
// request records.
type MailOpsRepo struct {
	db *DB
}

// NewMailOpsRepo constructs the repository.
func NewMailOpsRepo(db *DB) *MailOpsRepo { return &MailOpsRepo{db: db} }

var _ biz.MailOpsRepo = (*MailOpsRepo)(nil)

// ListMailRecords returns mail records matching the filter, newest first.
func (r *MailOpsRepo) ListMailRecords(ctx context.Context, f biz.MailFilter, page biz.Page) ([]*biz.MailRecord, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, message_id, event_time, mailclass, sender, from_header, recipient,
		       recipient_domain, coalesce(vmta_id::text,''), egress_source, status, record_type, smtp_status, diagnostic, classification
		FROM mail_records
		WHERE ($1 = '' OR mailclass = $1)
		  -- sender/recipient/from are partial, case-insensitive matches so an
		  -- operator can search by just a domain (e.g. "gmail.com") or a name
		  -- fragment, not only a full address.
		  AND ($2 = '' OR sender ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR recipient ILIKE '%' || $3 || '%')
		  AND ($4 = '' OR egress_source = $4)
		  AND ($5::timestamptz IS NULL OR event_time >= $5)
		  AND ($6::timestamptz IS NULL OR event_time <= $6)
		  AND ($7 = '' OR from_header ILIKE '%' || $7 || '%')
		  AND ($8 = '' OR status = $8)
		  AND ($9 = '' OR record_type = $9)
		ORDER BY event_time DESC
		LIMIT $10 OFFSET $11`,
		f.Mailclass, f.Sender, f.Recipient, f.VMTAID, f.FromTime, f.ToTime, f.From, f.Status, f.RecordType, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query mail records: %w", err)
	}
	defer rows.Close()
	var out []*biz.MailRecord
	for rows.Next() {
		m := &biz.MailRecord{}
		if err := rows.Scan(&m.ID, &m.MessageID, &m.EventTime, &m.Mailclass, &m.Sender,
			&m.FromHeader, &m.Recipient, &m.RecipientDomain, &m.VMTAID, &m.EgressSource, &m.Status,
			&m.RecordType, &m.SMTPStatus, &m.Diagnostic, &m.Classification); err != nil {
			return nil, fmt.Errorf("scan mail record: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListRecordsByMessageID returns every event for one message, oldest first, so
// the retry-schedule estimator can reconstruct the full lifecycle (reception →
// deferrals → outcome) regardless of table pagination. Bounded for safety.
func (r *MailOpsRepo) ListRecordsByMessageID(ctx context.Context, messageID string) ([]*biz.MailRecord, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, message_id, event_time, mailclass, sender, from_header, recipient,
		       recipient_domain, coalesce(vmta_id::text,''), egress_source, status, record_type, smtp_status, diagnostic, classification
		FROM mail_records
		WHERE message_id = $1
		ORDER BY event_time
		LIMIT 1000`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query mail records by message id: %w", err)
	}
	defer rows.Close()
	var out []*biz.MailRecord
	for rows.Next() {
		m := &biz.MailRecord{}
		if err := rows.Scan(&m.ID, &m.MessageID, &m.EventTime, &m.Mailclass, &m.Sender,
			&m.FromHeader, &m.Recipient, &m.RecipientDomain, &m.VMTAID, &m.EgressSource, &m.Status,
			&m.RecordType, &m.SMTPStatus, &m.Diagnostic, &m.Classification); err != nil {
			return nil, fmt.Errorf("scan mail record: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListBounces returns bounce records matching the filter, newest first.
func (r *MailOpsRepo) ListBounces(ctx context.Context, f biz.BounceFilter, page biz.Page) ([]*biz.BounceRecord, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, coalesce(mail_record_id::text,''), event_time, recipient,
		       coalesce(vmta_id::text,''), mailclass, smtp_status, bounce_type,
		       diagnostic, classification, processing_state
		FROM bounce_records
		WHERE ($1 = '' OR recipient ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR mailclass = $2)
		  AND ($3 = '' OR lower(bounce_type) = $3)
		  AND ($4 = '' OR classification ILIKE '%' || $4 || '%')
		  AND ($5 = '' OR lower(processing_state) = $5)
		  AND ($6::timestamptz IS NULL OR event_time >= $6)
		  AND ($7::timestamptz IS NULL OR event_time <= $7)
		ORDER BY event_time DESC LIMIT $8 OFFSET $9`,
		f.Recipient, f.Mailclass, f.BounceType, f.Classification, f.ProcessingState,
		f.FromTime, f.ToTime, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query bounces: %w", err)
	}
	defer rows.Close()
	var out []*biz.BounceRecord
	for rows.Next() {
		b := &biz.BounceRecord{}
		if err := rows.Scan(&b.ID, &b.MailRecordID, &b.EventTime, &b.Recipient, &b.VMTAID,
			&b.Mailclass, &b.SMTPStatus, &b.BounceType, &b.Diagnostic, &b.Classification, &b.ProcessingState); err != nil {
			return nil, fmt.Errorf("scan bounce: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListDSNMessages returns the raw DSN messages archived for a recipient, newest
// first, bounded by limit. Reads the same dsn_messages table the DSN worker
// writes; scoped by recipient (the resolved original address).
func (r *MailOpsRepo) ListDSNMessages(ctx context.Context, recipient string, limit int) ([]*biz.DSNMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, recipient, message_id, raw_message, received_at
		FROM dsn_messages WHERE recipient = $1 ORDER BY received_at DESC LIMIT $2`,
		strings.ToLower(strings.TrimSpace(recipient)), limit)
	if err != nil {
		return nil, fmt.Errorf("query dsn messages: %w", err)
	}
	defer rows.Close()
	var out []*biz.DSNMessage
	for rows.Next() {
		m := &biz.DSNMessage{}
		if err := rows.Scan(&m.ID, &m.Recipient, &m.MessageID, &m.RawMessage, &m.ReceivedAt); err != nil {
			return nil, fmt.Errorf("scan dsn message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListFeedbackReports returns feedback reports newest first.
func (r *MailOpsRepo) ListFeedbackReports(ctx context.Context, page biz.Page) ([]*biz.FeedbackReport, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, received_at, source, report_type, recipient,
		       coalesce(mail_record_id::text,''), processing_state, raw_ref, verified, verification
		FROM feedback_reports ORDER BY received_at DESC LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query feedback: %w", err)
	}
	defer rows.Close()
	var out []*biz.FeedbackReport
	for rows.Next() {
		fr := &biz.FeedbackReport{}
		if err := rows.Scan(&fr.ID, &fr.ReceivedAt, &fr.Source, &fr.ReportType, &fr.Recipient,
			&fr.MailRecordID, &fr.ProcessingState, &fr.RawRef, &fr.Verified, &fr.Verification); err != nil {
			return nil, fmt.Errorf("scan feedback: %w", err)
		}
		out = append(out, fr)
	}
	return out, rows.Err()
}

// ListQueues returns the current per-mailclass queue snapshots.
func (r *MailOpsRepo) ListQueues(ctx context.Context, page biz.Page) ([]*biz.MailclassQueue, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT mailclass, state, depth, oldest_message_age_seconds, last_observed_at
		FROM mailclass_queues ORDER BY mailclass LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query queues: %w", err)
	}
	defer rows.Close()
	var out []*biz.MailclassQueue
	for rows.Next() {
		q := &biz.MailclassQueue{}
		if err := rows.Scan(&q.Mailclass, &q.State, &q.Depth, &q.OldestMessageAgeSeconds, &q.LastObservedAt); err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// CreateServiceControlRequest inserts a new service-control request row in the
// requested state and returns its generated id.
func (r *MailOpsRepo) CreateServiceControlRequest(ctx context.Context, rec *biz.ServiceControlRecord) (*biz.ServiceControlRecord, error) {
	actor := nullableUUID(rec.RequestedBy)
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO service_control_requests (requested_by, operation, confirmation_id, status)
		VALUES ($1, $2, $3, 'requested')
		RETURNING id, requested_at, operation, status`,
		actor, rec.Operation, rec.ConfirmationID)
	out := &biz.ServiceControlRecord{RequestedBy: rec.RequestedBy, ConfirmationID: rec.ConfirmationID}
	if err := row.Scan(&out.ID, &out.RequestedAt, &out.Operation, &out.Status); err != nil {
		return nil, mapConstraint(err, "service_control_request")
	}
	return out, nil
}

// ActiveServiceControlExists reports whether a request is currently requested or
// running, enforcing that only one service-control op is active at a time.
func (r *MailOpsRepo) ActiveServiceControlExists(ctx context.Context) (bool, error) {
	var ok bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM service_control_requests
		WHERE status IN ('requested','running'))`).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check active service control: %w", err)
	}
	return ok, nil
}

// UpdateServiceControlStatus advances a request's lifecycle state.
func (r *MailOpsRepo) UpdateServiceControlStatus(ctx context.Context, id, status, resultSummary string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE service_control_requests
		SET status = $2, result_summary = $3,
		    started_at = CASE WHEN $2 = 'running' THEN now() ELSE started_at END,
		    finished_at = CASE WHEN $2 IN ('succeeded','failed','cancelled','timed_out') THEN now() ELSE finished_at END
		WHERE id = $1`, id, status, resultSummary)
	if err != nil {
		return fmt.Errorf("update service control status: %w", err)
	}
	return nil
}

// GetAppliedChecksum returns the last applied policy + init checksums and timestamp.
func (r *MailOpsRepo) GetAppliedChecksum(ctx context.Context) (string, string, *time.Time, error) {
	var checksum, initChecksum string
	var appliedAt *time.Time
	err := r.db.Pool.QueryRow(ctx,
		`SELECT applied_checksum, applied_init_checksum, applied_at FROM config_state WHERE id = 1`).
		Scan(&checksum, &initChecksum, &appliedAt)
	if err == pgx.ErrNoRows {
		return "", "", nil, nil
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("get applied checksum: %w", err)
	}
	return checksum, initChecksum, appliedAt, nil
}

// SetAppliedChecksum records a successful config apply on the singleton row,
// including the full policy content so the UI can diff pending vs running.
func (r *MailOpsRepo) SetAppliedChecksum(ctx context.Context, checksum, initChecksum, content, by string) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO config_state (id, applied_checksum, applied_init_checksum, applied_content, applied_at, applied_by)
		VALUES (1, $1, $2, $3, now(), $4)
		ON CONFLICT (id) DO UPDATE
		SET applied_checksum = EXCLUDED.applied_checksum,
		    applied_init_checksum = EXCLUDED.applied_init_checksum,
		    applied_content = EXCLUDED.applied_content,
		    applied_at = EXCLUDED.applied_at,
		    applied_by = EXCLUDED.applied_by`,
		checksum, initChecksum, content, nullableUUID(by))
	if err != nil {
		return fmt.Errorf("set applied checksum: %w", err)
	}
	return nil
}

// GetAppliedContent returns the full policy content last successfully applied,
// its checksum, and when it was applied (empty content when never applied).
func (r *MailOpsRepo) GetAppliedContent(ctx context.Context) (content, checksum string, appliedAt *time.Time, err error) {
	err = r.db.Pool.QueryRow(ctx,
		`SELECT applied_content, applied_checksum, applied_at FROM config_state WHERE id = 1`).
		Scan(&content, &checksum, &appliedAt)
	if err == pgx.ErrNoRows {
		return "", "", nil, nil
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("get applied content: %w", err)
	}
	return content, checksum, appliedAt, nil
}

// InsertMailEvent appends a mail-record row for a KumoMTA log event. Each event
// in a message's lifecycle (Reception → retries → Delivery/Bounce) is a row, so
// the Logs UI can reconstruct a single message's timeline by message_id.
func (r *MailOpsRepo) InsertMailEvent(ctx context.Context, rec *biz.MailRecord) error {
	// RETURNING id so callers (e.g. the webhook producer) can reference the
	// persisted record.
	if err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO mail_records
			(message_id, event_time, mailclass, sender, from_header, recipient, recipient_domain, egress_source, status, record_type, smtp_status, diagnostic)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`,
		rec.MessageID, rec.EventTime, rec.Mailclass, rec.Sender, rec.FromHeader, rec.Recipient,
		rec.RecipientDomain, rec.EgressSource, rec.Status, rec.RecordType, rec.SMTPStatus, rec.Diagnostic).Scan(&rec.ID); err != nil {
		return fmt.Errorf("insert mail event: %w", err)
	}
	return nil
}

// InsertBounce appends a bounce-record row (used for KumoMTA Bounce log events).
// RETURNING id so the row can be referenced (e.g. pushed over SSE).
func (r *MailOpsRepo) InsertBounce(ctx context.Context, b *biz.BounceRecord) error {
	if err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO bounce_records
			(event_time, recipient, mailclass, smtp_status, bounce_type, diagnostic, classification, processing_state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		b.EventTime, b.Recipient, b.Mailclass, b.SMTPStatus, b.BounceType, b.Diagnostic, b.Classification,
		strOrDefault(b.ProcessingState, biz.ProcessingNew)).Scan(&b.ID); err != nil {
		return fmt.Errorf("insert bounce: %w", err)
	}
	return nil
}

// UpdateClassification backfills the subject-derived label on every event row
// for a message (the Reception row plus any deliveries already recorded). Only
// the label is written; the raw subject is never stored on mail_records.
func (r *MailOpsRepo) UpdateClassification(ctx context.Context, messageID, label string) error {
	if messageID == "" || label == "" {
		return nil
	}
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE mail_records SET classification = $2 WHERE message_id = $1`, messageID, label)
	if err != nil {
		return fmt.Errorf("update classification for message %s: %w", messageID, err)
	}
	return nil
}

// RecipientForMessageID returns the most recent recipient recorded for a sent
// message id, used to correlate a VERP async bounce back to who it was for.
func (r *MailOpsRepo) RecipientForMessageID(ctx context.Context, messageID string) (string, error) {
	var recipient string
	err := r.db.Pool.QueryRow(ctx,
		`SELECT recipient FROM mail_records WHERE message_id = $1 AND recipient <> ''
		 ORDER BY event_time DESC LIMIT 1`, messageID).Scan(&recipient)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("lookup recipient for message %s: %w", messageID, err)
	}
	return recipient, nil
}

// InsertFeedbackReport appends a feedback (ARF/FBL) report row.
func (r *MailOpsRepo) InsertFeedbackReport(ctx context.Context, f *biz.FeedbackReport) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO feedback_reports
			(received_at, source, report_type, recipient, processing_state, raw_ref, verified, verification)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.ReceivedAt, f.Source, f.ReportType, f.Recipient,
		strOrDefault(f.ProcessingState, biz.ProcessingNew), f.RawRef, f.Verified, f.Verification)
	if err != nil {
		return fmt.Errorf("insert feedback report: %w", err)
	}
	return nil
}

// IncrementSoftBounce bumps a recipient's soft-bounce counter and returns the
// new count, used by the bounce pipeline's soft-bounce threshold suppression.
func (r *MailOpsRepo) IncrementSoftBounce(ctx context.Context, recipient string) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO recipient_bounce_counts (recipient, soft_count, updated_at)
		VALUES ($1, 1, now())
		ON CONFLICT (recipient) DO UPDATE
		SET soft_count = recipient_bounce_counts.soft_count + 1, updated_at = now()
		RETURNING soft_count`, recipient).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("increment soft bounce: %w", err)
	}
	return count, nil
}

func strOrDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// UpsertQueueState writes a queue snapshot and updates the current queue row.
func (r *MailOpsRepo) UpsertQueueState(ctx context.Context, q *biz.MailclassQueue) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO mailclass_queues (mailclass, state, depth, oldest_message_age_seconds, last_observed_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (mailclass) DO UPDATE
		SET state = EXCLUDED.state, depth = EXCLUDED.depth,
		    oldest_message_age_seconds = EXCLUDED.oldest_message_age_seconds,
		    last_observed_at = now(), updated_at = now()`,
		q.Mailclass, q.State, q.Depth, q.OldestMessageAgeSeconds)
	if err != nil {
		return fmt.Errorf("upsert queue state: %w", err)
	}
	return nil
}
