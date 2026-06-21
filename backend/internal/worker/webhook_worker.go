package worker

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

const webhookGroup = "iris-webhook-delivery"

// WebhookProducer publishes inbound mail events onto the webhook-delivery stream
// (the source the webhook worker consumes). It satisfies WebhookEnqueuer.
type WebhookProducer struct {
	streams *data.Streams
}

// NewWebhookProducer constructs the producer.
func NewWebhookProducer(streams *data.Streams) *WebhookProducer {
	return &WebhookProducer{streams: streams}
}

// EnqueueWebhook publishes a delivery event keyed by recipient.
func (p *WebhookProducer) EnqueueWebhook(ctx context.Context, recipient, mailRecordID, payload string) error {
	_, err := p.streams.Publish(ctx, data.StreamWebhookDelivery, map[string]any{
		"recipient":      recipient,
		"mail_record_id": mailRecordID,
		"payload":        payload,
	})
	return err
}

// WebhookStore records delivery attempts and resolves matching rules.
type WebhookStore interface {
	MatchWebhookRules(ctx context.Context, recipient string) ([]*biz.WebhookRule, error)
	RecordDelivery(ctx context.Context, e *biz.WebhookDeliveryEvent) error
}

// WebhookWorker consumes inbound mail events from Redis and delivers them to
// matching webhook destinations with bounded timeouts. Failed deliveries are
// recorded with a next-retry time and the message is acknowledged; a dedicated
// retry stream (dead-letter) is used for exhausted attempts.
type WebhookWorker struct {
	streams *data.Streams
	store   WebhookStore
	client  *http.Client
	log     *slog.Logger
}

// NewWebhookWorker constructs the worker.
func NewWebhookWorker(streams *data.Streams, store WebhookStore, log *slog.Logger) *WebhookWorker {
	return &WebhookWorker{
		streams: streams,
		store:   store,
		client:  &http.Client{Timeout: 30 * time.Second},
		log:     log,
	}
}

// Run consumes webhook delivery work until the context is cancelled. Multiple
// instances can run concurrently; the consumer group distributes work.
func (w *WebhookWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, data.StreamWebhookDelivery, webhookGroup); err != nil {
		return err
	}
	w.log.Info("webhook worker started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("webhook worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, data.StreamWebhookDelivery, webhookGroup, 10, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume webhook deliveries", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, data.StreamWebhookDelivery, webhookGroup, m.ID); err != nil {
				w.log.Error("ack webhook delivery", "id", m.ID, "error", err.Error())
			}
		}
	}
}

func (w *WebhookWorker) handle(ctx context.Context, m data.StreamMessage) {
	recipient, _ := m.Values["recipient"].(string)
	mailRecordID, _ := m.Values["mail_record_id"].(string)
	payload, _ := m.Values["payload"].(string)
	if recipient == "" {
		w.log.Warn("webhook event missing recipient", "id", m.ID)
		return
	}

	rules, err := w.store.MatchWebhookRules(ctx, recipient)
	if err != nil {
		w.log.Error("match webhook rules", "error", err.Error())
		return
	}
	for _, rule := range rules {
		w.deliver(ctx, rule, mailRecordID, []byte(payload))
	}
}

func (w *WebhookWorker) deliver(ctx context.Context, rule *biz.WebhookRule, mailRecordID string, payload []byte) {
	event := &biz.WebhookDeliveryEvent{
		WebhookRuleID: rule.ID,
		MailRecordID:  mailRecordID,
		Attempt:       1,
		Status:        biz.WebhookPending,
	}

	timeout := time.Duration(rule.TimeoutSeconds) * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, rule.DestinationURL, bytes.NewReader(payload))
	if err != nil {
		event.Status = biz.WebhookFailed
		event.ErrorSummary = "build request failed"
		w.record(ctx, rule, event)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iris-webhook/1")

	resp, err := w.client.Do(req)
	if err != nil {
		event.ResponseCode = 0
		event.Status = biz.WebhookRetrying
		next := time.Now().Add(time.Duration(rule.RetryPolicy.BackoffSeconds) * time.Second)
		event.NextRetryAt = &next
		event.ErrorSummary = fmt.Sprintf("request error: %v", err)
		w.record(ctx, rule, event)
		return
	}
	defer resp.Body.Close()

	event.ResponseCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		event.Status = biz.WebhookDelivered
	} else {
		event.Status = biz.WebhookRetrying
		next := time.Now().Add(time.Duration(rule.RetryPolicy.BackoffSeconds) * time.Second)
		event.NextRetryAt = &next
		event.ErrorSummary = fmt.Sprintf("non-2xx response: %d", resp.StatusCode)
	}
	w.record(ctx, rule, event)
}

func (w *WebhookWorker) record(ctx context.Context, rule *biz.WebhookRule, e *biz.WebhookDeliveryEvent) {
	metrics.RecordWebhookExecution(rule.Name, e.Status)
	if err := w.store.RecordDelivery(ctx, e); err != nil {
		w.log.Error("record webhook delivery", "error", err.Error())
	}
}
