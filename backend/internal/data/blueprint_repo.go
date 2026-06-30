package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// BlueprintRepo persists delivery blueprints (base shaping rules).
type BlueprintRepo struct {
	db *DB
}

// NewBlueprintRepo constructs the repository.
func NewBlueprintRepo(db *DB) *BlueprintRepo { return &BlueprintRepo{db: db} }

var _ biz.BlueprintRepo = (*BlueprintRepo)(nil)

const blueprintCols = `id, provider, mx_pattern, conn_rate, deliveries_per_conn,
	conn_limit, daily_cap, status, created_at, updated_at`

func scanBlueprint(row interface{ Scan(...any) error }) (*biz.DeliveryBlueprint, error) {
	b := &biz.DeliveryBlueprint{}
	if err := row.Scan(&b.ID, &b.Provider, &b.MXPattern, &b.ConnRate, &b.DeliveriesPerConn,
		&b.ConnLimit, &b.DailyCap, &b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return b, nil
}

// CreateBlueprint inserts a blueprint and returns the stored record.
func (r *BlueprintRepo) CreateBlueprint(ctx context.Context, b *biz.DeliveryBlueprint) (*biz.DeliveryBlueprint, error) {
	out, err := scanBlueprint(r.db.Pool.QueryRow(ctx, `
		INSERT INTO delivery_blueprints (provider, mx_pattern, conn_rate, deliveries_per_conn, conn_limit, daily_cap, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+blueprintCols,
		b.Provider, b.MXPattern, b.ConnRate, b.DeliveriesPerConn, b.ConnLimit, b.DailyCap, b.Status))
	if err != nil {
		return nil, mapConstraint(err, "delivery_blueprint")
	}
	return out, nil
}

// UpdateBlueprint updates a blueprint by id (edits and enable/disable toggles).
func (r *BlueprintRepo) UpdateBlueprint(ctx context.Context, id string, b *biz.DeliveryBlueprint) (*biz.DeliveryBlueprint, error) {
	out, err := scanBlueprint(r.db.Pool.QueryRow(ctx, `
		UPDATE delivery_blueprints
		SET provider = $2, mx_pattern = $3, conn_rate = $4, deliveries_per_conn = $5,
		    conn_limit = $6, daily_cap = $7, status = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+blueprintCols,
		id, b.Provider, b.MXPattern, b.ConnRate, b.DeliveriesPerConn, b.ConnLimit, b.DailyCap, b.Status))
	if err != nil {
		return nil, mapConstraint(err, "delivery_blueprint")
	}
	return out, nil
}

// GetBlueprint returns one blueprint by id.
func (r *BlueprintRepo) GetBlueprint(ctx context.Context, id string) (*biz.DeliveryBlueprint, error) {
	out, err := scanBlueprint(r.db.Pool.QueryRow(ctx,
		`SELECT `+blueprintCols+` FROM delivery_blueprints WHERE id = $1`, id))
	if err != nil {
		return nil, mapConstraint(err, "delivery_blueprint")
	}
	return out, nil
}

// ListBlueprints returns all blueprints grouped sensibly (provider then pattern).
func (r *BlueprintRepo) ListBlueprints(ctx context.Context) ([]*biz.DeliveryBlueprint, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+blueprintCols+` FROM delivery_blueprints ORDER BY provider, mx_pattern`)
	if err != nil {
		return nil, fmt.Errorf("query delivery_blueprints: %w", err)
	}
	defer rows.Close()
	var out []*biz.DeliveryBlueprint
	for rows.Next() {
		b, err := scanBlueprint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan delivery_blueprint: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListActiveBlueprintsForPolicy returns active blueprints for rendering.
func (r *BlueprintRepo) ListActiveBlueprintsForPolicy(ctx context.Context) ([]*biz.DeliveryBlueprint, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+blueprintCols+` FROM delivery_blueprints WHERE status = 'active' ORDER BY mx_pattern`)
	if err != nil {
		return nil, fmt.Errorf("query active delivery_blueprints: %w", err)
	}
	defer rows.Close()
	var out []*biz.DeliveryBlueprint
	for rows.Next() {
		b, err := scanBlueprint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan delivery_blueprint: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SeedDefaults inserts the built-in provider blueprints, skipping any MX pattern
// that already exists. Returns the number inserted.
func (r *BlueprintRepo) SeedDefaults(ctx context.Context, defaults []*biz.DeliveryBlueprint) (int, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	inserted := 0
	for _, b := range defaults {
		tag, err := tx.Exec(ctx, `
			INSERT INTO delivery_blueprints (provider, mx_pattern, conn_rate, deliveries_per_conn, conn_limit, daily_cap, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (mx_pattern) DO NOTHING`,
			b.Provider, b.MXPattern, b.ConnRate, b.DeliveriesPerConn, b.ConnLimit, b.DailyCap, b.Status)
		if err != nil {
			return 0, fmt.Errorf("seed blueprint %s: %w", b.MXPattern, err)
		}
		inserted += int(tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return inserted, nil
}
