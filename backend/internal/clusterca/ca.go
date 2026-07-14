// Package clusterca implements the minimal internal certificate authority for
// the iris↔agent control plane. The CA lives as two files on the iris host
// (ca.crt / ca.key, key 0600); node and client certificates are issued from it
// with `iris cluster issue-cert`. Everything is ECDSA P-256, TLS 1.3-ready.
package clusterca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	// CACertFile / CAKeyFile are the CA file names inside the CA directory.
	CACertFile = "ca.crt"
	CAKeyFile  = "ca.key"

	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 2 * 365 * 24 * time.Hour
)

// InitCA creates a new cluster CA in dir. It refuses to overwrite an existing
// CA so an established trust anchor cannot be clobbered accidentally.
func InitCA(dir, commonName string) error {
	certPath := filepath.Join(dir, CACertFile)
	keyPath := filepath.Join(dir, CAKeyFile)
	for _, p := range []string{certPath, keyPath} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("%s already exists; refusing to overwrite the cluster CA", p)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create CA directory: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return err
	}
	if commonName == "" {
		commonName = "iris-cluster-ca"
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"iris"}},
		NotBefore:             time.Now().Add(-5 * time.Minute),
		NotAfter:              time.Now().Add(caValidity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("self-sign CA: %w", err)
	}
	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal CA key: %w", err)
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0o600)
}

// IssueOptions describes a leaf certificate request.
type IssueOptions struct {
	// CommonName identifies the holder (node name or "iris-control-plane").
	CommonName string
	// DNSNames / IPs become subject alternative names. The agent's URL host
	// must be covered or iris will refuse the TLS handshake.
	DNSNames []string
	IPs      []net.IP
	// Server marks the cert for server auth (agents); every cert also carries
	// client auth so an agent could call back into iris later.
	Server bool
}

// IssueCert signs a new leaf key pair with the CA in caDir and writes
// <name>.crt / <name>.key into outDir (key 0600). It returns the certificate's
// SHA-256 fingerprint (hex) for pinning in the node registry.
func IssueCert(caDir, outDir, name string, opts IssueOptions) (fingerprint string, err error) {
	caCert, caKey, err := loadCA(caDir)
	if err != nil {
		return "", err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return "", err
	}
	if opts.CommonName == "" {
		opts.CommonName = name
	}
	eku := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if opts.Server {
		eku = append(eku, x509.ExtKeyUsageServerAuth)
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: opts.CommonName, Organization: []string{"iris"}},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  eku,
		DNSNames:     opts.DNSNames,
		IPAddresses:  opts.IPs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", fmt.Errorf("sign certificate: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	if err := writePEM(filepath.Join(outDir, name+".crt"), "CERTIFICATE", der, 0o644); err != nil {
		return "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal key: %w", err)
	}
	if err := writePEM(filepath.Join(outDir, name+".key"), "EC PRIVATE KEY", keyDER, 0o600); err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// Signer adapts a CA directory to the biz.CSRSigner interface.
type Signer struct {
	Dir string
}

// SignCSR implements biz.CSRSigner against the CA in Dir.
func (s Signer) SignCSR(csrPEM []byte) (certPEM, fingerprint, caPEM string, err error) {
	return SignCSR(s.Dir, csrPEM)
}

// SignCSR signs a PEM-encoded certificate signing request with the CA in dir
// and returns the certificate PEM, its SHA-256 fingerprint (hex), and the CA
// certificate PEM (for the client's trust store). The issued certificate
// carries client+server EKU and ONLY the SANs present in the CSR — the caller
// (enrollment use case) is responsible for authenticating the requester.
func SignCSR(dir string, csrPEM []byte) (certPEM, fingerprint, caPEM string, err error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return "", "", "", fmt.Errorf("payload is not a PEM certificate request")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", "", "", fmt.Errorf("parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return "", "", "", fmt.Errorf("CSR signature invalid: %w", err)
	}
	caCert, caKey, err := loadCA(dir)
	if err != nil {
		return "", "", "", err
	}
	serial, err := randomSerial()
	if err != nil {
		return "", "", "", err
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      csr.Subject,
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, csr.PublicKey, caKey)
	if err != nil {
		return "", "", "", fmt.Errorf("sign certificate: %w", err)
	}
	sum := sha256.Sum256(der)
	caRaw, err := os.ReadFile(filepath.Join(dir, CACertFile))
	if err != nil {
		return "", "", "", fmt.Errorf("read CA certificate: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		hex.EncodeToString(sum[:]), string(caRaw), nil
}

// Fingerprint returns the SHA-256 fingerprint (hex) of a PEM certificate file.
func Fingerprint(certPath string) (string, error) {
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("read certificate: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("%s is not a PEM certificate", certPath)
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

func loadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certRaw, err := os.ReadFile(filepath.Join(dir, CACertFile))
	if err != nil {
		return nil, nil, fmt.Errorf("read CA certificate: %w", err)
	}
	certBlock, _ := pem.Decode(certRaw)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("CA certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA certificate: %w", err)
	}
	keyRaw, err := os.ReadFile(filepath.Join(dir, CAKeyFile))
	if err != nil {
		return nil, nil, fmt.Errorf("read CA key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyRaw)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("CA key is not PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA key: %w", err)
	}
	return cert, key, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}
	return serial, nil
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
