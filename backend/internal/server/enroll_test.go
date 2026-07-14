package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/clusterca"
)

// enrollFakeRepo is an in-memory MTANodeRepo good enough for the enrollment
// flow: one node, real token storage with expiry/single-use semantics.
type enrollFakeRepo struct {
	node        *biz.MTANode
	tokens      []*biz.MTANodeEnrollToken
	fingerprint string
}

func (f *enrollFakeRepo) ListNodes(ctx context.Context) ([]*biz.MTANode, error) {
	return []*biz.MTANode{f.node}, nil
}
func (f *enrollFakeRepo) GetNode(ctx context.Context, id string) (*biz.MTANode, error) {
	if id != f.node.ID {
		return nil, biz.NotFound("MTA_NODE_NOT_FOUND", "not found")
	}
	return f.node, nil
}
func (f *enrollFakeRepo) CreateNode(ctx context.Context, n *biz.MTANode) (*biz.MTANode, error) {
	return n, nil
}
func (f *enrollFakeRepo) UpdateNode(ctx context.Context, n *biz.MTANode) (*biz.MTANode, error) {
	return n, nil
}
func (f *enrollFakeRepo) DeleteNode(ctx context.Context, id string) error { return nil }
func (f *enrollFakeRepo) SetNodeCertFingerprint(ctx context.Context, id, fp string) error {
	f.fingerprint = fp
	return nil
}
func (f *enrollFakeRepo) RecordNodeHeartbeat(ctx context.Context, id, v, c, s string) error {
	return nil
}
func (f *enrollFakeRepo) CreateEnrollToken(ctx context.Context, t *biz.MTANodeEnrollToken) (*biz.MTANodeEnrollToken, error) {
	cp := *t
	cp.ID = "tok-1"
	f.tokens = append(f.tokens, &cp)
	return &cp, nil
}
func (f *enrollFakeRepo) OpenEnrollTokens(ctx context.Context, nodeID string) ([]*biz.MTANodeEnrollToken, error) {
	var out []*biz.MTANodeEnrollToken
	for _, t := range f.tokens {
		if t.NodeID == nodeID && t.UsedAt == nil && !t.Expired(time.Now()) {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *enrollFakeRepo) ConsumeEnrollToken(ctx context.Context, id string) error {
	for _, t := range f.tokens {
		if t.ID == id && t.UsedAt == nil {
			now := time.Now()
			t.UsedAt = &now
			return nil
		}
	}
	return biz.Invalid("MTA_NODE_ENROLL_TOKEN_SPENT", "spent")
}

func adminCtx() context.Context {
	return biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "op",
		Email:       "op@example.com",
		Permissions: biz.NewPermissionSet([]string{"cluster:write"}),
		MFAVerified: true,
	})
}

// TestEnrollmentRoundTrip exercises the whole online flow: operator issues a
// token → agent generates key+CSR and redeems it over HTTP → the issued
// certificate completes a mutual-TLS handshake against the same CA → the
// fingerprint is pinned → the token is single-use.
func TestEnrollmentRoundTrip(t *testing.T) {
	caDir := t.TempDir()
	if err := clusterca.InitCA(caDir, "test-ca"); err != nil {
		t.Fatalf("InitCA: %v", err)
	}
	repo := &enrollFakeRepo{node: &biz.MTANode{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive}}
	uc := biz.NewClusterEnrollUsecase(repo, clusterca.Signer{Dir: caDir}, nil)

	srv := httptest.NewServer(NewEnrollHandler(uc))
	defer srv.Close()

	token, expiresAt, err := uc.IssueToken(adminCtx(), "n2")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token == "" || time.Until(expiresAt) <= 0 {
		t.Fatalf("token = %q expires %v", token, expiresAt)
	}

	outDir := t.TempDir()
	certPath, keyPath, caPath, err := clusterca.Enroll(clusterca.EnrollOptions{
		IrisURL:  srv.URL,
		NodeName: "node2",
		Token:    token,
		IPs:      []net.IP{net.ParseIP("127.0.0.1")},
		OutDir:   outDir,
	})
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("agent.key stat = %v, %v (want 0600)", info, err)
	}
	if repo.fingerprint == "" {
		t.Fatal("fingerprint was not pinned")
	}
	gotFP, err := clusterca.Fingerprint(certPath)
	if err != nil || gotFP != repo.fingerprint {
		t.Fatalf("pinned fingerprint %q != issued %q (%v)", repo.fingerprint, gotFP, err)
	}

	// The issued material must complete a real mutual-TLS handshake.
	serverCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("load issued pair: %v", err)
	}
	caPEM, _ := os.ReadFile(caPath)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	tlsSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	tlsSrv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}
	tlsSrv.StartTLS()
	defer tlsSrv.Close()

	// Issue an iris-side client cert from the same CA (as the control plane
	// would use) and call the agent.
	irisDir := t.TempDir()
	if _, err := clusterca.IssueCert(caDir, irisDir, "iris-control-plane", clusterca.IssueOptions{}); err != nil {
		t.Fatalf("IssueCert client: %v", err)
	}
	clientCert, err := tls.LoadX509KeyPair(filepath.Join(irisDir, "iris-control-plane.crt"), filepath.Join(irisDir, "iris-control-plane.key"))
	if err != nil {
		t.Fatalf("load client pair: %v", err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{clientCert},
	}}}
	resp, err := client.Get(tlsSrv.URL)
	if err != nil {
		t.Fatalf("mTLS request with enrolled cert: %v", err)
	}
	resp.Body.Close()

	// Single use: redeeming the same token again must fail.
	_, _, _, err = clusterca.Enroll(clusterca.EnrollOptions{
		IrisURL: srv.URL, NodeName: "node2", Token: token,
		IPs: []net.IP{net.ParseIP("127.0.0.1")}, OutDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("token replay should be rejected, got %v", err)
	}
}

// TestEnrollmentRejectsBadToken verifies wrong tokens and unknown node names
// both fail with the same indistinguishable error.
func TestEnrollmentRejectsBadToken(t *testing.T) {
	caDir := t.TempDir()
	if err := clusterca.InitCA(caDir, "test-ca"); err != nil {
		t.Fatalf("InitCA: %v", err)
	}
	repo := &enrollFakeRepo{node: &biz.MTANode{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive}}
	uc := biz.NewClusterEnrollUsecase(repo, clusterca.Signer{Dir: caDir}, nil)
	srv := httptest.NewServer(NewEnrollHandler(uc))
	defer srv.Close()

	if _, _, err := uc.IssueToken(adminCtx(), "n2"); err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	for name, node := range map[string]string{"wrong token": "node2", "unknown node": "ghost"} {
		_, _, _, err := clusterca.Enroll(clusterca.EnrollOptions{
			IrisURL: srv.URL, NodeName: node, Token: "not-the-token",
			IPs: []net.IP{net.ParseIP("127.0.0.1")}, OutDir: t.TempDir(),
		})
		if err == nil || !strings.Contains(err.Error(), "invalid, expired, or already used") {
			t.Fatalf("%s: expected uniform rejection, got %v", name, err)
		}
	}
}
