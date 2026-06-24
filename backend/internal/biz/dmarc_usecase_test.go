package biz

import (
	"context"
	"testing"
	"time"
)

type fakeDMARCRepo struct {
	ingested int
	stats    *DMARCStats
}

func (f *fakeDMARCRepo) InsertReport(_ context.Context, _ *DMARCReport, _ []DMARCRecord) error {
	f.ingested++
	return nil
}
func (f *fakeDMARCRepo) ListDomains(context.Context) ([]string, error) {
	return []string{"example.com"}, nil
}
func (f *fakeDMARCRepo) ListReports(context.Context, string, Page) ([]*DMARCReport, error) {
	return nil, nil
}
func (f *fakeDMARCRepo) Stats(context.Context, DMARCFilter) (*DMARCStats, error) {
	return f.stats, nil
}

func TestDMARCStatsRequiresPermission(t *testing.T) {
	uc := NewDMARCUsecase(&fakeDMARCRepo{}, nil)
	if _, err := uc.Stats(context.Background(), "", time.Time{}, time.Time{}); err == nil {
		t.Fatal("expected authorization error without identity")
	}
}

func TestDMARCIngestPassthrough(t *testing.T) {
	repo := &fakeDMARCRepo{}
	uc := NewDMARCUsecase(repo, nil)
	// Ingest runs on an internal context (no permission needed).
	if err := uc.Ingest(context.Background(), &DMARCReport{Domain: "example.com"}, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if repo.ingested != 1 {
		t.Fatalf("expected 1 ingest, got %d", repo.ingested)
	}
}

func TestDMARCStatsAuthorized(t *testing.T) {
	repo := &fakeDMARCRepo{stats: &DMARCStats{TotalMessages: 42}}
	uc := NewDMARCUsecase(repo, nil)
	st, err := uc.Stats(ownerCheckCtx(), "example.com", time.Time{}, time.Time{})
	if err != nil || st.TotalMessages != 42 {
		t.Fatalf("stats: st=%+v err=%v", st, err)
	}
}
