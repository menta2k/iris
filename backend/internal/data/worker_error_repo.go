package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/errlog"
)

// WorkerErrorRepo persists captured worker error-log entries (write side,
// implementing errlog.Sink) and serves the listing API (read side, implementing
// biz.WorkerErrorRepo).
type WorkerErrorRepo struct {
	db *DB
}

// NewWorkerErrorRepo constructs the repository.
func NewWorkerErrorRepo(db *DB) *WorkerErrorRepo { return &WorkerErrorRepo{db: db} }

var (
	_ biz.WorkerErrorRepo = (*WorkerErrorRepo)(nil)
	_ errlog.Sink         = (*WorkerErrorRepo)(nil)
)

// Insert persists a batch of captured entries. Implements errlog.Sink; called
// from the errlog handler's single drain goroutine.
func (r *WorkerErrorRepo) Insert(ctx context.Context, entries []errlog.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range entries {
		detail, err := json.Marshal(e.Detail)
		if err != nil || len(detail) == 0 {
			detail = []byte(`{}`)
		}
		level := e.Level
		if level != "warn" && level != "error" {
			level = "error"
		}
		batch.Queue(`
			INSERT INTO worker_error_logs (event_time, level, worker, message, detail)
			VALUES ($1, $2, $3, $4, $5)`,
			e.Time, level, e.Worker, e.Message, string(detail))
	}
	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range entries {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert worker error: %w", err)
		}
	}
	return nil
}

// List returns recent worker errors (newest first), filtered by level/worker and
// an optional event-time range.
func (r *WorkerErrorRepo) List(ctx context.Context, f biz.WorkerErrorFilter, page biz.Page) ([]*biz.WorkerError, error) {
	var from, to any
	if f.From != nil {
		from = *f.From
	}
	if f.To != nil {
		to = *f.To
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, event_time, level, worker, message, detail
		FROM worker_error_logs
		WHERE ($1 = '' OR level = $1)
		  AND ($2 = '' OR worker = $2)
		  AND ($3::timestamptz IS NULL OR event_time >= $3)
		  AND ($4::timestamptz IS NULL OR event_time <= $4)
		ORDER BY event_time DESC
		LIMIT $5 OFFSET $6`,
		f.Level, f.Worker, from, to, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query worker errors: %w", err)
	}
	defer rows.Close()
	var out []*biz.WorkerError
	for rows.Next() {
		w := &biz.WorkerError{}
		var detail []byte
		if err := rows.Scan(&w.ID, &w.EventTime, &w.Level, &w.Worker, &w.Message, &detail); err != nil {
			return nil, fmt.Errorf("scan worker error: %w", err)
		}
		if len(detail) > 0 {
			_ = json.Unmarshal(detail, &w.Detail) //nolint:errcheck // tolerate malformed detail
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
