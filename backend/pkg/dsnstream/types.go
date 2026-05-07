// Package dsnstream consumes raw inbound DSN bodies that the kumomta
// catcher XADDed onto a Redis stream, parses them per RFC 3464, and hands
// the structured result to a Persister.
//
// The shape mirrors pkg/logstream so operators have one mental model for
// both pipelines: XREADGROUP loop, worker pool, auto-claim, DLQ on
// repeated parse failure. The differences are only in the message
// payload (RFC 3464 multipart/report instead of kumomta's JSON log
// record) and the persistence target (dsn_event instead of log_event).
package dsnstream

import "time"

// Parsed is the structured form of one DSN. All string fields are empty
// when the source DSN didn't carry that field — the persister decides
// what to store as NULL vs empty string.
//
// EmbeddedHeaders preserves the original message headers from the
// `message/rfc822` (or `text/rfc822-headers`) sub-part so the persister
// can do a fallback lookup of X-Kumo-Mail-Class when message-id-based
// correlation comes up empty.
type Parsed struct {
	// EnvelopeRecipient is the address kumomta wrote on the catcher
	// XADD — the local-part is the VERP token (when present). Carried
	// through verbatim because token validation happens in the consumer,
	// not the parser.
	EnvelopeRecipient string

	// VerpToken is the recovered token (without the "b+" prefix). Empty
	// when the inbound mail wasn't VERP'd or the format didn't match.
	// Validation against the secret happens at the persister layer.
	VerpToken string

	// MessageID is the RFC 5322 Message-ID extracted from the embedded
	// original. Used as the fallback correlation key when VERP is
	// disabled or the token didn't validate.
	MessageID string

	// RFC 3464 per-recipient fields. Most DSNs only emit one recipient
	// block; if multiple are present we keep the first (the only one
	// that matches the outer envelope-recipient anyway).
	OriginalRecipient string
	FinalRecipient    string
	Action            string // "failed" | "delayed" | "delivered" | "relayed" | "expanded"
	Status            string // RFC 3463, e.g. "5.1.1"
	DiagnosticCode    string
	RemoteMTA         string

	// ReceivedAt is the time at the consumer (not the original send
	// time). Persister timestamps the row with this if non-zero;
	// otherwise it falls back to time.Now() at insert.
	ReceivedAt time.Time

	// EmbeddedHeaders is the lower-cased header map from the embedded
	// `message/rfc822` part. Empty when the DSN didn't include the
	// original headers (some malformed DSNs).
	EmbeddedHeaders map[string]string

	// RawSize is the length of the original DSN body in bytes — the
	// persister stores it for "is this small enough to render inline?"
	// decisions in the UI.
	RawSize int
}

// MailClass returns the X-Kumo-Mail-Class value from the embedded
// headers (case-insensitive). Empty when the original message wasn't
// tagged or the embedded headers are missing.
func (p *Parsed) MailClass() string {
	if p.EmbeddedHeaders == nil {
		return ""
	}
	return p.EmbeddedHeaders["x-kumo-mail-class"]
}

// Tenant returns the X-Kumo-Tenant header value. Empty when not present.
func (p *Parsed) Tenant() string {
	if p.EmbeddedHeaders == nil {
		return ""
	}
	return p.EmbeddedHeaders["x-kumo-tenant"]
}

// Campaign returns the X-Kumo-Campaign header value. Empty when not present.
func (p *Parsed) Campaign() string {
	if p.EmbeddedHeaders == nil {
		return ""
	}
	return p.EmbeddedHeaders["x-kumo-campaign"]
}
