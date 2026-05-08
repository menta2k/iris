package service

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
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
	// ListNearExpiry returns rows whose expires_at < `before` and whose
	// status is either "issued" (normal renewal) or "failed" (retry). The
	// renewer applies its own time-based backoff on top of the result.
	// Sorted by expires_at ascending so the soonest-to-expire goes first.
	ListNearExpiry(ctx context.Context, before time.Time) ([]AcmeCertificateRow, error)
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

	// inFlight gates concurrent Issue calls per domain. Using sync.Map
	// so begin/end are lock-free on the hot path. The map carries
	// struct{} values — only the keys matter.
	inFlight sync.Map
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

// ListCertificatesNearExpiry is the renewer's hook into the cert
// table. PEM bodies are zeroed for the same bandwidth reason as
// ListCertificates — the renewer only needs metadata.
func (s *AcmeService) ListCertificatesNearExpiry(ctx context.Context, before time.Time) ([]AcmeCertificateRow, error) {
	rows, err := s.certs.ListNearExpiry(ctx, before)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].CertPEM = ""
		rows[i].KeyPEM = ""
	}
	return rows, nil
}

// DeleteCertificate removes a cert row. The PEM files on disk are NOT
// removed — kumomta may still be holding open file descriptors and
// Listener rows may still reference the path.
func (s *AcmeService) DeleteCertificate(ctx context.Context, id uint32) error {
	return s.certs.Delete(ctx, id)
}

// IssueCertificate validates the request, persists a `pending` row,
// and kicks the actual ACME flow off in a goroutine. The HTTP caller
// returns in <100ms; the operator polls the row (or just hits Refresh
// on the Certificates page) to see status flip to `issued` / `failed`.
//
// Why async: lego's HTTP client doesn't honour the inbound request
// context, and DNS-01 propagation routinely takes 15–90 seconds — far
// longer than the kratos rest.timeout (10s by default). Running the
// flow synchronously meant the cert WAS issued at the CA but our row
// got stuck in `pending` because ent refused writes through a
// cancelled context. Decoupling the request lifetime from the issuance
// lifetime fixes that and gives operators a "submit + walk away"
// workflow.
//
// Concurrency: an in-flight map keyed by domain prevents two parallel
// Issue calls for the same domain from racing on the row. A duplicate
// call returns the existing pending row instead of starting a second
// goroutine.
func (s *AcmeService) IssueCertificate(ctx context.Context, req AcmeIssueRequest) (*AcmeCertificateRow, error) {
	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		return nil, errors.New("acme: domain required")
	}
	if req.ChallengeType != "http-01" && req.ChallengeType != "dns-01" {
		return nil, errors.New("acme: challenge_type must be http-01 or dns-01")
	}

	// Synchronous validation pass — we want config errors (no account,
	// no DNS provider) to surface in the HTTP response, not silently in
	// a goroutine. After this point we know the issue *can* run.
	state, err := s.loadAccountState(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := acmeissuer.New(state); err != nil {
		return nil, err
	}
	if req.ChallengeType == "http-01" && s.HttpTokens == nil {
		return nil, errors.New("acme: http-01 challenge requires the :80 challenge listener (set IRIS_ACME_HTTP_BIND)")
	}
	if req.ChallengeType == "dns-01" {
		if _, err := s.buildDnsProvider(ctx, req.DnsProvider); err != nil {
			return nil, err
		}
	}

	// Reject overlapping issuances for the same domain. The first one
	// is still in flight — return its pending row instead of stomping
	// it.
	if !s.beginIssue(req.Domain) {
		log.Printf("acme: issue already in flight for %s — returning existing row", req.Domain)
		if existing, lerr := s.certs.GetByDomain(ctx, req.Domain); lerr == nil && existing != nil {
			return existing, nil
		}
		return nil, errors.New("acme: an issue for this domain is already in progress")
	}

	// Persist the pending row synchronously so the HTTP caller sees an
	// id immediately and can poll.
	pending, err := s.certs.Upsert(ctx, AcmeCertificateRow{
		Domain:        req.Domain,
		AltNames:      req.AltNames,
		ChallengeType: req.ChallengeType,
		DnsProvider:   req.DnsProvider,
		Status:        "pending",
	})
	if err != nil {
		s.endIssue(req.Domain)
		return nil, fmt.Errorf("acme: persist pending row: %w", err)
	}

	// Kick off the actual issuance in the background. Background
	// context with a 10-minute ceiling — DNS-01 propagation worst case
	// is in the 5-minute neighbourhood; anything past that is almost
	// certainly stuck and should be marked failed so the operator gets
	// a clear signal.
	go func(req AcmeIssueRequest) {
		defer s.endIssue(req.Domain)
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		s.runIssue(bg, req, state)
	}(req)

	return pending, nil
}

// runIssue executes the synchronous ACME flow on a background context.
// All errors are logged AND persisted onto the cert row's last_error
// so operators see them in the UI.
func (s *AcmeService) runIssue(ctx context.Context, req AcmeIssueRequest, state *acmeissuer.AccountState) {
	log.Printf("acme: starting issue domain=%s challenge=%s provider=%s",
		req.Domain, req.ChallengeType, req.DnsProvider)

	issuer, err := acmeissuer.New(state)
	if err != nil {
		log.Printf("acme: issuer construct failed for %s: %v", req.Domain, err)
		s.markFailed(ctx, req.Domain, err)
		return
	}
	if state.NeedsRegister {
		if _, err := issuer.Register(); err != nil {
			log.Printf("acme: register failed for %s: %v", req.Domain, err)
			s.markFailed(ctx, req.Domain, err)
			return
		}
		if err := s.persistRegistration(ctx, state); err != nil {
			log.Printf("acme: persistRegistration failed (non-fatal): %v", err)
		}
	}

	opts := acmeissuer.IssueOptions{
		PrimaryDomain: req.Domain,
		AltNames:      req.AltNames,
		UseHTTP01:     req.ChallengeType == "http-01",
	}
	if req.ChallengeType == "http-01" {
		opts.Provider = s.HttpTokens
	} else {
		dns, err := s.buildDnsProvider(ctx, req.DnsProvider)
		if err != nil {
			log.Printf("acme: build dns provider failed for %s: %v", req.Domain, err)
			s.markFailed(ctx, req.Domain, err)
			return
		}
		opts.Provider = dns
	}

	res, err := issuer.Issue(opts)
	if err != nil {
		log.Printf("acme: issue failed for %s: %v", req.Domain, err)
		s.markFailed(ctx, req.Domain, err)
		return
	}
	log.Printf("acme: cert obtained for %s — writing to %s", req.Domain, s.CertBaseDir)

	certPath, keyPath, err := acmeissuer.WriteCertFiles(s.CertBaseDir, req.Domain, res)
	if err != nil {
		log.Printf("acme: WriteCertFiles failed for %s: %v", req.Domain, err)
		s.markFailed(ctx, req.Domain, err)
		return
	}
	log.Printf("acme: wrote PEMs cert=%s key=%s", certPath, keyPath)

	now := time.Now().UTC()
	// Real validity window from the leaf cert. Falls back to the
	// LE 90-day default if parsing fails (which it shouldn't — we
	// just parsed it on disk above).
	expires := acmeissuer.ParseExpiry(res.Certificate)
	if expires.IsZero() {
		expires = now.AddDate(0, 0, 90)
	}
	if _, err := s.certs.Upsert(ctx, AcmeCertificateRow{
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
	}); err != nil {
		log.Printf("acme: persist row failed for %s: %v (cert IS on disk at %s)",
			req.Domain, err, certPath)
		return
	}
	log.Printf("acme: issued domain=%s expires_at=%s", req.Domain, expires.Format(time.RFC3339))
}

// beginIssue marks the domain as in-flight. Returns false when another
// issuance for the same domain is already running, so the caller can
// short-circuit instead of racing.
func (s *AcmeService) beginIssue(domain string) bool {
	_, loaded := s.inFlight.LoadOrStore(domain, struct{}{})
	return !loaded
}

func (s *AcmeService) endIssue(domain string) { s.inFlight.Delete(domain) }

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
