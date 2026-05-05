package logstream

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

func jsonUnmarshalString(b []byte, s *string) error  { return json.Unmarshal(b, s) }
func jsonUnmarshalAlias(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

// RedisLogRecord is the JSON shape kumomta XADDs into the configured stream.
// It mirrors the kumomta log_record schema (subset we care about) and the
// embedded ARF payload when type == "Feedback".
//
// Field names are kumomta-canonical (snake_case); we don't rename them to Go
// conventions because the raw JSON is also persisted to LogEvent.extra_json
// for unbounded inspection.
type RedisLogRecord struct {
	Type                 string              `json:"type"`
	ID                   string              `json:"id"`
	Sender               string              `json:"sender"`
	Recipient            string              `json:"recipient"`
	QueueName            string              `json:"queue"`
	SiteName             string              `json:"site"`
	Size                 int64               `json:"size"`
	Timestamp            int64               `json:"timestamp"`
	Created              int64               `json:"created"`
	NumAttempts          int                 `json:"num_attempts"`
	Response             *RedisResponse      `json:"response,omitempty"`
	EgressPool           string              `json:"egress_pool,omitempty"`
	EgressSource         string              `json:"egress_source,omitempty"`
	BounceClassification string              `json:"bounce_classification,omitempty"`
	// PeerAddress is `{name, addr}` in modern kumomta. Older versions
	// shipped a plain string; we accept both via the struct's UnmarshalJSON.
	PeerAddress    *RedisPeerAddress   `json:"peer_address,omitempty"`
	Headers        map[string]any      `json:"headers,omitempty"`
	Meta           map[string]any      `json:"meta,omitempty"`
	FeedbackReport *RedisFeedbackBlock `json:"feedback_report,omitempty"`
}

// RedisPeerAddress decodes either {name, addr} (the modern shape kumomta
// emits) or a bare string (the legacy shape).
type RedisPeerAddress struct {
	Name string `json:"name,omitempty"`
	Addr string `json:"addr,omitempty"`
}

// UnmarshalJSON tolerates a bare string by populating Addr.
func (p *RedisPeerAddress) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := jsonUnmarshalString(b, &s); err != nil {
			return err
		}
		p.Addr = s
		return nil
	}
	type alias RedisPeerAddress
	return jsonUnmarshalAlias(b, (*alias)(p))
}

// AddressString returns the IP/host portion regardless of input shape, or "".
func (p *RedisPeerAddress) AddressString() string {
	if p == nil {
		return ""
	}
	return p.Addr
}

// RedisFeedbackBlock is the parsed ARF payload kumomta attaches to records of
// type "Feedback".
type RedisFeedbackBlock struct {
	FeedbackType     string `json:"feedback_type,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	Version          string `json:"version,omitempty"`
	OriginalRcptTo   string `json:"original_rcpt_to,omitempty"`
	OriginalMailFrom string `json:"original_mail_from,omitempty"`
	ArrivalDate      string `json:"arrival_date,omitempty"`
	SourceIP         string `json:"source_ip,omitempty"`
	ReportedDomain   string `json:"reported_domain,omitempty"`
}

// RedisResponse is the SMTP / disposition response captured for delivery
// and bounce events.
type RedisResponse struct {
	Code    int    `json:"code"`
	Content string `json:"content"`
	Command string `json:"command,omitempty"`
}

// EventTime returns the canonical timestamp for the record. kumomta sets
// `timestamp` for most events and `created` for receptions; we prefer the
// most-specific value and fall back to "now" so a malformed record can still
// be stored without losing the row.
func (r RedisLogRecord) EventTime() time.Time {
	if r.Timestamp > 0 {
		return time.Unix(r.Timestamp, 0).UTC()
	}
	if r.Created > 0 {
		return time.Unix(r.Created, 0).UTC()
	}
	return time.Now().UTC()
}

// ConsumerStats are atomic counters surfaced for /metrics-style introspection.
type ConsumerStats struct {
	Processed atomic.Int64
	Failed    atomic.Int64
	Claimed   atomic.Int64
	DLQ       atomic.Int64
}
