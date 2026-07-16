package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/internal/biz"
)

// TLSPolicyKey returns the Redis key for a destination-domain TLS policy. The
// generated KumoMTA policy builds the same key ("tls:d:<domain>") for its
// memoized GET lookup in get_egress_path_config, so the two MUST stay in sync.
func TLSPolicyKey(domain string) string {
	return "tls:d:" + strings.ToLower(strings.TrimSpace(domain))
}

// TLSPolicyCache is the Redis-backed live per-domain TLS policy the rendered
// policy consults at delivery time. Postgres remains the source of truth; this
// is a write-through cache keyed per domain so adding/removing a policy takes
// effect within the policy's memoize TTL WITHOUT a KumoMTA reload. The value is
// KumoMTA's enable_tls string ("Disabled" | "Required" | "RequiredInsecure" |
// "OpportunisticInsecure"). All methods are no-ops on a nil cache / nil client
// so deployments without Redis fall back to the inline-rendered table.
type TLSPolicyCache struct {
	rdb redis.UniversalClient
}

// NewTLSPolicyCache wraps a Redis client (nil-safe). Pass streams.Client.
func NewTLSPolicyCache(rdb redis.UniversalClient) *TLSPolicyCache {
	return &TLSPolicyCache{rdb: rdb}
}

// Enabled reports whether a live Redis cache is wired.
func (c *TLSPolicyCache) Enabled() bool { return c != nil && c.rdb != nil }

// Put sets the domain's enable_tls value (permanent; no TTL — a policy stays
// until removed). enableTLS must be a valid KumoMTA enable_tls string.
func (c *TLSPolicyCache) Put(ctx context.Context, domain, enableTLS string) error {
	if !c.Enabled() {
		return nil
	}
	if strings.TrimSpace(enableTLS) == "" {
		return nil
	}
	if err := c.rdb.Set(ctx, TLSPolicyKey(domain), enableTLS, 0).Err(); err != nil {
		return fmt.Errorf("tls policy cache put: %w", err)
	}
	return nil
}

// Del removes a domain's TLS policy key (idempotent).
func (c *TLSPolicyCache) Del(ctx context.Context, domain string) error {
	if !c.Enabled() {
		return nil
	}
	if err := c.rdb.Del(ctx, TLSPolicyKey(domain)).Err(); err != nil {
		return fmt.Errorf("tls policy cache del: %w", err)
	}
	return nil
}

// Backfill repopulates Redis from the active DB policies (startup / after a
// Redis flush). Returns the number of keys written.
func (c *TLSPolicyCache) Backfill(ctx context.Context, policies []*biz.TLSPolicy) (int, error) {
	if !c.Enabled() {
		return 0, nil
	}
	written := 0
	for _, p := range policies {
		if p == nil || p.Status != biz.TLSPolicyActive {
			continue
		}
		if err := c.Put(ctx, p.Domain, p.EnableTLSValue()); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}
