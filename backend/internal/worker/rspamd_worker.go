package worker

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

const rspamdGroup = "iris-rspamd-ingest"

// RspamdStore persists ingested filter results.
type RspamdStore interface {
	IngestRspamdResult(ctx context.Context, res *biz.RspamdFilterResult) error
}

// RspamdWorker consumes Rspamd filter results from Redis and persists them.
type RspamdWorker struct {
	streams *data.Streams
	store   RspamdStore
	log     *slog.Logger
}

// NewRspamdWorker constructs the worker.
func NewRspamdWorker(streams *data.Streams, store RspamdStore, log *slog.Logger) *RspamdWorker {
	return &RspamdWorker{streams: streams, store: store, log: log}
}

// Run consumes Rspamd results until the context is cancelled.
func (w *RspamdWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, data.StreamRspamdResults, rspamdGroup); err != nil {
		return err
	}
	w.log.Info("rspamd worker started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("rspamd worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, data.StreamRspamdResults, rspamdGroup, 10, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume rspamd results", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, data.StreamRspamdResults, rspamdGroup, m.ID); err != nil {
				w.log.Error("ack rspamd result", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *RspamdWorker) handle(ctx context.Context, m data.StreamMessage) {
	score := 0.0
	if s, ok := m.Values["score"].(string); ok {
		score, _ = strconv.ParseFloat(s, 64)
	}
	res := &biz.RspamdFilterResult{
		MailRecordID: stringValue(m.Values["mail_record_id"]),
		Action:       stringValue(m.Values["action"]),
		Score:        score,
		Reason:       stringValue(m.Values["reason"]),
		RawRef:       stringValue(m.Values["raw_ref"]),
	}
	if err := w.store.IngestRspamdResult(ctx, res); err != nil {
		w.log.Error("ingest rspamd result", "id", m.ID, "error", err.Error())
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
