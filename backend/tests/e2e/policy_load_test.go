//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// TestGeneratedPolicyLoadsInKumod is the L1 gate: it renders the full iris
// policy surface and loads it into a real kumod via `kumod --validate`. This
// catches every divergence between the gopher-lua lint (which only parses) and
// a real kumod (which actually evaluates require 'redis', kumo.dkim,
// kumo.make_listener_domain, custom_lua queue constructors, etc.).
//
// A clean lint with a failing load here is exactly the signal this test exists
// to surface.
func TestGeneratedPolicyLoadsInKumod(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	snap := representativeSnapshot()
	r, err := biz.RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Sanity: it must at least pass our own lint before we ask kumod.
	if !r.Valid {
		t.Fatalf("rendered policy failed gopher-lua lint: %v\n%s", r.LintIssues, r.Content)
	}

	// Write the policy where the container can read it. MkdirTemp is 0700, which
	// the in-container kumod user cannot traverse, so widen the dir and file.
	dir, err := os.MkdirTemp("", "iris-e2e-policy-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	policyPath := filepath.Join(dir, "iris_generated.lua")
	if err := os.WriteFile(policyPath, []byte(r.Content), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	// `docker run` auto-pulls the image on first use, so allow generous time.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{
		"run", "--rm",
		"-v", dir + ":/policy:ro",
		kumoImage(),
		"kumod", "--validate", "--policy", "/policy/iris_generated.lua", "--user", "kumod",
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	t.Logf("kumod --validate output:\n%s", out)

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("kumod validate timed out (image pull or hang)")
	}
	if err != nil {
		t.Fatalf("kumod rejected the generated policy: %v\n--- policy ---\n%s", err, numbered(r.Content))
	}

	// kumod exited 0: the policy loaded. Guard against a validate build that
	// prints errors but still exits 0 (defensive — surfaces obvious failures).
	if lower := strings.ToLower(string(out)); strings.Contains(lower, "error") && !strings.Contains(lower, "0 error") {
		t.Fatalf("kumod validate reported an error despite exit 0:\n%s", out)
	}
}

// numbered prefixes each line with its number to make kumod's line:col errors
// easy to map back onto the generated policy.
func numbered(s string) string {
	var b strings.Builder
	for i, line := range strings.Split(s, "\n") {
		b.WriteString(itoaPad(i+1, 4))
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func itoaPad(n, width int) string {
	s := []byte{}
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	if len(s) == 0 {
		s = []byte{'0'}
	}
	for len(s) < width {
		s = append([]byte{' '}, s...)
	}
	return string(s)
}
