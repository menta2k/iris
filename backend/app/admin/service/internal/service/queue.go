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
	Deferred  uint64
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
	// Deferred / retrying messages don't appear in ready-q-states; they
	// live in scheduled queues exposed via /metrics.json. Best-effort: a
	// metrics fetch failure must not break the queue listing.
	scheduled, _ := s.client.ListScheduledQueues(ctx)
	at := s.now().UTC()
	out := make([]QueueListItem, 0, len(raw)+len(scheduled))
	snaps := make([]QueueSnapshot, 0, len(raw)+len(scheduled))
	seen := make(map[string]struct{}, len(raw))
	for _, r := range raw {
		seen[r.Name] = struct{}{}
		if filter != "" && !strings.Contains(r.Name, filter) {
			continue
		}
		// Ready queues from /api/admin/ready-q-states/v1 don't carry the
		// scheduled-queue deferred count — they're keyed differently
		// (site name vs tenant@domain). Fall back to whatever the ready
		// state exposes (kumomta currently returns zero there).
		def := r.Deferred
		if v, ok := scheduled[r.Name]; ok {
			def = v
		}
		out = append(out, QueueListItem{
			Name:      r.Name,
			QueueSize: r.QueueSize,
			Delivered: r.Delivered,
			Failed:    r.Failed,
			Deferred:  def,
			Suspended: r.Suspended,
			SampledAt: at,
		})
		snaps = append(snaps, QueueSnapshot{
			At:        at,
			Queue:     r.Name,
			QueueSize: r.QueueSize,
			Delivered: r.Delivered,
			Failed:    r.Failed,
			Deferred:  def,
			Suspended: r.Suspended,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	// Append scheduled queues that don't appear in the ready set so
	// operators can see deferred mail that has no live ready queue
	// (e.g. all attempts have transiently failed and the retry timer
	// hasn't fired yet).
	if limit <= 0 || len(out) < limit {
		for name, count := range scheduled {
			if _, dup := seen[name]; dup {
				continue
			}
			if filter != "" && !strings.Contains(name, filter) {
				continue
			}
			out = append(out, QueueListItem{
				Name:      name,
				Deferred:  count,
				SampledAt: at,
			})
			snaps = append(snaps, QueueSnapshot{
				At:       at,
				Queue:    name,
				Deferred: count,
			})
			if limit > 0 && len(out) >= limit {
				break
			}
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

// ScheduledQueueMessage is the API-friendly shape of one entry in the
// scheduled queue inspection. We pass the kumomta meta map through verbatim
// so operators can see tenant/campaign/source headers without a schema
// change every time kumomta adds a field.
type ScheduledQueueMessage struct {
	ID          string
	Sender      string
	Recipient   string
	DueAt       time.Time
	NumAttempts uint32
	Tenant      string
	Campaign    string
	Meta        map[string]any
}

// InspectScheduledQueue returns a sample of messages currently held in the
// named scheduled queue. limit ≤ 0 falls back to a small default; the
// upstream is rate-sensitive so we cap at a sane maximum.
func (s *QueueService) InspectScheduledQueue(ctx context.Context, name string, limit int) ([]ScheduledQueueMessage, error) {
	if name == "" {
		return nil, ErrQueueNameRequired
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	r, err := s.client.InspectScheduledQueue(ctx, name, limit, false)
	if err != nil {
		return nil, err
	}
	out := make([]ScheduledQueueMessage, 0, len(r.Messages))
	for _, m := range r.Messages {
		// Due is RFC3339 from kumomta; tolerate a missing/odd value
		// rather than failing the whole listing on one bad row.
		due, _ := time.Parse(time.RFC3339Nano, m.Message.Due)
		out = append(out, ScheduledQueueMessage{
			ID:          m.ID,
			Sender:      m.Message.Sender,
			Recipient:   m.Message.Recipient,
			DueAt:       due,
			NumAttempts: m.Message.NumAttempts,
			Tenant:      metaString(m.Message.Meta, "tenant"),
			Campaign:    metaString(m.Message.Meta, "campaign"),
			Meta:        m.Message.Meta,
		})
	}
	return out, nil
}

func metaString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
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
