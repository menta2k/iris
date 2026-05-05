package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/pkg/kumopolicy"
)

// SnapshotProvider returns the current configuration snapshot from the
// database. Implemented by the data layer.
type SnapshotProvider interface {
	CurrentSnapshot(ctx context.Context) (*kumopolicy.Snapshot, error)
}

// PolicyHistoryWriter persists Apply outcomes.
type PolicyHistoryWriter interface {
	Append(ctx context.Context, sha256Hex, note, luaSource string, actorUserID uint32) error
	List(ctx context.Context, limit int) ([]PolicyHistoryRow, error)
}

// PolicyHistoryRow is the data-layer view.
type PolicyHistoryRow struct {
	ID          uint64
	SHA256      string
	Note        string
	ActorUserID uint32
	AppliedAt   time.Time
}

// KumoReloader signals the kumomta daemon to reload after a successful Apply.
type KumoReloader interface {
	Reload(ctx context.Context) error
}

// PolicyService renders, validates, applies, and lists historical policies.
type PolicyService struct {
	provider   SnapshotProvider
	history    PolicyHistoryWriter
	reloader   KumoReloader
	policyDir  string
	policyName string // e.g., "init.lua"
	now        func() time.Time

	// metrics is optional; when set, every Apply increments
	// iris_policy_apply_total{result=ok|error}. Wired via SetMetrics
	// rather than the constructor so tests don't need to know about
	// Prometheus, and so the PolicyService API doesn't change for
	// existing callers.
	metrics PolicyMetrics
}

// PolicyMetrics is the small slice of *metrics.Metrics that
// PolicyService actually touches. Defining it here keeps the service
// package free of a hard dep on pkg/metrics and lets tests stub it.
type PolicyMetrics interface {
	RecordApply(result string)
}

// SetMetrics wires the metrics sink. Call once at boot.
func (s *PolicyService) SetMetrics(m PolicyMetrics) { s.metrics = m }

// NewPolicyService constructs the service. policyDir must be an absolute
// path on the same filesystem as the kumomta process.
func NewPolicyService(p SnapshotProvider, h PolicyHistoryWriter, r KumoReloader, policyDir string) (*PolicyService, error) {
	if !filepath.IsAbs(policyDir) {
		return nil, fmt.Errorf("policy: policy_dir must be absolute, got %q", policyDir)
	}
	// Refuse if the input is not in canonical form. This catches ".."
	// segments because Clean rewrites them, breaking equality. It also
	// catches trailing slashes and double slashes which can confuse
	// downstream tools.
	if filepath.Clean(policyDir) != policyDir || strings.Contains(policyDir, "..") {
		return nil, errors.New("policy: policy_dir must be a canonical absolute path with no '..'")
	}
	clean := policyDir
	return &PolicyService{
		provider:   p,
		history:    h,
		reloader:   r,
		policyDir:  clean,
		policyName: "init.lua",
		now:        time.Now,
	}, nil
}

// Render produces the Lua source for the current snapshot.
func (s *PolicyService) Render(ctx context.Context, dryRun bool, actorUsername string) (lua, sha256Hex string, err error) {
	snap, err := s.provider.CurrentSnapshot(ctx)
	if err != nil {
		return "", "", fmt.Errorf("policy: load snapshot: %w", err)
	}
	out, err := kumopolicy.Render(snap, kumopolicy.RenderOptions{
		DryRun:      dryRun,
		GeneratedAt: s.now().UTC(),
		GeneratedBy: actorUsername,
	})
	if err != nil {
		return "", "", err
	}
	return out.Lua, out.SHA256, nil
}

// Validate checks the current snapshot and Lua-lints the rendered output.
// Returns issues as a slice of human-readable strings.
func (s *PolicyService) Validate(ctx context.Context) ([]string, error) {
	snap, err := s.provider.CurrentSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy: load snapshot: %w", err)
	}
	if err := snap.Validate(); err != nil {
		var v *kumopolicy.ValidationError
		if errors.As(err, &v) {
			return v.Issues, nil
		}
		return []string{err.Error()}, nil
	}
	out, err := kumopolicy.Render(snap, kumopolicy.RenderOptions{GeneratedAt: s.now().UTC()})
	if err != nil {
		return []string{err.Error()}, nil
	}
	return kumopolicy.Lint(out.Lua), nil
}

// Apply renders, writes atomically to disk, and signals reload. Recorded in
// PolicyHistory. Returns the SHA-256 of the applied Lua.
func (s *PolicyService) Apply(ctx context.Context, note string, actorUserID uint32, actorUsername string) (sha256Hex string, at time.Time, retErr error) {
	// Single deferred metric tick: the function has multiple early
	// returns and we want every one tagged. retErr is the named
	// return so the deferred closure sees the value the function
	// will actually return, not whatever was set up to that point.
	defer func() {
		if s.metrics == nil {
			return
		}
		if retErr != nil {
			s.metrics.RecordApply("error")
			return
		}
		s.metrics.RecordApply("ok")
	}()

	snap, err := s.provider.CurrentSnapshot(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("policy: load snapshot: %w", err)
	}
	out, err := kumopolicy.Render(snap, kumopolicy.RenderOptions{
		GeneratedAt: s.now().UTC(),
		GeneratedBy: actorUsername,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.atomicWrite(out.Lua); err != nil {
		return "", time.Time{}, fmt.Errorf("policy: write: %w", err)
	}
	// Reload is best-effort. Modern kumomta builds drop the
	// /api/admin/reload[-config]/v1 endpoints in favor of epoch-based
	// polling — the daemon hashes init.lua every 10s and reloads on change.
	// So an explicit reload failure is just an optimization miss, not a
	// real apply failure: the on-disk write is the source of truth and
	// kumomta will pick it up momentarily. We log and continue.
	if s.reloader != nil {
		if err := s.reloader.Reload(ctx); err != nil {
			log.Printf("policy: kumomta reload signal failed (epoch poll will pick it up within ~10s): %v", err)
		}
	}
	at = s.now().UTC()
	if err := s.history.Append(ctx, out.SHA256, note, out.Lua, actorUserID); err != nil {
		return "", time.Time{}, fmt.Errorf("policy: record history: %w", err)
	}
	return out.SHA256, at, nil
}

// History returns the most recent N apply records.
func (s *PolicyService) History(ctx context.Context, limit int) ([]PolicyHistoryRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return s.history.List(ctx, limit)
}

// Active returns the policy file currently on disk — the one kumomta is
// (or will shortly be) running. This is distinct from Render() which
// produces a *preview* from the current DB snapshot: between the last
// Apply and any subsequent DB edits, the two diverge. The Policy editor
// in the UI loads this so operators see what's actually in effect, not
// a hypothetical re-render of the latest config.
//
// Returns an empty string + zero SHA when no init.lua exists yet
// (fresh deploy that hasn't applied once); the handler surfaces this as
// a 200 with empty body so the editor can render its "no policy
// applied yet" placeholder.
func (s *PolicyService) Active() (lua, sha256Hex string, err error) {
	path := filepath.Join(s.policyDir, s.policyName)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("policy: read active: %w", err)
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]), nil
}

// atomicWrite is a write-then-rename so a partially-written policy never
// shows up to kumomta. We use the same directory so rename is atomic.
func (s *PolicyService) atomicWrite(content string) error {
	dest := filepath.Join(s.policyDir, s.policyName)
	tmp, err := os.CreateTemp(s.policyDir, ".init.lua.tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	// 0o644: world-readable. The kumomta container runs as a different UID
	// (and the docker bind-mount doesn't share group memberships), so 0640
	// would block reads. The directory is private to the deployment so
	// world-readable is acceptable; the keys in dkim/ stay 0600.
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

