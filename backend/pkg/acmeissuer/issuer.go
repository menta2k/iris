// Package acmeissuer wraps go-acme/lego to issue X.509 certificates
// against an ACME directory (Let's Encrypt prod / staging or any other
// RFC 8555 server). It supports both HTTP-01 (via a shared
// TokenStore the admin-service serves on :80) and DNS-01 (via the DNS
// provider registry in pkg/acmedns).
//
// The issuer is stateless beyond what its caller wires in: account
// state is loaded into a User, certs are returned in-memory, file
// mirroring to disk is a separate, optional helper. That keeps the
// hot path testable without a filesystem and lets the persistence
// layer (data/acme_*_repo) own the "save back to PG" step.
package acmeissuer

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// LetsEncryptProd / LetsEncryptStaging are convenience constants for
// the most common ACME directory URLs. Operators can paste either into
// the Settings page; staging exists so they can validate the pipeline
// without burning the prod rate limit.
const (
	LetsEncryptProd    = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// AccountState is the persistent shape of an ACME account: the email,
// directory URL, the lego registration resource (which we treat as
// opaque blob), and the account-level RSA private key. The service
// layer reads/writes a serialised form of this from acme_account.
type AccountState struct {
	Email           string
	ServerURL       string
	Registration    *registration.Resource
	PrivateKey      *rsa.PrivateKey
	NeedsRegister   bool // true when no Registration is set yet
}

// Issuer is the high-level lego facade the iris service layer talks to.
// One Issuer instance per account; cheap to construct, no goroutines.
type Issuer struct {
	state  *AccountState
	client *lego.Client
}

// New constructs an Issuer from an AccountState. The state's
// PrivateKey must be set; the caller is responsible for generating one
// (NewRSAKey) on first use and persisting it. ServerURL must be set.
//
// If state.NeedsRegister is true, the caller should follow up with
// Register() once the user has accepted ToS.
func New(state *AccountState) (*Issuer, error) {
	if state == nil {
		return nil, errors.New("acmeissuer: nil state")
	}
	if state.PrivateKey == nil {
		return nil, errors.New("acmeissuer: state.PrivateKey required")
	}
	if state.ServerURL == "" {
		return nil, errors.New("acmeissuer: state.ServerURL required")
	}
	user := &User{
		Email:        state.Email,
		Registration: state.Registration,
		PrivateKey:   state.PrivateKey,
	}
	cfg := lego.NewConfig(user)
	cfg.CADirURL = state.ServerURL
	cfg.UserAgent = "iris-admin/0.3 (https://github.com/menta2k/iris)"
	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("acmeissuer: lego.NewClient: %w", err)
	}
	return &Issuer{state: state, client: client}, nil
}

// Register accepts the directory's terms-of-service and registers the
// account. Idempotent — when state.Registration is already set we
// return the existing one without contacting the CA.
func (i *Issuer) Register() (*registration.Resource, error) {
	if i.state.Registration != nil {
		return i.state.Registration, nil
	}
	reg, err := i.client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return nil, fmt.Errorf("acmeissuer: register: %w", err)
	}
	i.state.Registration = reg
	i.state.NeedsRegister = false
	return reg, nil
}

// IssueOptions covers a single ObtainCertificate call. PrimaryDomain
// is the CN; AltNames are SANs (additional DNS names on the same cert).
// ChallengeProvider is whichever lego solver the caller has set up —
// the http01.TokenStore in this package, or a DNS-01 provider from
// pkg/acmedns.
type IssueOptions struct {
	PrimaryDomain string
	AltNames      []string
	Provider      challenge.Provider
	UseHTTP01     bool // true → register Provider with HTTP01 solver; false → DNS01
}

// Issue runs ObtainCertificate. The caller must have ensured the
// account is registered (Register()) before this point.
func (i *Issuer) Issue(opts IssueOptions) (*certificate.Resource, error) {
	if opts.PrimaryDomain == "" {
		return nil, errors.New("acmeissuer: PrimaryDomain required")
	}
	if opts.Provider == nil {
		return nil, errors.New("acmeissuer: Provider required")
	}
	if opts.UseHTTP01 {
		if err := i.client.Challenge.SetHTTP01Provider(opts.Provider); err != nil {
			return nil, fmt.Errorf("acmeissuer: SetHTTP01Provider: %w", err)
		}
	} else {
		if err := i.client.Challenge.SetDNS01Provider(opts.Provider); err != nil {
			return nil, fmt.Errorf("acmeissuer: SetDNS01Provider: %w", err)
		}
	}
	domains := append([]string{opts.PrimaryDomain}, opts.AltNames...)
	req := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true, // include intermediate(s) → fullchain.pem ready for kumomta
	}
	res, err := i.client.Certificate.Obtain(req)
	if err != nil {
		return nil, fmt.Errorf("acmeissuer: Obtain(%s): %w", opts.PrimaryDomain, err)
	}
	return res, nil
}

// WriteCertFiles mirrors the issued cert to disk under the convention
//
//	<baseDir>/<domain>/fullchain.pem  (perm 0644 — readable by kumomta)
//	<baseDir>/<domain>/privkey.pem    (perm 0600 — secret)
//
// Returns the absolute paths so the caller can persist them on the
// AcmeCertificate row (and Listener rows can reference them).
//
// The directory is created with 0750. Because admin-service runs as a
// non-root user (UID 65532 in distroless) and kumomta runs as `kumod`,
// they share the volume via group permissions in the prod compose —
// see deploy/docker-compose.yaml's `:0666` mount option (existing
// pattern for /opt/kumomta/etc/dkim).
func WriteCertFiles(baseDir, domain string, res *certificate.Resource) (certPath, keyPath string, err error) {
	if res == nil {
		return "", "", errors.New("acmeissuer: nil certificate.Resource")
	}
	if domain == "" {
		return "", "", errors.New("acmeissuer: empty domain")
	}
	domainDir := filepath.Join(baseDir, domain)
	if err := os.MkdirAll(domainDir, 0o750); err != nil {
		return "", "", fmt.Errorf("acmeissuer: mkdir %s: %w", domainDir, err)
	}
	certPath = filepath.Join(domainDir, "fullchain.pem")
	keyPath = filepath.Join(domainDir, "privkey.pem")
	if err := os.WriteFile(certPath, res.Certificate, 0o644); err != nil {
		return "", "", fmt.Errorf("acmeissuer: write %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, res.PrivateKey, 0o600); err != nil {
		return "", "", fmt.Errorf("acmeissuer: write %s: %w", keyPath, err)
	}
	return certPath, keyPath, nil
}
