package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// MTANodeRepo persists the KumoMTA cluster node registry and its enrollment
// tokens.
type MTANodeRepo struct {
	db *DB
}

// NewMTANodeRepo constructs the repository.
func NewMTANodeRepo(db *DB) *MTANodeRepo { return &MTANodeRepo{db: db} }

var _ biz.MTANodeRepo = (*MTANodeRepo)(nil)

const mtaNodeCols = `id, name, agent_url, proxy_host, proxy_port, status,
	cert_fingerprint, version, applied_checksum, kumo_state, last_seen_at, notes`

func scanMTANode(row interface{ Scan(...any) error }) (*biz.MTANode, error) {
	n := &biz.MTANode{}
	if err := row.Scan(&n.ID, &n.Name, &n.AgentURL, &n.ProxyHost, &n.ProxyPort,
		&n.Status, &n.CertFingerprint, &n.Version, &n.AppliedChecksum,
		&n.KumoState, &n.LastSeenAt, &n.Notes); err != nil {
		return nil, err
	}
	return n, nil
}

// ListNodes returns all nodes ordered by name.
func (r *MTANodeRepo) ListNodes(ctx context.Context) ([]*biz.MTANode, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+mtaNodeCols+` FROM mta_nodes ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list mta_nodes: %w", err)
	}
	defer rows.Close()
	var out []*biz.MTANode
	for rows.Next() {
		n, err := scanMTANode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan mta_node: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNode returns one node by id.
func (r *MTANodeRepo) GetNode(ctx context.Context, id string) (*biz.MTANode, error) {
	n, err := scanMTANode(r.db.Pool.QueryRow(ctx,
		`SELECT `+mtaNodeCols+` FROM mta_nodes WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get mta_node: %w", err)
	}
	return n, nil
}

// CreateNode inserts a node and returns the stored record.
func (r *MTANodeRepo) CreateNode(ctx context.Context, n *biz.MTANode) (*biz.MTANode, error) {
	out, err := scanMTANode(r.db.Pool.QueryRow(ctx, `
		INSERT INTO mta_nodes (name, agent_url, proxy_host, proxy_port, status, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+mtaNodeCols,
		n.Name, n.AgentURL, n.ProxyHost, n.ProxyPort, n.Status, n.Notes))
	if err != nil {
		return nil, mapConstraint(err, "mta_node")
	}
	return out, nil
}

// UpdateNode edits operator-owned fields (name, agent_url, proxy endpoint,
// status, notes). Agent-reported fields are only written by their dedicated
// methods.
func (r *MTANodeRepo) UpdateNode(ctx context.Context, n *biz.MTANode) (*biz.MTANode, error) {
	out, err := scanMTANode(r.db.Pool.QueryRow(ctx, `
		UPDATE mta_nodes
		SET name = $2, agent_url = $3, proxy_host = $4, proxy_port = $5,
		    status = $6, notes = $7, updated_at = now()
		WHERE id = $1
		RETURNING `+mtaNodeCols,
		n.ID, n.Name, n.AgentURL, n.ProxyHost, n.ProxyPort, n.Status, n.Notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", n.ID)
	}
	if err != nil {
		return nil, mapConstraint(err, "mta_node")
	}
	return out, nil
}

// DeleteNode removes a node by id (enrollment tokens cascade).
func (r *MTANodeRepo) DeleteNode(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM mta_nodes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete mta_node: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	return nil
}

// SetNodeCertFingerprint pins the enrolled agent certificate for the node.
func (r *MTANodeRepo) SetNodeCertFingerprint(ctx context.Context, id, fingerprint string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE mta_nodes SET cert_fingerprint = $2, updated_at = now()
		WHERE id = $1`, id, fingerprint)
	if err != nil {
		return fmt.Errorf("set mta_node cert fingerprint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	return nil
}

// RecordNodeHeartbeat stores agent-reported state and bumps last_seen_at.
// Empty values preserve the stored ones (see the interface contract).
func (r *MTANodeRepo) RecordNodeHeartbeat(ctx context.Context, id, version, appliedChecksum, kumoState string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE mta_nodes
		SET version = CASE WHEN $2 = '' THEN version ELSE $2 END,
		    applied_checksum = CASE WHEN $3 = '' THEN applied_checksum ELSE $3 END,
		    kumo_state = CASE WHEN $4 = '' THEN kumo_state ELSE $4 END,
		    last_seen_at = now(), updated_at = now()
		WHERE id = $1`, id, version, appliedChecksum, kumoState)
	if err != nil {
		return fmt.Errorf("record mta_node heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("MTA_NODE_NOT_FOUND", "mta node %s not found", id)
	}
	return nil
}

const enrollTokenCols = `id, node_id, token_hash, expires_at, used_at, created_by, created_at`

func scanEnrollToken(row interface{ Scan(...any) error }) (*biz.MTANodeEnrollToken, error) {
	t := &biz.MTANodeEnrollToken{}
	if err := row.Scan(&t.ID, &t.NodeID, &t.TokenHash, &t.ExpiresAt,
		&t.UsedAt, &t.CreatedBy, &t.CreatedAt); err != nil {
		return nil, err
	}
	return t, nil
}

// CreateEnrollToken inserts a single-use enrollment token (hash only).
func (r *MTANodeRepo) CreateEnrollToken(ctx context.Context, t *biz.MTANodeEnrollToken) (*biz.MTANodeEnrollToken, error) {
	out, err := scanEnrollToken(r.db.Pool.QueryRow(ctx, `
		INSERT INTO mta_node_enroll_tokens (node_id, token_hash, expires_at, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+enrollTokenCols,
		t.NodeID, t.TokenHash, t.ExpiresAt, t.CreatedBy))
	if err != nil {
		return nil, mapConstraint(err, "mta_node_enroll_token")
	}
	return out, nil
}

// OpenEnrollTokens returns unused, unexpired tokens for the node, newest first.
func (r *MTANodeRepo) OpenEnrollTokens(ctx context.Context, nodeID string) ([]*biz.MTANodeEnrollToken, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+enrollTokenCols+`
		FROM mta_node_enroll_tokens
		WHERE node_id = $1 AND used_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list mta_node_enroll_tokens: %w", err)
	}
	defer rows.Close()
	var out []*biz.MTANodeEnrollToken
	for rows.Next() {
		t, err := scanEnrollToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan mta_node_enroll_token: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ConsumeEnrollToken atomically marks the token used; the WHERE clause makes
// replay (already used) and expiry both fail the update.
func (r *MTANodeRepo) ConsumeEnrollToken(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `
		UPDATE mta_node_enroll_tokens SET used_at = now()
		WHERE id = $1 AND used_at IS NULL AND expires_at > now()`, id)
	if err != nil {
		return fmt.Errorf("consume mta_node_enroll_token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.Invalid("MTA_NODE_ENROLL_TOKEN_SPENT", "enrollment token is used, expired, or unknown")
	}
	return nil
}
