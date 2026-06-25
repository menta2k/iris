package biz

import "time"

// RspamdResultsStream is the Redis stream the generated kumod policy XADDs each
// inbound spam-scan verdict onto, and which the rspamd ingestion worker consumes.
// data.StreamRspamdResults is an alias of this so the producer (Lua) and consumer
// (Go) never drift.
const RspamdResultsStream = "iris.rspamd.results"

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
