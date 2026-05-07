package acmeissuer

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

// User is the lego registration.User the Issuer signs ACME requests as.
// We persist the account-level RSA-2048 private key on the
// AcmeAccount row; rotating it invalidates the registration (lego will
// re-register with the same email on the next issue).
//
// The Registration field is opaque: lego serializes it into the row's
// registration_json after first registration; on subsequent boots we
// load it back so the account doesn't have to re-accept ToS.
type User struct {
	Email        string
	Registration *registration.Resource
	PrivateKey   crypto.PrivateKey
}

func (u *User) GetEmail() string                        { return u.Email }
func (u *User) GetRegistration() *registration.Resource { return u.Registration }
func (u *User) GetPrivateKey() crypto.PrivateKey        { return u.PrivateKey }

// NewRSAKey generates a fresh 2048-bit RSA key for an ACME account.
// 2048 is the lego default; 4096 works but doubles signing latency on
// every request and Let's Encrypt accepts both equally.
func NewRSAKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// EncodeKeyPEM serialises an RSA key to PEM for storage in the
// AcmeAccount row.
func EncodeKeyPEM(k *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return "", fmt.Errorf("marshal pkcs8: %w", err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return string(out), nil
}

// DecodeKeyPEM is the round-trip of EncodeKeyPEM. We accept either
// PKCS#1 ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") so a key written
// by an older lego version still loads.
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
