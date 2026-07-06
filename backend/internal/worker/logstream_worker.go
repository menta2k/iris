package worker

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/metrics"
)

const logStreamGroup = "iris-logstream"

// MailEventStore persists mail, bounce, and feedback events parsed from the
// KumoMTA log stream.
type MailEventStore interface {
	InsertMailEvent(ctx context.Context, rec *biz.MailRecord) error
	InsertBounce(ctx context.Context, b *biz.BounceRecord) error
	InsertFeedbackReport(ctx context.Context, f *biz.FeedbackReport) error
	// IncrementSoftBounce bumps and returns a recipient's soft-bounce count.
	IncrementSoftBounce(ctx context.Context, recipient string) (int, error)
	// RecipientForMessageID returns the original recipient for a sent message id
	// (used to correlate a VERP async bounce). "" when not found.
	RecipientForMessageID(ctx context.Context, messageID string) (string, error)
}

// Suppressor auto-suppresses a recipient. Used by the feedback-loop ingest and
// the bounce pipeline. Optional (nil disables it).
type Suppressor interface {
	SuppressRecipient(ctx context.Context, email, source, reason string) error
	// SuppressRecipientFor suppresses with an explicit TTL override (ttl <= 0 uses
	// the global default), for per-rule bounce suppression lifetimes.
	SuppressRecipientFor(ctx context.Context, email, source, reason string, ttl time.Duration) error
}

// BouncePolicyProvider supplies the current bounce-handling policy (auto-suppress
// hard bounces, soft-bounce threshold). Optional (nil = auto-suppress hard).
type BouncePolicyProvider interface {
	BouncePolicyNow(ctx context.Context) biz.BouncePolicy
}

// DKIMKeyResolver returns the published DKIM TXT value for one of our own
// domain+selector keys (derived from the stored private key), for verifying that
// an FBL report's embedded original was signed by us. Optional (nil disables the
// DKIM provenance check).
type DKIMKeyResolver interface {
	DKIMPublicKey(ctx context.Context, domain, selector string) (string, bool)
}

// FeedbackPolicyProvider supplies the FBL-handling policy (whether suppression
// requires proven provenance). Optional (nil = permissive, suppress all).
type FeedbackPolicyProvider interface {
	FeedbackPolicyNow(ctx context.Context) biz.FeedbackPolicy
}

// ClassifyPolicyProvider reports the current subject-classification policy. When
// enabled, the log worker enqueues each Reception's subject for the async
// classification worker. Optional (nil disables enqueueing).
type ClassifyPolicyProvider interface {
	ClassifyPolicyNow(ctx context.Context) biz.ClassifyPolicy
}

// BounceRuleSource supplies the active bounce-action ruleset used to classify a
// bounce into a system action. Optional (nil = legacy hard/soft policy only).
type BounceRuleSource interface {
	ActiveRules(ctx context.Context) ([]*biz.BounceActionRule, error)
}

// LogStreamWorker consumes KumoMTA structured log records from the Redis stream
// (produced by the generated policy's log_hook) and persists them into the
// mail_records / bounce_records hypertables. This is how the Logs UI is
// populated — from KumoMTA's own logs, not manual inserts.
type LogStreamWorker struct {
	streams     *data.Streams
	store       MailEventStore
	suppressor  Suppressor
	policy      BouncePolicyProvider
	dkimKeys    DKIMKeyResolver
	feedbackPol FeedbackPolicyProvider
	classifyPol ClassifyPolicyProvider
	stream      string
	log         *slog.Logger

	// Bounce-action rules, cached briefly to avoid a DB hit per bounce.
	bounceRules   BounceRuleSource
	bounceMu      sync.Mutex
	bounceCache   []*biz.BounceActionRule
	bounceCacheAt time.Time
}

// bounceRuleCacheTTL bounds how stale the worker's cached ruleset may be.
const bounceRuleCacheTTL = 30 * time.Second

// WithBounceRules makes the worker classify each bounce against the operator
// ruleset: a matching rule is authoritative (suppress only when it says so).
// Returns the worker for chaining.
func (w *LogStreamWorker) WithBounceRules(src BounceRuleSource) *LogStreamWorker {
	w.bounceRules = src
	return w
}

// WithClassification enables enqueueing Reception subjects to the classification
// worker when the feature is on. Returns the worker for chaining.
func (w *LogStreamWorker) WithClassification(policy ClassifyPolicyProvider) *LogStreamWorker {
	w.classifyPol = policy
	return w
}

// WithFeedbackVerification enables FBL provenance verification: complaints are
// checked (X-KumoRef trace / send-log / DKIM-by-us) and, when policy requires it,
// only verified complaints auto-suppress. Returns the worker for chaining.
func (w *LogStreamWorker) WithFeedbackVerification(keys DKIMKeyResolver, policy FeedbackPolicyProvider) *LogStreamWorker {
	w.dkimKeys = keys
	w.feedbackPol = policy
	return w
}

// NewLogStreamWorker constructs the worker. streamName must match the policy's
// LogStreamName (defaults to data.StreamMailEvents). suppressor/policy may be
// nil to disable auto-suppression (a nil policy auto-suppresses hard bounces).
func NewLogStreamWorker(streams *data.Streams, store MailEventStore, suppressor Suppressor, policy BouncePolicyProvider, streamName string, log *slog.Logger) *LogStreamWorker {
	if streamName == "" {
		streamName = data.StreamMailEvents
	}
	return &LogStreamWorker{streams: streams, store: store, suppressor: suppressor, policy: policy, stream: streamName, log: log}
}

func (w *LogStreamWorker) bouncePolicy(ctx context.Context) biz.BouncePolicy {
	if w.policy == nil {
		return biz.BouncePolicy{AutoSuppressHardBounces: true}
	}
	return w.policy.BouncePolicyNow(ctx)
}

// Run consumes log records until the context is cancelled. Multiple instances
// share the consumer group.
func (w *LogStreamWorker) Run(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, w.stream, logStreamGroup); err != nil {
		return err
	}
	w.log.Info("log-stream worker started", "stream", w.stream)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("log-stream worker stopping")
			return ctx.Err()
		default:
		}
		msgs, err := w.streams.Consume(ctx, w.stream, logStreamGroup, 100, 2*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("consume log stream", "error", err.Error())
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
			if err := w.streams.Ack(ctx, w.stream, logStreamGroup, m.ID); err != nil {
				w.log.Error("ack log record", "id", m.ID, "error", err.Error())
			}
		}
	}
}

// handleFeedback persists an ARF feedback report and auto-suppresses the
// complainant (the reference build's "FBL events automatically add a
// suppression entry as a side effect" behavior).
func (w *LogStreamWorker) handleFeedback(ctx context.Context, rec *biz.KumoLogRecord, now time.Time) {
	recipient := rec.ComplainantRecipient()

	// Prove the complaint is about mail we sent (X-KumoRef trace / send-log / our
	// DKIM signature). kumod already guarantees the report is structurally valid.
	var keyFn biz.DKIMPublicKeyFunc
	if w.dkimKeys != nil {
		keyFn = func(domain, selector string) (string, bool) { return w.dkimKeys.DKIMPublicKey(ctx, domain, selector) }
	}
	sentFn := func(messageID string) string {
		r, _ := w.store.RecipientForMessageID(ctx, messageID)
		return r
	}
	verified, method := biz.VerifyFeedback(rec, keyFn, sentFn)

	if err := w.store.InsertFeedbackReport(ctx, &biz.FeedbackReport{
		ReceivedAt:      rec.EventTime(now),
		Source:          rec.FeedbackSource(),
		ReportType:      rec.FeedbackReportType(),
		Recipient:       recipient,
		ProcessingState: biz.ProcessingProcessed,
		Verified:        verified,
		Verification:    method,
	}); err != nil {
		w.log.Error("persist feedback report", "error", err.Error())
		return
	}

	// Gate suppression on verification when the policy requires it (default
	// permissive: suppress every complaint, as before).
	requireVerification := false
	if w.feedbackPol != nil {
		requireVerification = w.feedbackPol.FeedbackPolicyNow(ctx).RequireVerification
	}
	if requireVerification && !verified {
		w.log.Warn("fbl complaint unverified; not auto-suppressing",
			"recipient", recipient, "type", rec.FeedbackReportType())
		return
	}
	if w.suppressor != nil && recipient != "" {
		reason := "feedback complaint (" + rec.FeedbackReportType() + ")"
		if verified {
			reason += " [verified:" + method + "]"
		}
		if err := w.suppressor.SuppressRecipient(ctx, recipient, "fbl", reason); err != nil {
			w.log.Error("auto-suppress complainant", "recipient", recipient, "error", err.Error())
		}
	}
}

func (w *LogStreamWorker) handle(ctx context.Context, m data.StreamMessage) {
	// The policy XADDs each record with fields type=<EventType> and data=<json>.
	payload, _ := m.Values["data"].(string)
	if payload == "" {
		return
	}
	rec, err := biz.ParseKumoLogRecord([]byte(payload))
	if err != nil {
		w.log.Warn("drop malformed log record", "id", m.ID, "error", err.Error())
		return
	}
	now := time.Now().UTC()

	// Feedback (ARF/FBL) complaints: persist the report and auto-suppress the
	// complainant so future mail to that address is blocked.
	if rec.Type == biz.KumoFeedback {
		w.handleFeedback(ctx, rec, now)
		return
	}

	status := rec.MailStatus()
	if status == "" {
		// Other non-mail records are not stored as mail events.
		return
	}

	mr := &biz.MailRecord{
		MessageID:       rec.ID,
		EventTime:       rec.EventTime(now),
		Mailclass:       rec.Mailclass(),
		Sender:          rec.Sender,
		FromHeader:      rec.FromHeader(),
		Recipient:       rec.Recipient,
		RecipientDomain: rec.RecipientDomainOf(),
		EgressSource:    strings.TrimSpace(rec.EgressSource),
		Status:          status,
		RecordType:      rec.Type,
		Diagnostic:      strings.TrimSpace(rec.Response.Content),
	}
	// Carry the SMTP response code (e.g. 4xx on a deferral) so the Logs UI can
	// show why a message deferred/bounced, not just that it did.
	if rec.Response.Code > 0 {
		mr.SMTPStatus = strconv.Itoa(int(rec.Response.Code))
	}
	if err := w.store.InsertMailEvent(ctx, mr); err != nil {
		w.log.Error("persist mail event", "type", rec.Type, "error", err.Error())
		return
	}

	// Metrics: mail events by status/class/domain, and outbound events by VMTA
	// (egress source is present on Delivery/Bounce, absent on Reception).
	metrics.RecordMailEvent(mr.Status, mr.Mailclass, mr.RecipientDomain)
	metrics.RecordVMTAEvent(rec.EgressSource, mr.Status)

	// Queue latency: on a successful Delivery, observe how long the message sat
	// in the queue (Reception → Delivery) into the histogram, by mail class.
	if rec.Type == biz.KumoDelivery {
		if d, ok := rec.QueueLatency(now); ok {
			metrics.RecordQueueTime(mr.Mailclass, d.Seconds())
		}
	}

	// Optional subject classification: the Subject header is only on Reception.
	// When the feature is on, hand {message_id, subject} to the async worker via
	// a transient stream — the subject is never persisted on mail_records.
	if rec.Type == biz.KumoReception {
		w.enqueueClassification(ctx, rec, mr.EventTime)
	}

	if rec.Type == biz.KumoBounce {
		smtp := ""
		if rec.Response.Code > 0 {
			smtp = strconv.Itoa(int(rec.Response.Code))
		}
		bounce := &biz.BounceRecord{
			EventTime:       rec.EventTime(now),
			Recipient:       rec.Recipient,
			Mailclass:       rec.Mailclass(),
			SMTPStatus:      smtp,
			Diagnostic:      rec.Response.Content,
			Classification:  rec.BounceClassification,
			ProcessingState: biz.ProcessingNew,
		}
		if err := w.store.InsertBounce(ctx, bounce); err != nil {
			w.log.Error("persist bounce", "error", err.Error())
		}
		bounceType := "soft"
		if bounce.IsHardBounce() {
			bounceType = "hard"
		}
		metrics.RecordBounce(bounceType, bounce.Mailclass)
		w.applyBouncePolicy(ctx, bounce)
	}

	// Deferrals (transient failures) are also matched against the bounce rules so
	// a suppress rule can fire during the retry window — e.g. suppress a
	// persistently-full mailbox after N attempts — without waiting for expiry.
	if rec.Type == biz.KumoTransientFailure {
		w.applyDeferralRules(ctx, rec, now)
	}
}

// applyDeferralRules matches a transient failure against the bounce ruleset and,
// when a suppress rule applies at the message's current attempt count, suppresses
// the recipient. throttle/suspend rules are enforced via traffic shaping, not
// here; retry / no-match leave the message to keep retrying.
func (w *LogStreamWorker) applyDeferralRules(ctx context.Context, rec *biz.KumoLogRecord, now time.Time) {
	if w.suppressor == nil || w.bounceRules == nil {
		return
	}
	recipient := strings.ToLower(strings.TrimSpace(rec.Recipient))
	if recipient == "" {
		return
	}
	rules := w.activeBounceRules(ctx)
	if len(rules) == 0 {
		return
	}
	smtp := ""
	if rec.Response.Code > 0 {
		smtp = strconv.Itoa(int(rec.Response.Code))
	}
	rule := biz.MatchBounceRule(rules, biz.BounceSignature{
		SMTPCode:   smtp,
		Domain:     recipientDomain(recipient),
		Diagnostic: rec.Response.Content,
		Attempts:   rec.NumAttempts,
	})
	if rule == nil || rule.Action != biz.BounceActionSuppress {
		return
	}
	reason := "bounce rule: " + rule.Category + " (attempt " + strconv.Itoa(rec.NumAttempts) + ")"
	if err := w.suppressor.SuppressRecipientFor(ctx, recipient, "bounce", reason, bounceRuleTTL(rule)); err != nil {
		w.log.Error("suppress by deferral rule", "recipient", recipient, "error", err.Error())
	}
}

// bounceRuleTTL resolves a suppress rule's per-rule TTL override (0 = use the
// global suppression TTL).
func bounceRuleTTL(rule *biz.BounceActionRule) time.Duration {
	if rule == nil || rule.SuppressTTL == "" {
		return 0
	}
	d, _ := biz.ParseFlexDuration(rule.SuppressTTL)
	return d
}

// enqueueClassification hands a received message's subject to the async
// classification worker via a transient Redis stream, but only when the feature
// is enabled and a subject is present. Best-effort: a failure is logged, never
// fatal to log ingestion.
func (w *LogStreamWorker) enqueueClassification(ctx context.Context, rec *biz.KumoLogRecord, eventTime time.Time) {
	if w.classifyPol == nil {
		return
	}
	subject := rec.SubjectHeader()
	if subject == "" {
		return
	}
	if !w.classifyPol.ClassifyPolicyNow(ctx).Enabled {
		return
	}
	if _, err := w.streams.Publish(ctx, data.StreamClassifyPending, map[string]any{
		"message_id": rec.ID,
		"event_time": eventTime.Format(time.RFC3339Nano),
		"subject":    subject,
	}); err != nil {
		w.log.Error("enqueue classification", "message_id", rec.ID, "error", err.Error())
	}
}

// activeBounceRules returns the operator ruleset, cached for bounceRuleCacheTTL
// to avoid a database round-trip on every bounce. On a load error it serves the
// last-known set (possibly empty), never blocking bounce processing.
func (w *LogStreamWorker) activeBounceRules(ctx context.Context) []*biz.BounceActionRule {
	if w.bounceRules == nil {
		return nil
	}
	w.bounceMu.Lock()
	defer w.bounceMu.Unlock()
	if !w.bounceCacheAt.IsZero() && time.Since(w.bounceCacheAt) < bounceRuleCacheTTL {
		return w.bounceCache
	}
	rules, err := w.bounceRules.ActiveRules(ctx)
	if err != nil {
		w.log.Error("load bounce rules", "error", err.Error())
		return w.bounceCache // serve stale on error
	}
	w.bounceCache = rules
	w.bounceCacheAt = time.Now()
	return rules
}

// recipientDomain returns the domain part of an email address, or "".
func recipientDomain(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 {
		return email[i+1:]
	}
	return ""
}

// applyBouncePolicy auto-suppresses the recipient on a hard bounce (5xx) or once
// soft bounces reach the configured threshold.
func (w *LogStreamWorker) applyBouncePolicy(ctx context.Context, b *biz.BounceRecord) {
	if w.suppressor == nil {
		return
	}
	recipient := strings.ToLower(strings.TrimSpace(b.Recipient))
	if recipient == "" {
		return
	}

	// Rule engine (additive to the legacy net, never weakening it): a matched
	// suppress rule suppresses the recipient; a matched throttle/suspend rule
	// leaves the recipient in place (enforced via traffic shaping, not here).
	// retry rules and unmatched bounces fall through to the legacy policy below,
	// which preserves the existing hard-bounce and soft-threshold behavior.
	if rules := w.activeBounceRules(ctx); len(rules) > 0 {
		sig := biz.BounceSignature{SMTPCode: b.SMTPStatus, Domain: recipientDomain(recipient), Diagnostic: b.Diagnostic}
		if rule := biz.MatchBounceRule(rules, sig); rule != nil {
			switch rule.Action {
			case biz.BounceActionSuppress:
				reason := "bounce rule: " + rule.Category
				if b.SMTPStatus != "" {
					reason += " " + b.SMTPStatus
				}
				if err := w.suppressor.SuppressRecipient(ctx, recipient, "bounce", reason); err != nil {
					w.log.Error("suppress by bounce rule", "recipient", recipient, "error", err.Error())
				}
				return
			case biz.BounceActionThrottle, biz.BounceActionSuspendDomain:
				// Traffic shaping handles the back-off; do not suppress the address.
				return
			}
			// retry: fall through to the legacy policy.
		}
	}

	policy := w.bouncePolicy(ctx)

	if b.IsHardBounce() {
		// Don't suppress an otherwise-valid recipient when the hard failure is
		// not their fault (spam block, quota, policy) per the classifier.
		if policy.AutoSuppressHardBounces && b.ShouldSuppressOnHardBounce() {
			reason := "hard bounce " + b.SMTPStatus
			if b.Classification != "" {
				reason += " (" + b.Classification + ")"
			}
			if err := w.suppressor.SuppressRecipient(ctx, recipient, "bounce", reason); err != nil {
				w.log.Error("auto-suppress hard bounce", "recipient", recipient, "error", err.Error())
			}
		}
		return
	}
	// Soft bounce: count toward the threshold (0 = disabled).
	if policy.SoftBounceThreshold <= 0 {
		return
	}
	count, err := w.store.IncrementSoftBounce(ctx, recipient)
	if err != nil {
		w.log.Error("increment soft bounce", "recipient", recipient, "error", err.Error())
		return
	}
	if count >= policy.SoftBounceThreshold {
		if err := w.suppressor.SuppressRecipient(ctx, recipient, "bounce",
			"soft bounce threshold reached ("+strconv.Itoa(count)+")"); err != nil {
			w.log.Error("auto-suppress soft bounce", "recipient", recipient, "error", err.Error())
		}
	}
}
