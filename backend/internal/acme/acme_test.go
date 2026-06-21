package acme

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/certificate"
)

func TestEncodeDecodeKeyPEM(t *testing.T) {
	key, err := NewRSAKey()
	if err != nil {
		t.Fatalf("new key: %v", err)
	}
	pemStr, err := EncodeKeyPEM(key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodeKeyPEM(pemStr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Fatal("round-tripped key differs from original")
	}
	if _, err := DecodeKeyPEM("not a pem"); err == nil {
		t.Fatal("expected decode error for garbage input")
	}
}

func TestTokenStoreServesChallenge(t *testing.T) {
	store := NewTokenStore()
	_ = store.Present("example.com", "tok123", "keyauth-value")

	rr := httptest.NewRecorder()
	store.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok123", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "keyauth-value" {
		t.Fatalf("expected 200 keyauth, got %d %q", rr.Code, rr.Body.String())
	}

	// Unknown token and unrelated path → 404.
	for _, p := range []string{"/.well-known/acme-challenge/other", "/", "/v1/foo"} {
		rr := httptest.NewRecorder()
		store.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %q, got %d", p, rr.Code)
		}
	}

	// After cleanup the token is gone.
	_ = store.CleanUp("example.com", "tok123", "")
	rr = httptest.NewRecorder()
	store.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok123", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after cleanup, got %d", rr.Code)
	}
}

func TestWriteCertFilesAndParseExpiry(t *testing.T) {
	certPEM, keyPEM, notAfter := selfSigned(t, "mail.example.com")
	dir := t.TempDir()
	certPath, keyPath, err := WriteCertFiles(dir, "mail.example.com", &certificate.Resource{
		Certificate: certPEM, PrivateKey: keyPEM,
	})
	if err != nil {
		t.Fatalf("write cert files: %v", err)
	}
	if certPath != filepath.Join(dir, "mail.example.com", "fullchain.pem") {
		t.Fatalf("unexpected cert path %q", certPath)
	}
	if data, _ := os.ReadFile(certPath); string(data) != string(certPEM) {
		t.Fatal("fullchain.pem content mismatch")
	}
	// The private key file must be 0600.
	if info, _ := os.Stat(keyPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("privkey.pem perms = %v, want 0600", info.Mode().Perm())
	}

	got := ParseExpiry(certPEM)
	if got.Unix() != notAfter.Unix() {
		t.Fatalf("ParseExpiry = %v, want %v", got, notAfter)
	}
	if !ParseExpiry([]byte("garbage")).IsZero() {
		t.Fatal("ParseExpiry of garbage should be zero")
	}
}

// selfSigned returns a self-signed leaf cert + key PEM and its NotAfter.
func selfSigned(t *testing.T, cn string) (certPEM, keyPEM []byte, notAfter time.Time) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	notAfter = time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, notAfter
}

func TestSanitizeDomainDir(t *testing.T) {
	cases := map[string]string{
		"*.kmx.jobs.bg":     "star.kmx.jobs.bg",
		"mail.kmx.jobs.bg":  "mail.kmx.jobs.bg",
		"*.a.b.example.com": "star.a.b.example.com",
	}
	for in, want := range cases {
		if got := sanitizeDomainDir(in); got != want {
			t.Errorf("sanitizeDomainDir(%q) = %q, want %q", in, got, want)
		}
	}
}
