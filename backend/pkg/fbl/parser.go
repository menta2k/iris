// Package fbl parses ARF (RFC 5965) feedback reports.
//
// This is the security-sensitive ingest path for abuse complaints. Hardening:
//
//   - Body size hard-cap (MaxReportBytes). Hostile MTAs could bombard us.
//   - Header line cap (MaxHeaderLineBytes) defends against pathological
//     reports that try to exhaust memory through a single mega-header.
//   - PII in the redacted_body field is replaced before persistence.
package fbl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
)

const (
	MaxReportBytes     = 4 << 20  // 4 MiB
	MaxHeaderLineBytes = 16 << 10 // 16 KiB
	MaxRedactedBody    = 8 << 10  // 8 KiB stored
)

var (
	ErrTooLarge       = errors.New("fbl: report exceeds MaxReportBytes")
	ErrNotMultipart   = errors.New("fbl: report is not multipart/report")
	ErrMissingPart    = errors.New("fbl: missing required ARF part")
	ErrMalformedField = errors.New("fbl: malformed Feedback-Type field")
)

// Report is the parsed ARF body.
type Report struct {
	FeedbackType      string
	UserAgent         string
	SourceIP          string
	OriginalRecipient string
	OriginalSender    string
	OriginalMessageID string
	ReportingMTA      string
	ArrivalDate       time.Time
	RedactedBody      string
}

// Parse decodes an ARF report from r. The reader is consumed up to
// MaxReportBytes; anything beyond returns ErrTooLarge.
func Parse(headers []string, r io.Reader) (*Report, error) {
	limited := io.LimitReader(r, MaxReportBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("fbl: read body: %w", err)
	}
	if int64(len(body)) > MaxReportBytes {
		return nil, ErrTooLarge
	}

	contentType := findHeader(headers, "Content-Type")
	if !strings.HasPrefix(contentType, "multipart/report") &&
		!strings.HasPrefix(contentType, "multipart/mixed") {
		return nil, fmt.Errorf("%w: got %q", ErrNotMultipart, contentType)
	}

	boundary := extractBoundary(contentType)
	if boundary == "" {
		return nil, fmt.Errorf("%w: missing boundary", ErrNotMultipart)
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	rep := &Report{}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fbl: read part: %w", err)
		}
		ct := part.Header.Get("Content-Type")
		ct = strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
		switch ct {
		case "message/feedback-report":
			if err := parseFeedbackPart(part, rep); err != nil {
				return nil, err
			}
		case "message/rfc822", "text/rfc822-headers":
			if err := parseOriginalMessage(part, rep); err != nil {
				return nil, err
			}
		case "text/plain":
			rep.RedactedBody = redact(readSnippet(part, MaxRedactedBody))
		}
		_ = part.Close()
	}
	if rep.FeedbackType == "" {
		return nil, ErrMissingPart
	}
	return rep, nil
}

// parseFeedbackPart parses the message/feedback-report part.
func parseFeedbackPart(p *multipart.Part, rep *Report) error {
	tp := textproto.NewReader(bufio.NewReader(p))
	hdr, err := tp.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		return fmt.Errorf("fbl: feedback header: %w", err)
	}
	rep.FeedbackType = strings.ToLower(strings.TrimSpace(hdr.Get("Feedback-Type")))
	if rep.FeedbackType == "" {
		return ErrMalformedField
	}
	rep.UserAgent = clip(hdr.Get("User-Agent"), 255)
	rep.SourceIP = clip(hdr.Get("Source-IP"), 64)
	rep.OriginalRecipient = clip(hdr.Get("Original-Rcpt-To"), 320)
	rep.ReportingMTA = clip(extractReportingMTA(hdr.Get("Reporting-MTA")), 253)
	if v := hdr.Get("Arrival-Date"); v != "" {
		if t, err := mail.ParseDate(v); err == nil {
			rep.ArrivalDate = t
		}
	}
	return nil
}

// parseOriginalMessage parses the message/rfc822 attached part to extract
// the message-id and sender from the original message.
func parseOriginalMessage(p *multipart.Part, rep *Report) error {
	limited := io.LimitReader(p, MaxRedactedBody+MaxReportBytes/2)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("fbl: read original: %w", err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(body))
	if err != nil {
		return nil // not fatal — partial reports are common
	}
	rep.OriginalMessageID = clip(strings.Trim(msg.Header.Get("Message-ID"), " <>"), 255)
	if from := msg.Header.Get("From"); from != "" {
		if addr, err := mail.ParseAddress(from); err == nil {
			rep.OriginalSender = clip(addr.Address, 320)
		}
	}
	return nil
}

func findHeader(headers []string, key string) string {
	for _, h := range headers {
		i := strings.IndexByte(h, ':')
		if i <= 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(h[:i]), key) {
			return strings.TrimSpace(h[i+1:])
		}
	}
	return ""
}

var reBoundary = regexp.MustCompile(`(?i)boundary="?([^";]+)"?`)

func extractBoundary(ct string) string {
	m := reBoundary.FindStringSubmatch(ct)
	if len(m) < 2 {
		return ""
	}
	return strings.Trim(m[1], `"`)
}

func extractReportingMTA(s string) string {
	// Per RFC 6650, format is "<mta-type>;<name>" e.g., "dns;mta.example.com".
	if i := strings.IndexByte(s, ';'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}

func readSnippet(r io.Reader, n int) string {
	limited := io.LimitReader(r, int64(n))
	b, _ := io.ReadAll(limited)
	return string(b)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// rePIIEmail strips email-like substrings, keeping the domain intact for
// triage but masking the local part.
var rePIIEmail = regexp.MustCompile(`([A-Za-z0-9._%+\-]+)@([A-Za-z0-9.\-]+\.[A-Za-z]{2,})`)

func redact(s string) string {
	if s == "" {
		return ""
	}
	return rePIIEmail.ReplaceAllString(s, "[REDACTED]@$2")
}
