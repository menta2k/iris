package data

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

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
	if n, err := c.Clear(ctx); err != nil || n != 0 {
		t.Fatalf("nil Clear: n=%d err=%v", n, err)
	}
}

func TestSuppressionCacheClear(t *testing.T) {
	addr := os.Getenv("IRIS_TEST_REDIS")
	if addr == "" {
		t.Skip("IRIS_TEST_REDIS not set")
	}
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()

	// Seed suppression keys plus an unrelated shared key that must survive.
	c := NewSuppressionCache(rdb)
	if err := c.Put(ctx, biz.SuppressEmail, "x@example.com", 0); err != nil {
		t.Fatalf("put email: %v", err)
	}
	if err := c.Put(ctx, biz.SuppressDomain, "blocked.example", 0); err != nil {
		t.Fatalf("put domain: %v", err)
	}
	const shared = "iris:test:logstream:keep"
	if err := rdb.Set(ctx, shared, "1", time.Minute).Err(); err != nil {
		t.Fatalf("set shared: %v", err)
	}
	defer rdb.Del(ctx, shared)

	n, err := c.Clear(ctx)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if n < 2 {
		t.Fatalf("expected >=2 keys cleared, got %d", n)
	}
	if v, _ := rdb.Exists(ctx, SuppressionKey(biz.SuppressEmail, "x@example.com")).Result(); v != 0 {
		t.Fatal("email suppression key should be gone")
	}
	if v, _ := rdb.Exists(ctx, SuppressionKey(biz.SuppressDomain, "blocked.example")).Result(); v != 0 {
		t.Fatal("domain suppression key should be gone")
	}
	if v, _ := rdb.Exists(ctx, shared).Result(); v != 1 {
		t.Fatal("Clear must not delete non-suppression keys")
	}
}
