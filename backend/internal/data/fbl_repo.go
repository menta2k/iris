package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// FBLRepo persists feedback-loop endpoints (per-entry FBL enrollments).
type FBLRepo struct {
	db *DB
}

// NewFBLRepo constructs the repository.
func NewFBLRepo(db *DB) *FBLRepo { return &FBLRepo{db: db} }

var _ biz.FBLRepo = (*FBLRepo)(nil)

const fblCols = `id, domain, feedback_address, forward_address, status`

func scanFBL(row interface{ Scan(...any) error }) (*biz.FBLEndpoint, error) {
	f := &biz.FBLEndpoint{}
	if err := row.Scan(&f.ID, &f.Domain, &f.FeedbackAddress, &f.ForwardAddress, &f.Status); err != nil {
		return nil, err
	}
	return f, nil
}

// CreateFBLEndpoint inserts a feedback-loop endpoint.
func (r *FBLRepo) CreateFBLEndpoint(ctx context.Context, f *biz.FBLEndpoint) (*biz.FBLEndpoint, error) {
	out, err := scanFBL(r.db.Pool.QueryRow(ctx, `
		INSERT INTO fbl_endpoints (domain, feedback_address, forward_address, status)
		VALUES ($1, $2, $3, $4)
		RETURNING `+fblCols,
		f.Domain, f.FeedbackAddress, f.ForwardAddress, f.Status))
	if err != nil {
		return nil, mapConstraint(err, "fbl_endpoint")
	}
	return out, nil
}

// UpdateFBLEndpoint updates a feedback-loop endpoint by id.
func (r *FBLRepo) UpdateFBLEndpoint(ctx context.Context, id string, f *biz.FBLEndpoint) (*biz.FBLEndpoint, error) {
	out, err := scanFBL(r.db.Pool.QueryRow(ctx, `
		UPDATE fbl_endpoints SET domain = $2, feedback_address = $3,
			forward_address = $4, status = $5, updated_at = now()
		WHERE id = $1
		RETURNING `+fblCols,
		id, f.Domain, f.FeedbackAddress, f.ForwardAddress, f.Status))
	if err != nil {
		return nil, mapConstraint(err, "fbl_endpoint")
	}
	return out, nil
}

// DeleteFBLEndpoint removes a feedback-loop endpoint by id.
func (r *FBLRepo) DeleteFBLEndpoint(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM fbl_endpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete fbl endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("FBL_ENDPOINT_NOT_FOUND", "feedback-loop endpoint not found")
	}
	return nil
}

// ListFBLEndpoints returns a page of feedback-loop endpoints ordered by domain.
func (r *FBLRepo) ListFBLEndpoints(ctx context.Context, page biz.Page) ([]*biz.FBLEndpoint, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+fblCols+`
		FROM fbl_endpoints ORDER BY domain, feedback_address LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query fbl endpoints: %w", err)
	}
	defer rows.Close()
	return scanFBLRows(rows)
}

// ListFBLEndpointsForPolicy returns all feedback-loop endpoints for rendering.
func (r *FBLRepo) ListFBLEndpointsForPolicy(ctx context.Context) ([]*biz.FBLEndpoint, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+fblCols+` FROM fbl_endpoints ORDER BY domain, feedback_address`)
	if err != nil {
		return nil, fmt.Errorf("query fbl endpoints for policy: %w", err)
	}
	defer rows.Close()
	return scanFBLRows(rows)
}

func scanFBLRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*biz.FBLEndpoint, error) {
	var out []*biz.FBLEndpoint
	for rows.Next() {
		f, err := scanFBL(rows)
		if err != nil {
			return nil, fmt.Errorf("scan fbl endpoint: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
