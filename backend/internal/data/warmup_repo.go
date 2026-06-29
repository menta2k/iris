package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// WarmupRepo persists IP-warmup schedules.
type WarmupRepo struct {
	db *DB
}

// NewWarmupRepo constructs the repository.
func NewWarmupRepo(db *DB) *WarmupRepo { return &WarmupRepo{db: db} }

var _ biz.WarmupRepo = (*WarmupRepo)(nil)

// warmupSelect projects a schedule with its VMTA's resolved name.
const warmupSelect = `w.id, w.vmta_id, w.start_date, w.curve, w.stages,
	w.status, w.paused_reason, w.held_day, w.created_at, w.updated_at, coalesce(v.name, '')`

func scanWarmup(row interface{ Scan(...any) error }) (*biz.WarmupSchedule, error) {
	w := &biz.WarmupSchedule{}
	var stages []byte
	if err := row.Scan(&w.ID, &w.VMTAID, &w.StartDate, &w.Curve, &stages,
		&w.Status, &w.PausedReason, &w.HeldDay, &w.CreatedAt, &w.UpdatedAt, &w.VMTAName); err != nil {
		return nil, err
	}
	if len(stages) > 0 {
		if err := json.Unmarshal(stages, &w.Stages); err != nil {
			return nil, fmt.Errorf("decode warmup stages: %w", err)
		}
	}
	return w, nil
}

// CreateWarmup inserts a schedule and returns the stored record.
func (r *WarmupRepo) CreateWarmup(ctx context.Context, w *biz.WarmupSchedule) (*biz.WarmupSchedule, error) {
	stages, err := json.Marshal(w.Stages)
	if err != nil {
		return nil, fmt.Errorf("encode warmup stages: %w", err)
	}
	out, err := scanWarmup(r.db.Pool.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO warmup_schedules (vmta_id, start_date, curve, stages, status, paused_reason, held_day)
			VALUES ($1, $2::date, $3, $4::jsonb, $5, $6, $7)
			RETURNING id, vmta_id, start_date, curve, stages, status, paused_reason, held_day, created_at, updated_at
		)
		SELECT `+warmupSelect+` FROM ins w JOIN vmtas v ON v.id = w.vmta_id`,
		w.VMTAID, w.StartDate, w.Curve, stages, w.Status, w.PausedReason, w.HeldDay))
	if err != nil {
		return nil, mapConstraint(err, "warmup")
	}
	return out, nil
}

// UpdateWarmup updates a schedule by id (used for edits and lifecycle changes).
func (r *WarmupRepo) UpdateWarmup(ctx context.Context, id string, w *biz.WarmupSchedule) (*biz.WarmupSchedule, error) {
	stages, err := json.Marshal(w.Stages)
	if err != nil {
		return nil, fmt.Errorf("encode warmup stages: %w", err)
	}
	out, err := scanWarmup(r.db.Pool.QueryRow(ctx, `
		WITH upd AS (
			UPDATE warmup_schedules
			SET start_date = $2::date, curve = $3, stages = $4::jsonb,
			    status = $5, paused_reason = $6, held_day = $7, updated_at = now()
			WHERE id = $1
			RETURNING id, vmta_id, start_date, curve, stages, status, paused_reason, held_day, created_at, updated_at
		)
		SELECT `+warmupSelect+` FROM upd w JOIN vmtas v ON v.id = w.vmta_id`,
		id, w.StartDate, w.Curve, stages, w.Status, w.PausedReason, w.HeldDay))
	if err != nil {
		return nil, mapConstraint(err, "warmup")
	}
	return out, nil
}

// GetWarmup returns one schedule by id.
func (r *WarmupRepo) GetWarmup(ctx context.Context, id string) (*biz.WarmupSchedule, error) {
	out, err := scanWarmup(r.db.Pool.QueryRow(ctx,
		`SELECT `+warmupSelect+` FROM warmup_schedules w JOIN vmtas v ON v.id = w.vmta_id WHERE w.id = $1`, id))
	if err != nil {
		return nil, mapConstraint(err, "warmup")
	}
	return out, nil
}

// ListWarmups returns schedules (most recent first), filtered by status when set.
func (r *WarmupRepo) ListWarmups(ctx context.Context, status string, page biz.Page) ([]*biz.WarmupSchedule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+warmupSelect+`
		FROM warmup_schedules w JOIN vmtas v ON v.id = w.vmta_id
		WHERE ($1 = '' OR w.status = $1)
		ORDER BY w.created_at DESC
		LIMIT $2 OFFSET $3`, status, page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query warmups: %w", err)
	}
	defer rows.Close()
	return collectWarmups(rows)
}

// ListActiveWarmupsForPolicy returns the schedules that affect the rendered
// policy: those that are active or paused (paused holds the current cap).
func (r *WarmupRepo) ListActiveWarmupsForPolicy(ctx context.Context) ([]*biz.WarmupSchedule, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+warmupSelect+`
		FROM warmup_schedules w JOIN vmtas v ON v.id = w.vmta_id
		WHERE w.status IN ('active', 'paused')
		ORDER BY v.name`)
	if err != nil {
		return nil, fmt.Errorf("query active warmups: %w", err)
	}
	defer rows.Close()
	return collectWarmups(rows)
}

func collectWarmups(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*biz.WarmupSchedule, error) {
	var out []*biz.WarmupSchedule
	for rows.Next() {
		w, err := scanWarmup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan warmup: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
