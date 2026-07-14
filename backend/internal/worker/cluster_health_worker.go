package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// clusterHealthInterval is how often node health is polled. Frequent enough
// that the Cluster page's state/drift/last-seen are current, cheap enough
// (one small HTTPS request per remote node) to run continuously.
const clusterHealthInterval = 30 * time.Second

// NodeHealthCollector polls every registered node's live health.
// Satisfied by data.FileKumoMTA.
type NodeHealthCollector interface {
	CollectNodeHealth(ctx context.Context) ([]biz.MTANodeHealth, error)
}

// NodeHeartbeatStore persists collected health. Satisfied by data.MTANodeRepo.
type NodeHeartbeatStore interface {
	RecordNodeHeartbeat(ctx context.Context, id, version, appliedChecksum, kumoState string) error
}

// ClusterHealthWorker keeps the node registry's live fields (kumod state,
// agent version, applied checksum, last_seen_at) fresh, so the Cluster page
// shows current health and config drift instead of apply-time snapshots.
type ClusterHealthWorker struct {
	collector NodeHealthCollector
	store     NodeHeartbeatStore
	log       *slog.Logger
	interval  time.Duration
}

// NewClusterHealthWorker constructs the worker.
func NewClusterHealthWorker(collector NodeHealthCollector, store NodeHeartbeatStore, log *slog.Logger) *ClusterHealthWorker {
	return &ClusterHealthWorker{collector: collector, store: store, log: log, interval: clusterHealthInterval}
}

// Run polls until ctx is cancelled.
func (w *ClusterHealthWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.poll(ctx) // immediate first pass so the UI is fresh right after startup
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *ClusterHealthWorker) poll(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, w.interval)
	defer cancel()
	health, err := w.collector.CollectNodeHealth(pollCtx)
	if err != nil {
		w.log.Warn("cluster health poll failed", "error", err.Error())
		return
	}
	for _, h := range health {
		if err := w.store.RecordNodeHeartbeat(pollCtx, h.NodeID, h.Version, h.AppliedChecksum, h.KumoState); err != nil {
			w.log.Warn("record node heartbeat failed", "node", h.Name, "error", err.Error())
		}
	}
}
