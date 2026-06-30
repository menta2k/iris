package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

const warmupDateFmt = "2006-01-02"

// ListWarmupSchedules returns warmup schedules plus the built-in curve templates.
func (s *Service) ListWarmupSchedules(ctx context.Context, req *adminv1.ListWarmupSchedulesRequest) (*adminv1.ListWarmupSchedulesReply, error) {
	if s.warmup == nil {
		return nil, notImplemented("ListWarmupSchedules")
	}
	page := pageFrom(req.GetPage())
	items, err := s.warmup.List(ctx, req.GetStatus(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListWarmupSchedules", err)
	}
	out := &adminv1.ListWarmupSchedulesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, w := range items {
		out.Items = append(out.Items, warmupToProto(w))
	}
	for _, c := range s.warmup.Curves() {
		out.Curves = append(out.Curves, &adminv1.WarmupCurve{Name: c.Name, Stages: stagesToProto(c.Stages)})
	}
	return out, nil
}

// CreateWarmupSchedule starts a new ramp for a VMTA using a curve template.
func (s *Service) CreateWarmupSchedule(ctx context.Context, req *adminv1.CreateWarmupScheduleRequest) (*adminv1.WarmupSchedule, error) {
	if s.warmup == nil {
		return nil, notImplemented("CreateWarmupSchedule")
	}
	start, err := parseWarmupDate(req.GetStartDate())
	if err != nil {
		return nil, s.fail(ctx, "CreateWarmupSchedule", err)
	}
	out, err := s.warmup.Create(ctx, &biz.WarmupSchedule{
		VMTAID: req.GetVmtaId(), StartDate: start, Curve: req.GetCurve(), Stages: stagesFromProto(req.GetStages()),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateWarmupSchedule", err)
	}
	return warmupToProto(out), nil
}

// UpdateWarmupSchedule edits a non-completed schedule's curve/start date.
func (s *Service) UpdateWarmupSchedule(ctx context.Context, req *adminv1.UpdateWarmupScheduleRequest) (*adminv1.WarmupSchedule, error) {
	if s.warmup == nil {
		return nil, notImplemented("UpdateWarmupSchedule")
	}
	start, err := parseWarmupDate(req.GetStartDate())
	if err != nil {
		return nil, s.fail(ctx, "UpdateWarmupSchedule", err)
	}
	out, err := s.warmup.Update(ctx, req.GetId(), &biz.WarmupSchedule{
		StartDate: start, Curve: req.GetCurve(), Stages: stagesFromProto(req.GetStages()),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateWarmupSchedule", err)
	}
	return warmupToProto(out), nil
}

// PauseWarmupSchedule freezes an active ramp at its current cap.
func (s *Service) PauseWarmupSchedule(ctx context.Context, req *adminv1.PauseWarmupScheduleRequest) (*adminv1.WarmupSchedule, error) {
	if s.warmup == nil {
		return nil, notImplemented("PauseWarmupSchedule")
	}
	out, err := s.warmup.Pause(ctx, req.GetId(), req.GetReason())
	if err != nil {
		return nil, s.fail(ctx, "PauseWarmupSchedule", err)
	}
	return warmupToProto(out), nil
}

// ResumeWarmupSchedule continues a paused ramp from its held day.
func (s *Service) ResumeWarmupSchedule(ctx context.Context, req *adminv1.ResumeWarmupScheduleRequest) (*adminv1.WarmupSchedule, error) {
	if s.warmup == nil {
		return nil, notImplemented("ResumeWarmupSchedule")
	}
	out, err := s.warmup.Resume(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "ResumeWarmupSchedule", err)
	}
	return warmupToProto(out), nil
}

func parseWarmupDate(s string) (time.Time, error) {
	t, err := time.Parse(warmupDateFmt, s)
	if err != nil {
		return time.Time{}, biz.Invalid("WARMUP_START_INVALID", "start_date %q must be YYYY-MM-DD", s)
	}
	return t, nil
}

func warmupToProto(w *biz.WarmupSchedule) *adminv1.WarmupSchedule {
	p := &adminv1.WarmupSchedule{
		Id:           w.ID,
		VmtaId:       w.VMTAID,
		VmtaName:     w.VMTAName,
		StartDate:    w.StartDate.Format(warmupDateFmt),
		Curve:        w.Curve,
		Stages:       stagesToProto(w.Stages),
		Status:       w.Status,
		PausedReason: w.PausedReason,
		HeldDay:      int32(w.HeldDay),
	}
	if !w.CreatedAt.IsZero() {
		p.CreatedAt = w.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !w.UpdatedAt.IsZero() {
		p.UpdatedAt = w.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return p
}

func stagesFromProto(in []*adminv1.WarmupStage) []biz.WarmupStage {
	out := make([]biz.WarmupStage, 0, len(in))
	for _, s := range in {
		caps := make(map[string]int, len(s.GetCaps()))
		for k, v := range s.GetCaps() {
			caps[k] = int(v)
		}
		out = append(out, biz.WarmupStage{DayFrom: int(s.GetDayFrom()), DayTo: int(s.GetDayTo()), Caps: caps})
	}
	return out
}

func stagesToProto(stages []biz.WarmupStage) []*adminv1.WarmupStage {
	out := make([]*adminv1.WarmupStage, 0, len(stages))
	for _, s := range stages {
		caps := make(map[string]int32, len(s.Caps))
		for k, v := range s.Caps {
			caps[k] = int32(v)
		}
		out = append(out, &adminv1.WarmupStage{DayFrom: int32(s.DayFrom), DayTo: int32(s.DayTo), Caps: caps})
	}
	return out
}
