package data

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

func TestSuppressionKey(t *testing.T) {
	cases := []struct {
		typ, value, want string
	}{
		// The key format MUST match what the rendered policy builds
		// ('supp:e:' .. recipient / 'supp:d:' .. domain), and values are normalized.
		{biz.SuppressEmail, "Blocked@Example.com", "supp:e:blocked@example.com"},
		{biz.SuppressDomain, " Blocked.Example ", "supp:d:blocked.example"},
		{"unknown", "x@y.com", "supp:e:x@y.com"}, // defaults to the email namespace
	}
	for _, tc := range cases {
		if got := SuppressionKey(tc.typ, tc.value); got != tc.want {
			t.Errorf("SuppressionKey(%q, %q) = %q, want %q", tc.typ, tc.value, got, tc.want)
		}
	}
}

func TestSuppressionCacheNilSafe(t *testing.T) {
	// A nil/Redis-less cache must be a no-op, not a panic (DB-only deployments).
	ctx := context.Background()
	var c *SuppressionCache
	if err := c.Put(ctx, biz.SuppressEmail, "a@b.com", 0); err != nil {
		t.Fatalf("nil Put: %v", err)
	}
	if err := c.Del(ctx, biz.SuppressEmail, "a@b.com"); err != nil {
		t.Fatalf("nil Del: %v", err)
	}
	c2 := NewSuppressionCache(nil)
	if n, err := c2.Backfill(ctx, []*biz.SuppressionEntry{{Type: "email", Value: "a@b.com", Status: biz.SuppressActive}}, time.Time{}); err != nil || n != 0 {
		t.Fatalf("nil-client Backfill: n=%d err=%v", n, err)
	}
}
