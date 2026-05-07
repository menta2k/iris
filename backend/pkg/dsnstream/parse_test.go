package dsnstream

import (
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/pkg/verp"
)

const sampleDSNGmail = `Return-Path: <>
Delivered-To: bounces+TOKEN@bounces.example.com
From: Mail Delivery Subsystem <mailer-daemon@gmail.com>
To: bounces+TOKEN@bounces.example.com
Subject: Delivery Status Notification (Failure)
MIME-Version: 1.0
Content-Type: multipart/report; boundary="000_BOUND"; report-type=delivery-status

--000_BOUND
Content-Type: text/plain; charset=utf-8

Address not found.

--000_BOUND
Content-Type: message/delivery-status

Reporting-MTA: dns; gmail-smtp-in.l.google.com
Received-From-MTA: dns; mta.example.com

Final-Recipient: rfc822; alice@gmail.com
Action: failed
Status: 5.1.1
Diagnostic-Code: smtp; 550-5.1.1 The email account that you tried to reach does not exist.
Remote-MTA: dns; gmail-smtp-in.l.google.com

--000_BOUND
Content-Type: message/rfc822

From: Marketing <news@example.com>
To: alice@gmail.com
Message-ID: <ORIG-MID@example.com>
X-Kumo-Mail-Class: marketing
X-Kumo-Tenant: marketing
Subject: Hello

(body)
--000_BOUND--
`

const sampleDSNNoVerp = `Return-Path: <>
From: postmaster@example.org
To: bounces@bounces.example.com
Subject: bounce
MIME-Version: 1.0
Content-Type: multipart/report; boundary="X"; report-type=delivery-status

--X
Content-Type: text/plain

User unknown.

--X
Content-Type: message/delivery-status

Reporting-MTA: dns; mx.example.org

Final-Recipient: rfc822; bob@example.org
Action: failed
Status: 5.1.1
Diagnostic-Code: smtp; 550 unknown user

--X
Content-Type: text/rfc822-headers

From: Support <support@example.com>
To: bob@example.org
Message-ID: <FALLBACK-MID@example.com>
X-Kumo-Mail-Class: transactional

--X--
`

func TestParseGmailStyle(t *testing.T) {
	const secret = "supersecret"
	tok, err := verp.Encode(secret, "cd7b9a40e3")
	if err != nil {
		t.Fatalf("verp encode: %v", err)
	}
	body := strings.ReplaceAll(sampleDSNGmail, "TOKEN", tok)
	envelope := "b+" + tok + "@bounces.example.com"

	p, err := Parse(envelope, secret, []byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if p.MessageID != "cd7b9a40e3" {
		t.Errorf("MessageID via VERP: got %q want cd7b9a40e3", p.MessageID)
	}
	if p.VerpToken != tok {
		t.Errorf("VerpToken: got %q want %q", p.VerpToken, tok)
	}
	if p.Action != "failed" {
		t.Errorf("Action: got %q", p.Action)
	}
	if p.Status != "5.1.1" {
		t.Errorf("Status: got %q", p.Status)
	}
	if p.FinalRecipient != "alice@gmail.com" {
		t.Errorf("FinalRecipient: got %q", p.FinalRecipient)
	}
	if p.RemoteMTA != "gmail-smtp-in.l.google.com" {
		t.Errorf("RemoteMTA: got %q", p.RemoteMTA)
	}
	if !strings.Contains(p.DiagnosticCode, "5.1.1") {
		t.Errorf("DiagnosticCode missing status: %q", p.DiagnosticCode)
	}
	if p.MailClass() != "marketing" {
		t.Errorf("MailClass via embedded headers: got %q", p.MailClass())
	}
	if p.Tenant() != "marketing" {
		t.Errorf("Tenant: got %q", p.Tenant())
	}
}

func TestParseNoVerpFallsBackToEmbedded(t *testing.T) {
	// envelopeRcpt is the shared bounce address — no VERP token. Parser
	// should still recover MessageID + MailClass from the embedded
	// rfc822-headers part.
	p, err := Parse("bounces@bounces.example.com", "doesntmatter", []byte(sampleDSNNoVerp))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.VerpToken != "" {
		t.Errorf("VerpToken should be empty, got %q", p.VerpToken)
	}
	if p.MessageID != "FALLBACK-MID@example.com" {
		t.Errorf("MessageID via embedded: got %q", p.MessageID)
	}
	if p.MailClass() != "transactional" {
		t.Errorf("MailClass: got %q", p.MailClass())
	}
	if p.Status != "5.1.1" || p.FinalRecipient != "bob@example.org" {
		t.Errorf("DSN fields lost: status=%q final=%q", p.Status, p.FinalRecipient)
	}
}

func TestParseTolerantOfFreeformBody(t *testing.T) {
	// Some old MTAs send a non-multipart bounce. We should still get
	// VERP-derived correlation and not crash.
	const secret = "supersecret"
	tok, _ := verp.Encode(secret, "abc123")
	envelope := "b+" + tok + "@bounces.example.com"
	body := []byte("From: postmaster@old\r\nTo: " + envelope + "\r\nSubject: bounce\r\n\r\nFree-form body, no DSN structure.\r\n")
	p, err := Parse(envelope, secret, body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.MessageID != "abc123" {
		t.Errorf("VERP fallback: got %q want abc123", p.MessageID)
	}
	// Other fields will be empty — that's expected for a freeform body.
	if p.Action != "" || p.Status != "" {
		t.Errorf("expected empty Action/Status for freeform, got action=%q status=%q", p.Action, p.Status)
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if _, err := Parse("anything", "", nil); err == nil {
		t.Errorf("expected error on empty body")
	}
}
