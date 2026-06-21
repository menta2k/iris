package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// serviceControlGroup is the Redis consumer group for service-control commands.
const serviceControlGroup = "iris-service-control"

// ServiceControlStore is the persistence the worker needs to advance request
// lifecycle state.
type ServiceControlStore interface {
	UpdateServiceControlStatus(ctx context.Context, id, status, resultSummary string) error
}

// ServiceControlWorker consumes service-control commands one at a time and
// applies them through the KumoMTA adapter. A single consumer enforces
// serialized execution so conflicting operations never overlap.
type ServiceControlWorker struct {
	streams *data.Streams
	store   ServiceControlStore
	kumo    biz.KumoMTAAdapter
	log     *slog.Logger
	timeout time.Duration
}

// NewServiceControlWorker constructs the worker.
func NewServiceControlWorker(streams *data.Streams, store ServiceControlStore, kumo biz.KumoMTAAdapter, log *slog.Logger) *ServiceControlWorker {
	return &ServiceControlWorker{streams: streams, store: store, kumo: kumo, log: log, timeout: 30 * time.Second}
}

// Run consumes commands until the context is cancelled.
func (w *ServiceControlWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, data.StreamServiceCommands, serviceControlGroup); err != nil {
		return err
	}
	w.log.Info("service-control worker started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("service-control worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, data.StreamServiceCommands, serviceControlGroup, 1, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume service commands", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, data.StreamServiceCommands, serviceControlGroup, m.ID); err != nil {
				w.log.Error("ack service command", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *ServiceControlWorker) handle(ctx context.Context, m data.StreamMessage) {
	requestID, _ := m.Values["request_id"].(string)
	operation, _ := m.Values["operation"].(string)
	if requestID == "" || operation == "" {
		w.log.Warn("malformed service command", "id", m.ID)
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()

	if err := w.store.UpdateServiceControlStatus(opCtx, requestID, biz.SvcRunning, ""); err != nil {
		w.log.Error("mark running", "request_id", requestID, "error", err.Error())
	}

	summary, err := w.kumo.ApplyServiceControl(opCtx, biz.ServiceOperation(operation))
	status, result := biz.SvcSucceeded, summary
	if err != nil {
		status = biz.SvcFailed
		if opCtx.Err() == context.DeadlineExceeded {
			status = biz.SvcTimedOut
		}
		result = fmt.Sprintf("error: %v", err)
		w.log.Error("service control failed", "request_id", requestID, "operation", operation, "error", err.Error())
	}
	// Use the parent ctx for the final status write so a per-op timeout does
	// not prevent recording the terminal state.
	if err := w.store.UpdateServiceControlStatus(ctx, requestID, status, result); err != nil {
		w.log.Error("mark terminal status", "request_id", requestID, "status", status, "error", err.Error())
	}
}
