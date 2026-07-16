package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/internal/biz"
)

// TLSPoliciesKey is the single Redis key holding the whole per-domain TLS policy
// set as a JSON object { "<domain>": "<enable_tls>" }. The generated KumoMTA
// policy GETs this one key once per memoize TTL (not per domain) and looks the
// domain up in-memory in get_egress_path_config, so delivery never does a
// per-domain Redis round-trip — the policy set is tiny (dozens of domains).
const TLSPoliciesKey = "iris:tls_policies"

// TLSPolicyCache is the Redis-backed snapshot of the live per-domain TLS policy
// set the rendered policy consults. Postgres remains the source of truth; the
// whole active set is written as one JSON key so add/remove/auto-disable takes
// effect within the policy's memoize TTL WITHOUT a KumoMTA reload, and steady-
// state Redis load is constant regardless of send volume or domain spread. The
// value per domain is KumoMTA's enable_tls string ("Disabled" | "Required" |
// "RequiredInsecure" | "OpportunisticInsecure"). All methods are no-ops on a nil
// cache / nil client so deployments without Redis fall back to the inline table.
type TLSPolicyCache struct {
	rdb redis.UniversalClient
}

// NewTLSPolicyCache wraps a Redis client (nil-safe). Pass streams.Client.
func NewTLSPolicyCache(rdb redis.UniversalClient) *TLSPolicyCache {
	return &TLSPolicyCache{rdb: rdb}
}

// Enabled reports whether a live Redis cache is wired.
func (c *TLSPolicyCache) Enabled() bool { return c != nil && c.rdb != nil }

// Sync writes the whole active policy set as one JSON key (replacing any prior
// value), so kumod's next memoized load sees the current set. Non-active policies
// are excluded. An empty set writes "{}". Called after every policy mutation and
// at startup (Backfill).
func (c *TLSPolicyCache) Sync(ctx context.Context, policies []*biz.TLSPolicy) error {
	if !c.Enabled() {
		return nil
	}
	blob, err := tlsPolicyBlob(policies)
	if err != nil {
		return err
	}
	if err := c.rdb.Set(ctx, TLSPoliciesKey, blob, 0).Err(); err != nil {
		return fmt.Errorf("tls policy cache set: %w", err)
	}
	return nil
}

// tlsPolicyBlob renders the active policy set as the JSON object kumod parses:
// { "<lower-domain>": "<enable_tls>" }. Inactive/blank-domain entries are
// excluded; an empty set yields "{}". Pure — the unit of the cache tested
// without Redis.
func tlsPolicyBlob(policies []*biz.TLSPolicy) ([]byte, error) {
	m := make(map[string]string, len(policies))
	for _, p := range policies {
		if p == nil || p.Status != biz.TLSPolicyActive {
			continue
		}
		d := strings.ToLower(strings.TrimSpace(p.Domain))
		if d == "" {
			continue
		}
		m[d] = p.EnableTLSValue()
	}
	blob, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("tls policy cache marshal: %w", err)
	}
	return blob, nil
}

// Backfill writes the active policy set at startup (or after a Redis flush).
func (c *TLSPolicyCache) Backfill(ctx context.Context, policies []*biz.TLSPolicy) error {
	return c.Sync(ctx, policies)
}
