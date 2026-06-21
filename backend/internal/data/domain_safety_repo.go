package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// DomainSafetyRepo persists DKIM domains and suppression entries.
type DomainSafetyRepo struct {
	db *DB
}

// NewDomainSafetyRepo constructs the repository.
func NewDomainSafetyRepo(db *DB) *DomainSafetyRepo { return &DomainSafetyRepo{db: db} }

var _ biz.DomainSafetyRepo = (*DomainSafetyRepo)(nil)

// CreateDKIMDomain inserts a DKIM domain configuration.
func (r *DomainSafetyRepo) CreateDKIMDomain(ctx context.Context, d *biz.DKIMDomain) (*biz.DKIMDomain, error) {
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO dkim_domains (domain, selector, public_key_fingerprint, private_key_ref, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, domain, selector, public_key_fingerprint, private_key_ref, status`,
		d.Domain, d.Selector, d.PublicKeyFingerprint, d.PrivateKeyRef, d.Status)
	out := &biz.DKIMDomain{}
	if err := row.Scan(&out.ID, &out.Domain, &out.Selector, &out.PublicKeyFingerprint,
		&out.PrivateKeyRef, &out.Status); err != nil {
		return nil, mapConstraint(err, "dkim_domain")
	}
	return out, nil
}

// ListDKIMDomains returns DKIM configurations. Private key refs are returned to
// the use case, which strips them before exposing data over the API.
func (r *DomainSafetyRepo) ListDKIMDomains(ctx context.Context, page biz.Page) ([]*biz.DKIMDomain, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, domain, selector, public_key_fingerprint, private_key_ref, status
		FROM dkim_domains ORDER BY domain, selector LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query dkim domains: %w", err)
	}
	defer rows.Close()
	var out []*biz.DKIMDomain
	for rows.Next() {
		d := &biz.DKIMDomain{}
		if err := rows.Scan(&d.ID, &d.Domain, &d.Selector, &d.PublicKeyFingerprint, &d.PrivateKeyRef, &d.Status); err != nil {
			return nil, fmt.Errorf("scan dkim domain: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDKIMDomain updates a DKIM configuration by id. An empty private_key_ref
// preserves the stored key (and its fingerprint); the UI never re-sends the key.
func (r *DomainSafetyRepo) UpdateDKIMDomain(ctx context.Context, id string, d *biz.DKIMDomain) (*biz.DKIMDomain, error) {
	row := r.db.Pool.QueryRow(ctx, `
		UPDATE dkim_domains SET selector = $2,
			public_key_fingerprint = COALESCE(NULLIF($3, ''), public_key_fingerprint),
			private_key_ref = COALESCE(NULLIF($4, ''), private_key_ref),
			status = $5, updated_at = now()
		WHERE id = $1
		RETURNING id, domain, selector, public_key_fingerprint, private_key_ref, status`,
		id, d.Selector, d.PublicKeyFingerprint, d.PrivateKeyRef, d.Status)
	out := &biz.DKIMDomain{}
	if err := row.Scan(&out.ID, &out.Domain, &out.Selector, &out.PublicKeyFingerprint,
		&out.PrivateKeyRef, &out.Status); err != nil {
		return nil, mapConstraint(err, "dkim_domain")
	}
	return out, nil
}

// CreateSuppression inserts a suppression entry.
func (r *DomainSafetyRepo) CreateSuppression(ctx context.Context, s *biz.SuppressionEntry) (*biz.SuppressionEntry, error) {
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO suppression_entries (type, value, reason, source, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, type, value, reason, source, status`,
		s.Type, s.Value, s.Reason, s.Source, s.Status)
	out := &biz.SuppressionEntry{}
	if err := row.Scan(&out.ID, &out.Type, &out.Value, &out.Reason, &out.Source, &out.Status); err != nil {
		return nil, mapConstraint(err, "suppression")
	}
	return out, nil
}

// UpdateSuppression updates a suppression entry's reason and status by id.
func (r *DomainSafetyRepo) UpdateSuppression(ctx context.Context, id string, s *biz.SuppressionEntry) (*biz.SuppressionEntry, error) {
	row := r.db.Pool.QueryRow(ctx, `
		UPDATE suppression_entries SET reason = $2, status = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, type, value, reason, source, status`,
		id, s.Reason, s.Status)
	out := &biz.SuppressionEntry{}
	if err := row.Scan(&out.ID, &out.Type, &out.Value, &out.Reason, &out.Source, &out.Status); err != nil {
		return nil, mapConstraint(err, "suppression")
	}
	return out, nil
}

// ListSuppressions returns suppression entries.
func (r *DomainSafetyRepo) ListSuppressions(ctx context.Context, page biz.Page) ([]*biz.SuppressionEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, type, value, reason, source, status
		FROM suppression_entries ORDER BY value LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query suppressions: %w", err)
	}
	defer rows.Close()
	var out []*biz.SuppressionEntry
	for rows.Next() {
		s := &biz.SuppressionEntry{}
		if err := rows.Scan(&s.ID, &s.Type, &s.Value, &s.Reason, &s.Source, &s.Status); err != nil {
			return nil, fmt.Errorf("scan suppression: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SuppressRecipient upserts an active email suppression for a recipient. Used
// by the feedback-loop ingest to auto-suppress complainants. Idempotent on
// (type, value): an existing entry is reactivated and its reason/source updated.
func (r *DomainSafetyRepo) SuppressRecipient(ctx context.Context, email, source, reason string) error {
	value := biz.NormalizeSuppressionValue(biz.SuppressEmail, email)
	if value == "" {
		return nil
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO suppression_entries (type, value, reason, source, status)
		VALUES ('email', $1, $2, $3, 'active')
		ON CONFLICT (type, value) DO UPDATE
		SET status = 'active', reason = EXCLUDED.reason, source = EXCLUDED.source, updated_at = now()`,
		value, reason, source)
	if err != nil {
		return fmt.Errorf("auto-suppress recipient: %w", err)
	}
	return nil
}

// IsSuppressed reports whether an active suppression blocks the recipient,
// matching either the exact email or its domain.
func (r *DomainSafetyRepo) IsSuppressed(ctx context.Context, recipient string) (bool, error) {
	domain := biz.RecipientDomain(recipient)
	var ok bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM suppression_entries
			WHERE status = 'active'
			  AND ((type = 'email' AND value = $1) OR (type = 'domain' AND value = $2))
		)`, biz.NormalizeSuppressionValue(biz.SuppressEmail, recipient), domain).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check suppression: %w", err)
	}
	return ok, nil
}
