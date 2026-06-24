package biz

import (
	"strings"
	"time"
)

// Mail record status values.
const (
	MailReceived   = "received"
	MailQueued     = "queued"
	MailSent       = "sent"
	MailDeferred   = "deferred"
	MailBounced    = "bounced"
	MailSuppressed = "suppressed"
	MailFailed     = "failed"
)

// MailRecord is a single message event in the mail log.
type MailRecord struct {
	ID        string
	MessageID string
	EventTime time.Time
	Mailclass string
	Sender    string
	// FromHeader is the original From header. The envelope Sender is
	// VERP-rewritten at reception, so it no longer shows who sent the mail.
	FromHeader      string
	Recipient       string
	RecipientDomain string
	VMTAID          string
	RouteID         string
	Status          string
	// SMTPStatus and Diagnostic carry the server's SMTP response for this event
	// (e.g. the 4xx code + text on a deferral, 5xx on a bounce, 250 on delivery).
	// Empty for events with no response (e.g. Reception).
	SMTPStatus string
	Diagnostic string
}

// MailFilter is a validated, bounded set of mail-log query filters.
type MailFilter struct {
	Mailclass string
	Sender    string
	// From is a case-insensitive substring match on the original From header.
	From      string
	Recipient string
	VMTAID    string
	// Status filters by mail-record status (e.g. "deferred" to see what's stuck
	// in the queue). Empty matches all.
	Status   string
	FromTime *time.Time
	ToTime   *time.Time
}

// NormalizeMailFilter sanitizes and bounds the free-text filter fields.
func NormalizeMailFilter(f MailFilter) (MailFilter, error) {
	f.Mailclass = SanitizeFilter(f.Mailclass)
	f.Sender = strings.ToLower(SanitizeFilter(f.Sender))
	f.From = strings.ToLower(SanitizeFilter(f.From))
	f.Recipient = strings.ToLower(SanitizeFilter(f.Recipient))
	f.VMTAID = SanitizeFilter(f.VMTAID)
	f.Status = strings.ToLower(SanitizeFilter(f.Status))
	if f.FromTime != nil && f.ToTime != nil && f.ToTime.Before(*f.FromTime) {
		return f, Invalid("MAIL_FILTER_RANGE", "to_time must not be before from_time")
	}
	return f, nil
}
