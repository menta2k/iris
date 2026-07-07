package biz

import (
	"net/url"
	"strings"
	"time"
)

// Event types the Event Processor can forward to external services.
const (
	EventBounce             = "bounce"
	EventSuppressionCreated = "suppression_created"
	EventFeedbackReport     = "feedback_report"
	EventDMARCReceived      = "dmarc_received"
)

var eventTypes = map[string]struct{}{
	EventBounce: {}, EventSuppressionCreated: {},
	EventFeedbackReport: {}, EventDMARCReceived: {},
}

// Delivery drivers. New drivers register a factory under a new name here and in
// the driver registry — no other code changes required.
const (
	EventDriverWebhook = "webhook"
	EventDriverRedis   = "redis"
	// EventDriverGreenArrow speaks GreenArrow's Event-Notification wire format
	// (bare JSON array of per-event objects) so an existing ga_handler endpoint
	// can ingest iris bounces/complaints unchanged. Bounce-and-complaint only.
	EventDriverGreenArrow = "greenarrow"
)

// Firing modes: one delivery per event, or accumulate and deliver in batches.
const (
	EventModeSingle = "single"
	EventModeBatch  = "batch"
)

// EventProcessor is an operator-defined rule: when an event of a matching type
// (and mailclass) occurs, forward it via the configured driver, either
// immediately (single) or accumulated (batch).
type EventProcessor struct {
	ID   string
	Name string
	// EventTypes is the set of events this processor forwards (non-empty).
	EventTypes []string
	// Mailclasses restricts matching to these classes; empty matches all classes.
	Mailclasses []string
	// Driver is the delivery mechanism (webhook | redis | …).
	Driver string
	// DriverConfig holds driver-specific settings (webhook: url/secret/headers/
	// timeout; redis: addr/stream). Kept as a string map so new drivers add keys
	// without a schema change.
	DriverConfig map[string]string
	Mode         string // single | batch
	// BatchMaxSize / BatchMaxWait bound a batch: it flushes when it reaches the
	// size or the oldest buffered event exceeds the wait, whichever comes first.
	BatchMaxSize int
	BatchMaxWait string // duration (e.g. "5s", "1m"); empty = size-only
	Status       string // active | disabled
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Matches reports whether an event of the given type/mailclass should be
// forwarded by this processor.
func (p *EventProcessor) Matches(eventType, mailclass string) bool {
	if p.Status != "" && p.Status != "active" {
		return false
	}
	if !containsFold(p.EventTypes, eventType) {
		return false
	}
	if len(p.Mailclasses) > 0 && !containsFold(p.Mailclasses, mailclass) {
		return false
	}
	return true
}

// batchWait resolves the batch flush window (0 when unset/size-only).
func (p *EventProcessor) batchWait() time.Duration {
	d, _ := ParseFlexDuration(p.BatchMaxWait)
	return d
}

// ValidateEventProcessor normalizes and checks a processor before persistence.
func ValidateEventProcessor(p *EventProcessor) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return Invalid("EVENT_PROCESSOR_NAME_REQUIRED", "name is required")
	}
	p.EventTypes = cleanList(p.EventTypes)
	if len(p.EventTypes) == 0 {
		return Invalid("EVENT_PROCESSOR_TYPES_REQUIRED", "at least one event type is required")
	}
	for _, t := range p.EventTypes {
		if _, ok := eventTypes[t]; !ok {
			return Invalid("EVENT_PROCESSOR_TYPE_INVALID", "event type %q is not valid", t)
		}
	}
	p.Mailclasses = cleanList(p.Mailclasses)

	switch p.Driver {
	case EventDriverWebhook:
		raw := strings.TrimSpace(p.DriverConfig["url"])
		u, err := url.Parse(raw)
		if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Invalid("EVENT_PROCESSOR_WEBHOOK_URL_INVALID", "webhook driver requires a valid http(s) url")
		}
	case EventDriverRedis:
		if strings.TrimSpace(p.DriverConfig["stream"]) == "" {
			return Invalid("EVENT_PROCESSOR_REDIS_STREAM_REQUIRED", "redis driver requires a stream name")
		}
	case EventDriverGreenArrow:
		raw := strings.TrimSpace(p.DriverConfig["url"])
		u, err := url.Parse(raw)
		if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Invalid("EVENT_PROCESSOR_GREENARROW_URL_INVALID", "greenarrow driver requires a valid http(s) url")
		}
		// GreenArrow's consumer only branches on bounces and complaints; other
		// iris events have no GreenArrow representation, so reject them early.
		for _, t := range p.EventTypes {
			if t != EventBounce && t != EventFeedbackReport {
				return Invalid("EVENT_PROCESSOR_GREENARROW_TYPE_INVALID",
					"greenarrow driver supports only bounce and feedback_report events, not %q", t)
			}
		}
	default:
		return Invalid("EVENT_PROCESSOR_DRIVER_INVALID", "driver %q is not valid", p.Driver)
	}

	if !ValidEventFormat(p.DriverConfig["format"]) {
		return Invalid("EVENT_PROCESSOR_FORMAT_INVALID", "format %q is not valid", p.DriverConfig["format"])
	}

	if p.Mode == "" {
		p.Mode = EventModeSingle
	}
	if p.Mode != EventModeSingle && p.Mode != EventModeBatch {
		return Invalid("EVENT_PROCESSOR_MODE_INVALID", "mode must be single or batch")
	}
	if p.Mode == EventModeBatch {
		if p.BatchMaxSize <= 0 {
			p.BatchMaxSize = 100
		}
		p.BatchMaxWait = strings.TrimSpace(p.BatchMaxWait)
		if p.BatchMaxWait != "" {
			if _, ok := ParseFlexDuration(p.BatchMaxWait); !ok {
				return Invalid("EVENT_PROCESSOR_BATCH_WAIT_INVALID", "batch_max_wait %q is not a valid duration", p.BatchMaxWait)
			}
		}
	}
	if p.Status == "" {
		p.Status = "active"
	}
	return nil
}

func cleanList(list []string) []string {
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
