package biz

import "time"

// Rspamd actions.
const (
	RspamdNoAction    = "no action"
	RspamdGreylist    = "greylist"
	RspamdAddHeader   = "add header"
	RspamdRewriteSubj = "rewrite subject"
	RspamdReject      = "reject"
)

// RspamdFilterResult records the filtering decision for a message.
type RspamdFilterResult struct {
	ID           string
	EventTime    time.Time
	MailRecordID string
	Action       string
	Score        float64
	Symbols      []string
	Reason       string
	RawRef       string
}

// IsSpam reports whether the result indicates the message was treated as spam.
func (r *RspamdFilterResult) IsSpam() bool {
	return r.Action == RspamdReject || r.Action == RspamdRewriteSubj || r.Action == RspamdAddHeader
}
