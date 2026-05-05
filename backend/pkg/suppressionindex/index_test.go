package suppressionindex

import (
	"context"
	"testing"
)

func TestNoopIsNoOp(t *testing.T) {
	n := NewNoop()
	ctx := context.Background()
	if err := n.Add(ctx, ScopeAddress, "x@y.z"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := n.Remove(ctx, ScopeDomain, "y.z"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := n.Resync(ctx, []Entry{{Scope: ScopeAddress, Value: "a@b.c"}}); err != nil {
		t.Fatalf("Resync: %v", err)
	}
	if err := n.Healthy(ctx); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
}

func TestKeyForScopeRejectsUnknown(t *testing.T) {
	if k := keyForScope("garbage"); k != "" {
		t.Fatalf("expected empty key for unknown scope, got %q", k)
	}
	if k := keyForScope(ScopeAddress); k != KeyAddrSet {
		t.Fatalf("address scope: %q", k)
	}
	if k := keyForScope(ScopeDomain); k != KeyDomSet {
		t.Fatalf("domain scope: %q", k)
	}
}

func TestNewRedisRejectsEmptyURL(t *testing.T) {
	if _, err := NewRedis(""); err == nil {
		t.Fatal("expected error for empty url")
	}
	if _, err := NewRedis("   "); err == nil {
		t.Fatal("expected error for whitespace url")
	}
}
