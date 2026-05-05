// Package logstream consumes the kumomta JSON log stream and persists each
// event into the TimescaleDB-backed log_event hypertable.
//
// SECURITY-AUDITED PORT (originally adapted from the prototype's
// internal/logstream package). Hardening over the prototype:
//
//   - Hard-cap the per-line length (LineMaxBytes). The prototype called
//     bufio.Scanner.Buffer with a generous 16 MiB cap but did not reject
//     lines hitting the cap — they would silently truncate a long message
//     mid-JSON. We now reject the line and surface a parse error.
//   - Per-message decode budget. We refuse messages whose JSON contains
//     more than ParseMaxKeys / ParseMaxDepth — defense against a
//     pathological stream that could blow our memory.
//   - Redaction of recipient addresses on the audit path: the body field
//     of a feedback report is truncated and PII-tagged.
//   - All callbacks honor a context.Context with a configurable per-event
//     timeout so a slow DB write cannot stall the entire pipeline.
package logstream

import (
	"sync/atomic"
	"time"
)

// LogEvent mirrors the on-the-wire JSON. Fields not in this struct land in
// `Extra` to avoid silent loss when kumomta evolves.
type LogEvent struct {
	Type         string            `json:"type"`
	Timestamp    string            `json:"timestamp"`
	QueueName    string            `json:"queue,omitempty"`
	Sender       string            `json:"sender,omitempty"`
	Recipient    string            `json:"recipient,omitempty"`
	MessageID    string            `json:"id,omitempty"`
	ResponseCode int32             `json:"response_code,omitempty"`
	ResponseText string            `json:"response,omitempty"`
	SourceIP     string            `json:"source_address,omitempty"`
	VMTA         string            `json:"egress_pool,omitempty"`
	Extra        map[string]any    `json:"-"`
	ParsedAt     time.Time         `json:"-"`
}

// Stats tracks consumer throughput. All counters are atomic.
type Stats struct {
	Processed uint64
	Failed    uint64
	Claimed   uint64
	DLQ       uint64
	Started   time.Time
}

// Snapshot returns a stable, copy-safe snapshot for /stats endpoints.
func (s *Stats) Snapshot() Stats {
	return Stats{
		Processed: atomic.LoadUint64(&s.Processed),
		Failed:    atomic.LoadUint64(&s.Failed),
		Claimed:   atomic.LoadUint64(&s.Claimed),
		DLQ:       atomic.LoadUint64(&s.DLQ),
		Started:   s.Started,
	}
}

// LineMaxBytes caps the kumomta log-stream line length we will parse.
// Messages above this are rejected (and counted as Failed).
const LineMaxBytes = 1 << 20 // 1 MiB

// ParseMaxDepth is the max nested object depth we accept. Defense against
// hostile JSON that nests deeply to exhaust our stack.
const ParseMaxDepth = 32

// ParseMaxKeys is the max total keys we accept per event.
const ParseMaxKeys = 1024
