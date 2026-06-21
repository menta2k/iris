package data

import (
	"context"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// AcmeRepo persists the ACME account (singleton) and issued certificates.
type AcmeRepo struct {
	db *DB
}

// NewAcmeRepo constructs the repository.
func NewAcmeRepo(db *DB) *AcmeRepo { return &AcmeRepo{db: db} }

var (
	_ biz.AcmeAccountRepo     = (*AcmeRepo)(nil)
	_ biz.AcmeCertificateRepo = (*AcmeRepo)(nil)
)

// --- account ---

// GetAccount returns the singleton account row.
func (r *AcmeRepo) GetAccount(ctx context.Context) (*biz.AcmeAccount, error) {
	a := &biz.AcmeAccount{}
	err := r.db.Pool.QueryRow(ctx,
		`SELECT email, server_url, registration_json, private_key_pem, updated_at
		 FROM acme_account WHERE id = 1`).
		Scan(&a.Email, &a.ServerURL, &a.RegistrationJSON, &a.PrivateKeyPEM, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get acme account: %w", err)
	}
	return a, nil
}

// SaveAccount upserts the email + directory URL, preserving the key/registration.
func (r *AcmeRepo) SaveAccount(ctx context.Context, email, serverURL string) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO acme_account (id, email, server_url, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, server_url = EXCLUDED.server_url, updated_at = now()`,
		email, serverURL)
	if err != nil {
		return fmt.Errorf("save acme account: %w", err)
	}
	return nil
}

// SaveAccountKey stores the account private key.
func (r *AcmeRepo) SaveAccountKey(ctx context.Context, keyPEM string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE acme_account SET private_key_pem = $1, updated_at = now() WHERE id = 1`, keyPEM)
	if err != nil {
		return fmt.Errorf("save acme key: %w", err)
	}
	return nil
}

// SaveRegistration stores the lego registration resource JSON.
func (r *AcmeRepo) SaveRegistration(ctx context.Context, registrationJSON string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE acme_account SET registration_json = $1, updated_at = now() WHERE id = 1`, registrationJSON)
	if err != nil {
		return fmt.Errorf("save acme registration: %w", err)
	}
	return nil
}

// --- certificates ---

// UpsertCertificate inserts or updates a certificate keyed by domain.
func (r *AcmeRepo) UpsertCertificate(ctx context.Context, c *biz.AcmeCertificate) (*biz.AcmeCertificate, error) {
	out := &biz.AcmeCertificate{}
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO acme_certificate
			(domain, alt_names, challenge_type, cert_pem, key_pem, cert_path, key_path,
			 expires_at, last_renewed_at, status, last_error, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
		ON CONFLICT (domain) DO UPDATE SET
			alt_names = EXCLUDED.alt_names, challenge_type = EXCLUDED.challenge_type,
			cert_pem = CASE WHEN EXCLUDED.cert_pem <> '' THEN EXCLUDED.cert_pem ELSE acme_certificate.cert_pem END,
			key_pem  = CASE WHEN EXCLUDED.key_pem  <> '' THEN EXCLUDED.key_pem  ELSE acme_certificate.key_pem  END,
			cert_path = CASE WHEN EXCLUDED.cert_path <> '' THEN EXCLUDED.cert_path ELSE acme_certificate.cert_path END,
			key_path  = CASE WHEN EXCLUDED.key_path  <> '' THEN EXCLUDED.key_path  ELSE acme_certificate.key_path END,
			expires_at = EXCLUDED.expires_at, last_renewed_at = EXCLUDED.last_renewed_at,
			status = EXCLUDED.status, last_error = EXCLUDED.last_error, updated_at = now()
		RETURNING id, domain, alt_names, challenge_type, cert_pem, cert_path, key_path,
			expires_at, last_renewed_at, status, last_error`,
		c.Domain, nonNilStrings(c.AltNames), c.ChallengeType, c.CertPEM, c.KeyPEM, c.CertPath, c.KeyPath,
		c.ExpiresAt, c.LastRenewedAt, c.Status, c.LastError).
		Scan(&out.ID, &out.Domain, &out.AltNames, &out.ChallengeType, &out.CertPEM, &out.CertPath, &out.KeyPath,
			&out.ExpiresAt, &out.LastRenewedAt, &out.Status, &out.LastError)
	if err != nil {
		return nil, fmt.Errorf("upsert acme certificate: %w", err)
	}
	return out, nil
}

func (r *AcmeRepo) scanCert(rows interface {
	Scan(dest ...any) error
}) (*biz.AcmeCertificate, error) {
	c := &biz.AcmeCertificate{}
	err := rows.Scan(&c.ID, &c.Domain, &c.AltNames, &c.ChallengeType, &c.CertPath, &c.KeyPath,
		&c.ExpiresAt, &c.LastRenewedAt, &c.Status, &c.LastError)
	return c, err
}

const acmeCertCols = `id, domain, alt_names, challenge_type, cert_path, key_path,
	expires_at, last_renewed_at, status, last_error`

// ListCertificates returns all certificates, newest first.
func (r *AcmeRepo) ListCertificates(ctx context.Context) ([]*biz.AcmeCertificate, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT `+acmeCertCols+` FROM acme_certificate ORDER BY domain`)
	if err != nil {
		return nil, fmt.Errorf("list acme certificates: %w", err)
	}
	defer rows.Close()
	var out []*biz.AcmeCertificate
	for rows.Next() {
		c, err := r.scanCert(rows)
		if err != nil {
			return nil, fmt.Errorf("scan acme certificate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCertificateByDomain returns the certificate for a domain, or nil.
func (r *AcmeRepo) GetCertificateByDomain(ctx context.Context, domain string) (*biz.AcmeCertificate, error) {
	c, err := r.scanCert(r.db.Pool.QueryRow(ctx,
		`SELECT `+acmeCertCols+` FROM acme_certificate WHERE domain = $1`, domain))
	if err != nil {
		return nil, nil
	}
	return c, nil
}

// DeleteCertificate removes a certificate row by id.
func (r *AcmeRepo) DeleteCertificate(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM acme_certificate WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete acme certificate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("ACME_CERT_NOT_FOUND", "certificate not found")
	}
	return nil
}

// ListDueForRenewal returns issued certs expiring before the cutoff.
func (r *AcmeRepo) ListDueForRenewal(ctx context.Context, before time.Time) ([]*biz.AcmeCertificate, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+acmeCertCols+` FROM acme_certificate
		 WHERE status = 'issued' AND expires_at IS NOT NULL AND expires_at < $1`, before)
	if err != nil {
		return nil, fmt.Errorf("list renewals: %w", err)
	}
	defer rows.Close()
	var out []*biz.AcmeCertificate
	for rows.Next() {
		c, err := r.scanCert(rows)
		if err != nil {
			return nil, fmt.Errorf("scan renewal: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkCertStatus updates a certificate's status and last error.
func (r *AcmeRepo) MarkCertStatus(ctx context.Context, id, status, lastErr string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE acme_certificate SET status = $2, last_error = $3, updated_at = now() WHERE id = $1`,
		id, status, lastErr)
	if err != nil {
		return fmt.Errorf("mark cert status: %w", err)
	}
	return nil
}
