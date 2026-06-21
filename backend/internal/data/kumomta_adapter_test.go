package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// TestFileKumoMTAApplyConfigWritesPolicy verifies the file adapter writes the
// rendered policy atomically to the configured path. With no reload command or
// URL configured, the reload step is a no-op and apply succeeds.
func TestFileKumoMTAApplyConfigWritesPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy", "iris_generated.lua")
	adapter := NewFileKumoMTA(conf.External{ConfigPath: path})

	rendered := biz.RenderedConfig{
		Content:   "-- test policy\nreturn {}\n",
		Checksum:  "abc",
		VMTACount: 2, PoolCount: 1, RouteCount: 1,
	}
	got, summary, err := adapter.ApplyConfig(context.Background(), rendered, false)
	if err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	if got != path {
		t.Fatalf("expected applied path %q, got %q", path, got)
	}
	if summary == "" {
		t.Fatal("expected a result summary")
	}

	// An init-change apply with no restart mechanism configured falls back to a
	// reload but flags that a manual restart is required.
	_, restartSummary, err := adapter.ApplyConfig(context.Background(), rendered, true)
	if err != nil {
		t.Fatalf("ApplyConfig (restart): %v", err)
	}
	if !strings.Contains(restartSummary, "MANUAL RESTART REQUIRED") {
		t.Fatalf("expected a manual-restart warning, got %q", restartSummary)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written policy: %v", err)
	}
	if string(data) != rendered.Content {
		t.Fatalf("written policy mismatch:\n%s", string(data))
	}
	// The temp file must not linger after the atomic rename.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file should not remain after apply")
	}
}

// TestFileKumoMTAApplyConfigRequiresPath verifies a missing config path is a
// failed-precondition rather than a panic.
func TestFileKumoMTAApplyConfigRequiresPath(t *testing.T) {
	adapter := NewFileKumoMTA(conf.External{})
	_, _, err := adapter.ApplyConfig(context.Background(), biz.RenderedConfig{}, false)
	if de, ok := biz.AsDomainError(err); !ok || de.Reason != "KUMO_CONFIG_PATH_UNSET" {
		t.Fatalf("expected KUMO_CONFIG_PATH_UNSET, got %v", err)
	}
}
