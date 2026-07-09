package biz

import (
	"strings"
	"time"
)

// Processing states shared by bounce and feedback records.
const (
	ProcessingNew       = "new"
	ProcessingProcessed = "processed"
	ProcessingIgnored   = "ignored"
	ProcessingFailed    = "failed"
)

// BounceRecord captures an SMTP bounce for a delivered message.
type BounceRecord struct {
	ID              string
	MailRecordID    string
	EventTime       time.Time
	Recipient       string
	VMTAID          string
	Mailclass       string
	SMTPStatus      string
	BounceType      string
	Diagnostic      string
	Classification  string
	ProcessingState string
}

// BounceFilter is a validated, bounded set of bounce query filters.
type BounceFilter struct {
	// Recipient is a case-insensitive substring match on the address.
	Recipient string
	Mailclass string
	// BounceType filters by hard/soft/dsn. Empty matches all.
	BounceType string
	// Classification is a case-insensitive substring match on the KumoMTA
	// bounce-classifier category. Empty matches all.
	Classification string
	// ProcessingState filters by the record's pipeline state (new, processing,
	// processed, suppressed, retried). Empty matches all.
	ProcessingState string
	FromTime        *time.Time
	ToTime          *time.Time
}

// NormalizeBounceFilter sanitizes and bounds the free-text filter fields.
func NormalizeBounceFilter(f BounceFilter) (BounceFilter, error) {
	f.Recipient = strings.ToLower(SanitizeFilter(f.Recipient))
	f.Mailclass = SanitizeFilter(f.Mailclass)
	f.BounceType = strings.ToLower(SanitizeFilter(f.BounceType))
	f.Classification = SanitizeFilter(f.Classification)
	f.ProcessingState = strings.ToLower(SanitizeFilter(f.ProcessingState))
	if f.FromTime != nil && f.ToTime != nil && f.ToTime.Before(*f.FromTime) {
		return f, Invalid("BOUNCE_FILTER_RANGE", "to_time must not be before from_time")
	}
	return f, nil
}

// IsHardBounce reports whether the SMTP status indicates a permanent failure
// (5.x.x), which downstream logic may use to auto-suppress recipients.
func (b *BounceRecord) IsHardBounce() bool {
	return len(b.SMTPStatus) > 0 && b.SMTPStatus[0] == '5'
}

// nonRecipientFaultClasses are bounce classifications that are NOT the
// recipient's fault — a permanent failure here should not auto-suppress an
// otherwise-valid recipient. Two groups:
//   - Remote policy/content: the receiver blocked us as spam, by policy, or the
//     mailbox was temporarily over quota and reported 5xx.
//   - Routing / infrastructure: iris (or DNS/the network) could not route to a
//     domain it is responsible for. The recipient may be perfectly valid; the
//     failure is on our side, so it must never suppress the address. This covers
//     the loopback/prohibited-MX case (RoutingErrors).
var nonRecipientFaultClasses = map[string]struct{}{
	// Remote policy / content.
	"SpamBlock":      {},
	"SpamContent":    {},
	"ContentRelated": {},
	"QuotaIssue":     {},
	"PolicyRelated":  {},
	"RelayDenied":    {},
	"ProtocolErrors": {},
	// Routing / infrastructure (not the recipient's fault).
	"RoutingErrors":      {},
	"BadConnection":      {},
	"BadConfiguration":   {},
	"NoAnswerFromHost":   {},
	"DNSFailure":         {},
	"ServiceUnavailable": {},
	"TooManyConnections": {},
}

// ShouldSuppressOnHardBounce reports whether a hard bounce with this
// classification should auto-suppress the recipient. An empty classification
// (classifier disabled / no match) keeps the prior behavior of suppressing.
func (b *BounceRecord) ShouldSuppressOnHardBounce() bool {
	if b.Classification == "" {
		return true
	}
	_, blocked := nonRecipientFaultClasses[b.Classification]
	return !blocked
}

// FeedbackReport captures a feedback-loop or abuse report.
type FeedbackReport struct {
	ID              string
	ReceivedAt      time.Time
	Source          string
	ReportType      string
	Recipient       string
	MailRecordID    string
	ProcessingState string
	RawRef          string
	// Verified is true when the complaint was proven to be about mail we sent;
	// Verification is the method that proved it (supplemental-trace/send-log/dkim)
	// or "" when unverified. Auto-suppression can be gated on Verified.
	Verified     bool
	Verification string
}
