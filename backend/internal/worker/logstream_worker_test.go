package worker

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// fakeBounceStore records the bounce-pipeline side effects the policy drives.
type fakeBounceStore struct {
	soft             map[string]int
	suppressed       map[string]string // recipient -> source
	recipientByMsgID map[string]string
	suppressErr      error
	mailEvents       []*biz.MailRecord
}

func newFakeBounceStore() *fakeBounceStore {
	return &fakeBounceStore{soft: map[string]int{}, suppressed: map[string]string{}, recipientByMsgID: map[string]string{}}
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

func (f *fakeBounceStore) SuppressRecipient(_ context.Context, email, source, _ string) error {
	if f.suppressErr != nil {
		return f.suppressErr
	}
	f.suppressed[email] = source
	return nil
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
	payload := `{"type":"TransientFailure","id":"m1","sender":"a@s.example","recipient":"vesco@jobs.bg",` +
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
