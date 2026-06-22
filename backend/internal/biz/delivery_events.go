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
// recipient's fault — a permanent failure here (e.g. the receiver blocked us as
// spam, or the mailbox was temporarily over quota and reported 5xx) should not
// auto-suppress an otherwise-valid recipient.
var nonRecipientFaultClasses = map[string]struct{}{
	"SpamBlock":      {},
	"SpamContent":    {},
	"ContentRelated": {},
	"QuotaIssue":     {},
	"PolicyRelated":  {},
	"RelayDenied":    {},
	"ProtocolErrors": {},
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
}
