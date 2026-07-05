package biz

import (
	"testing"
	"time"
)

func TestParseKumoLogRecord(t *testing.T) {
	rec, err := ParseKumoLogRecord([]byte(`{
		"type":"Delivery","id":"msg-1","timestamp":"2026-06-20T10:00:00Z",
		"sender":"a@send.example","recipient":"b@Dest.example","egress_pool":"bulk-pool",
		"response":{"code":250,"content":"OK"},"meta":{"tenant":"bulk-pool","mailclass":"bulk"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rec.MailStatus() != MailSent {
		t.Fatalf("expected sent, got %q", rec.MailStatus())
	}
	if rec.RecipientDomainOf() != "dest.example" {
		t.Fatalf("expected lowercased domain, got %q", rec.RecipientDomainOf())
	}
	// The class is the 'mailclass' meta (not the 'tenant'/egress pool).
	if rec.Mailclass() != "bulk" {
		t.Fatalf("expected mailclass from the mailclass meta, got %q", rec.Mailclass())
	}
	if got := rec.EventTime(time.Now()); got.Year() != 2026 || got.Hour() != 10 {
		t.Fatalf("unexpected event time: %v", got)
	}
}

func TestKumoLogRecordMailStatus(t *testing.T) {
	cases := map[string]string{
		KumoReception:        MailReceived,
		KumoDelivery:         MailSent,
		KumoBounce:           MailBounced,
		KumoTransientFailure: MailDeferred,
		KumoSuppressed:       MailSuppressed,
		"Rejection":          "", // untracked types map to no status
	}
	for recType, want := range cases {
		rec := &KumoLogRecord{Type: recType}
		if got := rec.MailStatus(); got != want {
			t.Errorf("MailStatus(%q) = %q, want %q", recType, got, want)
		}
	}
}

func TestParseSuppressedRecord(t *testing.T) {
	// Mirrors what the reception hook's iris_log_suppressed XADDs.
	rec, err := ParseKumoLogRecord([]byte(`{
		"type":"Suppressed","id":"msg-9","sender":"a@send.example",
		"recipient":"blocked@Dest.example","headers":{"From":"Real Sender <a@send.example>"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rec.MailStatus() != MailSuppressed {
		t.Fatalf("expected suppressed, got %q", rec.MailStatus())
	}
	if rec.RecipientDomainOf() != "dest.example" {
		t.Fatalf("expected lowercased domain, got %q", rec.RecipientDomainOf())
	}
	if rec.FromHeader() != "Real Sender <a@send.example>" {
		t.Fatalf("expected From header, got %q", rec.FromHeader())
	}
}

func TestKumoLogFromHeader(t *testing.T) {
	// The log hook captures From into headers; FromHeader recovers it past the
	// VERP-rewritten envelope sender.
	rec, err := ParseKumoLogRecord([]byte(`{
		"type":"Delivery","id":"m1",
		"sender":"b+abc.def@bounce.kumo.example.com","recipient":"x@dest.example",
		"headers":{"From":"Monitoring <sentry@infra.example.com>","Subject":"hi"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := rec.FromHeader(); got != "Monitoring <sentry@infra.example.com>" {
		t.Fatalf("unexpected From header: %q", got)
	}
	// Absent headers yield an empty string, not a panic.
	none, _ := ParseKumoLogRecord([]byte(`{"type":"Reception","id":"m2","sender":"a@b.example","recipient":"c@d.example"}`))
	if none.FromHeader() != "" {
		t.Fatalf("expected empty From header, got %q", none.FromHeader())
	}
}

func TestParseKumoFeedbackRecord(t *testing.T) {
	// Field names/types match KumoMTA's actual ARFReport: original_rcpto_to is a
	// list, reporting_mta is an object.
	rec, err := ParseKumoLogRecord([]byte(`{
		"type":"Feedback","recipient":"envelope@dest.example",
		"feedback_report":{"feedback_type":"ABUSE","original_rcpto_to":["Victim@Dest.Example"],
			"reporting_mta":{"mta_type":"dns","name":"fbl.provider.net"},"source_ip":"198.51.100.7"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rec.Type != KumoFeedback {
		t.Fatalf("expected Feedback type, got %q", rec.Type)
	}
	// Complainant comes from Original-Rcpt-To, lowercased.
	if rec.ComplainantRecipient() != "victim@dest.example" {
		t.Fatalf("unexpected complainant: %q", rec.ComplainantRecipient())
	}
	if rec.FeedbackReportType() != "abuse" {
		t.Fatalf("unexpected feedback type: %q", rec.FeedbackReportType())
	}
	if rec.FeedbackSource() != "fbl.provider.net" {
		t.Fatalf("unexpected source: %q", rec.FeedbackSource())
	}
}

func TestFeedbackDefaultsWithoutArfPart(t *testing.T) {
	// A Feedback record lacking the ARF sub-object falls back to the envelope
	// recipient and sane defaults.
	rec, _ := ParseKumoLogRecord([]byte(`{"type":"Feedback","recipient":"who@dest.example"}`))
	if rec.ComplainantRecipient() != "who@dest.example" {
		t.Fatalf("expected envelope recipient fallback, got %q", rec.ComplainantRecipient())
	}
	if rec.FeedbackReportType() != "complaint" || rec.FeedbackSource() != "fbl" {
		t.Fatalf("unexpected defaults: type=%q source=%q", rec.FeedbackReportType(), rec.FeedbackSource())
	}
}

func TestKumoLogStatusMapping(t *testing.T) {
	cases := map[string]string{
		KumoReception:        MailReceived,
		KumoDelivery:         MailSent,
		KumoBounce:           MailBounced,
		KumoTransientFailure: MailDeferred,
		KumoFeedback:         "", // not stored as a mail event
	}
	for typ, want := range cases {
		r := &KumoLogRecord{Type: typ}
		if got := r.MailStatus(); got != want {
			t.Fatalf("type %q: expected %q, got %q", typ, want, got)
		}
	}
}

func TestParseKumoLogRecordRejectsBadInput(t *testing.T) {
	if _, err := ParseKumoLogRecord([]byte("not json")); err == nil {
		t.Fatal("expected error for non-JSON")
	}
	big := make([]byte, KumoLogMaxBytes+1)
	for i := range big {
		big[i] = '{'
	}
	if _, err := ParseKumoLogRecord(big); err == nil {
		t.Fatal("expected error for oversized record")
	}
}

func TestKumoLogEventTimeFallbacks(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Numeric epoch.
	r, _ := ParseKumoLogRecord([]byte(`{"type":"Reception","timestamp":1750000000}`))
	if r.EventTime(now).Year() != 2025 {
		t.Fatalf("expected epoch 1750000000 → 2025, got %v", r.EventTime(now))
	}
	// Missing timestamp → now.
	r2 := &KumoLogRecord{Type: "Reception"}
	if !r2.EventTime(now).Equal(now) {
		t.Fatal("missing timestamp should fall back to now")
	}
}

func TestKumoLogQueueLatency(t *testing.T) {
	now := time.Now()
	// created → timestamp = 12s in the queue.
	r, _ := ParseKumoLogRecord([]byte(`{"type":"Delivery","id":"m1",` +
		`"created":"2026-06-20T10:00:00Z","timestamp":"2026-06-20T10:00:12Z"}`))
	d, ok := r.QueueLatency(now)
	if !ok || d != 12*time.Second {
		t.Fatalf("queue latency = %v ok=%v, want 12s", d, ok)
	}

	// Missing `created` → not available.
	noCreated, _ := ParseKumoLogRecord([]byte(`{"type":"Delivery","timestamp":"2026-06-20T10:00:12Z"}`))
	if _, ok := noCreated.QueueLatency(now); ok {
		t.Fatal("queue latency should be unavailable without created")
	}

	// Negative duration (clock skew) → dropped.
	skew, _ := ParseKumoLogRecord([]byte(`{"type":"Delivery",` +
		`"created":"2026-06-20T10:00:12Z","timestamp":"2026-06-20T10:00:00Z"}`))
	if _, ok := skew.QueueLatency(now); ok {
		t.Fatal("negative queue latency should be dropped")
	}
}
