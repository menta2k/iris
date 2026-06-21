package acme

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

// Common ACME directory URLs. Staging lets operators validate the pipeline
// without burning the prod rate limit.
const (
	LetsEncryptProd    = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// AccountState is the persistent shape of an ACME account.
type AccountState struct {
	Email         string
	ServerURL     string
	Registration  *registration.Resource
	PrivateKey    *rsa.PrivateKey
	NeedsRegister bool // true when no Registration is set yet
}

// Issuer is the lego facade the service layer talks to (one per account).
type Issuer struct {
	state  *AccountState
	client *lego.Client
}

// New constructs an Issuer from an AccountState (PrivateKey and ServerURL
// required). If state.NeedsRegister, follow up with Register().
func New(state *AccountState) (*Issuer, error) {
	if state == nil {
		return nil, errors.New("acme: nil state")
	}
	if state.PrivateKey == nil {
		return nil, errors.New("acme: state.PrivateKey required")
	}
	if state.ServerURL == "" {
		return nil, errors.New("acme: state.ServerURL required")
	}
	user := &User{Email: state.Email, Registration: state.Registration, PrivateKey: state.PrivateKey}
	cfg := lego.NewConfig(user)
	cfg.CADirURL = state.ServerURL
	cfg.UserAgent = "iris-admin (https://github.com/menta2k/iris/backend)"
	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("acme: lego.NewClient: %w", err)
	}
	return &Issuer{state: state, client: client}, nil
}

// Register accepts the directory ToS and registers the account. Idempotent —
// returns the existing registration when already set.
func (i *Issuer) Register() (*registration.Resource, error) {
	if i.state.Registration != nil {
		return i.state.Registration, nil
	}
	reg, err := i.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, fmt.Errorf("acme: register: %w", err)
	}
	i.state.Registration = reg
	i.state.NeedsRegister = false
	return reg, nil
}

// IssueOptions covers a single ObtainCertificate call.
type IssueOptions struct {
	PrimaryDomain string
	AltNames      []string
	Provider      challenge.Provider
	UseHTTP01     bool // true → HTTP-01 solver; false → DNS-01
}

// Issue runs ObtainCertificate. The account must already be registered.
func (i *Issuer) Issue(opts IssueOptions) (*certificate.Resource, error) {
	if opts.PrimaryDomain == "" {
		return nil, errors.New("acme: PrimaryDomain required")
	}
	if opts.Provider == nil {
		return nil, errors.New("acme: Provider required")
	}
	if opts.UseHTTP01 {
		if err := i.client.Challenge.SetHTTP01Provider(opts.Provider); err != nil {
			return nil, fmt.Errorf("acme: SetHTTP01Provider: %w", err)
		}
	} else {
		if err := i.client.Challenge.SetDNS01Provider(opts.Provider); err != nil {
			return nil, fmt.Errorf("acme: SetDNS01Provider: %w", err)
		}
	}
	domains := append([]string{opts.PrimaryDomain}, opts.AltNames...)
	res, err := i.client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true, // leaf + intermediates → fullchain ready for kumomta
	})
	if err != nil {
		return nil, fmt.Errorf("acme: Obtain(%s): %w", opts.PrimaryDomain, err)
	}
	return res, nil
}

// ParseExpiry returns the leaf certificate's NotAfter from a bundled PEM (zero
// time on parse failure — callers fall back to the LE 90-day default).
func ParseExpiry(certPEM []byte) time.Time {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}
	}
	return leaf.NotAfter
}

// WriteCertFiles mirrors the issued cert to disk under
//
//	<baseDir>/<domain>/fullchain.pem  (0644 — readable by kumomta)
//	<baseDir>/<domain>/privkey.pem    (0600 — secret)
//
// and returns the paths so they can be persisted and referenced by listeners.
func WriteCertFiles(baseDir, domain string, res *certificate.Resource) (certPath, keyPath string, err error) {
	if res == nil {
		return "", "", errors.New("acme: nil certificate.Resource")
	}
	if domain == "" {
		return "", "", errors.New("acme: empty domain")
	}
	domainDir := filepath.Join(baseDir, domain)
	if err := os.MkdirAll(domainDir, 0o750); err != nil {
		return "", "", fmt.Errorf("acme: mkdir %s: %w", domainDir, err)
	}
	certPath = filepath.Join(domainDir, "fullchain.pem")
	keyPath = filepath.Join(domainDir, "privkey.pem")
	if err := os.WriteFile(certPath, res.Certificate, 0o644); err != nil {
		return "", "", fmt.Errorf("acme: write %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, res.PrivateKey, 0o600); err != nil {
		return "", "", fmt.Errorf("acme: write %s: %w", keyPath, err)
	}
	return certPath, keyPath, nil
}
