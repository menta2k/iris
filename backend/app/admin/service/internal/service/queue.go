package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/pkg/kumomta"
)

// MetricSnapshotWriter persists per-queue snapshots into the TimescaleDB
// hypertable.
type MetricSnapshotWriter interface {
	WriteSnapshots(ctx context.Context, snaps []QueueSnapshot) error
}

// QueueSnapshot is the persistence-shape of a sample.
type QueueSnapshot struct {
	At        time.Time
	Queue     string
	QueueSize uint64
	Delivered uint64
	Failed    uint64
	Deferred  uint64
	Suspended bool
}

// QueueService talks to the kumomta admin API and (optionally) persists
// snapshots for trend dashboards.
type QueueService struct {
	client  *kumomta.Client
	metrics MetricSnapshotWriter
	now     func() time.Time
}

// NewQueueService constructs the service.
func NewQueueService(c *kumomta.Client, m MetricSnapshotWriter) *QueueService {
	return &QueueService{client: c, metrics: m, now: time.Now}
}

// QueueListItem is the API-friendly shape returned by List.
type QueueListItem struct {
	Name      string
	QueueSize uint64
	Delivered uint64
	Failed    uint64
	Suspended bool
	SampledAt time.Time
}

// List fetches the live queue summary, optionally filters by name substring,
// and (best-effort) records the snapshot into TimescaleDB.
func (s *QueueService) List(ctx context.Context, filter string, limit int) ([]QueueListItem, error) {
	raw, err := s.client.ListQueues(ctx)
	if err != nil {
		return nil, err
	}
	at := s.now().UTC()
	out := make([]QueueListItem, 0, len(raw))
	snaps := make([]QueueSnapshot, 0, len(raw))
	for _, r := range raw {
		if filter != "" && !strings.Contains(r.Name, filter) {
			continue
		}
		out = append(out, QueueListItem{
			Name:      r.Name,
			QueueSize: r.QueueSize,
			Delivered: r.Delivered,
			Failed:    r.Failed,
			Suspended: r.Suspended,
			SampledAt: at,
		})
		snaps = append(snaps, QueueSnapshot{
			At:        at,
			Queue:     r.Name,
			QueueSize: r.QueueSize,
			Delivered: r.Delivered,
			Failed:    r.Failed,
			Deferred:  r.Deferred,
			Suspended: r.Suspended,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if s.metrics != nil && len(snaps) > 0 {
		// Best-effort: a snapshot write failure must not break the query.
		_ = s.metrics.WriteSnapshots(ctx, snaps)
	}
	return out, nil
}

var ErrQueueNameRequired = errors.New("queue: name required")

// Suspend pauses a queue. The action is best-effort idempotent.
func (s *QueueService) Suspend(ctx context.Context, name string) error {
	if name == "" {
		return ErrQueueNameRequired
	}
	return s.client.SuspendQueue(ctx, name)
}

// Resume resumes a queue.
func (s *QueueService) Resume(ctx context.Context, name string) error {
	if name == "" {
		return ErrQueueNameRequired
	}
	return s.client.ResumeQueue(ctx, name)
}

// Bounce empties a queue, bouncing all messages with the given reason.
func (s *QueueService) Bounce(ctx context.Context, name, reason string) error {
	if name == "" {
		return ErrQueueNameRequired
	}
	if reason == "" {
		reason = "operator-initiated bounce"
	}
	return s.client.BounceQueue(ctx, name, reason)
}
