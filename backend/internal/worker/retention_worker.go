package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

const retentionGroup = "iris-retention"

// RetentionProducer enqueues on-demand retention runs. It satisfies
// biz.RetentionCommandProducer.
type RetentionProducer struct {
	streams *data.Streams
}

// NewRetentionProducer constructs the producer.
func NewRetentionProducer(streams *data.Streams) *RetentionProducer {
	return &RetentionProducer{streams: streams}
}

var _ biz.RetentionCommandProducer = (*RetentionProducer)(nil)

// EnqueueRetentionRun publishes a run request; an empty table means all tables.
func (p *RetentionProducer) EnqueueRetentionRun(ctx context.Context, table string) error {
	_, err := p.streams.Publish(ctx, data.StreamRetentionCommands, map[string]any{"table": table})
	return err
}

// RetentionStore is the persistence boundary the worker drives.
type RetentionStore interface {
	ListPolicies(ctx context.Context) ([]*biz.RetentionPolicy, error)
	RunRetention(ctx context.Context, p *biz.RetentionPolicy, table biz.ManagedTable) (*biz.RetentionRun, error)
}

// RetentionWorker compresses and drops old TimescaleDB chunks on a daily cadence
// and on demand (the iris.retention.commands stream). Cleanup is chunk-based, so
// disk is returned to the OS immediately — no VACUUM FULL.
type RetentionWorker struct {
	streams  *data.Streams
	store    RetentionStore
	interval time.Duration
	log      *slog.Logger
}

// NewRetentionWorker constructs the worker. interval is the automatic cadence
// (e.g. 24h); a non-positive value defaults to 24h.
func NewRetentionWorker(streams *data.Streams, store RetentionStore, interval time.Duration, log *slog.Logger) *RetentionWorker {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &RetentionWorker{streams: streams, store: store, interval: interval, log: log}
}

// Run consumes on-demand commands and runs the automatic cadence until the
// context is cancelled.
func (w *RetentionWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, data.StreamRetentionCommands, retentionGroup); err != nil {
		return err
	}
	w.log.Info("retention worker started", "interval", w.interval.String())
	// Don't run immediately on boot; first automatic pass is one interval out.
	lastAuto := time.Now()
	for {
		select {
		case <-ctx.Done():
			w.log.Info("retention worker stopping")
			return ctx.Err()
		default:
		}

		msgs, err := w.streams.Consume(ctx, data.StreamRetentionCommands, retentionGroup, 5, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume retention commands", "error", err.Error())
		}
		for _, m := range msgs {
			w.runAll(ctx, stringValue(m.Values["table"]))
			if err := w.streams.Ack(ctx, data.StreamRetentionCommands, retentionGroup, m.ID); err != nil {
				w.log.Error("ack retention command", "id", m.ID, "error", err.Error())
			}
		}

		if time.Since(lastAuto) >= w.interval {
			w.runAll(ctx, "")
			lastAuto = time.Now()
		}
	}
}

// runAll runs retention for every enabled policy (or only `only` when set).
func (w *RetentionWorker) runAll(ctx context.Context, only string) {
	policies, err := w.store.ListPolicies(ctx)
	if err != nil {
		w.log.Error("retention: list policies", "error", err.Error())
		return
	}
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if only != "" && p.TableName != only {
			continue
		}
		if p.RetentionDays == 0 && p.CompressAfterDays == 0 {
			continue // nothing configured to do
		}
		table, ok := biz.ManagedTableByName(p.TableName)
		if !ok {
			continue
		}
		run, err := w.store.RunRetention(ctx, p, table)
		if err != nil {
			w.log.Error("retention run failed", "table", p.TableName, "error", err.Error())
			continue
		}
		if run.Error != "" {
			w.log.Warn("retention run completed with errors", "table", p.TableName, "error", run.Error)
			continue
		}
		w.log.Info("retention run",
			"table", p.TableName,
			"compressed", run.ChunksCompressed,
			"dropped", run.ChunksDropped,
			"freed_bytes", run.BytesFreed())
	}
}
