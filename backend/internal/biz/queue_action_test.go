package biz

import (
	"context"
	"testing"
)

type fakeQueueAdmin struct {
	summary    []*QueueState
	lastAction string
	lastDomain string
}

func (f *fakeQueueAdmin) QueueSummary(context.Context) ([]*QueueState, error) { return f.summary, nil }
func (f *fakeQueueAdmin) SuspendQueue(_ context.Context, d, _ string) (string, error) {
	f.lastAction, f.lastDomain = "suspend", d
	return "ok", nil
}
func (f *fakeQueueAdmin) ResumeQueue(_ context.Context, d string) (string, error) {
	f.lastAction, f.lastDomain = "resume", d
	return "ok", nil
}
func (f *fakeQueueAdmin) BounceQueue(_ context.Context, d, _ string) (string, error) {
	f.lastAction, f.lastDomain = "bounce", d
	return "ok", nil
}

func queueUC(q KumoQueueAdmin) *MailOpsUsecase {
	return NewMailOpsUsecase(&fakeMailOpsRepo{}, &fakeProducer{}, nil).WithQueueAdmin(q)
}

func TestListQueuesDelegatesToAdmin(t *testing.T) {
	q := &fakeQueueAdmin{summary: []*QueueState{{Domain: "jobs.bg", Depth: 7}}}
	got, err := queueUC(q).ListQueues(ownerCtx())
	if err != nil || len(got) != 1 || got[0].Domain != "jobs.bg" || got[0].Depth != 7 {
		t.Fatalf("ListQueues: got=%+v err=%v", got, err)
	}
}

func TestQueueActionSuspendResume(t *testing.T) {
	q := &fakeQueueAdmin{}
	uc := queueUC(q)
	if _, err := uc.RequestQueueAction(ownerCtx(), "suspend", "jobs.bg", "maint", ""); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if q.lastAction != "suspend" || q.lastDomain != "jobs.bg" {
		t.Fatalf("suspend not dispatched: %+v", q)
	}
	if _, err := uc.RequestQueueAction(ownerCtx(), "resume", "jobs.bg", "", ""); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if q.lastAction != "resume" {
		t.Fatalf("resume not dispatched: %+v", q)
	}
}

func TestQueueBounceRequiresConfirmation(t *testing.T) {
	q := &fakeQueueAdmin{}
	uc := queueUC(q)
	// No confirmation id → rejected, adapter not called.
	if _, err := uc.RequestQueueAction(ownerCtx(), "bounce", "jobs.bg", "", ""); err == nil {
		t.Fatal("expected confirmation-required error")
	}
	if q.lastAction != "" {
		t.Fatalf("bounce should not dispatch without confirmation: %+v", q)
	}
	// With confirmation → dispatched.
	if _, err := uc.RequestQueueAction(ownerCtx(), "bounce", "jobs.bg", "", "c1"); err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if q.lastAction != "bounce" {
		t.Fatalf("bounce not dispatched: %+v", q)
	}
}

func TestQueueActionInvalidVerb(t *testing.T) {
	if _, err := queueUC(&fakeQueueAdmin{}).RequestQueueAction(ownerCtx(), "drain", "jobs.bg", "", ""); err == nil {
		t.Fatal("expected invalid-action error")
	}
}
