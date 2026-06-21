// Package worker implements asynchronous Redis Streams producers and consumers
// for queue commands, service-control commands, webhook delivery, and event
// ingestion.
package worker

import (
	"context"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// OpsProducer publishes queue and service-control commands to Redis Streams.
type OpsProducer struct {
	streams *data.Streams
}

// NewOpsProducer constructs the producer. It satisfies biz.CommandProducer.
func NewOpsProducer(streams *data.Streams) *OpsProducer { return &OpsProducer{streams: streams} }

var _ biz.CommandProducer = (*OpsProducer)(nil)

// PublishQueueCommand enqueues a queue-control command.
func (p *OpsProducer) PublishQueueCommand(ctx context.Context, mailclass, action, confirmationID string) (string, error) {
	return p.streams.Publish(ctx, data.StreamQueueCommands, map[string]any{
		"mailclass":       mailclass,
		"action":          action,
		"confirmation_id": confirmationID,
	})
}

// PublishServiceCommand enqueues a serialized service-control command keyed by
// the persisted request id so the worker can update its lifecycle.
func (p *OpsProducer) PublishServiceCommand(ctx context.Context, requestID, operation string) (string, error) {
	return p.streams.Publish(ctx, data.StreamServiceCommands, map[string]any{
		"request_id": requestID,
		"operation":  operation,
	})
}
