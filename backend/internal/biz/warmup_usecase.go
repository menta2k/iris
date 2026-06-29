package biz

import (
	"context"
	"strings"
	"time"
)

// WarmupRepo is the persistence boundary for IP-warmup schedules.
type WarmupRepo interface {
	CreateWarmup(ctx context.Context, w *WarmupSchedule) (*WarmupSchedule, error)
	UpdateWarmup(ctx context.Context, id string, w *WarmupSchedule) (*WarmupSchedule, error)
	GetWarmup(ctx context.Context, id string) (*WarmupSchedule, error)
	ListWarmups(ctx context.Context, status string, page Page) ([]*WarmupSchedule, error)
	// ListActiveWarmupsForPolicy returns active+paused schedules (those that
	// affect the rendered policy).
	ListActiveWarmupsForPolicy(ctx context.Context) ([]*WarmupSchedule, error)
}

// VMTAChecker reports whether a VMTA exists (so warmup gives a friendly error
// instead of relying on the FK violation). Satisfied by the outbound repo.
type VMTAChecker interface {
	VMTAExists(ctx context.Context, id string) (bool, error)
}

// WarmupUsecase manages IP-warmup schedules: CRUD, curve selection, and the
// lifecycle (scheduled→active→completed, plus pause/resume). The rendered cap is
// applied per egress path; see ResolveWarmupRates and writeWarmupTables.
type WarmupUsecase struct {
	repo    WarmupRepo
	vmtas   VMTAChecker
	auditor *Auditor
}

// NewWarmupUsecase constructs the use case. vmtas may be nil (existence is then
// enforced only by the DB foreign key).
func NewWarmupUsecase(repo WarmupRepo, vmtas VMTAChecker, auditor *Auditor) *WarmupUsecase {
	return &WarmupUsecase{repo: repo, vmtas: vmtas, auditor: auditor}
}

// WarmupCurveInfo describes a built-in curve template for the UI.
type WarmupCurveInfo struct {
	Name   string
	Stages []WarmupStage
}

// Curves returns the built-in warmup curve templates (name + resolved stages).
// No permission check — descriptive reference data for the editor.
func (uc *WarmupUsecase) Curves() []WarmupCurveInfo {
	names := WarmupCurveNames()
	out := make([]WarmupCurveInfo, 0, len(names))
	for _, n := range names {
		stages, _ := ResolveWarmupCurve(n)
		out = append(out, WarmupCurveInfo{Name: n, Stages: stages})
	}
	return out
}

// List returns warmup schedules, optionally filtered by status.
func (uc *WarmupUsecase) List(ctx context.Context, status string, page Page) ([]*WarmupSchedule, error) {
	if _, err := RequirePermission(ctx, PermVMTARead); err != nil {
		return nil, err
	}
	return uc.repo.ListWarmups(ctx, strings.TrimSpace(status), page)
}

// ActiveForPolicy returns the active+paused schedules the renderer needs.
// Internal (used by the snapshot loader); no permission check.
func (uc *WarmupUsecase) ActiveForPolicy(ctx context.Context) ([]*WarmupSchedule, error) {
	return uc.repo.ListActiveWarmupsForPolicy(ctx)
}

// Create validates and persists a new schedule, deriving its initial status from
// the start date (active when it starts today or earlier, else scheduled).
func (uc *WarmupUsecase) Create(ctx context.Context, w *WarmupSchedule) (*WarmupSchedule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	w.Status = WarmupScheduled
	w.HeldDay = 0
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if err := uc.requireVMTA(ctx, w.VMTAID); err != nil {
		return nil, err
	}
	w.Status = initialWarmupStatus(w, dayStart(time.Now()))
	out, err := uc.repo.CreateWarmup(ctx, w)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "warmup.create", out.ID, map[string]any{"vmta_id": out.VMTAID, "curve": out.Curve})
	return out, nil
}

// Update edits a non-completed schedule's curve / start date and re-derives the
// scheduled/active status. Pause state is cleared (edit implies a fresh ramp).
func (uc *WarmupUsecase) Update(ctx context.Context, id string, w *WarmupSchedule) (*WarmupSchedule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	existing, err := uc.repo.GetWarmup(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.Status == WarmupCompleted {
		return nil, FailedPrecondition("WARMUP_COMPLETED", "a completed warmup cannot be edited")
	}
	w.VMTAID = existing.VMTAID // VMTA is immutable for a schedule
	w.Status = WarmupScheduled
	w.HeldDay = 0
	w.PausedReason = ""
	if err := w.Validate(); err != nil {
		return nil, err
	}
	w.Status = initialWarmupStatus(w, dayStart(time.Now()))
	out, err := uc.repo.UpdateWarmup(ctx, id, w)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "warmup.update", id, map[string]any{"curve": out.Curve})
	return out, nil
}

// Pause freezes an active ramp at its current day (the cap holds until resumed).
func (uc *WarmupUsecase) Pause(ctx context.Context, id, reason string) (*WarmupSchedule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	w, err := uc.repo.GetWarmup(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != WarmupActive {
		return nil, FailedPrecondition("WARMUP_NOT_ACTIVE", "only an active warmup can be paused")
	}
	w.Status = WarmupPaused
	w.HeldDay = w.DayIndex(dayStart(time.Now()))
	if w.HeldDay < 1 {
		w.HeldDay = 1
	}
	w.PausedReason = strings.TrimSpace(reason)
	out, err := uc.repo.UpdateWarmup(ctx, id, w)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "warmup.pause", id, map[string]any{"held_day": out.HeldDay})
	return out, nil
}

// Resume continues a paused ramp from the held day, shifting StartDate so the
// calendar day index lines up with the held day again.
func (uc *WarmupUsecase) Resume(ctx context.Context, id string) (*WarmupSchedule, error) {
	if _, err := RequirePermission(ctx, PermVMTAWrite); err != nil {
		return nil, err
	}
	w, err := uc.repo.GetWarmup(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != WarmupPaused {
		return nil, FailedPrecondition("WARMUP_NOT_PAUSED", "only a paused warmup can be resumed")
	}
	today := dayStart(time.Now())
	w.StartDate = today.AddDate(0, 0, -(w.HeldDay - 1))
	w.HeldDay = 0
	w.PausedReason = ""
	w.Status = WarmupActive
	if w.DayIndex(today) > w.DurationDays() {
		w.Status = WarmupCompleted
	}
	out, err := uc.repo.UpdateWarmup(ctx, id, w)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, AuditSuccess, "warmup.resume", id, nil)
	return out, nil
}

// Tick advances scheduled→active and active→completed for the given date and
// reports whether anything changed (so the worker re-applies the policy). Paused
// schedules are left frozen. No permission check — internal worker call.
func (uc *WarmupUsecase) Tick(ctx context.Context, today time.Time) (bool, error) {
	today = dayStart(today)
	sched, err := uc.repo.ListWarmups(ctx, "", Page{Size: MaxPageSize})
	if err != nil {
		return false, err
	}
	changed := false
	for _, w := range sched {
		next := w.Status
		switch w.Status {
		case WarmupScheduled:
			if !w.StartDate.After(today) {
				next = WarmupActive
			}
		case WarmupActive:
			if w.DayIndex(today) > w.DurationDays() {
				next = WarmupCompleted
			}
		}
		if next == w.Status {
			continue
		}
		w.Status = next
		if _, err := uc.repo.UpdateWarmup(ctx, w.ID, w); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

func (uc *WarmupUsecase) requireVMTA(ctx context.Context, id string) error {
	if uc.vmtas == nil {
		return nil
	}
	ok, err := uc.vmtas.VMTAExists(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return Invalid("WARMUP_VMTA_NOT_FOUND", "vmta %q does not exist", id)
	}
	return nil
}

// initialWarmupStatus is active when the ramp has already started, else scheduled.
func initialWarmupStatus(w *WarmupSchedule, today time.Time) string {
	if w.StartDate.After(today) {
		return WarmupScheduled
	}
	if w.DayIndex(today) > w.DurationDays() {
		return WarmupCompleted
	}
	return WarmupActive
}

func (uc *WarmupUsecase) audit(ctx context.Context, outcome AuditOutcome, action, id string, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, action, "warmup", id, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", action, "error", err.Error())
	}
}
