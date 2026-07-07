package worker

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// fakeBounceStore records the bounce-pipeline side effects the policy drives.
type fakeBounceStore struct {
	soft             map[string]int
	suppressed       map[string]string // recipient -> source
	suppressClass    map[string]string // recipient -> triggering mailclass
	suppressTTL      map[string]time.Duration
	recipientByMsgID map[string]string
	suppressErr      error
	mailEvents       []*biz.MailRecord
}

func newFakeBounceStore() *fakeBounceStore {
	return &fakeBounceStore{soft: map[string]int{}, suppressed: map[string]string{}, suppressClass: map[string]string{}, suppressTTL: map[string]time.Duration{}, recipientByMsgID: map[string]string{}}
}

func (f *fakeBounceStore) InsertMailEvent(_ context.Context, m *biz.MailRecord) error {
	f.mailEvents = append(f.mailEvents, m)
	return nil
}
func (f *fakeBounceStore) InsertBounce(context.Context, *biz.BounceRecord) error { return nil }
func (f *fakeBounceStore) InsertFeedbackReport(context.Context, *biz.FeedbackReport) error {
	return nil
}
func (f *fakeBounceStore) IncrementSoftBounce(_ context.Context, r string) (int, error) {
	f.soft[r]++
	return f.soft[r], nil
}

func (f *fakeBounceStore) RecipientForMessageID(_ context.Context, msgID string) (string, error) {
	return f.recipientByMsgID[msgID], nil
}

func (f *fakeBounceStore) SuppressRecipient(_ context.Context, email, source, _, mailclass string) error {
	if f.suppressErr != nil {
		return f.suppressErr
	}
	f.suppressed[email] = source
	if mailclass != "" {
		f.suppressClass[email] = mailclass
	}
	return nil
}

func (f *fakeBounceStore) SuppressRecipientFor(ctx context.Context, email, source, reason, mailclass string, ttl time.Duration) error {
	if ttl > 0 {
		f.suppressTTL[email] = ttl
	}
	return f.SuppressRecipient(ctx, email, source, reason, mailclass)
}

// fakePolicy returns a fixed bounce policy.
type fakePolicy struct{ p biz.BouncePolicy }

func (f fakePolicy) BouncePolicyNow(context.Context) biz.BouncePolicy { return f.p }

func newWorker(store MailEventStore, sup Suppressor, p BouncePolicyProvider) *LogStreamWorker {
	return NewLogStreamWorker(nil, store, sup, p, "s", biz.NewLogger("error"))
}

func TestApplyBouncePolicyHardBounce(t *testing.T) {
	ctx := context.Background()

	// Hard bounce + auto-suppress on → recipient suppressed via the bounce source.
	store := newFakeBounceStore()
	w := newWorker(store, store, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: true}})
	w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "Bad@Dest.Example", SMTPStatus: "550"})
	if store.suppressed["bad@dest.example"] != "bounce" {
		t.Fatalf("hard bounce should auto-suppress (normalized), got %+v", store.suppressed)
	}

	// Hard bounce + auto-suppress off → no suppression.
	store = newFakeBounceStore()
	w = newWorker(store, store, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: false}})
	w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "bad@dest.example", SMTPStatus: "550"})
	if len(store.suppressed) != 0 {
		t.Fatalf("auto-suppress off should not suppress, got %+v", store.suppressed)
	}
}

// fakeBounceRules serves a fixed active ruleset to the worker.
type fakeBounceRules struct{ rules []*biz.BounceActionRule }

func (f fakeBounceRules) ActiveRules(context.Context) ([]*biz.BounceActionRule, error) {
	return f.rules, nil
}

func TestApplyBouncePolicyRuleEngine(t *testing.T) {
	ctx := context.Background()
	rules := []*biz.BounceActionRule{
		{EnhancedCode: "5.1.1", Action: biz.BounceActionSuppress, Category: "Invalid Recipient", Priority: 100, Status: "active"},
		{SMTPCode: "550", Pattern: "spam", Action: biz.BounceActionSuspendDomain, Priority: 100, Status: "active"},
	}

	// A 5.1.1 user-unknown bounce → suppressed by the suppress rule, even with
	// the legacy auto-suppress switch OFF (the rule is authoritative).
	store := newFakeBounceStore()
	w := newWorker(store, store, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: false}}).WithBounceRules(fakeBounceRules{rules})
	w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "ghost@dest.example", SMTPStatus: "550", Diagnostic: "550 5.1.1 user unknown"})
	if store.suppressed["ghost@dest.example"] != "bounce" {
		t.Fatalf("suppress rule should suppress, got %+v", store.suppressed)
	}

	// A 5xx spam-block bounce → suspend_domain rule matches → NOT suppressed
	// (shaping handles it), even though legacy auto-suppress is ON.
	store = newFakeBounceStore()
	w = newWorker(store, store, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: true}}).WithBounceRules(fakeBounceRules{rules})
	w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "ok@dest.example", SMTPStatus: "550", Diagnostic: "550 5.7.1 message flagged as spam"})
	if len(store.suppressed) != 0 {
		t.Fatalf("suspend_domain rule must not suppress, got %+v", store.suppressed)
	}

	// An unmatched hard bounce falls through to the legacy net (auto-suppress on).
	store = newFakeBounceStore()
	w = newWorker(store, store, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: true}}).WithBounceRules(fakeBounceRules{rules})
	w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "gone@dest.example", SMTPStatus: "550", Diagnostic: "550 mailbox unavailable"})
	if store.suppressed["gone@dest.example"] != "bounce" {
		t.Fatalf("unmatched hard bounce should use legacy suppression, got %+v", store.suppressed)
	}
}

func TestApplyDeferralRulesThresholdAndTTL(t *testing.T) {
	ctx := context.Background()
	rules := []*biz.BounceActionRule{
		{SMTPCode: "452", Pattern: "out of storage", Action: biz.BounceActionSuppress,
			Category: "Mailbox Full (persistent)", MinAttempts: 7, SuppressTTL: "30d", Priority: 110, Status: "active"},
		{SMTPCode: "452", Pattern: "storage", Action: biz.BounceActionRetry, Priority: 80, Status: "active"},
	}
	defer452 := func(attempts int) *biz.KumoLogRecord {
		r := &biz.KumoLogRecord{Type: biz.KumoTransientFailure, Recipient: "full@dest.example", NumAttempts: attempts}
		r.Response.Code = 452
		r.Response.Content = "452 the recipient's inbox is out of storage space"
		return r
	}

	// Below the threshold → the retry rule wins → no suppression.
	store := newFakeBounceStore()
	w := newWorker(store, store, fakePolicy{}).WithBounceRules(fakeBounceRules{rules})
	w.applyDeferralRules(ctx, defer452(3), time.Time{})
	if len(store.suppressed) != 0 {
		t.Fatalf("attempt 3 must not suppress (below MinAttempts), got %+v", store.suppressed)
	}

	// At/above the threshold → suppress with the per-rule 30d TTL.
	store = newFakeBounceStore()
	w = newWorker(store, store, fakePolicy{}).WithBounceRules(fakeBounceRules{rules})
	w.applyDeferralRules(ctx, defer452(7), time.Time{})
	if store.suppressed["full@dest.example"] != "bounce" {
		t.Fatalf("attempt 7 should suppress, got %+v", store.suppressed)
	}
	if store.suppressTTL["full@dest.example"] != 30*24*time.Hour {
		t.Fatalf("expected 30d TTL, got %v", store.suppressTTL["full@dest.example"])
	}
}

func TestApplyBouncePolicySoftThreshold(t *testing.T) {
	ctx := context.Background()
	store := newFakeBounceStore()
	w := newWorker(store, store, fakePolicy{biz.BouncePolicy{SoftBounceThreshold: 3}})

	soft := func() *biz.BounceRecord {
		return &biz.BounceRecord{Recipient: "soft@dest.example", SMTPStatus: "451"}
	}
	// First two soft bounces accumulate but do not suppress.
	w.applyBouncePolicy(ctx, soft())
	w.applyBouncePolicy(ctx, soft())
	if len(store.suppressed) != 0 {
		t.Fatalf("below threshold must not suppress, got %+v", store.suppressed)
	}
	// Third reaches the threshold → suppressed.
	w.applyBouncePolicy(ctx, soft())
	if store.suppressed["soft@dest.example"] != "bounce" {
		t.Fatalf("reaching the soft threshold should suppress, got %+v", store.suppressed)
	}
}

func TestApplyBouncePolicySoftDisabled(t *testing.T) {
	ctx := context.Background()
	store := newFakeBounceStore()
	// Threshold 0 disables soft-bounce suppression entirely.
	w := newWorker(store, store, fakePolicy{biz.BouncePolicy{SoftBounceThreshold: 0}})
	for i := 0; i < 10; i++ {
		w.applyBouncePolicy(ctx, &biz.BounceRecord{Recipient: "soft@dest.example", SMTPStatus: "451"})
	}
	if len(store.suppressed) != 0 || len(store.soft) != 0 {
		t.Fatalf("soft threshold 0 should neither count nor suppress, got soft=%+v sup=%+v", store.soft, store.suppressed)
	}
}

func TestApplyBouncePolicyNilSuppressorIsNoop(t *testing.T) {
	store := newFakeBounceStore()
	// A nil suppressor disables the whole pipeline (no panic, no counting).
	w := newWorker(store, nil, fakePolicy{biz.BouncePolicy{AutoSuppressHardBounces: true}})
	w.applyBouncePolicy(context.Background(), &biz.BounceRecord{Recipient: "x@y.example", SMTPStatus: "550"})
	if len(store.suppressed) != 0 {
		t.Fatal("nil suppressor must be a no-op")
	}
}

func TestHandleCapturesDeferralReason(t *testing.T) {
	store := newFakeBounceStore()
	w := newWorker(store, store, fakePolicy{biz.BouncePolicy{}})
	// A TransientFailure (deferral) record carrying the server's 4xx response.
	payload := `{"type":"TransientFailure","id":"m1","sender":"a@s.example","recipient":"vesco@example.com",` +
		`"response":{"code":451,"content":"4.7.1 greylisted, try again later"}}`
	w.handle(context.Background(), data.StreamMessage{ID: "1", Values: map[string]any{"type": "TransientFailure", "data": payload}})

	if len(store.mailEvents) != 1 {
		t.Fatalf("expected 1 mail event, got %d", len(store.mailEvents))
	}
	mr := store.mailEvents[0]
	if mr.Status != biz.MailDeferred {
		t.Fatalf("status = %q, want deferred", mr.Status)
	}
	if mr.SMTPStatus != "451" {
		t.Fatalf("smtp_status = %q, want 451", mr.SMTPStatus)
	}
	if mr.Diagnostic != "4.7.1 greylisted, try again later" {
		t.Fatalf("diagnostic = %q", mr.Diagnostic)
	}
}

func TestBouncePolicyDefaultsToHardSuppress(t *testing.T) {
	// A nil policy provider defaults to auto-suppressing hard bounces.
	w := newWorker(newFakeBounceStore(), newFakeBounceStore(), nil)
	if !w.bouncePolicy(context.Background()).AutoSuppressHardBounces {
		t.Fatal("nil policy provider should default to auto-suppress hard bounces")
	}
}
