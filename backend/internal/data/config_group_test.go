package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChgrpConfig(t *testing.T) {
	f := filepath.Join(t.TempDir(), "policy.lua")
	if err := os.WriteFile(f, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Empty group is a no-op.
	if err := chgrpConfig(f, ""); err != nil {
		t.Fatalf("empty group should be a no-op, got %v", err)
	}
	if err := chgrpConfig(f, "   "); err != nil {
		t.Fatalf("blank group should be a no-op, got %v", err)
	}
	// An unknown group is a clear, surfaced error (not a silent skip).
	if err := chgrpConfig(f, "definitely-not-a-real-group-xyz"); err == nil {
		t.Fatal("unknown group should error")
	}
}
