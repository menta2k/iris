package worker

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

const dsnGroup = "iris-dsn"

// DSNWorker consumes inbound DSN (asynchronous bounce) messages captured at the
// configured bounce domain. Each entry carries the original recipient; the
// worker records a bounce and auto-suppresses the recipient (async bounces are
// treated as hard failures).
type DSNWorker struct {
	streams    *data.Streams
	store      MailEventStore
	suppressor Suppressor
	verpSecret string
	stream     string
	log        *slog.Logger
}

// NewDSNWorker constructs the worker. streamName must match the policy's DSN
// stream (defaults to biz.DSNStreamName). verpSecret decodes the VERP envelope
// return-path to the original recipient (empty = VERP off).
func NewDSNWorker(streams *data.Streams, store MailEventStore, suppressor Suppressor, verpSecret, streamName string, log *slog.Logger) *DSNWorker {
	if streamName == "" {
		streamName = biz.DSNStreamName
	}
	return &DSNWorker{streams: streams, store: store, suppressor: suppressor, verpSecret: verpSecret, stream: streamName, log: log}
}

// Run consumes DSN messages until the context is cancelled.
func (w *DSNWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, w.stream, dsnGroup); err != nil {
		return err
	}
	w.log.Info("dsn worker started", "stream", w.stream)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("dsn worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, w.stream, dsnGroup, 50, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume dsn stream", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, w.stream, dsnGroup, m.ID); err != nil {
				w.log.Error("ack dsn", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *DSNWorker) handle(ctx context.Context, m data.StreamMessage) {
	envelope := strings.ToLower(strings.TrimSpace(stringValue(m.Values["recipient"])))
	if envelope == "" {
		w.log.Warn("dsn missing recipient", "id", m.ID)
		return
	}

	// The envelope recipient is our VERP return-path. Decode it to the original
	// message id and resolve the recipient the bounce is actually for, so we
	// record/suppress the real address — not the bounce-domain address.
	recipient := envelope
	resolved := false
	if msgID, signed, ok := biz.ParseBounceVERP(w.verpSecret, envelope); ok {
		if !signed {
			w.log.Warn("dsn verp signature mismatch", "envelope", envelope)
		}
		if orig, err := w.store.RecipientForMessageID(ctx, msgID); err != nil {
			w.log.Error("dsn recipient lookup", "msgid", msgID, "error", err.Error())
		} else if orig != "" {
			recipient = strings.ToLower(strings.TrimSpace(orig))
			resolved = true
		} else {
			w.log.Warn("dsn verp message not found; not suppressing", "msgid", msgID)
		}
	}

	if err := w.store.InsertBounce(ctx, &biz.BounceRecord{
		EventTime:       time.Now().UTC(),
		Recipient:       recipient,
		SMTPStatus:      "550",
		BounceType:      "dsn",
		Diagnostic:      "asynchronous DSN at bounce domain",
		ProcessingState: biz.ProcessingProcessed,
	}); err != nil {
		w.log.Error("persist dsn bounce", "error", err.Error())
	}
	metrics.RecordBounce("dsn", "")

	// Only auto-suppress when we resolved a real recipient (or VERP was off and
	// the envelope is the actual recipient). Never suppress a bare VERP address.
	if w.suppressor != nil && (resolved || w.verpSecret == "") {
		if err := w.suppressor.SuppressRecipient(ctx, recipient, "dsn", "asynchronous bounce (DSN)"); err != nil {
			w.log.Error("auto-suppress dsn recipient", "recipient", recipient, "error", err.Error())
		}
	}
}
