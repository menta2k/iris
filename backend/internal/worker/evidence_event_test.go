package worker

import (
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

func TestMailRecordEvent(t *testing.T) {
	mr := &biz.MailRecord{
		MessageID: "m1", Recipient: "u@x.com", RecipientDomain: "x.com",
		Status: "deferred", RecordType: "TransientFailure", SMTPStatus: "400",
		Node: "kmx", Diagnostic: "HandshakeFailure", EventTime: time.Unix(1700000000, 0).UTC(),
	}
	ev := mailRecordEvent(mr)
	for k, want := range map[string]any{
		"message_id": "m1", "recipient": "u@x.com", "status": "deferred",
		"record_type": "TransientFailure", "smtp_status": "400", "node": "kmx",
		"diagnostic": "HandshakeFailure",
	} {
		if ev[k] != want {
			t.Fatalf("event[%q] = %v, want %v", k, ev[k], want)
		}
	}
	if ev["event_time"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("event_time = %v", ev["event_time"])
	}
}

func TestBounceEvent(t *testing.T) {
	b := &biz.BounceRecord{
		Recipient: "u@x.com", SMTPStatus: "550", BounceType: "hard",
		Classification: "mailbox_full", Diagnostic: "550 5.2.2 over quota",
		EventTime: time.Unix(1700000000, 0).UTC(),
	}
	ev := bounceEvent(b, "m9")
	if ev["message_id"] != "m9" || ev["smtp_status"] != "550" || ev["bounce_type"] != "hard" ||
		ev["classification"] != "mailbox_full" {
		t.Fatalf("bounce event = %v", ev)
	}
}
