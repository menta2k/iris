package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/data"
)

const classifyGroup = "iris-classify"

// SubjectClassifier resolves a ≤2-word label for a raw subject. Satisfied by
// biz.SubjectClassifierUsecase. Returns "" (no error) when nothing applies.
type SubjectClassifier interface {
	Classify(ctx context.Context, subject string) (string, error)
}

// ClassificationStore backfills a resolved label onto a message's event rows.
type ClassificationStore interface {
	UpdateClassification(ctx context.Context, messageID, label string) error
}

// ClassificationWorker consumes {message_id, subject} from the transient
// classify-pending stream (produced by the log worker when the feature is on),
// resolves a label (trigram match → LLM fallback), and writes only the label
// back onto mail_records by message_id. The subject never leaves this pipeline.
type ClassificationWorker struct {
	streams    *data.Streams
	classifier SubjectClassifier
	store      ClassificationStore
	stream     string
	log        *slog.Logger
}

// NewClassificationWorker constructs the worker. streamName defaults to
// data.StreamClassifyPending.
func NewClassificationWorker(streams *data.Streams, classifier SubjectClassifier, store ClassificationStore, streamName string, log *slog.Logger) *ClassificationWorker {
	if streamName == "" {
		streamName = data.StreamClassifyPending
	}
	return &ClassificationWorker{streams: streams, classifier: classifier, store: store, stream: streamName, log: log}
}

// Run consumes pending subjects until the context is cancelled. Instances share
// the consumer group. When the feature is off nothing is enqueued, so this loop
// simply idles on the blocking read.
func (w *ClassificationWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, w.stream, classifyGroup); err != nil {
		return err
	}
	w.log.Info("classification worker started", "stream", w.stream)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("classification worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, w.stream, classifyGroup, 50, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume classify stream", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, w.stream, classifyGroup, m.ID); err != nil {
				w.log.Error("ack classify record", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *ClassificationWorker) handle(ctx context.Context, m data.StreamMessage) {
	messageID, _ := m.Values["message_id"].(string)
	subject, _ := m.Values["subject"].(string)
	if messageID == "" || subject == "" {
		return
	}
	label, err := w.classifier.Classify(ctx, subject)
	if err != nil {
		// Transport/DB failure — log and drop (the message is still acked; a
		// missing label is non-critical and re-classifying every retry could
		// hammer the LLM).
		w.log.Warn("classify subject", "message_id", messageID, "error", err.Error())
		return
	}
	if label == "" {
		return
	}
	if err := w.store.UpdateClassification(ctx, messageID, label); err != nil {
		w.log.Error("persist classification", "message_id", messageID, "error", err.Error())
	}
}
