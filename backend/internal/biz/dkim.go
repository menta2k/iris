package biz

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"regexp"
	"strings"
)

// dkimSelectorRe validates a DKIM selector (alphanumeric, dot, hyphen, underscore).
var dkimSelectorRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,63}$`)

// DKIM domain status values.
const (
	DKIMReady          = "ready"
	DKIMDisabled       = "disabled"
	DKIMNeedsAttention = "needs_attention"
)

// dkimKeyBits is the RSA key size generated for DKIM signing keys.
const dkimKeyBits = 2048

// DKIMDomain holds DKIM signing configuration for a domain. PrivateKeyRef holds
// the PEM-encoded RSA private key material used to sign; it is accepted on
// create/update but never returned over the API (only the public-key fingerprint
// is exposed).
type DKIMDomain struct {
	ID                   string
	Domain               string
	Selector             string
	PublicKeyFingerprint string
	PrivateKeyRef        string // PEM-encoded RSA private key (write-only)
	Status               string
}

// Validate checks DKIM invariants. When private key material is supplied it must
// be a parseable RSA private key (PEM).
func (d *DKIMDomain) Validate() error {
	d.Domain = strings.ToLower(strings.TrimSpace(d.Domain))
	d.Selector = strings.TrimSpace(d.Selector)
	if d.Status == "" {
		d.Status = DKIMNeedsAttention
	}
	if d.Domain == "" {
		return Invalid("DKIM_DOMAIN_REQUIRED", "domain is required")
	}
	if len(d.Domain) > 253 || !dnsNameRe.MatchString(d.Domain) {
		return Invalid("DKIM_DOMAIN_INVALID", "domain %q is not a valid DNS name", d.Domain)
	}
	if d.Selector == "" {
		return Invalid("DKIM_SELECTOR_REQUIRED", "selector is required")
	}
	if !dkimSelectorRe.MatchString(d.Selector) {
		return Invalid("DKIM_SELECTOR_INVALID", "selector %q is not valid", d.Selector)
	}
	if strings.TrimSpace(d.PrivateKeyRef) != "" {
		if _, err := ParseDKIMPrivateKey(d.PrivateKeyRef); err != nil {
			return Invalid("DKIM_PRIVATE_KEY_INVALID", "private key is not a valid RSA PEM: %v", err)
		}
	}
	switch d.Status {
	case DKIMReady, DKIMDisabled, DKIMNeedsAttention:
	default:
		return Invalid("DKIM_STATUS_INVALID", "status %q is not valid", d.Status)
	}
	return nil
}

// GenerateDKIMPrivateKey generates a fresh RSA private key and returns it
// PEM-encoded (PKCS#1, "RSA PRIVATE KEY"), the form KumoMTA's signer accepts.
func GenerateDKIMPrivateKey() (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, dkimKeyBits)
	if err != nil {
		return "", Internal(err, "generate dkim key")
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block)), nil
}

// ParseDKIMPrivateKey decodes a PEM-encoded RSA private key (PKCS#1 or PKCS#8).
func ParseDKIMPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, Invalid("DKIM_PRIVATE_KEY_INVALID", "no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, Invalid("DKIM_PRIVATE_KEY_INVALID", "%v", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, Invalid("DKIM_PRIVATE_KEY_INVALID", "key is not RSA")
	}
	return key, nil
}

// DKIMPublicRecord derives, from a PEM private key, the DKIM DNS TXT record value
// (v=DKIM1; k=rsa; p=<base64 DER public key>) and a fingerprint of the public key.
func DKIMPublicRecord(pemStr string) (recordValue, fingerprint string, err error) {
	key, err := ParseDKIMPrivateKey(pemStr)
	if err != nil {
		return "", "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", Internal(err, "marshal dkim public key")
	}
	recordValue = "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)
	sum := sha256.Sum256(der)
	fingerprint = "sha256:" + hex.EncodeToString(sum[:])
	return recordValue, fingerprint, nil
}

// DKIMRecordName returns the DNS name a DKIM TXT record is published at.
func DKIMRecordName(selector, domain string) string {
	return selector + "._domainkey." + domain
}
