package fbl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleARF = "--bdry\r\n" +
	"Content-Type: text/plain\r\n\r\n" +
	"This is an abuse report. Email: complainant@example.com\r\n" +
	"--bdry\r\n" +
	"Content-Type: message/feedback-report\r\n\r\n" +
	"Feedback-Type: abuse\r\n" +
	"User-Agent: SomeMUA/1.2\r\n" +
	"Version: 1\r\n" +
	"Original-Rcpt-To: victim@example.com\r\n" +
	"Source-IP: 198.51.100.5\r\n" +
	"Reporting-MTA: dns; mta.reporter.example\r\n" +
	"Arrival-Date: Thu, 1 Jan 2026 00:00:00 +0000\r\n\r\n" +
	"--bdry\r\n" +
	"Content-Type: message/rfc822\r\n\r\n" +
	"Message-ID: <abc@example.com>\r\n" +
	"From: \"S\" <sender@example.com>\r\n" +
	"To: victim@example.com\r\n" +
	"Subject: hi\r\n\r\n" +
	"hello\r\n" +
	"--bdry--\r\n"

func TestParseHappyPath(t *testing.T) {
	headers := []string{`Content-Type: multipart/report; report-type=feedback-report; boundary="bdry"`}
	rep, err := Parse(headers, strings.NewReader(sampleARF))
	require.NoError(t, err)
	require.Equal(t, "abuse", rep.FeedbackType)
	require.Equal(t, "SomeMUA/1.2", rep.UserAgent)
	require.Equal(t, "victim@example.com", rep.OriginalRecipient)
	require.Equal(t, "198.51.100.5", rep.SourceIP)
	require.Equal(t, "mta.reporter.example", rep.ReportingMTA)
	require.Equal(t, "abc@example.com", rep.OriginalMessageID)
	require.Equal(t, "sender@example.com", rep.OriginalSender)
	require.False(t, rep.ArrivalDate.IsZero())
	require.Contains(t, rep.RedactedBody, "[REDACTED]@example.com")
	require.NotContains(t, rep.RedactedBody, "complainant@example.com")
}

func TestParseRejectsNonMultipart(t *testing.T) {
	headers := []string{`Content-Type: text/plain`}
	_, err := Parse(headers, strings.NewReader("hi"))
	require.ErrorIs(t, err, ErrNotMultipart)
}

func TestParseRejectsTooLarge(t *testing.T) {
	headers := []string{`Content-Type: multipart/report; boundary=bdry`}
	big := bytes.Repeat([]byte("x"), MaxReportBytes+10)
	_, err := Parse(headers, bytes.NewReader(big))
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestParseMissingFeedbackPart(t *testing.T) {
	body := "--bdry\r\nContent-Type: text/plain\r\n\r\nhi\r\n--bdry--\r\n"
	headers := []string{`Content-Type: multipart/report; boundary="bdry"`}
	_, err := Parse(headers, strings.NewReader(body))
	require.ErrorIs(t, err, ErrMissingPart)
}

func TestRedactPreservesDomain(t *testing.T) {
	in := "User: alice@bad.example said something"
	out := redact(in)
	require.Equal(t, "User: [REDACTED]@bad.example said something", out)
}
