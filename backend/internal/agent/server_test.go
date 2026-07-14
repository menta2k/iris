package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/menta2k/iris/backend/internal/agentapi"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// fakeApplier records ApplyConfig calls.
type fakeApplier struct {
	applied  []biz.RenderedConfig
	restarts []bool
	reloads  int
	failNext bool
}

func (f *fakeApplier) ApplyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool) (string, string, error) {
	if f.failNext {
		f.failNext = false
		return "", "", biz.Unavailable("KUMO_RELOAD_FAILED", "boom")
	}
	f.applied = append(f.applied, rendered)
	f.restarts = append(f.restarts, restart)
	return "/etc/policy.lua", "applied", nil
}

func (f *fakeApplier) ApplyServiceControl(ctx context.Context, op biz.ServiceOperation) (string, error) {
	f.reloads++
	return "reload triggered", nil
}

func (f *fakeApplier) Status(ctx context.Context) (biz.KumoStatus, error) {
	return biz.KumoStatus{State: "running"}, nil
}

func newTestServer(t *testing.T) (*Server, *fakeApplier) {
	t.Helper()
	applier := &fakeApplier{}
	srv, err := New(conf.Agent{StatePath: filepath.Join(t.TempDir(), "state.json")},
		applier, "", "test/1", slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv, applier
}

func postJSON(t *testing.T, h http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func testBundle(generation int64) agentapi.ConfigBundle {
	policy := "-- policy\n"
	base := "[base]\n"
	return agentapi.ConfigBundle{
		Generation:   generation,
		Policy:       agentapi.File{Name: "iris_generated.lua", Content: policy, SHA256: sha256Hex(policy)},
		Shaping:      []agentapi.File{{Name: "iris-base.toml", Content: base, SHA256: sha256Hex(base)}},
		Checksum:     "sum-1",
		InitChecksum: "init-1",
	}
}

func TestAgentStageActivateFlow(t *testing.T) {
	srv, applier := newTestServer(t)
	h := srv.Handler()

	if w := postJSON(t, h, agentapi.PathStage, testBundle(10)); w.Code != http.StatusOK {
		t.Fatalf("stage = %d: %s", w.Code, w.Body)
	}
	w := postJSON(t, h, agentapi.PathActivate, agentapi.ActivateRequest{Checksum: "sum-1", Restart: true})
	if w.Code != http.StatusOK {
		t.Fatalf("activate = %d: %s", w.Code, w.Body)
	}
	var reply agentapi.ActivateReply
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if reply.Action != "restarted" || reply.AppliedChecksum != "sum-1" {
		t.Fatalf("activate reply = %+v", reply)
	}
	if len(applier.applied) != 1 || !applier.restarts[0] {
		t.Fatalf("applier calls = %+v restarts=%v", applier.applied, applier.restarts)
	}
	if applier.applied[0].ShapingBase != "[base]\n" || applier.applied[0].Content != "-- policy\n" {
		t.Fatalf("rendered config mismatch: %+v", applier.applied[0])
	}

	// Health reflects the applied state.
	req := httptest.NewRequest(http.MethodGet, agentapi.PathHealth, nil)
	hw := httptest.NewRecorder()
	h.ServeHTTP(hw, req)
	var health agentapi.Health
	_ = json.Unmarshal(hw.Body.Bytes(), &health)
	if health.AppliedChecksum != "sum-1" || health.Generation != 10 || health.Kumo != "running" {
		t.Fatalf("health = %+v", health)
	}
}

func TestAgentRejectsChecksumMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	b := testBundle(10)
	b.Policy.SHA256 = "deadbeef"
	if w := postJSON(t, srv.Handler(), agentapi.PathStage, b); w.Code != http.StatusBadRequest {
		t.Fatalf("stage with bad sha = %d", w.Code)
	}
}

func TestAgentRejectsReplay(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()
	postJSON(t, h, agentapi.PathStage, testBundle(10))
	postJSON(t, h, agentapi.PathActivate, agentapi.ActivateRequest{Checksum: "sum-1"})

	// Same (or older) generation must be refused after activation.
	if w := postJSON(t, h, agentapi.PathStage, testBundle(10)); w.Code != http.StatusConflict {
		t.Fatalf("replayed stage = %d", w.Code)
	}
	if w := postJSON(t, h, agentapi.PathStage, testBundle(11)); w.Code != http.StatusOK {
		t.Fatalf("newer stage = %d", w.Code)
	}
}

func TestAgentActivateRequiresStagedChecksum(t *testing.T) {
	srv, _ := newTestServer(t)
	w := postJSON(t, srv.Handler(), agentapi.PathActivate, agentapi.ActivateRequest{Checksum: "nope"})
	if w.Code != http.StatusConflict {
		t.Fatalf("activate unstaged = %d", w.Code)
	}
}

func TestAgentBareReload(t *testing.T) {
	srv, applier := newTestServer(t)
	w := postJSON(t, srv.Handler(), agentapi.PathActivate, agentapi.ActivateRequest{})
	if w.Code != http.StatusOK {
		t.Fatalf("bare reload = %d: %s", w.Code, w.Body)
	}
	if applier.reloads != 1 {
		t.Fatalf("reloads = %d", applier.reloads)
	}
}

func TestAgentStatePersistsAcrossRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	applier := &fakeApplier{}
	srv, err := New(conf.Agent{StatePath: statePath}, applier, "", "test/1", slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := srv.Handler()
	postJSON(t, h, agentapi.PathStage, testBundle(42))
	postJSON(t, h, agentapi.PathActivate, agentapi.ActivateRequest{Checksum: "sum-1"})

	reborn, err := New(conf.Agent{StatePath: statePath}, applier, "", "test/1", slog.Default())
	if err != nil {
		t.Fatalf("New (restart): %v", err)
	}
	// Replay protection must survive the restart.
	if w := postJSON(t, reborn.Handler(), agentapi.PathStage, testBundle(42)); w.Code != http.StatusConflict {
		t.Fatalf("stage after restart = %d", w.Code)
	}
}
