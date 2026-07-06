package data

import (
	"context"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// DomainSafetyRepo persists DKIM domains and suppression entries.
type DomainSafetyRepo struct {
	db    *DB
	cache *SuppressionCache
	// ttl resolves the current suppression lifetime (0 = permanent); nil = permanent.
	ttl func(context.Context) time.Duration
}

// NewDomainSafetyRepo constructs the repository.
func NewDomainSafetyRepo(db *DB) *DomainSafetyRepo { return &DomainSafetyRepo{db: db} }

// WithSuppressionCache attaches the Redis live-suppression cache and the TTL
// provider used to age entries. Without it the repo is DB-only (cache writes are
// no-ops), which keeps tests and Redis-less deployments working.
func (r *DomainSafetyRepo) WithSuppressionCache(cache *SuppressionCache, ttl func(context.Context) time.Duration) *DomainSafetyRepo {
	r.cache = cache
	r.ttl = ttl
	return r
}

// suppressionExpiry resolves the absolute expiry for a new/refreshed entry from
// the configured TTL: (expires_at, ttl). A zero TTL means permanent (nil, 0).
func (r *DomainSafetyRepo) suppressionExpiry(ctx context.Context) (*time.Time, time.Duration) {
	if r.ttl == nil {
		return nil, 0
	}
	d := r.ttl(ctx)
	if d <= 0 {
		return nil, 0
	}
	exp := time.Now().UTC().Add(d)
	return &exp, d
}

var _ biz.DomainSafetyRepo = (*DomainSafetyRepo)(nil)

// DKIMPublicKey returns the published DKIM TXT record value for one of our active
// signing keys (domain+selector), derived from the stored private key, and false
// when we hold none. Used to verify that an FBL report's embedded original was
// signed by us — no DNS lookup, so it proves WE signed it. Case-insensitive.
func (r *DomainSafetyRepo) DKIMPublicKey(ctx context.Context, domain, selector string) (string, bool) {
	var pem string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT private_key_ref FROM dkim_domains
		WHERE lower(domain) = lower($1) AND lower(selector) = lower($2)
		  AND status = $3 AND private_key_ref <> ''
		LIMIT 1`, domain, selector, biz.DKIMReady).Scan(&pem)
	if err != nil {
		return "", false
	}
	record, _, err := biz.DKIMPublicRecord(pem)
	if err != nil {
		return "", false
	}
	return record, true
}

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

// CreateSuppression inserts a suppression entry and mirrors it to the live
// Redis cache (with the configured TTL).
func (r *DomainSafetyRepo) CreateSuppression(ctx context.Context, s *biz.SuppressionEntry) (*biz.SuppressionEntry, error) {
	expiresAt, ttl := r.suppressionExpiry(ctx)
	row := r.db.Pool.QueryRow(ctx, `
		INSERT INTO suppression_entries (type, value, reason, source, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, type, value, reason, source, status, expires_at`,
		s.Type, s.Value, s.Reason, s.Source, s.Status, expiresAt)
	out := &biz.SuppressionEntry{}
	if err := row.Scan(&out.ID, &out.Type, &out.Value, &out.Reason, &out.Source, &out.Status, &out.ExpiresAt); err != nil {
		return nil, mapConstraint(err, "suppression")
	}
	if out.Status == biz.SuppressActive {
		if err := r.cache.Put(ctx, out.Type, out.Value, ttl); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// UpdateSuppression updates a suppression entry's reason and status by id. An
// entry set active is (re)written to the cache with a fresh TTL; any other
// status removes it from the live list.
func (r *DomainSafetyRepo) UpdateSuppression(ctx context.Context, id string, s *biz.SuppressionEntry) (*biz.SuppressionEntry, error) {
	expiresAt, ttl := r.suppressionExpiry(ctx)
	active := s.Status == biz.SuppressActive
	// Only refresh expires_at when (re)activating; otherwise leave it as stored.
	row := r.db.Pool.QueryRow(ctx, `
		UPDATE suppression_entries
		SET reason = $2, status = $3,
		    expires_at = CASE WHEN $3 = 'active' THEN $4 ELSE expires_at END,
		    updated_at = now()
		WHERE id = $1
		RETURNING id, type, value, reason, source, status, expires_at`,
		id, s.Reason, s.Status, expiresAt)
	out := &biz.SuppressionEntry{}
	if err := row.Scan(&out.ID, &out.Type, &out.Value, &out.Reason, &out.Source, &out.Status, &out.ExpiresAt); err != nil {
		return nil, mapConstraint(err, "suppression")
	}
	if active {
		if err := r.cache.Put(ctx, out.Type, out.Value, ttl); err != nil {
			return nil, err
		}
	} else if err := r.cache.Del(ctx, out.Type, out.Value); err != nil {
		return nil, err
	}
	return out, nil
}

// ClearAllSuppressions deletes every suppression entry from Postgres and flushes
// the Redis live-suppression cache. Returns the number of DB rows removed. The
// KumoMTA policy's memoized lookup (60s TTL) picks up the empty list within a
// minute, so no restart is required.
func (r *DomainSafetyRepo) ClearAllSuppressions(ctx context.Context) (int64, error) {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM suppression_entries`)
	if err != nil {
		return 0, fmt.Errorf("clear suppressions: %w", err)
	}
	if _, err := r.cache.Clear(ctx); err != nil {
		// DB is already cleared; report the cache error but keep the DB count.
		return tag.RowsAffected(), err
	}
	return tag.RowsAffected(), nil
}

// ListSuppressions returns suppression entries.
func (r *DomainSafetyRepo) ListSuppressions(ctx context.Context, page biz.Page) ([]*biz.SuppressionEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, type, value, reason, source, status, expires_at
		FROM suppression_entries ORDER BY value LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query suppressions: %w", err)
	}
	defer rows.Close()
	var out []*biz.SuppressionEntry
	for rows.Next() {
		s := &biz.SuppressionEntry{}
		if err := rows.Scan(&s.ID, &s.Type, &s.Value, &s.Reason, &s.Source, &s.Status, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan suppression: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListActiveSuppressions returns all non-expired active entries, used to
// backfill the Redis cache at startup.
func (r *DomainSafetyRepo) ListActiveSuppressions(ctx context.Context) ([]*biz.SuppressionEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, type, value, reason, source, status, expires_at
		FROM suppression_entries
		WHERE status = 'active' AND (expires_at IS NULL OR expires_at > now())`)
	if err != nil {
		return nil, fmt.Errorf("query active suppressions: %w", err)
	}
	defer rows.Close()
	var out []*biz.SuppressionEntry
	for rows.Next() {
		s := &biz.SuppressionEntry{}
		if err := rows.Scan(&s.ID, &s.Type, &s.Value, &s.Reason, &s.Source, &s.Status, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan suppression: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateTLSPolicy inserts a require-TLS destination-domain policy.
func (r *DomainSafetyRepo) CreateTLSPolicy(ctx context.Context, p *biz.TLSPolicy) (*biz.TLSPolicy, error) {
	out := &biz.TLSPolicy{}
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO require_tls_domains (domain, mode, status)
		VALUES ($1, $2, $3)
		RETURNING id, domain, mode, status`,
		p.Domain, p.Mode, p.Status).Scan(&out.ID, &out.Domain, &out.Mode, &out.Status)
	if err != nil {
		return nil, mapConstraint(err, "require_tls_domains")
	}
	return out, nil
}

// ListTLSPolicies returns require-TLS domain policies ordered by domain.
func (r *DomainSafetyRepo) ListTLSPolicies(ctx context.Context, page biz.Page) ([]*biz.TLSPolicy, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, domain, mode, status
		FROM require_tls_domains ORDER BY domain LIMIT $1 OFFSET $2`,
		page.Size, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("query tls policies: %w", err)
	}
	defer rows.Close()
	var out []*biz.TLSPolicy
	for rows.Next() {
		p := &biz.TLSPolicy{}
		if err := rows.Scan(&p.ID, &p.Domain, &p.Mode, &p.Status); err != nil {
			return nil, fmt.Errorf("scan tls policy: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteTLSPolicy removes a require-TLS domain policy by id.
func (r *DomainSafetyRepo) DeleteTLSPolicy(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM require_tls_domains WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tls policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("TLS_POLICY_NOT_FOUND", "tls policy not found")
	}
	return nil
}

// SuppressRecipient upserts an active email suppression for a recipient. Used
// by the feedback-loop ingest to auto-suppress complainants. Idempotent on
// (type, value): an existing entry is reactivated and its reason/source updated.
func (r *DomainSafetyRepo) SuppressRecipient(ctx context.Context, email, source, reason string) error {
	return r.SuppressRecipientFor(ctx, email, source, reason, 0)
}

// SuppressRecipientFor suppresses with an explicit TTL override: ttl > 0 sets the
// suppression to expire after that duration; ttl <= 0 uses the global suppression
// TTL. Used by bounce rules that carry a per-rule suppression lifetime.
func (r *DomainSafetyRepo) SuppressRecipientFor(ctx context.Context, email, source, reason string, ttl time.Duration) error {
	value := biz.NormalizeSuppressionValue(biz.SuppressEmail, email)
	if value == "" {
		return nil
	}
	expiresAt, effTTL := r.suppressionExpiry(ctx)
	if ttl > 0 {
		exp := time.Now().UTC().Add(ttl)
		expiresAt, effTTL = &exp, ttl
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO suppression_entries (type, value, reason, source, status, expires_at)
		VALUES ('email', $1, $2, $3, 'active', $4)
		ON CONFLICT (type, value) DO UPDATE
		SET status = 'active', reason = EXCLUDED.reason, source = EXCLUDED.source,
		    expires_at = EXCLUDED.expires_at, updated_at = now()`,
		value, reason, source, expiresAt)
	if err != nil {
		return fmt.Errorf("auto-suppress recipient: %w", err)
	}
	if err := r.cache.Put(ctx, biz.SuppressEmail, value, effTTL); err != nil {
		return err
	}
	return nil
}

// IsSuppressed reports whether an active, unexpired suppression blocks the
// recipient, matching either the exact email or its domain.
func (r *DomainSafetyRepo) IsSuppressed(ctx context.Context, recipient string) (bool, error) {
	domain := biz.RecipientDomain(recipient)
	var ok bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM suppression_entries
			WHERE status = 'active'
			  AND (expires_at IS NULL OR expires_at > now())
			  AND ((type = 'email' AND value = $1) OR (type = 'domain' AND value = $2))
		)`, biz.NormalizeSuppressionValue(biz.SuppressEmail, recipient), domain).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check suppression: %w", err)
	}
	return ok, nil
}
