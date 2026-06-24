package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

const dmarcGroup = "iris-dmarc"

// DMARCStore persists parsed DMARC aggregate reports.
type DMARCStore interface {
	Ingest(ctx context.Context, report *biz.DMARCReport, records []biz.DMARCRecord) error
}

// DMARCWorker consumes raw DMARC aggregate-report messages captured at the
// configured report address, parses them, and persists the results.
type DMARCWorker struct {
	streams *data.Streams
	store   DMARCStore
	stream  string
	log     *slog.Logger
}

// NewDMARCWorker constructs the worker. streamName must match the policy's DMARC
// stream (defaults to biz.DMARCStreamName).
func NewDMARCWorker(streams *data.Streams, store DMARCStore, streamName string, log *slog.Logger) *DMARCWorker {
	if streamName == "" {
		streamName = biz.DMARCStreamName
	}
	return &DMARCWorker{streams: streams, store: store, stream: streamName, log: log}
}

// Run consumes DMARC report messages until the context is cancelled.
func (w *DMARCWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, w.stream, dmarcGroup); err != nil {
		return err
	}
	w.log.Info("dmarc worker started", "stream", w.stream)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("dmarc worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, w.stream, dmarcGroup, 20, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume dmarc stream", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			// Ack regardless: a malformed report must not wedge the stream.
			if err := w.streams.Ack(ctx, w.stream, dmarcGroup, m.ID); err != nil {
				w.log.Error("ack dmarc", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *DMARCWorker) handle(ctx context.Context, m data.StreamMessage) {
	raw, _ := m.Values["data"].(string)
	if raw == "" {
		w.log.Warn("dmarc message has no data", "id", m.ID)
		return
	}
	report, records, err := biz.ParseDMARCReport([]byte(raw))
	if err != nil {
		w.log.Warn("drop unparseable dmarc report", "id", m.ID, "error", err.Error())
		return
	}
	if err := w.store.Ingest(ctx, report, records); err != nil {
		w.log.Error("persist dmarc report", "org", report.OrgName, "report_id", report.ReportID, "error", err.Error())
		return
	}
	w.log.Info("dmarc report ingested", "domain", report.Domain, "org", report.OrgName, "records", len(records))
}
