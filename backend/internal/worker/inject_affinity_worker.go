package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// AffinitySnapshotSource loads the current configuration snapshot the affinity
// table is built from. Satisfied by the KumoMTA config snapshot repo.
type AffinitySnapshotSource interface {
	Snapshot(ctx context.Context) (biz.ConfigSnapshot, error)
}

// AffinityTarget is the resolver whose table is rebuilt. Satisfied by
// *biz.InjectAffinity.
type AffinityTarget interface {
	Rebuild(snap biz.ConfigSnapshot)
}

// InjectAffinityWorker periodically rebuilds the HTTP-injection egress-affinity
// table (mailclass → owning node) from the current config, so injection keeps
// placing messages on their egress-owning node as routing/VMTA ownership
// changes. A stale table never causes wrong delivery — at worst a message takes
// the same cross-node proxy hop it would have taken under plain round-robin — so
// a modest refresh interval is fine.
type InjectAffinityWorker struct {
	src      AffinitySnapshotSource
	affinity AffinityTarget
	interval time.Duration
	log      *slog.Logger
}

// NewInjectAffinityWorker constructs the worker. Non-positive interval defaults
// to 60s.
func NewInjectAffinityWorker(src AffinitySnapshotSource, affinity AffinityTarget, interval time.Duration, log *slog.Logger) *InjectAffinityWorker {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &InjectAffinityWorker{src: src, affinity: affinity, interval: interval, log: log}
}

// Run rebuilds once shortly after boot, then on the interval until cancelled.
func (w *InjectAffinityWorker) Run(ctx context.Context) error {
	if w.src == nil || w.affinity == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	w.log.Info("inject-affinity worker started", "interval", w.interval.String())
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			w.rebuild(ctx)
			timer.Reset(w.interval)
		}
	}
}

func (w *InjectAffinityWorker) rebuild(ctx context.Context) {
	snap, err := w.src.Snapshot(ctx)
	if err != nil {
		w.log.Error("inject-affinity snapshot failed", "error", err.Error())
		return
	}
	w.affinity.Rebuild(snap)
}
