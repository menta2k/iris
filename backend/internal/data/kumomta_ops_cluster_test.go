package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/internal/agentapi"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// fakeKumod emulates the kumod admin surface used by iris: /metrics,
// /api/admin/suspend/v1 (list/create/delete), /api/admin/bounce/v1, and
// /api/inject/v1.
type fakeKumod struct {
	name         string
	metrics      string
	suspensions  []map[string]any
	bounced      []string
	injected     int
	rejectInject bool
}

func (f *fakeKumod) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(f.metrics))
	})
	mux.HandleFunc("GET /api/admin/suspend/v1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(f.suspensions)
	})
	mux.HandleFunc("POST /api/admin/suspend/v1", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.suspensions = append(f.suspensions, map[string]any{
			"id": f.name + "-susp-1", "domain": req["domain"], "reason": req["reason"],
		})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("DELETE /api/admin/suspend/v1/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/admin/suspend/v1/")
		kept := f.suspensions[:0]
		for _, s := range f.suspensions {
			if s["id"] != id {
				kept = append(kept, s)
			}
		}
		f.suspensions = kept
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/admin/bounce/v1", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.bounced = append(f.bounced, req["domain"].(string))
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/inject/v1", func(w http.ResponseWriter, r *http.Request) {
		if f.rejectInject {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		f.injected++
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// agentProxyFor wraps a fakeKumod behind the agent's /v1/kumod prefix, the way
// a real agent reverse-proxies its localhost kumod.
func agentProxyFor(k *fakeKumod) http.Handler {
	inner := k.handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, agentapi.PathKumodPrefix) {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + strings.TrimPrefix(r.URL.Path, agentapi.PathKumodPrefix)
		inner.ServeHTTP(w, r2)
	})
}

// twoNodeAdapter builds a FileKumoMTA managing a local fake kumod plus one
// remote fake kumod behind an agent proxy.
func twoNodeAdapter(t *testing.T, local, remote *fakeKumod, remoteStatus string) (*FileKumoMTA, func()) {
	t.Helper()
	localSrv := httptest.NewServer(local.handler())
	remoteSrv := httptest.NewServer(agentProxyFor(remote))
	adapter := NewFileKumoMTA(conf.External{
		ConfigPath: filepath.Join(t.TempDir(), "p.lua"),
		BaseURL:    localSrv.URL,
	})
	adapter.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: remoteStatus, AgentURL: remoteSrv.URL},
	}}, remoteSrv.Client())
	return adapter, func() { localSrv.Close(); remoteSrv.Close() }
}

func TestQueueSummaryAggregatesAcrossNodes(t *testing.T) {
	local := &fakeKumod{name: "node1", metrics: "scheduled_by_domain{domain=\"gmail.com\"} 5\nscheduled_by_domain{domain=\"yahoo.com\"} 2\n"}
	remote := &fakeKumod{
		name:    "node2",
		metrics: "scheduled_by_domain{domain=\"gmail.com\"} 7\n",
		suspensions: []map[string]any{
			{"id": "node2-s1", "domain": "gmail.com", "reason": "backing off"},
		},
	}
	adapter, cleanup := twoNodeAdapter(t, local, remote, biz.MTANodeStatusActive)
	defer cleanup()

	states, err := adapter.QueueSummary(context.Background())
	if err != nil {
		t.Fatalf("QueueSummary: %v", err)
	}
	byDomain := map[string]*biz.QueueState{}
	for _, s := range states {
		byDomain[s.Domain] = s
	}
	if got := byDomain["gmail.com"]; got == nil || got.Depth != 12 || !got.Suspended {
		t.Fatalf("gmail.com state = %+v, want depth 12 suspended", got)
	}
	if got := byDomain["yahoo.com"]; got == nil || got.Depth != 2 || got.Suspended {
		t.Fatalf("yahoo.com state = %+v, want depth 2 not suspended", got)
	}
}

func TestSuspendQueueFansOutAndReportsPartialFailure(t *testing.T) {
	local := &fakeKumod{name: "node1"}
	remote := &fakeKumod{name: "node2"}
	adapter, cleanup := twoNodeAdapter(t, local, remote, biz.MTANodeStatusActive)
	defer cleanup()

	summary, err := adapter.SuspendQueue(context.Background(), "gmail.com", "test")
	if err != nil {
		t.Fatalf("SuspendQueue: %v", err)
	}
	if !strings.Contains(summary, "node1") || !strings.Contains(summary, "node2") {
		t.Fatalf("summary should name both nodes: %q", summary)
	}
	if len(local.suspensions) != 1 || len(remote.suspensions) != 1 {
		t.Fatalf("suspensions: local=%d remote=%d", len(local.suspensions), len(remote.suspensions))
	}

	// Resume clears the per-node suspension ids on both nodes.
	if _, err := adapter.ResumeQueue(context.Background(), "gmail.com"); err != nil {
		t.Fatalf("ResumeQueue: %v", err)
	}
	if len(local.suspensions) != 0 || len(remote.suspensions) != 0 {
		t.Fatalf("resume left suspensions: local=%d remote=%d", len(local.suspensions), len(remote.suspensions))
	}

	// Partial failure: kill the remote node and suspend again.
	cleanup() // closes both; recreate only the local
	localSrv := httptest.NewServer(local.handler())
	defer localSrv.Close()
	adapter2 := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(t.TempDir(), "p.lua"), BaseURL: localSrv.URL})
	adapter2.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: "https://127.0.0.1:1"},
	}}, &http.Client{})
	_, err = adapter2.SuspendQueue(context.Background(), "gmail.com", "test")
	if err == nil || !strings.Contains(err.Error(), "FAILED on node2") {
		t.Fatalf("partial failure must name the failed node, got %v", err)
	}
}

func TestInjectRoundRobinFailsOverUnreachableNode(t *testing.T) {
	local := &fakeKumod{name: "node1"}
	localSrv := httptest.NewServer(local.handler())
	defer localSrv.Close()

	adapter := NewFileKumoMTA(conf.External{ConfigPath: filepath.Join(t.TempDir(), "p.lua"), BaseURL: localSrv.URL})
	adapter.AttachCluster(&fakeClusterNodes{nodes: []*biz.MTANode{
		{ID: "n1", Name: "node1", Status: biz.MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: biz.MTANodeStatusActive, AgentURL: "https://127.0.0.1:1"}, // dead
	}}, &http.Client{})

	// Inject several times: every attempt must land on node1 (the only
	// reachable node), regardless of the round-robin start position.
	for i := range 4 {
		if err := adapter.InjectV1(context.Background(), biz.KumoInjectRequest{}); err != nil {
			t.Fatalf("InjectV1 #%d: %v", i, err)
		}
	}
	if local.injected != 4 {
		t.Fatalf("local injections = %d, want 4", local.injected)
	}
}

func TestInjectSkipsDrainingNodeAndTreatsRejectionAsFinal(t *testing.T) {
	local := &fakeKumod{name: "node1", rejectInject: true}
	remote := &fakeKumod{name: "node2"}
	adapter, cleanup := twoNodeAdapter(t, local, remote, biz.MTANodeStatusDraining)
	defer cleanup()

	// node2 is draining -> excluded; node1 rejects -> authoritative failure,
	// no fail-over, message is not silently retried elsewhere.
	err := adapter.InjectV1(context.Background(), biz.KumoInjectRequest{})
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("expected authoritative rejection, got %v", err)
	}
	if remote.injected != 0 {
		t.Fatalf("draining node must not receive injections")
	}
}
