package biz

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"time"

	legoreg "github.com/go-acme/lego/v4/registration"

	"github.com/menta2k/iris/backend/internal/acme"
)

// ACME certificate status values.
const (
	AcmeStatusPending  = "pending"
	AcmeStatusIssued   = "issued"
	AcmeStatusRenewing = "renewing"
	AcmeStatusFailed   = "failed"
)

// AcmeAccount is the operator's single ACME account.
type AcmeAccount struct {
	Email            string
	ServerURL        string
	RegistrationJSON string // lego registration.Resource (secret-ish)
	PrivateKeyPEM    string // account key (secret)
	UpdatedAt        time.Time
}

// Configured reports whether the account has the minimum to issue.
func (a *AcmeAccount) Configured() bool {
	return a != nil && a.Email != "" && a.ServerURL != ""
}

// Registered reports whether the account has registered with the directory.
func (a *AcmeAccount) Registered() bool { return a != nil && a.RegistrationJSON != "" }

// AcmeCertificate is one issued (or in-flight) certificate.
type AcmeCertificate struct {
	ID            string
	Domain        string
	AltNames      []string
	ChallengeType string // "http-01" (dns-01 is a planned follow-up)
	CertPEM       string // leaf + chain (public)
	KeyPEM        string // private key (secret — never returned over the API)
	CertPath      string
	KeyPath       string
	ExpiresAt     *time.Time
	LastRenewedAt *time.Time
	Status        string
	LastError     string
}

// AcmeAccountRepo persists the singleton ACME account.
type AcmeAccountRepo interface {
	GetAccount(ctx context.Context) (*AcmeAccount, error) // nil when unset
	SaveAccount(ctx context.Context, email, serverURL string) error
	SaveAccountKey(ctx context.Context, keyPEM string) error
	SaveRegistration(ctx context.Context, registrationJSON string) error
}

// AcmeCertificateRepo persists issued certificates.
type AcmeCertificateRepo interface {
	UpsertCertificate(ctx context.Context, c *AcmeCertificate) (*AcmeCertificate, error)
	ListCertificates(ctx context.Context) ([]*AcmeCertificate, error)
	GetCertificateByDomain(ctx context.Context, domain string) (*AcmeCertificate, error)
	DeleteCertificate(ctx context.Context, id string) error
	ListDueForRenewal(ctx context.Context, before time.Time) ([]*AcmeCertificate, error)
	MarkCertStatus(ctx context.Context, id, status, lastErr string) error
}

// AcmeUsecase manages the ACME account and issues/renews certificates via the
// lego-backed issuer. HTTP-01 only in this phase.
type AcmeUsecase struct {
	accounts AcmeAccountRepo
	certs    AcmeCertificateRepo
	tokens   *acme.TokenStore
	certDir  string
	auditor  *Auditor
}

// NewAcmeUsecase constructs the usecase. certDir is where issued PEMs are
// mirrored for KumoMTA to read (referenced by listener TLS paths).
func NewAcmeUsecase(accounts AcmeAccountRepo, certs AcmeCertificateRepo, tokens *acme.TokenStore, certDir string, auditor *Auditor) *AcmeUsecase {
	if certDir == "" {
		certDir = "/opt/kumomta/etc/tls"
	}
	return &AcmeUsecase{accounts: accounts, certs: certs, tokens: tokens, certDir: certDir, auditor: auditor}
}

// GetAccount returns the account with secrets stripped.
func (uc *AcmeUsecase) GetAccount(ctx context.Context) (*AcmeAccount, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	acc, err := uc.accounts.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	if acc == nil {
		return &AcmeAccount{}, nil
	}
	return &AcmeAccount{Email: acc.Email, ServerURL: acc.ServerURL,
		RegistrationJSON: redactNonEmpty(acc.RegistrationJSON), UpdatedAt: acc.UpdatedAt}, nil
}

// SaveAccount sets the email + directory URL (and seeds an account key on first
// use). Validates the directory URL is https.
func (uc *AcmeUsecase) SaveAccount(ctx context.Context, email, serverURL string) error {
	id, err := RequirePermission(ctx, PermServiceControl)
	if err != nil {
		return err
	}
	email = strings.TrimSpace(email)
	serverURL = strings.TrimSpace(serverURL)
	if email == "" {
		return Invalid("ACME_EMAIL_REQUIRED", "account email is required")
	}
	if !strings.HasPrefix(serverURL, "https://") {
		return Invalid("ACME_SERVER_INVALID", "server_url must be an https:// ACME directory URL")
	}
	if err := uc.accounts.SaveAccount(ctx, email, serverURL); err != nil {
		return err
	}
	uc.audit(ctx, "acme.account.save", "acme_account", email, AuditSuccess, map[string]any{
		"email": email, "server_url": serverURL, "by": id.UserID,
	})
	return nil
}

// RequestCertificate issues (or re-issues) a certificate for the domain via
// HTTP-01. It registers the account on first use. Synchronous: the call blocks
// for the ACME handshake.
func (uc *AcmeUsecase) RequestCertificate(ctx context.Context, domain string, altNames []string) (*AcmeCertificate, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" || len(domain) > 253 || !dnsNameRe.MatchString(domain) {
		return nil, Invalid("ACME_DOMAIN_INVALID", "domain %q is not a valid DNS name", domain)
	}
	out, err := uc.issue(ctx, domain, altNames)
	if err != nil {
		// Persist the failure so the UI can show last_error.
		_, _ = uc.certs.UpsertCertificate(ctx, &AcmeCertificate{
			Domain: domain, AltNames: altNames, ChallengeType: "http-01",
			Status: AcmeStatusFailed, LastError: err.Error(),
		})
		uc.audit(ctx, "acme.cert.request", "acme_certificate", domain, AuditFailure, map[string]any{"domain": domain})
		return nil, err
	}
	uc.audit(ctx, "acme.cert.request", "acme_certificate", domain, AuditSuccess, map[string]any{
		"domain": domain, "alt_names": altNames,
	})
	out.KeyPEM = "" // never return the key
	return out, nil
}

// issue performs the lego flow and persists the result.
func (uc *AcmeUsecase) issue(ctx context.Context, domain string, altNames []string) (*AcmeCertificate, error) {
	if uc.tokens == nil {
		return nil, FailedPrecondition("ACME_HTTP01_UNAVAILABLE", "http-01 challenge solver is not running")
	}
	issuer, err := uc.issuerFor(ctx)
	if err != nil {
		return nil, err
	}
	res, err := issuer.Issue(acme.IssueOptions{
		PrimaryDomain: domain, AltNames: altNames, Provider: uc.tokens, UseHTTP01: true,
	})
	if err != nil {
		return nil, Internal(err, "acme issue")
	}
	certPath, keyPath, err := acme.WriteCertFiles(uc.certDir, domain, res)
	if err != nil {
		return nil, Internal(err, "acme write cert files")
	}
	now := time.Now().UTC()
	expiry := acme.ParseExpiry(res.Certificate)
	cert := &AcmeCertificate{
		Domain: domain, AltNames: altNames, ChallengeType: "http-01",
		CertPEM: string(res.Certificate), KeyPEM: string(res.PrivateKey),
		CertPath: certPath, KeyPath: keyPath,
		ExpiresAt: timePtrOrNil(expiry), LastRenewedAt: &now,
		Status: AcmeStatusIssued,
	}
	return uc.certs.UpsertCertificate(ctx, cert)
}

// issuerFor loads the account, seeding a key and registering on first use.
func (uc *AcmeUsecase) issuerFor(ctx context.Context) (*acme.Issuer, error) {
	acc, err := uc.accounts.GetAccount(ctx)
	if err != nil {
		return nil, err
	}
	if !acc.Configured() {
		return nil, FailedPrecondition("ACME_NOT_CONFIGURED", "configure the ACME account (email + directory URL) first")
	}
	var key *rsa.PrivateKey
	if strings.TrimSpace(acc.PrivateKeyPEM) == "" {
		key, err = acme.NewRSAKey()
		if err != nil {
			return nil, Internal(err, "generate acme key")
		}
		pemStr, err := acme.EncodeKeyPEM(key)
		if err != nil {
			return nil, Internal(err, "encode acme key")
		}
		if err := uc.accounts.SaveAccountKey(ctx, pemStr); err != nil {
			return nil, err
		}
	} else {
		key, err = acme.DecodeKeyPEM(acc.PrivateKeyPEM)
		if err != nil {
			return nil, Internal(err, "decode acme key")
		}
	}

	var reg *legoreg.Resource
	if strings.TrimSpace(acc.RegistrationJSON) != "" {
		reg = &legoreg.Resource{}
		if err := json.Unmarshal([]byte(acc.RegistrationJSON), reg); err != nil {
			reg = nil // re-register if the stored registration is unreadable
		}
	}
	state := &acme.AccountState{
		Email: acc.Email, ServerURL: acc.ServerURL,
		Registration: reg, PrivateKey: key, NeedsRegister: reg == nil,
	}
	issuer, err := acme.New(state)
	if err != nil {
		return nil, Internal(err, "construct acme issuer")
	}
	if state.NeedsRegister {
		registered, err := issuer.Register()
		if err != nil {
			return nil, Internal(err, "acme register")
		}
		if b, err := json.Marshal(registered); err == nil {
			_ = uc.accounts.SaveRegistration(ctx, string(b))
		}
	}
	return issuer, nil
}

// ListCertificates returns issued certificates with the private keys stripped.
func (uc *AcmeUsecase) ListCertificates(ctx context.Context) ([]*AcmeCertificate, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	items, err := uc.certs.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range items {
		c.KeyPEM = ""
	}
	return items, nil
}

// DeleteCertificate removes a certificate record (the on-disk files are left in
// place so a referencing listener does not break mid-flight).
func (uc *AcmeUsecase) DeleteCertificate(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return err
	}
	if err := uc.certs.DeleteCertificate(ctx, id); err != nil {
		return err
	}
	uc.audit(ctx, "acme.cert.delete", "acme_certificate", id, AuditSuccess, nil)
	return nil
}

// RenewDue re-issues every issued certificate expiring before the cutoff. Used
// by the background renewer; returns the number renewed.
func (uc *AcmeUsecase) RenewDue(ctx context.Context, before time.Time) (int, error) {
	due, err := uc.certs.ListDueForRenewal(ctx, before)
	if err != nil {
		return 0, err
	}
	renewed := 0
	for _, c := range due {
		_ = uc.certs.MarkCertStatus(ctx, c.ID, AcmeStatusRenewing, "")
		if _, err := uc.issue(ctx, c.Domain, c.AltNames); err != nil {
			_ = uc.certs.MarkCertStatus(ctx, c.ID, AcmeStatusFailed, err.Error())
			continue
		}
		renewed++
	}
	return renewed, nil
}

func (uc *AcmeUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}

func redactNonEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "[stored]"
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
