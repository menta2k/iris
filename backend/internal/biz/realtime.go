package biz

import "context"

// RealtimePublisher pushes freshly-ingested records to connected UI clients
// (Server-Sent Events). It is best-effort and non-blocking: implementations
// must never block or error the ingestion path. The default is a no-op.
type RealtimePublisher interface {
	// PublishMailRecord announces a new mail-log record.
	PublishMailRecord(ctx context.Context, rec *MailRecord)
	// PublishBounce announces a new bounce record.
	PublishBounce(ctx context.Context, b *BounceRecord)
	// PublishProbe announces a created or updated inbox-monitoring probe.
	PublishProbe(ctx context.Context, p *MonitoringProbe)
}

// NoopRealtimePublisher discards all events (used when SSE is not wired).
type NoopRealtimePublisher struct{}

// PublishMailRecord does nothing.
func (NoopRealtimePublisher) PublishMailRecord(context.Context, *MailRecord) {}

// PublishBounce does nothing.
func (NoopRealtimePublisher) PublishBounce(context.Context, *BounceRecord) {}

// PublishProbe does nothing.
func (NoopRealtimePublisher) PublishProbe(context.Context, *MonitoringProbe) {}
