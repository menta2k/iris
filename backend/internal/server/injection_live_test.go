package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// okInjector is a stub biz.KumoInjector that always succeeds.
type okInjector struct{ n int }

func (o *okInjector) InjectV1(context.Context, biz.KumoInjectRequest) error { o.n++; return nil }

// TestInjectionServerLive starts the real dedicated listener on its own port
// and drives the exact GreenArrow payload over a real socket, end-to-end
// through the real use case and a stub injector.
func TestInjectionServerLive(t *testing.T) {
	inj := &okInjector{}
	uc := biz.NewGreenArrowInjectUsecase(inj, "apiuser", "s3cret", "")
	cfg := conf.Injection{Enabled: true, Addr: "127.0.0.1:18025", Path: "/api/inject", Timeout: 5 * time.Second}
	srv := NewInjectionServer(cfg, uc, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if srv == nil {
		t.Fatal("expected a server when injection is enabled")
	}
	go func() { _ = srv.Start(context.Background()) }()
	defer srv.Stop(context.Background())

	base := "http://127.0.0.1:18025"
	// Wait for the listener to come up via the health endpoint.
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Valid credentials → success.
	resp, err := http.Post(base+"/api/inject", "application/json", bytes.NewReader([]byte(greenArrowBody)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	var out biz.GAResponse
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode != http.StatusOK || out.Success != 1 {
		t.Fatalf("status=%d body=%s, want 200 {success:1}", resp.StatusCode, body)
	}
	if inj.n != 1 {
		t.Fatalf("injector called %d times, want 1", inj.n)
	}

	// Wrong password → 401 {success:0}.
	bad := `{"username":"apiuser","password":"nope","message":{"subject":"x","text":"y","from_email":"a@b.c","to":[{"email":"r@x.y"}]}}`
	resp2, err := http.Post(base+"/api/inject", "application/json", bytes.NewReader([]byte(bad)))
	if err != nil {
		t.Fatalf("post 2: %v", err)
	}
	var out2 biz.GAResponse
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	_ = json.Unmarshal(body2, &out2)
	if resp2.StatusCode != http.StatusUnauthorized || out2.Success != 0 {
		t.Fatalf("status=%d body=%s, want 401 {success:0}", resp2.StatusCode, body2)
	}
}

// TestInjectionServerLiveHTTPS starts the listener with a TLS config and drives
// the GreenArrow payload over real HTTPS.
func TestInjectionServerLiveHTTPS(t *testing.T) {
	cert, pool := selfSignedCert(t)
	inj := &okInjector{}
	uc := biz.NewGreenArrowInjectUsecase(inj, "apiuser", "s3cret", "")
	cfg := conf.Injection{Enabled: true, Addr: "127.0.0.1:18026", Path: "/api/inject", Timeout: 5 * time.Second, TLS: true}
	tlsConf := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv := NewInjectionServer(cfg, uc, tlsConf, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if srv == nil {
		t.Fatal("expected a server")
	}
	go func() { _ = srv.Start(context.Background()) }()
	defer srv.Stop(context.Background())

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	base := "https://127.0.0.1:18026"
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("HTTPS server did not become ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err := client.Post(base+"/api/inject", "application/json", bytes.NewReader([]byte(greenArrowBody)))
	if err != nil {
		t.Fatalf("https post: %v", err)
	}
	var out biz.GAResponse
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode != http.StatusOK || out.Success != 1 {
		t.Fatalf("status=%d body=%s, want 200 {success:1} over HTTPS", resp.StatusCode, body)
	}
	if inj.n != 1 {
		t.Fatalf("injector called %d times, want 1", inj.n)
	}
}

// selfSignedCert returns a throwaway TLS certificate for 127.0.0.1 and a CA
// pool that trusts it.
func selfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}
