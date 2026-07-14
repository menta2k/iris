package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/internal/agentapi"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

type fakeClusterNodes struct {
	nodes      []*biz.MTANode
	heartbeats map[string]string // node id -> applied checksum
}

func (f *fakeClusterNodes) ListNodes(ctx context.Context) ([]*biz.MTANode, error) {
	return f.nodes, nil
}

func (f *fakeClusterNodes) RecordNodeHeartbeat(ctx context.Context, id, version, checksum, kumoState string) error {
	if f.heartbeats == nil {
		f.heartbeats = map[string]string{}
	}
	f.heartbeats[id] = checksum
	return nil
}

// fakeAgent implements the agent stage/activate protocol for tests.
type fakeAgent struct {
	staged   *agentapi.ConfigBundle
	actions  []string
	failWith int // when non-zero, every request fails with this status
}

func (a *fakeAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(agentapi.PathStage, func(w http.ResponseWriter, r *http.Request) {
		if a.failWith != 0 {
			w.WriteHeader(a.failWith)
			_ = json.NewEncoder(w).Encode(agentapi.Error{Code: "TEST_FAIL", Message: "boom"})
			return
		}
		var b agentapi.ConfigBundle
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.staged = &b
		_ = json.NewEncoder(w).Encode(agentapi.StageReply{Staged: true, Checksum: b.Checksum})
	})
	mux.HandleFunc(agentapi.PathActivate, func(w http.ResponseWriter, r *http.Request) {
		if a.failWith != 0 {
			w.WriteHeader(a.failWith)
			return
		}
		var req agentapi.ActivateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		action := "reloaded"
		if req.Restart {
			action = "restarted"
		}
		a.actions = append(a.actions, action)
		_ = json.NewEncoder(w).Encode(agentapi.ActivateReply{Action: action, AppliedChecksum: req.Checksum})
	})
	return mux
}

func clusterRendered() biz.RenderedConfig {
	return biz.RenderedConfig{
		Content:      "-- policy\nreturn {}\n",
		Checksum:     "sum-1",
		InitChecksum: "init-1",
		ShapingBase:  "[base]\n",
	}
}

// TestApplyConfigClusterFanOut verifies a mixed local+remote rollout: the local
// node gets the file write, the remote node gets a staged+activated bundle, and
// both applied checksums land in the registry.
func TestApplyConfigClusterFanOut(t *testing.T) {
	agent := &fakeAgent{}
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "iris_generated.lua")
	adapter := NewFileKumoMTA(conf.External{ConfigPath: path})
	nodes := &fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},                    // local
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: srv.URL}, // remote
		{ID: "n3", Name: "node3", Status: biz.MTANodeStatusDisabled, AgentURL: srv.URL},
	}}
	adapter.AttachCluster(nodes, srv.Client())

	_, summary, err := adapter.ApplyConfig(context.Background(), clusterRendered(), false)
	if err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("local policy not written: %v", err)
	}
	// The local node's identity prelude is written next to the policy.
	prelude, err := os.ReadFile(filepath.Join(dir, biz.NodePreludeFile))
	if err != nil || string(prelude) != biz.NodePreludeContent("node1") {
		t.Fatalf("local node prelude = %q, %v", prelude, err)
	}
	// The remote bundle carries the node name for the agent-side prelude.
	if agent.staged.NodeName != "node2" {
		t.Fatalf("bundle NodeName = %q", agent.staged.NodeName)
	}
	if agent.staged == nil || agent.staged.Checksum != "sum-1" {
		t.Fatalf("remote bundle not staged: %+v", agent.staged)
	}
	if agent.staged.Policy.SHA256 == "" || agent.staged.Generation == 0 {
		t.Fatalf("bundle missing integrity fields: %+v", agent.staged)
	}
	if len(agent.actions) != 1 || agent.actions[0] != "reloaded" {
		t.Fatalf("remote actions = %v", agent.actions)
	}
	if !strings.Contains(summary, "node1") || !strings.Contains(summary, "node2") {
		t.Fatalf("summary should name both nodes: %q", summary)
	}
	if strings.Contains(summary, "node3") {
		t.Fatalf("disabled node applied: %q", summary)
	}
	if nodes.heartbeats["n1"] != "sum-1" || nodes.heartbeats["n2"] != "sum-1" {
		t.Fatalf("heartbeats = %v", nodes.heartbeats)
	}
}

// TestApplyConfigCarriesListenerTLSFiles verifies a listener TLS cert present on
// the control-plane host is read once, written to the local node, and shipped
// (with content + checksum) in the remote node's bundle — the DKIM-key-style
// propagation for cert files.
func TestApplyConfigCarriesListenerTLSFiles(t *testing.T) {
	agent := &fakeAgent{}
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()

	dir := t.TempDir()
	// The cert lives at an absolute path; use the temp dir so the local write
	// (rewrite of the same bytes) stays inside the sandbox.
	certPath := filepath.Join(dir, "certs", "mx.pem")
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		t.Fatal(err)
	}
	certPEM := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	if err := os.WriteFile(certPath, []byte(certPEM), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(dir, "iris_generated.lua")})
	adapter.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: srv.URL},
	}}, srv.Client())

	rendered := clusterRendered()
	rendered.TLSFiles = []biz.TLSFile{{Path: certPath}} // reference only; hydrated from disk
	if _, _, err := adapter.ApplyConfig(context.Background(), rendered, false); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Remote bundle carries the cert content + a correct checksum.
	if len(agent.staged.TLSFiles) != 1 {
		t.Fatalf("remote bundle TLS files = %d", len(agent.staged.TLSFiles))
	}
	f := agent.staged.TLSFiles[0]
	if f.Name != certPath || f.Content != certPEM || f.SHA256 != sha256Hex(certPEM) {
		t.Fatalf("remote TLS file = %+v", f)
	}
}

// TestApplyConfigTLSFileMissingCentrallyIsSkipped verifies a referenced cert
// that does not exist on the control-plane host does not block the apply and is
// not shipped (the node is expected to provide it).
func TestApplyConfigTLSFileMissingCentrallyIsSkipped(t *testing.T) {
	agent := &fakeAgent{}
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()

	dir := t.TempDir()
	adapter := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(dir, "p.lua")})
	adapter.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: srv.URL},
	}}, srv.Client())

	rendered := clusterRendered()
	rendered.TLSFiles = []biz.TLSFile{{Path: filepath.Join(dir, "absent", "mx.pem")}}
	if _, _, err := adapter.ApplyConfig(context.Background(), rendered, false); err != nil {
		t.Fatalf("ApplyConfig should tolerate a centrally-absent cert: %v", err)
	}
	if len(agent.staged.TLSFiles) != 0 {
		t.Fatalf("absent cert must not be shipped, got %+v", agent.staged.TLSFiles)
	}
}

// TestApplyConfigClusterHaltsOnFailure verifies the rolling apply stops at the
// first failing node and reports which nodes already changed.
func TestApplyConfigClusterHaltsOnFailure(t *testing.T) {
	agent := &fakeAgent{failWith: http.StatusInternalServerError}
	srv := httptest.NewServer(agent.handler())
	defer srv.Close()

	dir := t.TempDir()
	adapter := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(dir, "p.lua")})
	nodes := &fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: srv.URL},
	}}
	adapter.AttachCluster(nodes, srv.Client())

	_, _, err := adapter.ApplyConfig(context.Background(), clusterRendered(), false)
	if err == nil {
		t.Fatal("expected rollout failure")
	}
	if !strings.Contains(err.Error(), "node2") || !strings.Contains(err.Error(), "rollout halted") {
		t.Fatalf("error should name failing node and halt: %v", err)
	}
	if _, ok := nodes.heartbeats["n2"]; ok {
		t.Fatalf("failed node must not record an applied checksum")
	}
}

// TestApplyConfigRemoteNodeWithoutTLSClient verifies remote nodes are refused
// when no cluster mTLS client is configured.
func TestApplyConfigRemoteNodeWithoutTLSClient(t *testing.T) {
	adapter := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(t.TempDir(), "p.lua")})
	adapter.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: "https://10.0.0.9:8447"},
	}}, nil)

	_, _, err := adapter.ApplyConfig(context.Background(), clusterRendered(), false)
	if err == nil || !strings.Contains(err.Error(), "cluster TLS") {
		t.Fatalf("expected CLUSTER_TLS_UNCONFIGURED, got %v", err)
	}
}
