package clusterca

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCARefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := InitCA(dir, "test-ca"); err != nil {
		t.Fatalf("InitCA: %v", err)
	}
	if err := InitCA(dir, "test-ca"); err == nil {
		t.Fatal("second InitCA should refuse to overwrite")
	}
	info, err := os.Stat(filepath.Join(dir, CAKeyFile))
	if err != nil {
		t.Fatalf("stat ca.key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ca.key mode = %o, want 0600", info.Mode().Perm())
	}
}

// TestIssuedCertsPerformMutualTLS proves an agent server cert and an iris
// client cert issued by the same CA complete a mutual TLS 1.3 handshake, and
// that a client WITHOUT a certificate is rejected.
func TestIssuedCertsPerformMutualTLS(t *testing.T) {
	caDir := t.TempDir()
	if err := InitCA(caDir, "test-ca"); err != nil {
		t.Fatalf("InitCA: %v", err)
	}
	outDir := t.TempDir()
	fp, err := IssueCert(caDir, outDir, "node1", IssueOptions{
		Server: true,
		IPs:    []net.IP{net.ParseIP("127.0.0.1")},
	})
	if err != nil {
		t.Fatalf("IssueCert server: %v", err)
	}
	if fp == "" {
		t.Fatal("expected a fingerprint")
	}
	gotFP, err := Fingerprint(filepath.Join(outDir, "node1.crt"))
	if err != nil || gotFP != fp {
		t.Fatalf("Fingerprint = %q, %v; want %q", gotFP, err, fp)
	}
	if _, err := IssueCert(caDir, outDir, "iris-control-plane", IssueOptions{}); err != nil {
		t.Fatalf("IssueCert client: %v", err)
	}

	caPEM, _ := os.ReadFile(filepath.Join(caDir, CACertFile))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.LoadX509KeyPair(filepath.Join(outDir, "node1.crt"), filepath.Join(outDir, "node1.key"))
	if err != nil {
		t.Fatalf("load server pair: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}
	srv.StartTLS()
	defer srv.Close()

	clientCert, err := tls.LoadX509KeyPair(filepath.Join(outDir, "iris-control-plane.crt"), filepath.Join(outDir, "iris-control-plane.key"))
	if err != nil {
		t.Fatalf("load client pair: %v", err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      pool,
		Certificates: []tls.Certificate{clientCert},
	}}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("mTLS request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// No client certificate → handshake must fail.
	bare := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    pool,
	}}}
	if resp, err := bare.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("request without client certificate should be rejected")
	}
}
