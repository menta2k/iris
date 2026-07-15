package integration

import (
	"context"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestDashboardSummaryAggregates seeds queue, mail, and audit data and verifies
// the dashboard summary repository aggregates it correctly (exercising the base
// tables that back the continuous-aggregate views).
func TestDashboardSummaryAggregates(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	mailRepo := data.NewMailOpsRepo(db)
	if err := mailRepo.UpsertQueueState(ctx, &biz.MailclassQueue{Mailclass: "bulk", State: biz.QueueRunning, Depth: 9}); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	if err := mailRepo.UpsertQueueState(ctx, &biz.MailclassQueue{Mailclass: "transactional", State: biz.QueueRunning, Depth: 6}); err != nil {
		t.Fatalf("seed queue 2: %v", err)
	}

	// Seed a recent mail event and an audit entry.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO mail_records (message_id, mailclass, recipient, status) VALUES ('m1','bulk','a@b.com','sent')`); err != nil {
		t.Fatalf("seed mail: %v", err)
	}
	if err := data.NewAuditRepo(db).Write(ctx, biz.AuditEvent{Operation: "vmta.create", Outcome: biz.AuditSuccess}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	summary, err := data.NewDashboardRepo(db).Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.QueuedMessages != 15 {
		t.Fatalf("expected total queued 15, got %d", summary.QueuedMessages)
	}
	if summary.RecentMailEvents != 1 {
		t.Fatalf("expected 1 recent mail event, got %d", summary.RecentMailEvents)
	}
	if summary.RecentAuditEvents != 1 {
		t.Fatalf("expected 1 recent audit event, got %d", summary.RecentAuditEvents)
	}
}

// TestDeliveryStatsCountsDistinctDeferrals verifies the warmup "Deferred" column
// counts distinct messages, not retry attempts: a message that logs several
// TransientFailure (deferred) rows must count once.
func TestDeliveryStatsCountsDistinctDeferrals(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	// One message deferred 3 times (3 TransientFailure rows) on vmta-a/gmail.com.
	for range 3 {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO mail_records (message_id, mailclass, recipient, recipient_domain, egress_source, status)
			VALUES ('mdef1','bulk','x@gmail.com','gmail.com','vmta-a','deferred')`); err != nil {
			t.Fatalf("seed deferred: %v", err)
		}
	}
	// A second distinct message deferred once on the same vmta/domain.
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO mail_records (message_id, mailclass, recipient, recipient_domain, egress_source, status)
		VALUES ('mdef2','bulk','y@gmail.com','gmail.com','vmta-a','deferred')`); err != nil {
		t.Fatalf("seed deferred 2: %v", err)
	}

	rows, err := data.NewDashboardRepo(db).DeliveryStats(ctx, time.Now().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("delivery stats: %v", err)
	}
	var got *biz.WarmupDeliveryStat
	for i := range rows {
		if rows[i].VMTAName == "vmta-a" && rows[i].RecipientDomain == "gmail.com" {
			got = &rows[i]
		}
	}
	if got == nil {
		t.Fatal("expected a vmta-a/gmail.com row")
	}
	// 4 deferred rows across 2 distinct messages → Deferred must be 2, not 4.
	if got.Deferred != 2 {
		t.Fatalf("deferred = %d, want 2 (distinct messages, not attempts)", got.Deferred)
	}
}
