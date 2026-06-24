package biz

import (
	"encoding/json"
	"strings"
	"time"
)

// KumoLogMaxBytes caps the KumoMTA log-record JSON we will parse. Records above
// this are rejected rather than silently truncated.
const KumoLogMaxBytes = 1 << 20 // 1 MiB

// KumoLogRecord mirrors the subset of KumoMTA's JSON log record that Iris
// persists. Unknown fields are ignored. KumoMTA emits one record per event in
// a message's lifecycle (Reception → retries → Delivery/Bounce), which is how
// the Logs UI reconstructs a single message's timeline.
type KumoLogRecord struct {
	Type         string          `json:"type"`
	Timestamp    json.RawMessage `json:"timestamp"`
	ID           string          `json:"id"`
	Sender       string          `json:"sender"`
	Recipient    string          `json:"recipient"`
	Queue        string          `json:"queue"`
	EgressPool   string          `json:"egress_pool"`
	EgressSource string          `json:"egress_source"`
	Response     struct {
		Code    int32  `json:"code"`
		Content string `json:"content"`
	} `json:"response"`
	// BounceClassification is the category KumoMTA's bounce classifier assigns
	// (InvalidRecipient, SpamBlock, QuotaIssue, …); present on Bounce records
	// when configure_bounce_classifier is loaded.
	BounceClassification string         `json:"bounce_classification"`
	Meta                 map[string]any `json:"meta"`
	// Headers carries the headers KumoMTA's log hook is configured to capture
	// (the configure_log_hook allow-list). Used to recover the original From,
	// which the VERP rewrite removes from the envelope sender.
	Headers map[string]string `json:"headers"`
	// Feedback carries the ARF (RFC 5965) fields KumoMTA parses for a Feedback
	// log record. Present only when Type == "Feedback".
	Feedback *KumoFeedbackData `json:"feedback_report"`
}

// FromHeader returns the original From header captured by the log hook, or "".
// KumoMTA's header keys preserve their original case; match case-insensitively.
func (r *KumoLogRecord) FromHeader() string {
	if r.Headers == nil {
		return ""
	}
	for k, v := range r.Headers {
		if strings.EqualFold(k, "From") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// KumoFeedbackData mirrors the ARF feedback fields on a KumoMTA Feedback log
// record (kumo-log-types ARFReport). Field names/types match KumoMTA's actual
// JSON, verified end-to-end against a live kumod parsing a real ARF report.
type KumoFeedbackData struct {
	FeedbackType     string `json:"feedback_type"`
	UserAgent        string `json:"user_agent"`
	SourceIP         string `json:"source_ip"`
	OriginalMailFrom string `json:"original_mail_from"`
	// OriginalRcptTo matches KumoMTA's actual ARF field name (note the spelling,
	// "rcpto") and is a list, not a string.
	OriginalRcptTo []string `json:"original_rcpto_to"`
	// ReportingMTA is an object { mta_type, name }, not a string.
	ReportingMTA *KumoRemoteMTA `json:"reporting_mta"`
}

// KumoRemoteMTA mirrors KumoMTA's RemoteMta (the reporting_mta object).
type KumoRemoteMTA struct {
	MTAType string `json:"mta_type"`
	Name    string `json:"name"`
}

// KumoLogTypes are the record types Iris ingests.
const (
	KumoReception        = "Reception"
	KumoDelivery         = "Delivery"
	KumoBounce           = "Bounce"
	KumoTransientFailure = "TransientFailure"
	KumoFeedback         = "Feedback"
	// KumoSuppressed is a synthetic record type Iris emits from the reception hook
	// when a recipient is rejected by the suppression list (KumoMTA itself has no
	// such record type — the reject is otherwise invisible to the Logs UI).
	KumoSuppressed = "Suppressed"
)

// ParseKumoLogRecord decodes one KumoMTA JSON log record, rejecting oversized
// or non-object input.
func ParseKumoLogRecord(data []byte) (*KumoLogRecord, error) {
	if len(data) > KumoLogMaxBytes {
		return nil, Invalid("KUMO_LOG_TOO_LARGE", "log record exceeds %d bytes", KumoLogMaxBytes)
	}
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "{") {
		return nil, Invalid("KUMO_LOG_INVALID", "log record is not a JSON object")
	}
	var rec KumoLogRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, Invalid("KUMO_LOG_INVALID", "log record is not valid JSON: %v", err)
	}
	return &rec, nil
}

// MailStatus maps a KumoMTA record type to the Iris mail-record status.
func (r *KumoLogRecord) MailStatus() string {
	switch r.Type {
	case KumoReception:
		return MailReceived
	case KumoDelivery:
		return MailSent
	case KumoBounce:
		return MailBounced
	case KumoTransientFailure:
		return MailDeferred
	case KumoSuppressed:
		return MailSuppressed
	default:
		return ""
	}
}

// EventTime resolves the record timestamp, accepting either an RFC3339 string
// or a Unix epoch number. It falls back to now on absence/parse failure.
func (r *KumoLogRecord) EventTime(now time.Time) time.Time {
	if len(r.Timestamp) == 0 {
		return now
	}
	// String timestamp (RFC3339).
	var s string
	if err := json.Unmarshal(r.Timestamp, &s); err == nil && s != "" {
		if t, perr := time.Parse(time.RFC3339, s); perr == nil {
			return t.UTC()
		}
	}
	// Numeric (Unix seconds).
	var secs float64
	if err := json.Unmarshal(r.Timestamp, &secs); err == nil && secs > 0 {
		return time.Unix(int64(secs), 0).UTC()
	}
	return now
}

// RecipientDomainOf returns the lowercased domain of the record's recipient.
func (r *KumoLogRecord) RecipientDomainOf() string {
	return RecipientDomain(r.Recipient)
}

// ComplainantRecipient returns the address that filed the feedback complaint:
// the ARF Original-Rcpt-To when present, otherwise the record's recipient.
// Lowercased and trimmed.
func (r *KumoLogRecord) ComplainantRecipient() string {
	v := r.Recipient
	if r.Feedback != nil && len(r.Feedback.OriginalRcptTo) > 0 {
		v = r.Feedback.OriginalRcptTo[0]
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// FeedbackReportType returns the ARF feedback type (e.g. "abuse"), defaulting
// to "complaint" when unspecified.
func (r *KumoLogRecord) FeedbackReportType() string {
	if r.Feedback != nil && r.Feedback.FeedbackType != "" {
		return strings.ToLower(strings.TrimSpace(r.Feedback.FeedbackType))
	}
	return "complaint"
}

// FeedbackSource returns the reporting MTA/source for the feedback report.
func (r *KumoLogRecord) FeedbackSource() string {
	if r.Feedback != nil && r.Feedback.ReportingMTA != nil && r.Feedback.ReportingMTA.Name != "" {
		return strings.ToLower(strings.TrimSpace(r.Feedback.ReportingMTA.Name))
	}
	return "fbl"
}

// Mailclass returns the message's class from the 'mailclass' meta the policy's
// reception hook sets via classify_mail (the matched mailclass header value),
// or empty when the mail matched no configured class.
func (r *KumoLogRecord) Mailclass() string {
	if r.Meta == nil {
		return ""
	}
	if v, ok := r.Meta["mailclass"].(string); ok {
		return v
	}
	return ""
}
