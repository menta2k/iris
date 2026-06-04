// MFA session store: short-lived state for the stateful parts of MFA (the
// pending TOTP-enroll secret and WebAuthn ceremony SessionData). Backed by
// Redis when IRIS_LOGSTREAM_REDIS_URL is set (so it works across replicas),
// otherwise an in-memory TTL map that is correct for a single-node install.
package data

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

const mfaSessionRedisPrefix = "iris:mfa:sess:"

// RedisMFASessionStore implements service.MFASessionStore over go-redis.
type RedisMFASessionStore struct{ rdb *redis.Client }

func NewRedisMFASessionStore(rdb *redis.Client) *RedisMFASessionStore {
	return &RedisMFASessionStore{rdb: rdb}
}

func (s *RedisMFASessionStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.rdb.Set(ctx, mfaSessionRedisPrefix+key, value, ttl).Err()
}

func (s *RedisMFASessionStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := s.rdb.Get(ctx, mfaSessionRedisPrefix+key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *RedisMFASessionStore) GetDel(ctx context.Context, key string) (string, bool, error) {
	v, err := s.rdb.GetDel(ctx, mfaSessionRedisPrefix+key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// MemoryMFASessionStore is the single-node fallback.
type MemoryMFASessionStore struct {
	mu    sync.Mutex
	items map[string]memSessionItem
}

type memSessionItem struct {
	value     string
	expiresAt time.Time
}

func NewMemoryMFASessionStore() *MemoryMFASessionStore {
	return &MemoryMFASessionStore{items: map[string]memSessionItem{}}
}

func (s *MemoryMFASessionStore) Put(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	s.items[key] = memSessionItem{value: value, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *MemoryMFASessionStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[key]
	if !ok || time.Now().After(it.expiresAt) {
		delete(s.items, key)
		return "", false, nil
	}
	return it.value, true, nil
}

func (s *MemoryMFASessionStore) GetDel(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[key]
	delete(s.items, key)
	if !ok || time.Now().After(it.expiresAt) {
		return "", false, nil
	}
	return it.value, true, nil
}

// sweepLocked drops expired entries so the map doesn't grow unbounded under a
// burst of abandoned enrollments. Caller holds s.mu.
func (s *MemoryMFASessionStore) sweepLocked() {
	now := time.Now()
	for k, it := range s.items {
		if now.After(it.expiresAt) {
			delete(s.items, k)
		}
	}
}

var (
	_ service.MFASessionStore = (*RedisMFASessionStore)(nil)
	_ service.MFASessionStore = (*MemoryMFASessionStore)(nil)
)
