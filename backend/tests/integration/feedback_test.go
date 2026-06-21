package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/worker"
)

// TestFeedbackIngestionAutoSuppresses verifies the FBL path: a KumoMTA Feedback
// (ARF) log record is persisted as a feedback report AND the complainant is
// auto-suppressed so future mail to that address is blocked — matching the
// reference build's "FBL events automatically add a suppression entry".
func TestFeedbackIngestionAutoSuppresses(t *testing.T) {
	db := setupDB(t)
	streams := setupStreams(t)
	mailRepo := data.NewMailOpsRepo(db)
	safetyRepo := data.NewDomainSafetyRepo(db)

	const stream = "iris.mail.events.fbl"
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	complainant := "angry-user@dest.example"
	record := `{"type":"Feedback","recipient":"bounce@iris.local",
		"feedback_report":{"feedback_type":"abuse","original_rcpto_to":["` + complainant + `"],
			"reporting_mta":{"mta_type":"dns","name":"fbl.provider.net"}}}`
	if err := streams.EnsureGroup(ctx, stream, "iris-logstream"); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	if _, err := streams.Publish(ctx, stream, map[string]any{"type": "Feedback", "data": record}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	w := worker.NewLogStreamWorker(streams, mailRepo, safetyRepo, nil, stream, biz.NewLogger("error"))
	done := make(chan struct{})
	go func() { _ = w.Run(ctx); close(done) }()

	// Poll until the complainant is suppressed (the side effect of ingest).
	deadline := time.Now().Add(8 * time.Second)
	suppressed := false
	for time.Now().Before(deadline) {
		ok, err := safetyRepo.IsSuppressed(context.Background(), complainant)
		if err != nil {
			t.Fatalf("is suppressed: %v", err)
		}
		if ok {
			suppressed = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	<-done

	if !suppressed {
		t.Fatalf("expected complainant %s to be auto-suppressed", complainant)
	}

	// The feedback report was persisted with the parsed fields.
	reports, err := mailRepo.ListFeedbackReports(context.Background(), biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list feedback: %v", err)
	}
	found := false
	for _, f := range reports {
		if f.Recipient == complainant && f.ReportType == "abuse" && f.Source == "fbl.provider.net" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a persisted feedback report for %s, got %d reports", complainant, len(reports))
	}

	// And the suppression entry carries the FBL source.
	supps, err := safetyRepo.ListSuppressions(context.Background(), biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list suppressions: %v", err)
	}
	srcOK := false
	for _, s := range supps {
		if s.Value == complainant && s.Source == "fbl" && s.Status == biz.SuppressActive {
			srcOK = true
		}
	}
	if !srcOK {
		t.Fatalf("expected an active fbl-sourced suppression for %s", complainant)
	}
}
