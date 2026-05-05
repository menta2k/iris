package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/menta2k/iris/backend/pkg/kumopolicy"
)

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type fakeProvider struct{ snap *kumopolicy.Snapshot; err error }

func (f *fakeProvider) CurrentSnapshot(ctx context.Context) (*kumopolicy.Snapshot, error) {
	return f.snap, f.err
}

type fakeHistory struct {
	mu    sync.Mutex
	calls int
	rows  []PolicyHistoryRow
}

func (f *fakeHistory) Append(ctx context.Context, sha, note, lua string, uid uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.rows = append(f.rows, PolicyHistoryRow{ID: uint64(f.calls), SHA256: sha, Note: note, ActorUserID: uid, AppliedAt: time.Now()})
	return nil
}
func (f *fakeHistory) List(ctx context.Context, limit int) ([]PolicyHistoryRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]PolicyHistoryRow(nil), f.rows...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeReloader struct{ called bool; err error }

func (f *fakeReloader) Reload(ctx context.Context) error { f.called = true; return f.err }

func goodSnap() *kumopolicy.Snapshot {
	return &kumopolicy.Snapshot{
		GlobalSettings: kumopolicy.GlobalSettings{
			LogDir: "/var/log/kumo", SpoolDir: "/var/spool/kumo",
		},
		Listeners: []kumopolicy.Listener{{
			Name: "mx", ListenAddr: "0.0.0.0:25", Hostname: "mx.example.com",
		}},
	}
}

func TestPolicyServiceRejectsRelativeDir(t *testing.T) {
	_, err := NewPolicyService(nil, nil, nil, "relative")
	require.Error(t, err)
}

func TestPolicyServiceRejectsTraversal(t *testing.T) {
	_, err := NewPolicyService(nil, nil, nil, "/foo/../etc")
	require.Error(t, err)
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewPolicyService(&fakeProvider{snap: goodSnap()}, &fakeHistory{}, &fakeReloader{}, dir)
	require.NoError(t, err)
	lua, sum, err := svc.Render(context.Background(), false, "alice")
	require.NoError(t, err)
	require.NotEmpty(t, lua)
	require.Len(t, sum, 64)
}

func TestValidateClean(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewPolicyService(&fakeProvider{snap: goodSnap()}, &fakeHistory{}, &fakeReloader{}, dir)
	require.NoError(t, err)
	issues, err := svc.Validate(context.Background())
	require.NoError(t, err)
	require.Empty(t, issues)
}

func TestValidateReturnsIssues(t *testing.T) {
	bad := goodSnap()
	bad.Listeners[0].Hostname = "not a hostname"
	dir := t.TempDir()
	svc, err := NewPolicyService(&fakeProvider{snap: bad}, &fakeHistory{}, &fakeReloader{}, dir)
	require.NoError(t, err)
	issues, err := svc.Validate(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, issues)
}

func TestApplyAtomicWriteAndReload(t *testing.T) {
	dir := t.TempDir()
	hist := &fakeHistory{}
	rl := &fakeReloader{}
	svc, err := NewPolicyService(&fakeProvider{snap: goodSnap()}, hist, rl, dir)
	require.NoError(t, err)
	sha, at, err := svc.Apply(context.Background(), "first deploy", 1, "alice")
	require.NoError(t, err)
	require.Len(t, sha, 64)
	require.False(t, at.IsZero())
	require.True(t, rl.called)
	require.Equal(t, 1, hist.calls)

	// File on disk should match the rendered SHA.
	body, err := os.ReadFile(filepath.Join(dir, "init.lua"))
	require.NoError(t, err)
	require.Equal(t, sha, sha256Of(string(body)))
	// No tmp files left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".tmp.", "tmp file leaked: %s", e.Name())
	}
}

func TestApplyReloadFailureIsBestEffort(t *testing.T) {
	// Modern kumomta builds drop the explicit /api/admin/reload endpoints
	// in favor of epoch-based polling — so a reload-call error is just
	// "the optimization fired blank, but the on-disk write will be picked
	// up within ~10s." Apply must therefore SUCCEED even when the reloader
	// errors, and the history row must still be appended so operators have
	// a record of the change.
	dir := t.TempDir()
	hist := &fakeHistory{}
	svc, err := NewPolicyService(&fakeProvider{snap: goodSnap()}, hist, &fakeReloader{err: errors.New("404 not found")}, dir)
	require.NoError(t, err)
	_, _, err = svc.Apply(context.Background(), "x", 1, "alice")
	require.NoError(t, err)
	require.Equal(t, 1, hist.calls)
}
