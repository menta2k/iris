// Package suppressionindex maintains a fast-lookup index of suppression
// entries that lives outside the relational source of truth. The index is
// what kumomta consults at smtp_server_message_received time; PostgreSQL
// stays canonical (audit, history, UI lists), the index just absorbs the
// hot-path lookup load.
//
// Two backends ship today:
//
//   - Redis: production. Two SETs (kumo:supp:addr and kumo:supp:dom) plus
//     a metadata HASH that tracks the last full-resync timestamp. SADD /
//     SREM mutate; SISMEMBER reads.
//
//   - Noop: tests, dev, and any deployment running without Redis. Always
//     returns "not suppressed" — kumomta's Lua falls back to a fail-open
//     path when the index is unreachable, and Noop emulates that here so
//     the unit tests don't need a Redis container.
//
// The interface intentionally never returns an error from mutate methods:
// the suppression index is a *cache* of PG, and a stale entry is benign
// (the next full resync repairs it). Callers that need stronger
// guarantees (audit, list-display) consult PG directly.
package suppressionindex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/pkg/metrics"
)

// Scope mirrors the SuppressionRow.Scope values. Defined here as untyped
// constants to keep this package free of any service-layer import.
const (
	ScopeAddress = "address"
	ScopeDomain  = "domain"
)

// Redis key layout. Public so cmd/resync-suppressions and ops scripts can
// reference the same names without re-declaring strings.
const (
	KeyAddrSet = "kumo:supp:addr"
	KeyDomSet  = "kumo:supp:dom"
	KeyMetaHash = "kumo:supp:meta"

	MetaFieldResyncedAt = "resynced_at"
	MetaFieldEntryCount = "entry_count"
)

// Index is the cross-backend contract. All methods are best-effort: an
// error from Add/Remove means "the index isn't authoritative right now,
// the next Resync will repair it" — never propagated to the caller.
type Index interface {
	// Add inserts (scope, value). Lower-cased on the way in.
	Add(ctx context.Context, scope, value string) error

	// Remove deletes (scope, value). No-op if absent.
	Remove(ctx context.Context, scope, value string) error

	// Resync swaps the index atomically to exactly the supplied entries.
	// Used at admin-service startup and from cmd/resync-suppressions.
	Resync(ctx context.Context, entries []Entry) error

	// Healthy returns nil iff the backend is reachable and writable.
	Healthy(ctx context.Context) error
}

// Entry is one suppression row reduced to what the index cares about.
type Entry struct {
	Scope string // "address" | "domain"
	Value string // the address or domain, lower-cased
}

// ----------------------------------------------------------------------------
// Noop
// ----------------------------------------------------------------------------

// Noop is the no-op backend. Constructed when the deployment doesn't have
// Redis wired up (dev, single-node, tests). Every method is a successful
// no-op; kumomta's Lua handler then defaults to fail-open behavior.
type Noop struct{}

// NewNoop returns a Noop index.
func NewNoop() *Noop { return &Noop{} }

// Add is a no-op.
func (Noop) Add(context.Context, string, string) error { return nil }

// Remove is a no-op.
func (Noop) Remove(context.Context, string, string) error { return nil }

// Resync is a no-op.
func (Noop) Resync(context.Context, []Entry) error { return nil }

// Healthy always returns nil.
func (Noop) Healthy(context.Context) error { return nil }

// ----------------------------------------------------------------------------
// Redis
// ----------------------------------------------------------------------------

// RedisIndex is the production backend. Single client, no per-call
// connection setup; go-redis pools internally.
//
// `m` is optional; when set, every Add/Remove/Resync increments the
// matching counter and Resync also refreshes the suppression-entries
// gauge from the SCARD result. nil = no instrumentation, used in
// tests and Noop-equivalent paths.
type RedisIndex struct {
	rdb *redis.Client
	m   *metrics.Metrics
}

// NewRedis builds a RedisIndex from a Redis URL (e.g. redis://host:6379/0).
// The URL form matches what the log-stream consumer already accepts so a
// single env var configures both call sites.
func NewRedis(url string) (*RedisIndex, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("suppressionindex: redis url empty")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("suppressionindex: parse url: %w", err)
	}
	// Tighter timeouts than go-redis defaults: this is a hot-path index,
	// any call that takes >250ms means the backend is unhealthy and we'd
	// rather fail open than block the admin API.
	opts.DialTimeout = 1 * time.Second
	opts.ReadTimeout = 250 * time.Millisecond
	opts.WriteTimeout = 250 * time.Millisecond
	opts.PoolSize = 32
	return &RedisIndex{rdb: redis.NewClient(opts)}, nil
}

// WithMetrics attaches the shared Prometheus collectors. Called once
// at boot; subsequent Add/Remove/Resync calls update the counters +
// gauge. Returns the receiver so it composes with NewRedis cleanly.
func (r *RedisIndex) WithMetrics(m *metrics.Metrics) *RedisIndex {
	r.m = m
	return r
}

// recordOp bumps the per-operation counter. Split out so the call
// sites stay one-line and a nil m is silently a no-op.
func (r *RedisIndex) recordOp(op string, err error) {
	if r.m == nil {
		return
	}
	result := "ok"
	if err != nil {
		result = "error"
	}
	r.m.SuppressionOpsTotal.WithLabelValues(op, result).Inc()
}

// refreshGauge polls SCARD on both index sets and writes the values
// into iris_suppression_entries. Cheap (O(1) on Redis), so we run it
// on every Resync and after each Add/Remove batch — keeping the
// gauge tight without a separate ticker goroutine.
func (r *RedisIndex) refreshGauge(ctx context.Context) {
	if r.m == nil {
		return
	}
	if n, err := r.rdb.SCard(ctx, KeyAddrSet).Result(); err == nil {
		r.m.SuppressionEntries.WithLabelValues(ScopeAddress).Set(float64(n))
	}
	if n, err := r.rdb.SCard(ctx, KeyDomSet).Result(); err == nil {
		r.m.SuppressionEntries.WithLabelValues(ScopeDomain).Set(float64(n))
	}
}

// keyForScope picks the right SET key for a scope. Returns "" for an
// unknown scope so callers can no-op rather than corrupt the index.
func keyForScope(scope string) string {
	switch scope {
	case ScopeAddress:
		return KeyAddrSet
	case ScopeDomain:
		return KeyDomSet
	default:
		return ""
	}
}

// Add inserts the entry. Lower-casing happens here, not at the call site,
// so every code path produces consistent keys regardless of how careful
// the caller was.
func (r *RedisIndex) Add(ctx context.Context, scope, value string) error {
	key := keyForScope(scope)
	if key == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	err := r.rdb.SAdd(ctx, key, strings.ToLower(strings.TrimSpace(value))).Err()
	r.recordOp("add", err)
	return err
}

// Remove deletes the entry. Idempotent.
func (r *RedisIndex) Remove(ctx context.Context, scope, value string) error {
	key := keyForScope(scope)
	if key == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	err := r.rdb.SRem(ctx, key, strings.ToLower(strings.TrimSpace(value))).Err()
	r.recordOp("remove", err)
	return err
}

// Resync atomically replaces the index. Implementation: build the new
// SETs under temp keys, then RENAME each on top of the live key. RENAME
// is atomic in Redis, so kumomta's SISMEMBER queries always see either
// the old set or the new set, never an in-between half-built state.
//
// Memory peak during resync is ~2× the steady-state set size — fine for
// the operating range we care about (≤ ~50M entries). For larger lists
// switch to a streaming SDIFF/SDIFFSTORE update; the RENAME approach is
// chosen for simplicity over the lifetime of this implementation.
func (r *RedisIndex) Resync(ctx context.Context, entries []Entry) error {
	const (
		tmpAddr = KeyAddrSet + ":resync"
		tmpDom  = KeyDomSet + ":resync"
	)
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, tmpAddr, tmpDom)

	addrs := make([]any, 0, len(entries))
	doms := make([]any, 0, len(entries)/8)
	for _, e := range entries {
		v := strings.ToLower(strings.TrimSpace(e.Value))
		if v == "" {
			continue
		}
		switch e.Scope {
		case ScopeAddress:
			addrs = append(addrs, v)
		case ScopeDomain:
			doms = append(doms, v)
		}
	}
	// Add in chunks: SADD with millions of args at once is bounded by Redis'
	// proto-max-bulk-len. 10K per SADD keeps us well under any limit and
	// caps the memory the client buffers.
	const chunk = 10_000
	for i := 0; i < len(addrs); i += chunk {
		j := i + chunk
		if j > len(addrs) {
			j = len(addrs)
		}
		pipe.SAdd(ctx, tmpAddr, addrs[i:j]...)
	}
	for i := 0; i < len(doms); i += chunk {
		j := i + chunk
		if j > len(doms) {
			j = len(doms)
		}
		pipe.SAdd(ctx, tmpDom, doms[i:j]...)
	}
	// RENAME of an empty SET errors with "no such key". Sentinel-add a
	// dummy value first so the SET exists even when the resync is empty,
	// then SREM it after the RENAME settles.
	pipe.SAdd(ctx, tmpAddr, "__sentinel__")
	pipe.SAdd(ctx, tmpDom, "__sentinel__")
	pipe.Rename(ctx, tmpAddr, KeyAddrSet)
	pipe.Rename(ctx, tmpDom, KeyDomSet)
	pipe.SRem(ctx, KeyAddrSet, "__sentinel__")
	pipe.SRem(ctx, KeyDomSet, "__sentinel__")
	pipe.HSet(ctx, KeyMetaHash,
		MetaFieldResyncedAt, time.Now().UTC().Format(time.RFC3339),
		MetaFieldEntryCount, len(entries),
	)
	if _, err := pipe.Exec(ctx); err != nil {
		r.recordOp("resync", err)
		return fmt.Errorf("suppressionindex: resync: %w", err)
	}
	r.recordOp("resync", nil)
	r.refreshGauge(ctx) // pin the iris_suppression_entries gauge to the new size
	return nil
}

// Healthy pings Redis. Used by /healthz and as a precondition for the
// startup resync (don't try to rebuild a 10M-entry set against an
// unreachable Redis).
func (r *RedisIndex) Healthy(ctx context.Context) error {
	return r.rdb.Ping(ctx).Err()
}

// Close releases the underlying connection pool. Optional — process exit
// closes sockets, but a clean shutdown reduces noise in Redis logs.
func (r *RedisIndex) Close() error { return r.rdb.Close() }
