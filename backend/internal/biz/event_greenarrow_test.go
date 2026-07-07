package biz

import (
	"encoding/json"
	"testing"
	"time"
)

func bounceEvent(class, bounceType, smtp, diag string) DispatchEvent {
	return DispatchEvent{
		Type:       EventBounce,
		OccurredAt: time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
		Mailclass:  "acme_s",
		Data: map[string]any{
			"recipient": "invalid@example.com", "smtp_status": smtp,
			"diagnostic": diag, "classification": class, "bounce_type": bounceType,
			"message_id": "abc123", "egress_source": "vmta-03", "sender": "bounce@example.com",
		},
	}
}

// every key the ga_handler reads must be present so PHP never warns / null-casts.
var gaRequiredKeys = []string{
	"id", "event_type", "event_time", "email", "listid", "list_name", "list_label",
	"sendid", "bounce_type", "bounce_code", "bounce_text", "click_url", "click_tracking_id",
	"studio_rl_seq", "studio_rl_recipid", "studio_campaign_id", "studio_autoresponder_id",
	"studio_is_unique", "studio_mailing_list_id", "studio_subscriber_id", "studio_ip",
	"studio_rl_seq_id", "studio_rl_distinct_id", "engine_ip", "user_agent", "json_before", "json_after",
}

func assertGAKeys(t *testing.T, m map[string]any) {
	t.Helper()
	for _, k := range gaRequiredKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s event", k, m["event_type"])
		}
	}
	if id, _ := m["id"].(int64); id == 0 {
		t.Errorf("id must be non-zero (consumer drops zero-id events); got %v", m["id"])
	}
}

func TestGreenArrowBadAddressBounce(t *testing.T) {
	// Hard, bad-address bounce → bounce_all AND bounce_bad_address, both "h".
	evs := GreenArrowEvents(bounceEvent("InvalidRecipient", "hard", "550", "550 5.1.1 user unknown"))
	if len(evs) != 2 {
		t.Fatalf("want 2 events (bounce_all + bounce_bad_address), got %d", len(evs))
	}
	if evs[0]["event_type"] != GAEventBounceAll || evs[1]["event_type"] != GAEventBounceBadAddress {
		t.Fatalf("wrong types: %v, %v", evs[0]["event_type"], evs[1]["event_type"])
	}
	for _, m := range evs {
		assertGAKeys(t, m)
		if m["bounce_type"] != "h" {
			t.Errorf("bad-address bounce must be type h, got %v", m["bounce_type"])
		}
		if m["email"] != "invalid@example.com" || m["bounce_code"] != 550 {
			t.Errorf("email/code wrong: %+v", m)
		}
	}
	if evs[0]["id"] == evs[1]["id"] {
		t.Errorf("each event needs a distinct id, both are %v", evs[0]["id"])
	}
}

func TestGreenArrowSpamBlockNotBadAddress(t *testing.T) {
	// Hard block that is NOT a bad address → only bounce_all, so no deactivation.
	evs := GreenArrowEvents(bounceEvent("SpamBlock", "hard", "554", "554 5.7.1 blocked"))
	if len(evs) != 1 || evs[0]["event_type"] != GAEventBounceAll {
		t.Fatalf("spam block must yield only bounce_all, got %d: %v", len(evs), evs)
	}
}

func TestGreenArrowSoftBounce(t *testing.T) {
	evs := GreenArrowEvents(bounceEvent("QuotaIssues", "soft", "452", "452 4.2.2 mailbox full"))
	if len(evs) != 1 || evs[0]["bounce_type"] != "s" {
		t.Fatalf("soft bounce → one bounce_all type s, got %v", evs)
	}
}

func TestGreenArrowFeedbackToScomp(t *testing.T) {
	ev := DispatchEvent{
		Type: EventFeedbackReport, OccurredAt: time.Now().UTC(), Mailclass: "acme_a",
		Data: map[string]any{"recipient": "customer@example.com", "report_type": "abuse"},
	}
	evs := GreenArrowEvents(ev)
	if len(evs) != 1 || evs[0]["event_type"] != GAEventSComp {
		t.Fatalf("feedback → one scomp, got %v", evs)
	}
	assertGAKeys(t, evs[0])
	if evs[0]["email"] != "customer@example.com" || evs[0]["bounce_type"] != "" {
		t.Errorf("scomp shape wrong: %+v", evs[0])
	}
}

func TestGreenArrowIgnoresOtherEvents(t *testing.T) {
	for _, typ := range []string{EventDMARCReceived, EventSuppressionCreated} {
		if evs := GreenArrowEvents(DispatchEvent{Type: typ, OccurredAt: time.Now()}); evs != nil {
			t.Errorf("%s should yield no GreenArrow events, got %v", typ, evs)
		}
	}
}

// TestGreenArrowExamplePayload prints the exact wire array for review (go test -v).
func TestGreenArrowExamplePayload(t *testing.T) {
	batch := []map[string]any{}
	batch = append(batch, GreenArrowEvents(bounceEvent("InvalidRecipient", "hard", "550",
		"550 5.1.1 The email account that you tried to reach does not exist - gsmtp"))...)
	batch = append(batch, GreenArrowEvents(DispatchEvent{
		Type: EventFeedbackReport, OccurredAt: time.Date(2026, 7, 7, 10, 5, 0, 0, time.UTC),
		Mailclass: "acme_b", Data: map[string]any{"recipient": "customer@example.com"},
	})...)
	out, _ := json.MarshalIndent(batch, "", "  ")
	t.Logf("GreenArrow POST body:\n%s", out)
}
