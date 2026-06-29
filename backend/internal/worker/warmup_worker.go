package worker

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// WarmupScheduler is the warmup lifecycle + policy surface the worker drives.
type WarmupScheduler interface {
	// Tick advances scheduled→active and active→completed for the date.
	Tick(ctx context.Context, today time.Time) (bool, error)
	// ActiveForPolicy returns active+paused schedules (those that set caps).
	ActiveForPolicy(ctx context.Context) ([]*biz.WarmupSchedule, error)
}

// ConfigApplier applies the current rendered policy under a system actor.
type ConfigApplier interface {
	ApplyForAutomation(ctx context.Context, actor string) (*biz.ApplyResult, error)
}

// WarmupWorker rolls IP-warmup forward: on a periodic cadence it advances the
// schedule lifecycle and, only when the resolved per-day caps actually change
// (a new stage, a start, a completion, or a status change), applies the policy
// so KumoMTA picks up the new max_message_rate. The cap "fingerprint" gate keeps
// it from re-applying on every tick when nothing has changed.
type WarmupWorker struct {
	warmup   WarmupScheduler
	config   ConfigApplier
	interval time.Duration
	log      *slog.Logger
	lastFP   string
}

// NewWarmupWorker constructs the worker. interval is the check cadence (e.g. 1h
// — small so a day-boundary cap step goes live promptly); non-positive defaults
// to 1h.
func NewWarmupWorker(warmup WarmupScheduler, config ConfigApplier, interval time.Duration, log *slog.Logger) *WarmupWorker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &WarmupWorker{warmup: warmup, config: config, interval: interval, log: log}
}

// Run advances warmup on the cadence until the context is cancelled. A first
// pass runs shortly after boot so a restart reconciles KumoMTA to today's caps.
func (w *WarmupWorker) Run(ctx context.Context) error {
	w.log.Info("warmup worker started", "interval", w.interval.String())
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			w.log.Info("warmup worker stopping")
			return ctx.Err()
		case <-timer.C:
			w.runOnce(ctx)
			timer.Reset(w.interval)
		}
	}
}

func (w *WarmupWorker) runOnce(ctx context.Context) {
	now := time.Now().UTC()
	if _, err := w.warmup.Tick(ctx, now); err != nil {
		w.log.Error("warmup tick", "error", err.Error())
		return
	}
	sched, err := w.warmup.ActiveForPolicy(ctx)
	if err != nil {
		w.log.Error("warmup list active", "error", err.Error())
		return
	}
	fp := warmupFingerprint(biz.ResolveWarmupRates(sched, now))
	if fp == w.lastFP {
		return // caps unchanged since the last applied policy
	}
	if _, err := w.config.ApplyForAutomation(ctx, "warmup-worker"); err != nil {
		w.log.Error("warmup apply config", "error", err.Error())
		return // keep lastFP unchanged so the next cycle retries
	}
	w.lastFP = fp
	w.log.Info("warmup caps changed; applied policy", "sources", len(biz.ResolveWarmupRates(sched, now)))
}

// warmupFingerprint deterministically serializes the resolved per-source,
// per-bucket caps so the worker can detect a change without re-applying.
func warmupFingerprint(rates map[string]map[string]string) string {
	sources := make([]string, 0, len(rates))
	for s := range rates {
		sources = append(sources, s)
	}
	sort.Strings(sources)
	var b strings.Builder
	for _, src := range sources {
		buckets := rates[src]
		keys := make([]string, 0, len(buckets))
		for k := range buckets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(src)
		b.WriteByte('{')
		for _, k := range keys {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(buckets[k])
			b.WriteByte(';')
		}
		b.WriteString("}\n")
	}
	return b.String()
}
