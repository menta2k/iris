// Package acme wraps go-acme/lego to issue X.509 certificates against an ACME
// directory (Let's Encrypt prod/staging or any RFC 8555 server). This phase
// supports HTTP-01 (via the in-process TokenStore the challenge server serves);
// DNS-01 + a provider registry is a planned follow-up.
//
// The issuer is stateless beyond what the caller wires in: account state is
// loaded into a User, certs are returned in memory, and file mirroring to disk
// is a separate helper — so the persistence layer owns "save back to Postgres".
//
// Ported from the previous iris implementation (github.com/menta2k/iris).
package acme

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/go-acme/lego/v4/registration"
)

// User is the lego registration.User the Issuer signs ACME requests as. The
// account-level RSA key is persisted on the acme_account row; rotating it
// invalidates the registration (lego re-registers with the same email).
type User struct {
	Email        string
	Registration *registration.Resource
	PrivateKey   crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.PrivateKey }

// NewRSAKey generates a fresh 2048-bit RSA key for an ACME account (the lego
// default; Let's Encrypt accepts 2048 and 4096 equally).
func NewRSAKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// EncodeKeyPEM serialises an RSA key to PKCS#8 PEM for storage.
func EncodeKeyPEM(k *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return "", fmt.Errorf("marshal pkcs8: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// DecodeKeyPEM is the round-trip of EncodeKeyPEM, accepting PKCS#1 or PKCS#8.
func DecodeKeyPEM(s string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("acme user: PEM decode failed")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("acme user: pkcs8: %w", err)
		}
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("acme user: pkcs8 key is %T, expected *rsa.PrivateKey", k)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("acme user: unsupported PEM block type %q", block.Type)
	}
}
