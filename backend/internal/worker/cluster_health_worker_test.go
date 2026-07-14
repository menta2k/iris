package worker

import (
	"context"
	"log/slog"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
)

type fakeHealthCollector struct {
	health []biz.MTANodeHealth
}

func (f *fakeHealthCollector) CollectNodeHealth(ctx context.Context) ([]biz.MTANodeHealth, error) {
	return f.health, nil
}

type fakeHeartbeatStore struct {
	beats map[string]biz.MTANodeHealth
}

func (f *fakeHeartbeatStore) RecordNodeHeartbeat(ctx context.Context, id, version, checksum, state string) error {
	if f.beats == nil {
		f.beats = map[string]biz.MTANodeHealth{}
	}
	f.beats[id] = biz.MTANodeHealth{NodeID: id, Version: version, AppliedChecksum: checksum, KumoState: state}
	return nil
}

// TestClusterHealthWorkerPollRecordsHeartbeats verifies one poll pass writes
// every collected node's health — including unreachable nodes, which must be
// recorded (not skipped) so outages become visible.
func TestClusterHealthWorkerPollRecordsHeartbeats(t *testing.T) {
	collector := &fakeHealthCollector{health: []biz.MTANodeHealth{
		{NodeID: "n1", Name: "node1", KumoState: "running"},
		{NodeID: "n2", Name: "node2", Version: "iris-agent/1", AppliedChecksum: "sum-1", KumoState: "unreachable"},
	}}
	store := &fakeHeartbeatStore{}
	w := NewClusterHealthWorker(collector, store, slog.Default())

	w.poll(context.Background())

	if got := store.beats["n1"]; got.KumoState != "running" {
		t.Fatalf("n1 heartbeat = %+v", got)
	}
	if got := store.beats["n2"]; got.KumoState != "unreachable" || got.AppliedChecksum != "sum-1" {
		t.Fatalf("n2 heartbeat = %+v", got)
	}
}
