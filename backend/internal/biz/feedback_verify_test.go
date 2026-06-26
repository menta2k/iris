package biz

import (
	"bytes"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"
)

// signedOriginal returns a small message DKIM-signed by the given key, as the
// embedded "original" of a feedback report would appear.
func signedOriginal(t *testing.T, pem, domain, selector string) string {
	t.Helper()
	key, err := ParseDKIMPrivateKey(pem)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	raw := "From: news@" + domain + "\r\n" +
		"To: user@recipient.example\r\n" +
		"Subject: Newsletter\r\n" +
		"Message-ID: <msg-123@" + domain + ">\r\n" +
		"\r\nhello there\r\n"
	var buf bytes.Buffer
	opts := &dkim.SignOptions{
		Domain:     domain,
		Selector:   selector,
		Signer:     key,
		HeaderKeys: []string{"From", "To", "Subject", "Message-ID"},
	}
	if err := dkim.Sign(&buf, strings.NewReader(raw), opts); err != nil {
		t.Fatalf("sign: %v", err)
	}
	// kumod normalizes the embedded original to LF; mimic that.
	return strings.ReplaceAll(buf.String(), "\r\n", "\n")
}

func fbRecord(orig, traceRcpt string, complaint string) *KumoLogRecord {
	fb := &KumoFeedbackData{
		FeedbackType:    "abuse",
		OriginalRcptTo:  []string{complaint},
		OriginalMessage: orig,
	}
	if traceRcpt != "" {
		fb.SupplementalTrace = &KumoSupplementalTrace{Recipient: flexStrings{traceRcpt}}
	}
	return &KumoLogRecord{Type: KumoFeedback, Feedback: fb}
}

func TestVerifyFeedback(t *testing.T) {
	pem, err := GenerateDKIMPrivateKey()
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	ourKey := func(domain, selector string) (string, bool) {
		if domain == "sender.example" && selector == "s1" {
			rec, _, err := DKIMPublicRecord(pem)
			if err != nil {
				return "", false
			}
			return rec, true
		}
		return "", false
	}
	signed := signedOriginal(t, pem, "sender.example", "s1")

	t.Run("supplemental-trace wins first", func(t *testing.T) {
		rec := fbRecord("", "user@recipient.example", "user@recipient.example")
		ok, method := VerifyFeedback(rec, ourKey, nil)
		if !ok || method != FeedbackVerifiedTrace {
			t.Fatalf("got ok=%v method=%q", ok, method)
		}
	})

	t.Run("send-log correlation", func(t *testing.T) {
		rec := fbRecord(signed, "", "user@recipient.example")
		sent := func(messageID string) string {
			if messageID == "msg-123@sender.example" {
				return "user@recipient.example"
			}
			return ""
		}
		ok, method := VerifyFeedback(rec, nil, sent)
		if !ok || method != FeedbackVerifiedSendLog {
			t.Fatalf("got ok=%v method=%q", ok, method)
		}
	})

	t.Run("dkim verified by our key", func(t *testing.T) {
		rec := fbRecord(signed, "", "user@recipient.example")
		ok, method := VerifyFeedback(rec, ourKey, nil)
		if !ok || method != FeedbackVerifiedDKIM {
			t.Fatalf("got ok=%v method=%q", ok, method)
		}
	})

	t.Run("dkim from a key we do not hold is rejected", func(t *testing.T) {
		rec := fbRecord(signed, "", "user@recipient.example")
		notOurs := func(domain, selector string) (string, bool) { return "", false }
		ok, method := VerifyFeedback(rec, notOurs, nil)
		if ok || method != "" {
			t.Fatalf("expected unverified, got ok=%v method=%q", ok, method)
		}
	})

	t.Run("junk / forged with no provenance is unverified", func(t *testing.T) {
		rec := fbRecord("From: spammer@elsewhere.test\nSubject: x\n\nbody\n", "", "victim@recipient.example")
		ok, method := VerifyFeedback(rec, ourKey, func(string) string { return "" })
		if ok || method != "" {
			t.Fatalf("expected unverified, got ok=%v method=%q", ok, method)
		}
	})
}
