package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// SysMonWorker samples the host on an interval, publishes the snapshot to the
// use case (for the dashboard/API), and emails alerts when a resource crosses
// its threshold — throttled by a per-resource cooldown — or recovers.
type SysMonWorker struct {
	sampler  biz.HostSampler
	repo     biz.MonitorRepo
	notifier biz.AlertNotifier
	sink     func(biz.SystemSnapshot)
	log      *slog.Logger

	// per-resource alert state (in-memory): whether currently breached and when
	// we last emailed, so repeat notifications are throttled.
	state map[string]*resState
}

type resState struct {
	breached     bool
	lastNotified time.Time
	resource     string
	detail       string
	threshold    int
}

// NewSysMonWorker constructs the worker. notifier may be nil (alerts are then
// recorded but not emailed). sink receives every snapshot.
func NewSysMonWorker(sampler biz.HostSampler, repo biz.MonitorRepo, notifier biz.AlertNotifier, sink func(biz.SystemSnapshot), log *slog.Logger) *SysMonWorker {
	return &SysMonWorker{sampler: sampler, repo: repo, notifier: notifier, sink: sink, log: log, state: map[string]*resState{}}
}

// Run samples until the context is cancelled. Settings are re-read each cycle so
// threshold/interval changes take effect without a restart.
func (w *SysMonWorker) Run(ctx context.Context) error {
	w.log.Info("system monitor worker started")
	interval := 30 * time.Second
	timer := time.NewTimer(0) // fire immediately for the first sample
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			w.log.Info("system monitor worker stopping")
			return ctx.Err()
		case <-timer.C:
		}
		if d := w.tick(ctx); d > 0 {
			interval = d
		}
		timer.Reset(interval)
	}
}

func (w *SysMonWorker) tick(ctx context.Context) time.Duration {
	settings, err := w.repo.GetMonitorSettings(ctx)
	if err != nil {
		w.log.Error("load monitor settings", "error", err.Error())
		return 0
	}
	snap, err := w.sampler.Sample(ctx, settings.DiskPaths)
	if err != nil {
		w.log.Error("sample host", "error", err.Error())
		return settings.SampleInterval()
	}
	w.sink(snap)
	if settings.Enabled {
		w.evaluate(ctx, snap, settings)
	}
	return settings.SampleInterval()
}

func (w *SysMonWorker) evaluate(ctx context.Context, snap biz.SystemSnapshot, settings *biz.MonitorSettings) {
	now := time.Now().UTC()
	breachByKey := map[string]biz.MonitorAlert{}
	for _, b := range biz.EvaluateBreaches(snap, *settings) {
		breachByKey[b.Key()] = b
	}
	curVal := currentValues(snap)

	// New or cooldown-elapsed breaches → notify + record.
	for key, b := range breachByKey {
		st := w.state[key]
		if st == nil {
			st = &resState{}
			w.state[key] = st
		}
		fire := !st.breached || now.Sub(st.lastNotified) >= settings.Cooldown()
		st.breached, st.resource, st.detail, st.threshold = true, b.Resource, b.Detail, b.Threshold
		if fire {
			w.fire(ctx, settings, b, now)
			st.lastNotified = now
		}
	}

	// Resources previously breached that are no longer over the line → recovery.
	for key, st := range w.state {
		if !st.breached {
			continue
		}
		if _, still := breachByKey[key]; still {
			continue
		}
		st.breached = false
		val := curVal[key]
		w.fire(ctx, settings, biz.MonitorAlert{
			Resource:  st.resource,
			Detail:    st.detail,
			Level:     biz.AlertRecovered,
			Value:     val,
			Threshold: st.threshold,
			Message:   biz.RecoveryMessage(st.resource, st.detail, val, st.threshold),
		}, now)
	}
}

// fire emails the alert (best-effort) and records it in the history.
func (w *SysMonWorker) fire(ctx context.Context, settings *biz.MonitorSettings, alert biz.MonitorAlert, now time.Time) {
	alert.CreatedAt = now
	prefix := "ALERT"
	if alert.Level == biz.AlertRecovered {
		prefix = "RESOLVED"
	}
	subject := fmt.Sprintf("[iris] %s: %s", prefix, alert.Message)
	body := fmt.Sprintf("%s\n\nHost self-monitoring, %s.\n", alert.Message, now.Format(time.RFC3339))

	notified := false
	if w.notifier != nil && len(settings.NotifyEmails) > 0 && settings.FromEmail != "" {
		if err := w.notifier.Notify(ctx, settings.SMTPHost, settings.FromEmail, settings.NotifyEmails, subject, body); err != nil {
			w.log.Error("send alert email", "resource", alert.Key(), "error", err.Error())
		} else {
			notified = true
		}
	}
	alert.Notified = notified
	w.log.Warn("monitor alert", "level", alert.Level, "resource", alert.Key(), "value", alert.Value, "notified", notified)
	if err := w.repo.InsertMonitorAlert(ctx, &alert); err != nil {
		w.log.Error("record monitor alert", "error", err.Error())
	}
}

// currentValues maps each monitored key to its measured percent in the snapshot,
// for recovery messages.
func currentValues(snap biz.SystemSnapshot) map[string]float64 {
	m := map[string]float64{
		biz.ResourceCPU: snap.CPUPercent,
		biz.ResourceMem: snap.MemPercent,
	}
	for _, d := range snap.Disks {
		m[biz.ResourceDisk+":"+d.Path] = d.UsedPercent
	}
	return m
}
