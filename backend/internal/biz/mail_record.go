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
	ID              string
	MessageID       string
	EventTime       time.Time
	Mailclass       string
	Sender          string
	Recipient       string
	RecipientDomain string
	VMTAID          string
	RouteID         string
	Status          string
}

// MailFilter is a validated, bounded set of mail-log query filters.
type MailFilter struct {
	Mailclass string
	Sender    string
	Recipient string
	VMTAID    string
	FromTime  *time.Time
	ToTime    *time.Time
}

// NormalizeMailFilter sanitizes and bounds the free-text filter fields.
func NormalizeMailFilter(f MailFilter) (MailFilter, error) {
	f.Mailclass = SanitizeFilter(f.Mailclass)
	f.Sender = strings.ToLower(SanitizeFilter(f.Sender))
	f.Recipient = strings.ToLower(SanitizeFilter(f.Recipient))
	f.VMTAID = SanitizeFilter(f.VMTAID)
	if f.FromTime != nil && f.ToTime != nil && f.ToTime.Before(*f.FromTime) {
		return f, Invalid("MAIL_FILTER_RANGE", "to_time must not be before from_time")
	}
	return f, nil
}
