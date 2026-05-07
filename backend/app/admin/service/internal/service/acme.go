package service

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/registration"

	"github.com/menta2k/iris/backend/pkg/acmedns"
	"github.com/menta2k/iris/backend/pkg/acmeissuer"
)

// AcmeAccountRow is the wire-shape of the singleton acme_account row.
// PrivateKey is auto-generated on first Save when missing.
type AcmeAccountRow struct {
	Email            string
	ServerURL        string
	HasRegistration  bool
	RegistrationJSON string
	PrivateKeyPEM    string
	UpdatedAt        time.Time
}

// AcmeCertificateRow mirrors acme_certificate. The PEM blobs are
// returned but the UI doesn't need them on the list endpoint — the
// service-layer List call zeroes them out for bandwidth reasons.
type AcmeCertificateRow struct {
	ID             uint32
	Domain         string
	AltNames       []string
	ChallengeType  string // "http-01" | "dns-01"
	DnsProvider    string
	CertPEM        string
	KeyPEM         string
	CertPemPath    string
	KeyPemPath     string
	ExpiresAt      *time.Time
	LastRenewedAt  *time.Time
	Status         string // pending | issued | renewing | failed
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AcmeDnsProviderConfigRow is one provider's saved credentials. The
// raw config map is what the registry factories expect.
type AcmeDnsProviderConfigRow struct {
	Provider  string
	Config    map[string]string
	UpdatedAt time.Time
	UpdatedBy string
}

// AcmeAccountStore / AcmeCertificateStore / AcmeDnsProviderConfigStore
// are the data-layer contracts.
type AcmeAccountStore interface {
	Get(ctx context.Context) (*AcmeAccountRow, error)
	Save(ctx context.Context, in AcmeAccountRow) (*AcmeAccountRow, error)
}

type AcmeCertificateStore interface {
	List(ctx context.Context) ([]AcmeCertificateRow, error)
	Get(ctx context.Context, id uint32) (*AcmeCertificateRow, error)
	GetByDomain(ctx context.Context, domain string) (*AcmeCertificateRow, error)
	Upsert(ctx context.Context, in AcmeCertificateRow) (*AcmeCertificateRow, error)
	Delete(ctx context.Context, id uint32) error
}

type AcmeDnsProviderConfigStore interface {
	List(ctx context.Context) ([]AcmeDnsProviderConfigRow, error)
	Get(ctx context.Context, provider string) (*AcmeDnsProviderConfigRow, error)
	Upsert(ctx context.Context, in AcmeDnsProviderConfigRow, actor string) (*AcmeDnsProviderConfigRow, error)
	Delete(ctx context.Context, provider string) error
}

// AcmeIssueRequest describes one Issue / Renew request. The service
// resolves the DNS provider from acme_dns_provider_config when
// ChallengeType == "dns-01"; for "http-01" the caller must have the
// :80 listener up (HttpTokenStore set on the service).
type AcmeIssueRequest struct {
	Domain        string
	AltNames      []string
	ChallengeType string
	DnsProvider   string // ignored for http-01
}

// AcmeService is the public façade. It owns the single in-process
// HttpTokenStore (so HTTP-01 issuance and the public :80 listener
// share state) and the cert-on-disk base directory.
type AcmeService struct {
	accounts AcmeAccountStore
	certs    AcmeCertificateStore
	dnsCfg   AcmeDnsProviderConfigStore

	// HttpTokens is the shared HTTP-01 token store. The :80 listener
	// (server.AcmeChallengeServer) calls ServeHTTP on the same
	// instance.
	HttpTokens *acmeissuer.TokenStore

	// CertBaseDir is where issued PEMs are mirrored. Default
	// "/opt/kumomta/etc/tls"; override via env if iris is host-native.
	CertBaseDir string
}

// NewAcmeService constructs the service.
func NewAcmeService(
	accounts AcmeAccountStore,
	certs AcmeCertificateStore,
	dnsCfg AcmeDnsProviderConfigStore,
	tokens *acmeissuer.TokenStore,
	certBaseDir string,
) *AcmeService {
	if certBaseDir == "" {
		certBaseDir = "/opt/kumomta/etc/tls"
	}
	return &AcmeService{
		accounts:    accounts,
		certs:       certs,
		dnsCfg:      dnsCfg,
		HttpTokens:  tokens,
		CertBaseDir: certBaseDir,
	}
}

// --- account ----------------------------------------------------------------

// GetAccount returns the singleton account row. Empty fields signal
// "not set up yet" — the UI prompts the operator to fill email +
// server URL before issuing certs.
func (s *AcmeService) GetAccount(ctx context.Context) (*AcmeAccountRow, error) {
	return s.accounts.Get(ctx)
}

// SaveAccount upserts the account. Generates an RSA private key on
// first save and clears any prior registration (since the key change
// invalidates it) — the next Issue will re-register before obtaining.
func (s *AcmeService) SaveAccount(ctx context.Context, in AcmeAccountRow) (*AcmeAccountRow, error) {
	in.Email = strings.TrimSpace(in.Email)
	in.ServerURL = strings.TrimSpace(in.ServerURL)
	if in.Email == "" || in.ServerURL == "" {
		return nil, errors.New("acme: email and server_url are required")
	}
	existing, err := s.accounts.Get(ctx)
	if err != nil {
		return nil, err
	}
	keyPEM := strings.TrimSpace(in.PrivateKeyPEM)
	if keyPEM == "" {
		keyPEM = strings.TrimSpace(existing.PrivateKeyPEM)
	}
	if keyPEM == "" {
		k, err := acmeissuer.NewRSAKey()
		if err != nil {
			return nil, fmt.Errorf("acme: generate key: %w", err)
		}
		keyPEM, err = acmeissuer.EncodeKeyPEM(k)
		if err != nil {
			return nil, fmt.Errorf("acme: encode key: %w", err)
		}
	}
	in.PrivateKeyPEM = keyPEM
	// Email or server-URL change wipes registration; lego will
	// re-register before the next issue.
	if existing.Email != in.Email || existing.ServerURL != in.ServerURL {
		in.RegistrationJSON = ""
	} else if in.RegistrationJSON == "" {
		in.RegistrationJSON = existing.RegistrationJSON
	}
	return s.accounts.Save(ctx, in)
}

// loadAccountState turns the persisted row into the runtime state
// acmeissuer.New expects.
func (s *AcmeService) loadAccountState(ctx context.Context) (*acmeissuer.AccountState, error) {
	row, err := s.accounts.Get(ctx)
	if err != nil {
		return nil, err
	}
	if row.Email == "" || row.ServerURL == "" {
		return nil, errors.New("acme: account not configured (email + server_url missing)")
	}
	if strings.TrimSpace(row.PrivateKeyPEM) == "" {
		return nil, errors.New("acme: account private key missing — Save the account first")
	}
	pk, err := acmeissuer.DecodeKeyPEM(row.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("acme: decode account key: %w", err)
	}
	state := &acmeissuer.AccountState{
		Email:         row.Email,
		ServerURL:     row.ServerURL,
		PrivateKey:    pk,
		NeedsRegister: row.RegistrationJSON == "",
	}
	if row.RegistrationJSON != "" {
		var reg registration.Resource
		if err := json.Unmarshal([]byte(row.RegistrationJSON), &reg); err != nil {
			return nil, fmt.Errorf("acme: decode registration: %w", err)
		}
		state.Registration = &reg
	}
	return state, nil
}

// persistRegistration stores the post-Register registration resource
// back so future boots skip Register.
func (s *AcmeService) persistRegistration(ctx context.Context, state *acmeissuer.AccountState) error {
	row, err := s.accounts.Get(ctx)
	if err != nil {
		return err
	}
	if state.Registration == nil {
		return nil
	}
	js, err := json.Marshal(state.Registration)
	if err != nil {
		return fmt.Errorf("acme: encode registration: %w", err)
	}
	row.RegistrationJSON = string(js)
	row.PrivateKeyPEM = encodeKeyPEM(state.PrivateKey)
	_, err = s.accounts.Save(ctx, *row)
	return err
}

func encodeKeyPEM(k interface{}) string {
	if rsaKey, ok := k.(*rsa.PrivateKey); ok {
		s, _ := acmeissuer.EncodeKeyPEM(rsaKey)
		return s
	}
	return ""
}

// --- DNS provider config ----------------------------------------------------

// ListDnsProviderConfigs returns saved configs.
func (s *AcmeService) ListDnsProviderConfigs(ctx context.Context) ([]AcmeDnsProviderConfigRow, error) {
	return s.dnsCfg.List(ctx)
}

// UpsertDnsProviderConfig validates the provider name against the
// registry and writes the config.
func (s *AcmeService) UpsertDnsProviderConfig(ctx context.Context, in AcmeDnsProviderConfigRow, actor string) (*AcmeDnsProviderConfigRow, error) {
	in.Provider = strings.TrimSpace(in.Provider)
	if _, err := acmedns.GetProviderInfo(in.Provider); err != nil {
		return nil, err
	}
	if in.Config == nil {
		in.Config = map[string]string{}
	}
	return s.dnsCfg.Upsert(ctx, in, actor)
}

// DeleteDnsProviderConfig removes saved credentials for one provider.
func (s *AcmeService) DeleteDnsProviderConfig(ctx context.Context, provider string) error {
	return s.dnsCfg.Delete(ctx, strings.TrimSpace(provider))
}

// ListProviderRegistry returns registry-side metadata for every known
// provider. The UI uses this to render the provider dropdown + the
// dynamic credentials form.
func (s *AcmeService) ListProviderRegistry() map[string]*acmedns.ProviderInfo {
	return acmedns.GetAllProviderInfo()
}

// --- certificates -----------------------------------------------------------

// ListCertificates returns every cert (PEM blobs zeroed for
// bandwidth — the UI fetches PEMs via Get when downloading).
func (s *AcmeService) ListCertificates(ctx context.Context) ([]AcmeCertificateRow, error) {
	rows, err := s.certs.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].CertPEM = ""
		rows[i].KeyPEM = ""
	}
	return rows, nil
}

// GetCertificate returns one cert with its PEM bodies populated.
func (s *AcmeService) GetCertificate(ctx context.Context, id uint32) (*AcmeCertificateRow, error) {
	return s.certs.Get(ctx, id)
}

// DeleteCertificate removes a cert row. The PEM files on disk are NOT
// removed — kumomta may still be holding open file descriptors and
// Listener rows may still reference the path.
func (s *AcmeService) DeleteCertificate(ctx context.Context, id uint32) error {
	return s.certs.Delete(ctx, id)
}

// IssueCertificate runs the full ACME flow: ensure account
// registered, build a challenge.Provider (HTTP-01 token store or DNS
// provider from registry+saved config), call Issue, persist PEMs +
// mirror to disk, update the row.
func (s *AcmeService) IssueCertificate(ctx context.Context, req AcmeIssueRequest) (*AcmeCertificateRow, error) {
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		return nil, errors.New("acme: domain required")
	}
	if req.ChallengeType != "http-01" && req.ChallengeType != "dns-01" {
		return nil, errors.New("acme: challenge_type must be http-01 or dns-01")
	}
	state, err := s.loadAccountState(ctx)
	if err != nil {
		return nil, err
	}
	issuer, err := acmeissuer.New(state)
	if err != nil {
		return nil, err
	}
	if state.NeedsRegister {
		if _, err := issuer.Register(); err != nil {
			s.markFailed(ctx, req.Domain, err)
			return nil, err
		}
		if err := s.persistRegistration(ctx, state); err != nil {
			// Non-fatal: we just won't have the registration cached on
			// disk for next boot. The next Issue will re-register.
		}
	}
	opts := acmeissuer.IssueOptions{
		PrimaryDomain: req.Domain,
		AltNames:      req.AltNames,
		UseHTTP01:     req.ChallengeType == "http-01",
	}
	if req.ChallengeType == "http-01" {
		if s.HttpTokens == nil {
			return nil, errors.New("acme: http-01 challenge requires the :80 challenge listener (set IRIS_ACME_HTTP_BIND)")
		}
		opts.Provider = s.HttpTokens
	} else {
		dns, err := s.buildDnsProvider(ctx, req.DnsProvider)
		if err != nil {
			s.markFailed(ctx, req.Domain, err)
			return nil, err
		}
		opts.Provider = dns
	}

	// Mark renewing/pending so the UI can show a spinner. Best-effort.
	_, _ = s.certs.Upsert(ctx, AcmeCertificateRow{
		Domain: req.Domain, AltNames: req.AltNames, ChallengeType: req.ChallengeType,
		DnsProvider: req.DnsProvider, Status: "pending",
	})

	res, err := issuer.Issue(opts)
	if err != nil {
		s.markFailed(ctx, req.Domain, err)
		return nil, err
	}

	certPath, keyPath, err := acmeissuer.WriteCertFiles(s.CertBaseDir, req.Domain, res)
	if err != nil {
		s.markFailed(ctx, req.Domain, err)
		return nil, err
	}

	now := time.Now().UTC()
	expires := now.AddDate(0, 0, 90) // Let's Encrypt default; refined later by parsing the cert
	row := AcmeCertificateRow{
		Domain:        req.Domain,
		AltNames:      req.AltNames,
		ChallengeType: req.ChallengeType,
		DnsProvider:   req.DnsProvider,
		CertPEM:       string(res.Certificate),
		KeyPEM:        string(res.PrivateKey),
		CertPemPath:   certPath,
		KeyPemPath:    keyPath,
		ExpiresAt:     &expires,
		LastRenewedAt: &now,
		Status:        "issued",
	}
	return s.certs.Upsert(ctx, row)
}

// RenewCertificate re-runs Issue on an existing row, preserving its
// challenge configuration.
func (s *AcmeService) RenewCertificate(ctx context.Context, id uint32) (*AcmeCertificateRow, error) {
	existing, err := s.certs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.IssueCertificate(ctx, AcmeIssueRequest{
		Domain:        existing.Domain,
		AltNames:      existing.AltNames,
		ChallengeType: existing.ChallengeType,
		DnsProvider:   existing.DnsProvider,
	})
}

func (s *AcmeService) markFailed(ctx context.Context, domain string, err error) {
	_, _ = s.certs.Upsert(ctx, AcmeCertificateRow{
		Domain:    domain,
		Status:    "failed",
		LastError: err.Error(),
	})
}

func (s *AcmeService) buildDnsProvider(ctx context.Context, name string) (acmedns.ACMEChallenger, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("acme: dns-01 requires dns_provider")
	}
	cfg, err := s.dnsCfg.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("acme: load %s config: %w", name, err)
	}
	return acmedns.GetProvider(name, cfg.Config)
}
