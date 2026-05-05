// SuppressionResyncServer kicks off a one-shot rebuild of the suppression
// index from PostgreSQL on admin-service startup. It joins the kratos
// transport lifecycle so the boot graph reads cleanly, but the resync
// itself is best-effort and decoupled from the API listening sockets:
// kumomta's Lua handler fails open on Redis errors, so a slow or failing
// resync only delays "freshness", never delivery.
//
// Why on every boot? Three drift modes converge here:
//   - first-time deploy with no Redis state ⇒ rebuild from PG
//   - Redis flush / FLUSHALL / OOM eviction ⇒ rebuild from PG
//   - prior admin-service crash mid-write ⇒ reconcile to PG
//
// PostgreSQL is the source of truth and the rebuild is idempotent
// (Resync uses a temp-key + RENAME swap), so replaying it on every boot
// costs only the time to scan the suppression table.
package server

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data"
	"github.com/menta2k/iris/backend/pkg/suppressionindex"
)

// SuppressionResyncServer is the kratos transport.Server adapter. The
// resync runs in Start as a goroutine so the rest of the lifecycle (HTTP
// listener bind, gRPC bind, etc.) doesn't wait on it; on a 10M-row
// suppression table the scan can take seconds.
type SuppressionResyncServer struct {
	repo  *data.SuppressionRepo
	index suppressionindex.Index
	done  chan struct{}
}

// NewSuppressionResyncServer constructs the server. A nil repo or a Noop
// index disables the resync (idx is the explicit signal — Noop returns
// quickly anyway, but skipping the PG scan saves a few seconds at boot
// when there's no Redis backend to populate).
func NewSuppressionResyncServer(repo *data.SuppressionRepo, idx suppressionindex.Index) *SuppressionResyncServer {
	if repo == nil || idx == nil {
		return &SuppressionResyncServer{}
	}
	if _, ok := idx.(*suppressionindex.Noop); ok {
		return &SuppressionResyncServer{}
	}
	return &SuppressionResyncServer{repo: repo, index: idx}
}

// Start kicks the resync goroutine. Returns immediately — kratos waits on
// listener readiness, not on the resync. Health-check the index health
// endpoint if you need to know when the rebuild has settled.
func (s *SuppressionResyncServer) Start(_ context.Context) error {
	if s.repo == nil || s.index == nil {
		return nil
	}
	s.done = make(chan struct{})
	go s.run()
	return nil
}

// Stop joins the resync goroutine if it's still running. A long PG scan
// at shutdown is rare but possible; we wait up to Stop's deadline.
func (s *SuppressionResyncServer) Stop(ctx context.Context) error {
	if s.done == nil {
		return nil
	}
	select {
	case <-s.done:
	case <-ctx.Done():
	}
	return nil
}

// run performs the actual resync. Independent context with a 5-minute
// ceiling so a stuck PG query can't pin the goroutine forever.
func (s *SuppressionResyncServer) run() {
	defer close(s.done)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Pre-flight: don't scan PG if Redis is unreachable — the resync
	// would just discard the work. Backoff once and bail; the next
	// admin-service boot will retry.
	if err := s.index.Healthy(ctx); err != nil {
		log.Printf("suppression-resync: index unhealthy at boot, skipping: %v", err)
		return
	}

	start := time.Now()
	entries := make([]suppressionindex.Entry, 0, 4096)
	if err := s.repo.IterateAll(ctx, 5000, func(addr, scope string) error {
		entries = append(entries, suppressionindex.Entry{
			Scope: scope,
			Value: strings.ToLower(strings.TrimSpace(addr)),
		})
		return nil
	}); err != nil {
		log.Printf("suppression-resync: iterate failed after %s with %d entries: %v",
			time.Since(start).Round(time.Millisecond), len(entries), err)
		return
	}
	scanTook := time.Since(start)

	if err := s.index.Resync(ctx, entries); err != nil {
		log.Printf("suppression-resync: redis resync failed after %s with %d entries: %v",
			time.Since(start).Round(time.Millisecond), len(entries), err)
		return
	}
	log.Printf("suppression-resync: rebuilt index with %d entries in %s (scan=%s)",
		len(entries), time.Since(start).Round(time.Millisecond), scanTook.Round(time.Millisecond))
}
