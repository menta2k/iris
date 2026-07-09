package biz

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// GreenArrow Event Notification event types iris can emit. These match the
// event_type strings a GreenArrow "event_delivery" consumer (e.g. a ga_handler
// endpoint) branches on: bad-address hard bounces drive account deactivation,
// scomp drives spam-complaint handling. bounce_all is emitted for parity/logging
// but carries no action on the consumer side.
const (
	GAEventBounceAll        = "bounce_all"
	GAEventBounceBadAddress = "bounce_bad_address"
	GAEventSComp            = "scomp"
)

// gaEventSeq yields a process-unique, monotonic, non-zero event id. The consumer
// stores it as the GAevents primary key and silently drops any event whose id is
// zero, so it must always be > 0 and unique. Seeded from the wall clock in
// microseconds so ids stay ascending across restarts and sit well above any
// legacy GreenArrow serial (store the column as BIGINT).
var gaEventSeq = func() *atomic.Int64 {
	v := &atomic.Int64{}
	v.Store(time.Now().UnixMicro())
	return v
}()

func nextGAEventID() int64 { return gaEventSeq.Add(1) }

// GreenArrowEvents converts one iris dispatch event into zero or more GreenArrow
// Event-Notification objects. A bounce yields a bounce_all always, plus a
// bounce_bad_address when the address is confirmed bad (the actionable event); a
// feedback report yields a scomp. Other iris event types yield nothing, since
// the GreenArrow consumer has no branch for them.
func GreenArrowEvents(ev DispatchEvent) []map[string]any {
	switch ev.Type {
	case EventBounce:
		return greenArrowBounceEvents(ev)
	case EventFeedbackReport:
		return []map[string]any{gaEvent(ev, gaStr(ev, "recipient"), GAEventSComp, nil)}
	default:
		return nil
	}
}

func greenArrowBounceEvents(ev DispatchEvent) []map[string]any {
	recipient := gaStr(ev, "recipient")
	diagnostic := gaStr(ev, "diagnostic")
	classification := gaStr(ev, "classification")
	hard := strings.EqualFold(gaStr(ev, "bounce_type"), "hard")

	bounceType := "s"
	if hard {
		bounceType = "h"
	}
	bounceFields := map[string]any{
		"bounce_type": bounceType,
		"bounce_code": gaAtoi(gaStr(ev, "smtp_status")), // log-only on the consumer
		"bounce_text": diagnostic,
	}

	out := []map[string]any{gaEvent(ev, recipient, GAEventBounceAll, bounceFields)}
	if hard && gaIsBadAddress(classification, diagnostic) {
		// bounce_bad_address is the event the consumer acts on: it must carry
		// bounce_type "h" to trigger deactivation. Fresh id — it is a distinct row.
		out = append(out, gaEvent(ev, recipient, GAEventBounceBadAddress, bounceFields))
	}
	return out
}

// gaEvent builds a full GreenArrow event object with every key the consumer
// reads present (empty/zero where iris has no equivalent), a fresh unique id, and
// the given type-specific overrides applied last.
func gaEvent(ev DispatchEvent, recipient, eventType string, extra map[string]any) map[string]any {
	m := map[string]any{
		"id":                      nextGAEventID(),
		"event_type":              eventType,
		"event_time":              ev.OccurredAt.Unix(),
		"email":                   recipient,
		"listid":                  "",
		"list_name":               "",
		"list_label":              "",
		"sendid":                  "",
		"bounce_type":             "",
		"bounce_code":             0,
		"bounce_text":             "",
		"click_url":               "",
		"click_tracking_id":       "",
		"studio_rl_seq":           0,
		"studio_rl_recipid":       "",
		"studio_campaign_id":      0,
		"studio_autoresponder_id": 0,
		"studio_is_unique":        0,
		"studio_mailing_list_id":  0,
		"studio_subscriber_id":    0,
		"studio_ip":               "",
		"studio_rl_seq_id":        0,
		"studio_rl_distinct_id":   0,
		"engine_ip":               "",
		"user_agent":              "",
		"json_before":             "",
		"json_after":              "",
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// gaIsBadAddress reports whether a hard bounce identifies a bad/non-existent
// mailbox (GreenArrow code 10 territory) — the signal that should deactivate the
// account. Gated on the classifier so a hard block (e.g. SpamBlock) never
// deactivates; falls back to the user-unknown enhanced status when the record
// carries no classification.
func gaIsBadAddress(classification, diagnostic string) bool {
	switch classification {
	case "InvalidRecipient", "InactiveMailbox":
		return true
	}
	if classification == "" {
		return strings.Contains(diagnostic, "5.1.1") || strings.Contains(diagnostic, "5.1.0")
	}
	return false
}

func gaStr(ev DispatchEvent, k string) string {
	v, _ := ev.Data[k].(string)
	return v
}

func gaAtoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
