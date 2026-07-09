package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// MonitoringReconcilerWorker periodically correlates queued inbox-placement
// probes against the mail log, advancing their send status (sent/deferred/
// bounced) once KumoMTA reports the outcome.
type MonitoringReconcilerWorker struct {
	uc       *biz.MonitoringUsecase
	interval time.Duration
	lookback time.Duration
	log      *slog.Logger
}

// NewMonitoringReconcilerWorker constructs the reconciler. Sensible defaults are
// applied for a non-positive interval/lookback.
func NewMonitoringReconcilerWorker(uc *biz.MonitoringUsecase, interval, lookback time.Duration, log *slog.Logger) *MonitoringReconcilerWorker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if lookback <= 0 {
		lookback = time.Hour
	}
	return &MonitoringReconcilerWorker{uc: uc, interval: interval, lookback: lookback, log: log}
}

// Run reconciles on each tick until the context is cancelled.
func (w *MonitoringReconcilerWorker) Run(ctx context.Context) error {
	if w.uc == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	w.log.Info("monitoring reconciler started", "interval", w.interval.String())
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := w.uc.ReconcileSends(ctx, w.lookback)
			if err != nil {
				w.log.Error("monitoring reconcile failed", "error", err.Error())
				continue
			}
			if n > 0 {
				w.log.Info("monitoring probes reconciled", "count", n)
			}
		}
	}
}

// MonitoringFetchWorker periodically performs the phase-2 mailbox search for
// probes whose fetch delay has elapsed, classifying inbox placement.
type MonitoringFetchWorker struct {
	uc       *biz.MonitoringUsecase
	interval time.Duration
	log      *slog.Logger
}

// NewMonitoringFetchWorker constructs the fetch worker. A non-positive interval
// defaults to one minute.
func NewMonitoringFetchWorker(uc *biz.MonitoringUsecase, interval time.Duration, log *slog.Logger) *MonitoringFetchWorker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &MonitoringFetchWorker{uc: uc, interval: interval, log: log}
}

// Run fetches due probe mailboxes on each tick until the context is cancelled.
func (w *MonitoringFetchWorker) Run(ctx context.Context) error {
	if w.uc == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	w.log.Info("monitoring fetch worker started", "interval", w.interval.String())
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := w.uc.RunDueFetches(ctx)
			if err != nil {
				w.log.Error("monitoring fetch failed", "error", err.Error())
				continue
			}
			if n > 0 {
				w.log.Info("monitoring probe mailboxes fetched", "count", n)
			}
		}
	}
}

// MonitoringSchedulerWorker periodically sends probes for accounts whose
// recurring schedule is due.
type MonitoringSchedulerWorker struct {
	uc       *biz.MonitoringUsecase
	interval time.Duration
	log      *slog.Logger
}

// NewMonitoringSchedulerWorker constructs the scheduler. The interval is the
// scan cadence (not the per-account probe interval, which lives on the account).
func NewMonitoringSchedulerWorker(uc *biz.MonitoringUsecase, interval time.Duration, log *slog.Logger) *MonitoringSchedulerWorker {
	if interval <= 0 {
		interval = time.Minute
	}
	return &MonitoringSchedulerWorker{uc: uc, interval: interval, log: log}
}

// Run scans for due schedules on each tick until the context is cancelled.
func (w *MonitoringSchedulerWorker) Run(ctx context.Context) error {
	if w.uc == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	w.log.Info("monitoring scheduler started", "interval", w.interval.String())
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := w.uc.RunDueSchedules(ctx)
			if err != nil {
				w.log.Error("monitoring scheduler failed", "error", err.Error())
				continue
			}
			if n > 0 {
				w.log.Info("monitoring scheduled probes sent", "count", n)
			}
		}
	}
}
