package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

const logStreamGroup = "iris-logstream"

// MailEventStore persists mail, bounce, and feedback events parsed from the
// KumoMTA log stream.
type MailEventStore interface {
	InsertMailEvent(ctx context.Context, rec *biz.MailRecord) error
	InsertBounce(ctx context.Context, b *biz.BounceRecord) error
	InsertFeedbackReport(ctx context.Context, f *biz.FeedbackReport) error
	// IncrementSoftBounce bumps and returns a recipient's soft-bounce count.
	IncrementSoftBounce(ctx context.Context, recipient string) (int, error)
	// RecipientForMessageID returns the original recipient for a sent message id
	// (used to correlate a VERP async bounce). "" when not found.
	RecipientForMessageID(ctx context.Context, messageID string) (string, error)
}

// Suppressor auto-suppresses a recipient. Used by the feedback-loop ingest and
// the bounce pipeline. Optional (nil disables it).
type Suppressor interface {
	SuppressRecipient(ctx context.Context, email, source, reason string) error
}

// BouncePolicyProvider supplies the current bounce-handling policy (auto-suppress
// hard bounces, soft-bounce threshold). Optional (nil = auto-suppress hard).
type BouncePolicyProvider interface {
	BouncePolicyNow(ctx context.Context) biz.BouncePolicy
}

// WebhookEnqueuer publishes an inbound mail event onto the webhook-delivery
// stream for the webhook worker to fan out. Optional (nil disables webhooks).
type WebhookEnqueuer interface {
	EnqueueWebhook(ctx context.Context, recipient, mailRecordID, payload string) error
}

// LogStreamWorker consumes KumoMTA structured log records from the Redis stream
// (produced by the generated policy's log_hook) and persists them into the
// mail_records / bounce_records hypertables. This is how the Logs UI is
// populated — from KumoMTA's own logs, not manual inserts.
type LogStreamWorker struct {
	streams    *data.Streams
	store      MailEventStore
	suppressor Suppressor
	policy     BouncePolicyProvider
	webhooks   WebhookEnqueuer
	stream     string
	log        *slog.Logger
}

// WithWebhooks enables inbound-webhook fan-out: each received message is
// published to the webhook-delivery stream. Returns the worker for chaining.
func (w *LogStreamWorker) WithWebhooks(e WebhookEnqueuer) *LogStreamWorker {
	w.webhooks = e
	return w
}

// enqueueWebhook publishes a received message onto the webhook-delivery stream.
// Best-effort: a failure is logged but never blocks log ingestion.
func (w *LogStreamWorker) enqueueWebhook(ctx context.Context, mr *biz.MailRecord) {
	if w.webhooks == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"event":            "reception",
		"message_id":       mr.MessageID,
		"sender":           mr.Sender,
		"recipient":        mr.Recipient,
		"recipient_domain": mr.RecipientDomain,
		"mailclass":        mr.Mailclass,
		"event_time":       mr.EventTime.UTC().Format(time.RFC3339),
	})
	if err != nil {
		w.log.Error("marshal webhook payload", "error", err.Error())
		return
	}
	if err := w.webhooks.EnqueueWebhook(ctx, mr.Recipient, mr.ID, string(payload)); err != nil {
		w.log.Error("enqueue webhook", "recipient", mr.Recipient, "error", err.Error())
	}
}

// NewLogStreamWorker constructs the worker. streamName must match the policy's
// LogStreamName (defaults to data.StreamMailEvents). suppressor/policy may be
// nil to disable auto-suppression (a nil policy auto-suppresses hard bounces).
func NewLogStreamWorker(streams *data.Streams, store MailEventStore, suppressor Suppressor, policy BouncePolicyProvider, streamName string, log *slog.Logger) *LogStreamWorker {
	if streamName == "" {
		streamName = data.StreamMailEvents
	}
	return &LogStreamWorker{streams: streams, store: store, suppressor: suppressor, policy: policy, stream: streamName, log: log}
}

func (w *LogStreamWorker) bouncePolicy(ctx context.Context) biz.BouncePolicy {
	if w.policy == nil {
		return biz.BouncePolicy{AutoSuppressHardBounces: true}
	}
	return w.policy.BouncePolicyNow(ctx)
}

// Run consumes log records until the context is cancelled. Multiple instances
// share the consumer group.
func (w *LogStreamWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, w.stream, logStreamGroup); err != nil {
		return err
	}
	w.log.Info("log-stream worker started", "stream", w.stream)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("log-stream worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, w.stream, logStreamGroup, 100, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume log stream", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, w.stream, logStreamGroup, m.ID); err != nil {
				w.log.Error("ack log record", "id", m.ID, "error", err.Error())
			}
		}
	}
}

// handleFeedback persists an ARF feedback report and auto-suppresses the
// complainant (the reference build's "FBL events automatically add a
// suppression entry as a side effect" behavior).
func (w *LogStreamWorker) handleFeedback(ctx context.Context, rec *biz.KumoLogRecord, now time.Time) {
	recipient := rec.ComplainantRecipient()
	if err := w.store.InsertFeedbackReport(ctx, &biz.FeedbackReport{
		ReceivedAt:      rec.EventTime(now),
		Source:          rec.FeedbackSource(),
		ReportType:      rec.FeedbackReportType(),
		Recipient:       recipient,
		ProcessingState: biz.ProcessingProcessed,
	}); err != nil {
		w.log.Error("persist feedback report", "error", err.Error())
		return
	}
	if w.suppressor != nil && recipient != "" {
		if err := w.suppressor.SuppressRecipient(ctx, recipient, "fbl", "feedback complaint ("+rec.FeedbackReportType()+")"); err != nil {
			w.log.Error("auto-suppress complainant", "recipient", recipient, "error", err.Error())
		}
	}
}

func (w *LogStreamWorker) handle(ctx context.Context, m data.StreamMessage) {
	// The policy XADDs each record with fields type=<EventType> and data=<json>.
	payload, _ := m.Values["data"].(string)
	if payload == "" {
		return
	}
	rec, err := biz.ParseKumoLogRecord([]byte(payload))
	if err != nil {
		w.log.Warn("drop malformed log record", "id", m.ID, "error", err.Error())
		return
	}
	now := time.Now().UTC()

	// Feedback (ARF/FBL) complaints: persist the report and auto-suppress the
	// complainant so future mail to that address is blocked.
	if rec.Type == biz.KumoFeedback {
		w.handleFeedback(ctx, rec, now)
		return
	}

	status := rec.MailStatus()
	if status == "" {
		// Other non-mail records are not stored as mail events.
		return
	}

	mr := &biz.MailRecord{
		MessageID:       rec.ID,
		EventTime:       rec.EventTime(now),
		Mailclass:       rec.Mailclass(),
		Sender:          rec.Sender,
		FromHeader:      rec.FromHeader(),
		Recipient:       rec.Recipient,
		RecipientDomain: rec.RecipientDomainOf(),
		Status:          status,
		Diagnostic:      strings.TrimSpace(rec.Response.Content),
	}
	// Carry the SMTP response code (e.g. 4xx on a deferral) so the Logs UI can
	// show why a message deferred/bounced, not just that it did.
	if rec.Response.Code > 0 {
		mr.SMTPStatus = strconv.Itoa(int(rec.Response.Code))
	}
	if err := w.store.InsertMailEvent(ctx, mr); err != nil {
		w.log.Error("persist mail event", "type", rec.Type, "error", err.Error())
		return
	}

	// Metrics: mail events by status/class/domain, and outbound events by VMTA
	// (egress source is present on Delivery/Bounce, absent on Reception).
	metrics.RecordMailEvent(mr.Status, mr.Mailclass, mr.RecipientDomain)
	metrics.RecordVMTAEvent(rec.EgressSource, mr.Status)

	// A received message is the trigger for inbound webhooks: enqueue a delivery
	// event so the webhook worker can fan it out to matching destinations.
	if rec.Type == biz.KumoReception {
		w.enqueueWebhook(ctx, mr)
	}

	if rec.Type == biz.KumoBounce {
		smtp := ""
		if rec.Response.Code > 0 {
			smtp = strconv.Itoa(int(rec.Response.Code))
		}
		bounce := &biz.BounceRecord{
			EventTime:       rec.EventTime(now),
			Recipient:       rec.Recipient,
			Mailclass:       rec.Mailclass(),
			SMTPStatus:      smtp,
			Diagnostic:      rec.Response.Content,
			Classification:  rec.BounceClassification,
			ProcessingState: biz.ProcessingNew,
		}
		if err := w.store.InsertBounce(ctx, bounce); err != nil {
			w.log.Error("persist bounce", "error", err.Error())
		}
		bounceType := "soft"
		if bounce.IsHardBounce() {
			bounceType = "hard"
		}
		metrics.RecordBounce(bounceType, bounce.Mailclass)
		w.applyBouncePolicy(ctx, bounce)
	}
}

// applyBouncePolicy auto-suppresses the recipient on a hard bounce (5xx) or once
// soft bounces reach the configured threshold.
func (w *LogStreamWorker) applyBouncePolicy(ctx context.Context, b *biz.BounceRecord) {
	if w.suppressor == nil {
		return
	}
	recipient := strings.ToLower(strings.TrimSpace(b.Recipient))
	if recipient == "" {
		return
	}
	policy := w.bouncePolicy(ctx)

	if b.IsHardBounce() {
		// Don't suppress an otherwise-valid recipient when the hard failure is
		// not their fault (spam block, quota, policy) per the classifier.
		if policy.AutoSuppressHardBounces && b.ShouldSuppressOnHardBounce() {
			reason := "hard bounce " + b.SMTPStatus
			if b.Classification != "" {
				reason += " (" + b.Classification + ")"
			}
			if err := w.suppressor.SuppressRecipient(ctx, recipient, "bounce", reason); err != nil {
				w.log.Error("auto-suppress hard bounce", "recipient", recipient, "error", err.Error())
			}
		}
		return
	}
	// Soft bounce: count toward the threshold (0 = disabled).
	if policy.SoftBounceThreshold <= 0 {
		return
	}
	count, err := w.store.IncrementSoftBounce(ctx, recipient)
	if err != nil {
		w.log.Error("increment soft bounce", "recipient", recipient, "error", err.Error())
		return
	}
	if count >= policy.SoftBounceThreshold {
		if err := w.suppressor.SuppressRecipient(ctx, recipient, "bounce",
			"soft bounce threshold reached ("+strconv.Itoa(count)+")"); err != nil {
			w.log.Error("auto-suppress soft bounce", "recipient", recipient, "error", err.Error())
		}
	}
}
