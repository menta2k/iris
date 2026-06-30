package biz

import (
	"context"
	"testing"
	"time"
)

// fakeWarmupRepo is an in-memory WarmupRepo for usecase tests.
type fakeWarmupRepo struct {
	items map[string]*WarmupSchedule
	seq   int
}

func newFakeWarmupRepo() *fakeWarmupRepo { return &fakeWarmupRepo{items: map[string]*WarmupSchedule{}} }

func (f *fakeWarmupRepo) CreateWarmup(_ context.Context, w *WarmupSchedule) (*WarmupSchedule, error) {
	f.seq++
	cp := *w
	cp.ID = string(rune('a' + f.seq))
	f.items[cp.ID] = &cp
	out := cp
	return &out, nil
}
func (f *fakeWarmupRepo) UpdateWarmup(_ context.Context, id string, w *WarmupSchedule) (*WarmupSchedule, error) {
	cp := *w
	cp.ID = id
	f.items[id] = &cp
	out := cp
	return &out, nil
}
func (f *fakeWarmupRepo) GetWarmup(_ context.Context, id string) (*WarmupSchedule, error) {
	w, ok := f.items[id]
	if !ok {
		return nil, NotFound("WARMUP_NOT_FOUND", "no warmup %q", id)
	}
	out := *w
	return &out, nil
}
func (f *fakeWarmupRepo) ListWarmups(_ context.Context, status string, _ Page) ([]*WarmupSchedule, error) {
	var out []*WarmupSchedule
	for _, w := range f.items {
		if status == "" || w.Status == status {
			cp := *w
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeWarmupRepo) ListActiveWarmupsForPolicy(_ context.Context) ([]*WarmupSchedule, error) {
	var out []*WarmupSchedule
	for _, w := range f.items {
		if w.Status == WarmupActive || w.Status == WarmupPaused {
			cp := *w
			out = append(out, &cp)
		}
	}
	return out, nil
}

func TestWarmupLifecycle(t *testing.T) {
	repo := newFakeWarmupRepo()
	uc := NewWarmupUsecase(repo, nil, nil)
	ctx := ownerCheckCtx()

	// Create with a start 2 days ago (mid-ramp) → immediately active.
	start := dayStart(time.Now()).AddDate(0, 0, -2)
	created, err := uc.Create(ctx, &WarmupSchedule{VMTAID: "v1", StartDate: start, Curve: CurveStandard})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != WarmupActive {
		t.Fatalf("recent start should be active, got %q", created.Status)
	}

	// Unknown curve is rejected.
	if _, err := uc.Create(ctx, &WarmupSchedule{VMTAID: "v2", StartDate: start, Curve: "bogus"}); err == nil {
		t.Fatal("unknown curve must be rejected")
	}

	// Pause freezes at the current day; resume shifts StartDate so the held day
	// lines up with today again and the ramp continues.
	paused, err := uc.Pause(ctx, created.ID, "manual hold")
	if err != nil || paused.Status != WarmupPaused || paused.HeldDay < 1 {
		t.Fatalf("pause: status=%q held=%d err=%v", paused.Status, paused.HeldDay, err)
	}
	resumed, err := uc.Resume(ctx, created.ID)
	if err != nil || resumed.Status != WarmupActive || resumed.HeldDay != 0 {
		t.Fatalf("resume: status=%q held=%d err=%v", resumed.Status, resumed.HeldDay, err)
	}
	// After resume, the day index today equals the held day at pause time.
	if got := resumed.DayIndex(dayStart(time.Now())); got != paused.HeldDay {
		t.Fatalf("resume should continue from held day %d, got day %d", paused.HeldDay, got)
	}

	// Resuming a non-paused (now active) schedule is rejected.
	if _, err := uc.Resume(ctx, created.ID); err == nil {
		t.Fatal("resume on an active schedule must be rejected")
	}
}

func TestWarmupTickTransitions(t *testing.T) {
	repo := newFakeWarmupRepo()
	uc := NewWarmupUsecase(repo, nil, nil)
	ctx := ownerCheckCtx()

	// A schedule that starts tomorrow is 'scheduled'; Tick on/after the start
	// activates it.
	start := dayStart(time.Now()).AddDate(0, 0, 1)
	w, _ := uc.Create(ctx, &WarmupSchedule{VMTAID: "v1", StartDate: start, Curve: CurveAggressive})
	if w.Status != WarmupScheduled {
		t.Fatalf("future start should be scheduled, got %q", w.Status)
	}
	changed, err := uc.Tick(ctx, start)
	if err != nil || !changed {
		t.Fatalf("tick at start should activate: changed=%v err=%v", changed, err)
	}
	got, _ := repo.GetWarmup(ctx, w.ID)
	if got.Status != WarmupActive {
		t.Fatalf("after tick, want active, got %q", got.Status)
	}
	// Tick past the curve end completes it.
	if _, err := uc.Tick(ctx, start.AddDate(0, 0, got.DurationDays()+1)); err != nil {
		t.Fatalf("tick to completion: %v", err)
	}
	got, _ = repo.GetWarmup(ctx, w.ID)
	if got.Status != WarmupCompleted {
		t.Fatalf("past curve end should be completed, got %q", got.Status)
	}
}
