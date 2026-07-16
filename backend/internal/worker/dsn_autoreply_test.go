package worker

import "testing"

func TestAutoReplyReason(t *testing.T) {
	// The real out-of-office reply that wrongly suppressed a recipient.
	ooo := "From: <a.rusateva@delfin93.com>\r\n" +
		"To: <b+69b1796cef3985c4.ff0095a7811b11f1806dbc2411ec5db8@bounce.kumo.economy.bg>\r\n" +
		"Subject: Out of office\r\n" +
		"Auto-Submitted: auto-replied (vacation)\r\n" +
		"Precedence: bulk\r\n" +
		"X-Auto-Response-Suppress: All\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"I will be out of office till 21 July.\r\n"
	if _, auto := autoReplyReason(ooo); !auto {
		t.Fatal("out-of-office vacation reply must be detected as an auto-reply")
	}

	// A microsoft OOO without Auto-Submitted but with the suppress header.
	msOOO := "From: <x@example.com>\r\nSubject: Automatic reply\r\nX-Auto-Response-Suppress: OOF, AutoReply\r\n\r\nbody\r\n"
	if _, auto := autoReplyReason(msOOO); !auto {
		t.Fatal("X-Auto-Response-Suppress must be detected as an auto-reply")
	}

	// A REAL DSN (auto-generated) must NOT be treated as an auto-reply — it is a
	// genuine bounce and must still suppress.
	dsn := "From: MAILER-DAEMON@example.com\r\n" +
		"Subject: Undelivered Mail Returned to Sender\r\n" +
		"Auto-Submitted: auto-generated\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=b\r\n" +
		"\r\n" +
		"--b\r\nyour message could not be delivered\r\n--b--\r\n"
	if reason, auto := autoReplyReason(dsn); auto {
		t.Fatalf("a real DSN (auto-generated) must NOT be skipped, got reason=%q", reason)
	}

	// A plain message with no auto-reply markers → process normally.
	plain := "From: user@example.com\r\nSubject: hi\r\n\r\nhello\r\n"
	if _, auto := autoReplyReason(plain); auto {
		t.Fatal("a plain message must not be treated as an auto-reply")
	}

	// Unparseable / empty → fall through (not an auto-reply) so real bounces are
	// never silently dropped.
	if _, auto := autoReplyReason(""); auto {
		t.Fatal("empty raw must not be treated as an auto-reply")
	}
}
