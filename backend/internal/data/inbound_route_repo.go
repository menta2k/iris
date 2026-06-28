package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// InboundRouteRepo persists inbound routes (maildir / forward / webhook).
type InboundRouteRepo struct {
	db *DB
}

// NewInboundRouteRepo constructs the repository.
func NewInboundRouteRepo(db *DB) *InboundRouteRepo { return &InboundRouteRepo{db: db} }

var _ biz.InboundRouteRepo = (*InboundRouteRepo)(nil)

const inboundRouteCols = `id, name, match_type, match_value, action, priority, status,
	forward_host, forward_port, forward_tls, maildir_path,
	destination_url, timeout_seconds, retry_policy`

// scanInboundRoute scans a row in inboundRouteCols order. The webhook secret is
// not selected (write-only).
func scanInboundRoute(row interface{ Scan(...any) error }) (*biz.InboundRoute, error) {
	r := &biz.InboundRoute{}
	var policyRaw []byte
	if err := row.Scan(&r.ID, &r.Name, &r.MatchType, &r.MatchValue, &r.Action, &r.Priority, &r.Status,
		&r.ForwardHost, &r.ForwardPort, &r.ForwardTLS, &r.MaildirPath,
		&r.DestinationURL, &r.TimeoutSeconds, &policyRaw); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(policyRaw, &r.RetryPolicy)
	return r, nil
}

// CreateInboundRoute inserts a route.
func (r *InboundRouteRepo) CreateInboundRoute(ctx context.Context, in *biz.InboundRoute) (*biz.InboundRoute, error) {
	policy, _ := json.Marshal(in.RetryPolicy)
	out, err := scanInboundRoute(r.db.Pool.QueryRow(ctx, `
		INSERT INTO inbound_routes
			(name, match_type, match_value, action, priority, status,
			 forward_host, forward_port, forward_tls, maildir_path,
			 destination_url, secret_ref, timeout_seconds, retry_policy)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+inboundRouteCols,
		in.Name, in.MatchType, in.MatchValue, in.Action, in.Priority, in.Status,
		in.ForwardHost, in.ForwardPort, in.ForwardTLS, in.MaildirPath,
		in.DestinationURL, in.SecretRef, in.TimeoutSeconds, string(policy)))
	if err != nil {
		return nil, mapConstraint(err, "inbound_route")
	}
	return out, nil
}

// UpdateInboundRoute updates a route by id. An empty secret_ref preserves the
// stored webhook secret reference.
func (r *InboundRouteRepo) UpdateInboundRoute(ctx context.Context, id string, in *biz.InboundRoute) (*biz.InboundRoute, error) {
	policy, _ := json.Marshal(in.RetryPolicy)
	out, err := scanInboundRoute(r.db.Pool.QueryRow(ctx, `
		UPDATE inbound_routes SET
			name = $2, match_type = $3, match_value = $4, action = $5, priority = $6, status = $7,
			forward_host = $8, forward_port = $9, forward_tls = $10, maildir_path = $11,
			destination_url = $12, secret_ref = COALESCE(NULLIF($13, ''), secret_ref),
			timeout_seconds = $14, retry_policy = $15, updated_at = now()
		WHERE id = $1
		RETURNING `+inboundRouteCols,
		id, in.Name, in.MatchType, in.MatchValue, in.Action, in.Priority, in.Status,
		in.ForwardHost, in.ForwardPort, in.ForwardTLS, in.MaildirPath,
		in.DestinationURL, in.SecretRef, in.TimeoutSeconds, string(policy)))
	if err != nil {
		return nil, mapConstraint(err, "inbound_route")
	}
	return out, nil
}

// DeleteInboundRoute removes a route by id.
func (r *InboundRouteRepo) DeleteInboundRoute(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM inbound_routes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete inbound route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("INBOUND_ROUTE_NOT_FOUND", "inbound route %s not found", id)
	}
	return nil
}

// ListInboundRoutes returns routes ordered by priority (desc) then name. The
// webhook secret is not selected.
func (r *InboundRouteRepo) ListInboundRoutes(ctx context.Context, page biz.Page) ([]*biz.InboundRoute, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+inboundRouteCols+`
		FROM inbound_routes ORDER BY priority DESC, name LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query inbound routes: %w", err)
	}
	defer rows.Close()
	var out []*biz.InboundRoute
	for rows.Next() {
		route, err := scanInboundRoute(rows)
		if err != nil {
			return nil, fmt.Errorf("scan inbound route: %w", err)
		}
		out = append(out, route)
	}
	return out, rows.Err()
}

// ListInboundRoutesForPolicy returns all ACTIVE routes including the webhook
// secret, for rendering the policy. Unlike ListInboundRoutes it selects
// secret_ref (the webhook poster needs it to sign the HMAC).
func (r *InboundRouteRepo) ListInboundRoutesForPolicy(ctx context.Context) ([]*biz.InboundRoute, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+inboundRouteCols+`, secret_ref
		FROM inbound_routes WHERE status = 'active' ORDER BY priority DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("query inbound routes for policy: %w", err)
	}
	defer rows.Close()
	var out []*biz.InboundRoute
	for rows.Next() {
		route := &biz.InboundRoute{}
		var policyRaw []byte
		if err := rows.Scan(&route.ID, &route.Name, &route.MatchType, &route.MatchValue, &route.Action,
			&route.Priority, &route.Status, &route.ForwardHost, &route.ForwardPort, &route.ForwardTLS,
			&route.MaildirPath, &route.DestinationURL, &route.TimeoutSeconds, &policyRaw, &route.SecretRef); err != nil {
			return nil, fmt.Errorf("scan inbound route for policy: %w", err)
		}
		_ = json.Unmarshal(policyRaw, &route.RetryPolicy)
		out = append(out, route)
	}
	return out, rows.Err()
}
