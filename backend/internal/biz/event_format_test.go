package biz

import (
	"testing"
	"time"
)

func TestFormatEventGreenArrowBounceAll(t *testing.T) {
	ev := DispatchEvent{
		Type:       EventBounce,
		OccurredAt: time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		Mailclass:  "acme_s",
		Data: map[string]any{
			"recipient": "user@gmail.com", "smtp_status": "550", "diagnostic": "550 5.1.1 user unknown",
			"classification": "InvalidRecipient", "bounce_type": "hard",
			"message_id": "abc123", "egress_source": "vmta-03", "sender": "bounce@example.com",
		},
	}
	m := FormatEvent(FormatGreenArrowBounceAll, ev)

	if m["event_type"] != "bounce_all" {
		t.Fatalf("event_type = %v", m["event_type"])
	}
	if m["event_time"] != ev.OccurredAt.Unix() {
		t.Fatalf("event_time = %v, want %d", m["event_time"], ev.OccurredAt.Unix())
	}
	if m["email"] != "user@gmail.com" || m["sender"] != "bounce@example.com" {
		t.Fatalf("email/sender wrong: %v / %v", m["email"], m["sender"])
	}
	if m["bounce_type"] != "h" {
		t.Fatalf("hard bounce should map to 'h', got %v", m["bounce_type"])
	}
	if m["bounce_code"] != 550 {
		t.Fatalf("bounce_code = %v, want 550", m["bounce_code"])
	}
	if m["mailclass"] != "acme_s" || m["mtaid_name"] != "vmta-03" || m["sendid"] != "abc123" {
		t.Fatalf("mapping wrong: %+v", m)
	}
	if m["synchronous"] != true {
		t.Fatalf("synchronous should be true")
	}

	// A non-bounce event falls back to native even under the GreenArrow format.
	other := FormatEvent(FormatGreenArrowBounceAll, DispatchEvent{Type: EventDMARCReceived, OccurredAt: ev.OccurredAt})
	if other["type"] != EventDMARCReceived {
		t.Fatalf("non-bounce should be native, got %+v", other)
	}

	// Native format keeps iris's shape.
	nat := FormatEvent(FormatNative, ev)
	if nat["type"] != EventBounce || nat["mailclass"] != "acme_s" {
		t.Fatalf("native shape wrong: %+v", nat)
	}
}
