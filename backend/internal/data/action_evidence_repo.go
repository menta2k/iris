package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/menta2k/iris/backend/internal/biz"
)

// ActionEvidenceRepo persists the mail-log event behind an automatic action.
type ActionEvidenceRepo struct {
	db *DB
}

// NewActionEvidenceRepo constructs the repository.
func NewActionEvidenceRepo(db *DB) *ActionEvidenceRepo { return &ActionEvidenceRepo{db: db} }

var _ biz.ActionEvidenceRepo = (*ActionEvidenceRepo)(nil)

// RecordEvidence inserts one evidence row (event stored as JSONB).
func (r *ActionEvidenceRepo) RecordEvidence(ctx context.Context, ev *biz.ActionEvidence) error {
	event := ev.Event
	if event == nil {
		event = map[string]any{}
	}
	blob, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal evidence event: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO action_evidence (action_type, subject_type, subject_key, message_id, reason, event)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		ev.ActionType, ev.SubjectType, strings.ToLower(strings.TrimSpace(ev.SubjectKey)),
		ev.MessageID, ev.Reason, blob)
	if err != nil {
		return fmt.Errorf("insert action evidence: %w", err)
	}
	return nil
}

// ListEvidence returns evidence for a subject, newest first, capped at limit.
func (r *ActionEvidenceRepo) ListEvidence(ctx context.Context, subjectType, subjectKey string, limit int) ([]*biz.ActionEvidence, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, action_type, subject_type, subject_key, message_id, reason, event, created_at
		FROM action_evidence
		WHERE subject_type = $1 AND subject_key = $2
		ORDER BY created_at DESC LIMIT $3`,
		subjectType, subjectKey, limit)
	if err != nil {
		return nil, fmt.Errorf("query action evidence: %w", err)
	}
	defer rows.Close()
	var out []*biz.ActionEvidence
	for rows.Next() {
		ev := &biz.ActionEvidence{}
		var blob []byte
		if err := rows.Scan(&ev.ID, &ev.ActionType, &ev.SubjectType, &ev.SubjectKey,
			&ev.MessageID, &ev.Reason, &blob, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan action evidence: %w", err)
		}
		if len(blob) > 0 {
			_ = json.Unmarshal(blob, &ev.Event)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
