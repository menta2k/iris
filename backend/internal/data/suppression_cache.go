package data

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/internal/biz"
)

// suppKeyPrefix maps a suppression type to its Redis key prefix. The generated
// KumoMTA policy builds the same keys ("supp:e:<email>" / "supp:d:<domain>") for
// its memoized EXISTS lookup, so the two MUST stay in sync.
func suppKeyPrefix(typ string) string {
	if typ == biz.SuppressDomain {
		return "supp:d:"
	}
	return "supp:e:"
}

// SuppressionKey returns the Redis key for a suppression (type, value). Exposed
// for tests and to document the contract shared with the rendered policy.
func SuppressionKey(typ, value string) string {
	return suppKeyPrefix(typ) + biz.NormalizeSuppressionValue(typ, value)
}

// SuppressionCache is the Redis-backed live suppression list the rendered policy
// consults. Postgres remains the source of truth; this is a write-through cache
// keyed per address with a TTL so the list self-ages. All methods are no-ops on
// a nil cache so deployments/tests without Redis fall back to DB-only behavior.
type SuppressionCache struct {
	rdb *redis.Client
}

// NewSuppressionCache wraps a Redis client (nil-safe). Pass streams.Client.
func NewSuppressionCache(rdb *redis.Client) *SuppressionCache {
	return &SuppressionCache{rdb: rdb}
}

// Put marks (type, value) suppressed for ttl (0 = no expiry / permanent).
func (c *SuppressionCache) Put(ctx context.Context, typ, value string, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	if ttl < 0 {
		ttl = 0
	}
	if err := c.rdb.Set(ctx, SuppressionKey(typ, value), "1", ttl).Err(); err != nil {
		return fmt.Errorf("suppression cache put: %w", err)
	}
	return nil
}

// Del removes a suppression key (idempotent).
func (c *SuppressionCache) Del(ctx context.Context, typ, value string) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	if err := c.rdb.Del(ctx, SuppressionKey(typ, value)).Err(); err != nil {
		return fmt.Errorf("suppression cache del: %w", err)
	}
	return nil
}

// Clear removes every live suppression key (supp:e:* and supp:d:*) via SCAN so it
// never blocks Redis on a large keyspace, and never touches non-suppression keys
// (the log stream shares this Redis). Returns the number of keys removed.
func (c *SuppressionCache) Clear(ctx context.Context) (int, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	removed := 0
	for _, pattern := range []string{"supp:e:*", "supp:d:*"} {
		var cursor uint64
		for {
			keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 500).Result()
			if err != nil {
				return removed, fmt.Errorf("suppression cache scan: %w", err)
			}
			if len(keys) > 0 {
				if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
					return removed, fmt.Errorf("suppression cache clear: %w", err)
				}
				removed += len(keys)
			}
			if cursor = next; cursor == 0 {
				break
			}
		}
	}
	return removed, nil
}

// Backfill repopulates Redis from the active DB entries (e.g. at startup, or
// after a Redis flush). Entries already past their expiry are skipped; entries
// with a nil ExpiresAt are written without a TTL (permanent). Returns the number
// of keys written.
func (c *SuppressionCache) Backfill(ctx context.Context, entries []*biz.SuppressionEntry, now time.Time) (int, error) {
	if c == nil || c.rdb == nil {
		return 0, nil
	}
	written := 0
	for _, e := range entries {
		if e == nil || e.Status != biz.SuppressActive {
			continue
		}
		var ttl time.Duration
		if e.ExpiresAt != nil {
			ttl = e.ExpiresAt.Sub(now)
			if ttl <= 0 {
				continue // already expired; leave it out of the live list
			}
		}
		if err := c.Put(ctx, e.Type, e.Value, ttl); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}
