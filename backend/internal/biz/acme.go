package biz

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"strings"
	"time"

	legoreg "github.com/go-acme/lego/v4/registration"

	"github.com/menta2k/iris/backend/internal/acme"
	"github.com/menta2k/iris/backend/internal/acmedns"
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
	ChallengeType string // "dns-01" or "http-01"
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

// AcmeDnsProvider is the configured DNS-01 challenge provider (singleton).
type AcmeDnsProvider struct {
	Provider  string            // registry key, e.g. "cloudflare"; empty = unset
	Config    map[string]string // provider-specific credentials/tunables
	UpdatedAt time.Time
}

// Configured reports whether a DNS-01 provider is set.
func (p *AcmeDnsProvider) Configured() bool { return p != nil && p.Provider != "" }

// AcmeDnsProviderRepo persists the singleton DNS-01 provider config.
type AcmeDnsProviderRepo interface {
	GetDnsProvider(ctx context.Context) (*AcmeDnsProvider, error)
	SaveDnsProvider(ctx context.Context, provider string, config map[string]string, by string) error
	ClearDnsProvider(ctx context.Context, by string) error
}

// AcmeDnsProviderInfo is registry metadata for one DNS provider, surfaced to
// the UI so it can render a dynamic credentials form.
type AcmeDnsProviderInfo struct {
	Name           string
	Description    string
	RequiredFields []string
	OptionalFields []string
}

// AcmeUsecase manages the ACME account and issues/renews certificates via the
// lego-backed issuer. It prefers DNS-01 when a provider is configured, falling
// back to the HTTP-01 in-process solver.
type AcmeUsecase struct {
	accounts AcmeAccountRepo
	certs    AcmeCertificateRepo
	dns      AcmeDnsProviderRepo
	tokens   *acme.TokenStore
	certDir  string
	auditor  *Auditor
}

// NewAcmeUsecase constructs the usecase. certDir is where issued PEMs are
// mirrored for KumoMTA to read (referenced by listener TLS paths). dns may be
// nil to disable DNS-01 (HTTP-01 only).
func NewAcmeUsecase(accounts AcmeAccountRepo, certs AcmeCertificateRepo, dns AcmeDnsProviderRepo, tokens *acme.TokenStore, certDir string, auditor *Auditor) *AcmeUsecase {
	if certDir == "" {
		certDir = "/opt/kumomta/etc/tls"
	}
	return &AcmeUsecase{accounts: accounts, certs: certs, dns: dns, tokens: tokens, certDir: certDir, auditor: auditor}
}

// ListDnsProviders returns the registry of supported DNS-01 providers and the
// fields each needs, so the UI can render a credentials form.
func (uc *AcmeUsecase) ListDnsProviders(ctx context.Context) ([]*AcmeDnsProviderInfo, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	infos := acmedns.AllProviderInfo()
	out := make([]*AcmeDnsProviderInfo, 0, len(infos))
	for _, i := range infos {
		out = append(out, &AcmeDnsProviderInfo{
			Name: i.Name, Description: i.Description,
			RequiredFields: i.RequiredFields, OptionalFields: i.OptionalFields,
		})
	}
	return out, nil
}

// GetDnsProvider returns the configured DNS-01 provider with credential VALUES
// redacted (only which keys are set is revealed).
func (uc *AcmeUsecase) GetDnsProvider(ctx context.Context) (*AcmeDnsProvider, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	if uc.dns == nil {
		return &AcmeDnsProvider{}, nil
	}
	p, err := uc.dns.GetDnsProvider(ctx)
	if err != nil {
		return nil, err
	}
	redacted := map[string]string{}
	for k := range p.Config {
		redacted[k] = "[stored]"
	}
	return &AcmeDnsProvider{Provider: p.Provider, Config: redacted, UpdatedAt: p.UpdatedAt}, nil
}

// SetDnsProvider validates and stores the DNS-01 provider credentials. The
// config is validated by constructing the lego provider (catches missing
// required fields and malformed values before persistence).
func (uc *AcmeUsecase) SetDnsProvider(ctx context.Context, provider string, config map[string]string) error {
	id, err := RequirePermission(ctx, PermServiceControl)
	if err != nil {
		return err
	}
	if uc.dns == nil {
		return FailedPrecondition("ACME_DNS_UNAVAILABLE", "dns-01 provider storage is not available")
	}
	provider = strings.TrimSpace(provider)
	if !acmedns.IsRegistered(provider) {
		return Invalid("ACME_DNS_PROVIDER_UNKNOWN", "dns provider %q is not supported", provider)
	}
	if _, err := acmedns.GetProvider(provider, config); err != nil {
		return Invalid("ACME_DNS_CONFIG_INVALID", "%s", err.Error())
	}
	if err := uc.dns.SaveDnsProvider(ctx, provider, config, id.UserID); err != nil {
		return err
	}
	uc.audit(ctx, "acme.dns_provider.save", "acme_dns_provider", provider, AuditSuccess, map[string]any{"provider": provider})
	return nil
}

// ClearDnsProvider removes the DNS-01 provider so issuance falls back to HTTP-01.
func (uc *AcmeUsecase) ClearDnsProvider(ctx context.Context) error {
	id, err := RequirePermission(ctx, PermServiceControl)
	if err != nil {
		return err
	}
	if uc.dns == nil {
		return nil
	}
	if err := uc.dns.ClearDnsProvider(ctx, id.UserID); err != nil {
		return err
	}
	uc.audit(ctx, "acme.dns_provider.clear", "acme_dns_provider", "", AuditSuccess, nil)
	return nil
}

// challengeLabel reports the challenge type issuance would use ("dns-01" when a
// provider is configured, else "http-01"), without constructing the provider.
func (uc *AcmeUsecase) challengeLabel(ctx context.Context) string {
	if uc.dns != nil {
		if p, err := uc.dns.GetDnsProvider(ctx); err == nil && p.Configured() {
			return "dns-01"
		}
	}
	return "http-01"
}

// dnsChallenger returns a constructed DNS-01 provider when one is configured, or
// (nil, nil) when DNS-01 is not set up.
func (uc *AcmeUsecase) dnsChallenger(ctx context.Context) (acmedns.ACMEChallenger, error) {
	if uc.dns == nil {
		return nil, nil
	}
	p, err := uc.dns.GetDnsProvider(ctx)
	if err != nil {
		return nil, err
	}
	if !p.Configured() {
		return nil, nil
	}
	prov, err := acmedns.GetProvider(p.Provider, p.Config)
	if err != nil {
		return nil, Internal(err, "build dns-01 provider")
	}
	return prov, nil
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
			Domain: domain, AltNames: altNames, ChallengeType: uc.challengeLabel(ctx),
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

// issue performs the lego flow and persists the result. DNS-01 is used when a
// provider is configured; otherwise it falls back to the HTTP-01 solver.
func (uc *AcmeUsecase) issue(ctx context.Context, domain string, altNames []string) (*AcmeCertificate, error) {
	issuer, err := uc.issuerFor(ctx)
	if err != nil {
		return nil, err
	}

	dnsProv, err := uc.dnsChallenger(ctx)
	if err != nil {
		return nil, err
	}
	opts := acme.IssueOptions{PrimaryDomain: domain, AltNames: altNames}
	challengeType := "dns-01"
	switch {
	case dnsProv != nil:
		opts.Provider, opts.UseHTTP01 = dnsProv, false
	case uc.tokens != nil:
		opts.Provider, opts.UseHTTP01, challengeType = uc.tokens, true, "http-01"
	default:
		return nil, FailedPrecondition("ACME_NO_SOLVER",
			"no challenge solver available: configure a DNS-01 provider or enable the HTTP-01 listener")
	}

	res, err := issuer.Issue(opts)
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
		Domain: domain, AltNames: altNames, ChallengeType: challengeType,
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
