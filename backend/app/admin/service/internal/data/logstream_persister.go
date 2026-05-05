// LogstreamPersister bridges the pkg/logstream consumer to ent: it implements
// logstream.Persister by wrapping the ent client + the suppression repo, so
// FBL events automatically add a suppression entry as a side effect.
package data

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/suppressionentry"
	"github.com/menta2k/iris/backend/pkg/logstream"
)

// LogstreamPersister persists kumomta log records into ent. ID generation is
// the local snowflake-ish scheme: the high bits are the timestamp millisecond,
// the low bits are random — sortable by time without needing a sequence.
type LogstreamPersister struct {
	client *ent.Client
}

// NewLogstreamPersister wires the ent client.
func NewLogstreamPersister(c *ent.Client) *LogstreamPersister {
	return &LogstreamPersister{client: c}
}

// InsertLog writes one row into the log_event hypertable. The raw JSON is
// stashed in extra_json for unbounded inspection later (the typed columns
// only capture the most-queried fields).
func (p *LogstreamPersister) InsertLog(ctx context.Context, lr *logstream.RedisLogRecord, raw string) error {
	create := p.client.LogEvent.Create().
		SetID(genID()).
		SetAt(lr.EventTime()).
		SetEventType(lr.Type).
		SetExtraJSON(raw)
	if lr.QueueName != "" {
		create.SetQueue(lr.QueueName)
	}
	if lr.Sender != "" {
		create.SetSender(lr.Sender)
	}
	if lr.Recipient != "" {
		create.SetRecipient(lr.Recipient)
	}
	if lr.ID != "" {
		create.SetMessageID(lr.ID)
	}
	if lr.Response != nil {
		create.SetResponseCode(int32(lr.Response.Code))
		if lr.Response.Content != "" {
			create.SetResponseText(clip(lr.Response.Content, 512))
		}
	}
	if ip := lr.PeerAddress.AddressString(); ip != "" {
		create.SetSourceIP(clip(ip, 64))
	}
	if lr.EgressSource != "" {
		create.SetVmta(clip(lr.EgressSource, 64))
	}
	// Mail-class: kumomta's log_hook ships headers verbatim but only a
	// curated allowlist of metas, so the X-Kumo-Mail-Class header is the
	// source of truth here. Fall back to lr.Meta["mailclass"] in case a
	// future kumomta version expands the meta serializer.
	if mc := mailClassFromRecord(lr); mc != "" {
		create.SetMailClass(clip(mc, 64))
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("logstream_persister: insert log: %w", err)
	}
	return nil
}

// InsertFeedback writes one row into feedback_report when type == "Feedback"
// and the parsed FBL block is present. Caller (the consumer) is responsible
// for the type guard; this method assumes both preconditions hold.
func (p *LogstreamPersister) InsertFeedback(ctx context.Context, lr *logstream.RedisLogRecord, raw string) error {
	if lr.FeedbackReport == nil {
		return errors.New("logstream_persister: feedback record has no payload")
	}
	fr := lr.FeedbackReport
	create := p.client.FeedbackReport.Create().
		SetID(genID()).
		SetReceivedAt(lr.EventTime()).
		SetFeedbackType(strings.ToLower(strings.TrimSpace(orFallback(fr.FeedbackType, "abuse"))))
	if fr.UserAgent != "" {
		create.SetUserAgent(clip(fr.UserAgent, 255))
	}
	if fr.SourceIP != "" {
		create.SetSourceIP(clip(fr.SourceIP, 64))
	}
	if rcpt := normalizeAddr(fr.OriginalRcptTo); rcpt != "" {
		create.SetOriginalRecipient(clip(rcpt, 320))
	}
	if from := normalizeAddr(fr.OriginalMailFrom); from != "" {
		create.SetOriginalSender(clip(from, 320))
	}
	if lr.ID != "" {
		create.SetOriginalMessageID(clip(lr.ID, 255))
	}
	if fr.ReportedDomain != "" {
		create.SetReportingMta(clip(fr.ReportedDomain, 253))
	}
	if t := parseFlexibleTime(fr.ArrivalDate); !t.IsZero() {
		create.SetArrivalDate(t)
	}
	if _, err := create.Save(ctx); err != nil {
		return fmt.Errorf("logstream_persister: insert feedback: %w", err)
	}
	return nil
}

// AutoSuppress upserts a suppression entry for the FBL recipient so the next
// send to that address is blocked at policy time. Called only after a
// successful feedback insert.
func (p *LogstreamPersister) AutoSuppress(ctx context.Context, lr *logstream.RedisLogRecord) error {
	if lr.FeedbackReport == nil {
		return nil
	}
	addr := normalizeAddr(lr.FeedbackReport.OriginalRcptTo)
	if addr == "" {
		return nil
	}
	now := time.Now().UTC()
	// Upsert by (address, scope) — same key used by the manual suppression
	// flow, so a feedback-driven row replaces any prior manual one.
	existing, err := p.client.SuppressionEntry.Query().
		Where(
			suppressionentry.AddressEQ(addr),
			suppressionentry.ScopeEQ("address"),
		).Only(ctx)
	if err == nil {
		_, err := p.client.SuppressionEntry.UpdateOneID(existing.ID).
			SetReason("complaint").
			SetNote(clip(lr.FeedbackReport.FeedbackType, 512)).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("logstream_persister: update suppression: %w", err)
		}
		return nil
	}
	if !ent.IsNotFound(err) {
		return fmt.Errorf("logstream_persister: lookup suppression: %w", err)
	}
	if _, err := p.client.SuppressionEntry.Create().
		SetAddress(addr).
		SetScope("address").
		SetReason("complaint").
		SetNote(clip(lr.FeedbackReport.FeedbackType, 512)).
		SetCreatedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("logstream_persister: create suppression: %w", err)
	}
	return nil
}

var _ logstream.Persister = (*LogstreamPersister)(nil)

// --- helpers ---------------------------------------------------------------

// genID returns a sortable int64 with millisecond resolution + 22 bits of
// randomness. Collision risk is well below what the hypertable can encounter
// per millisecond, and sorting by ID approximates time order, useful for
// debugging without joining on the at column.
func genID() int64 {
	const lowBits = 22
	ts := time.Now().UnixMilli()
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	r := int64(binary.BigEndian.Uint32(rnd[:])) & ((1 << lowBits) - 1)
	v := (ts << lowBits) | r
	if v < 0 {
		v &^= 1 << 62 // strip the sign bit so the value remains positive
	}
	return v
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func orFallback(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func normalizeAddr(addr string) string {
	s := strings.TrimSpace(addr)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return strings.ToLower(strings.TrimSpace(s))
}

// mailClassFromRecord extracts the X-Kumo-Mail-Class value from the
// log_record. kumomta serializes headers in the `headers` map; the value
// can be a bare string (when the header appeared once) or a list of
// strings (when it appeared multiple times). We accept both shapes and
// fall back to the meta map for forward-compat with future kumomta
// versions that may include user-set metas in the log_record.
func mailClassFromRecord(lr *logstream.RedisLogRecord) string {
	if lr == nil {
		return ""
	}
	const headerName = "X-Kumo-Mail-Class"
	if v, ok := lr.Headers[headerName]; ok {
		if s, ok := v.(string); ok && s != "" {
			return strings.TrimSpace(s)
		}
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	if mc, ok := lr.Meta["mailclass"].(string); ok && mc != "" {
		return strings.TrimSpace(mc)
	}
	return ""
}

// parseFlexibleTime accepts a few timestamp formats kumomta and ARF reports
// can ship — RFC3339 most often, but also RFC1123Z (the email Date: header
// format) for ArrivalDate fields.
func parseFlexibleTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
	} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
