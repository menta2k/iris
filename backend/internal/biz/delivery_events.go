package biz

import "time"

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
