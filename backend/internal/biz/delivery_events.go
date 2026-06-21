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
	ProcessingState string
}

// IsHardBounce reports whether the SMTP status indicates a permanent failure
// (5.x.x), which downstream logic may use to auto-suppress recipients.
func (b *BounceRecord) IsHardBounce() bool {
	return len(b.SMTPStatus) > 0 && b.SMTPStatus[0] == '5'
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
