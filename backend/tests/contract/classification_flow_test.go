package contract

import (
	"context"
	"os"
	"testing"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/service"
)

// TestMailRecordClassificationRoundTrip guards the biz→proto mapping for the
// subject-classification label: a persisted mail_records.classification must
// actually surface in the ListMailRecords API response. (Regression: the label
// was written to the DB and scanned into the biz struct but dropped at the proto
// boundary, so the Logs UI never showed it.)
func TestMailRecordClassificationRoundTrip(t *testing.T) {
	dsn := os.Getenv("IRIS_TEST_DSN")
	if dsn == "" {
		t.Skip("IRIS_TEST_DSN not set; skipping contract test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, cleanup, err := data.NewDB(ctx, conf.Database{DSN: dsn, MaxConns: 4, MinConns: 1})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(cleanup)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `TRUNCATE mail_records`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	repo := data.NewMailOpsRepo(db)
	const mid = "cls-roundtrip-1"
	if err := repo.InsertMailEvent(ctx, &biz.MailRecord{
		MessageID: mid, EventTime: time.Now().UTC(), Mailclass: "kmx-test",
		Recipient: "a@b.com", RecipientDomain: "b.com", Status: biz.MailReceived, RecordType: "Reception",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.UpdateClassification(ctx, mid, "test email"); err != nil {
		t.Fatalf("update classification: %v", err)
	}

	svc := service.NewService(service.Deps{
		MailOps: biz.NewMailOpsUsecase(repo, noopProducer{}, biz.NewAuditor(data.NewAuditRepo(db))).
			WithQueueAdmin(noopQueueAdmin{}),
	})
	reply, err := svc.ListMailRecords(ownerCtx(), &adminv1.ListMailRecordsRequest{Mailclass: "kmx-test"})
	if err != nil {
		t.Fatalf("ListMailRecords: %v", err)
	}
	var found bool
	for _, it := range reply.GetItems() {
		if it.GetMessageId() == mid {
			found = true
			if it.GetClassification() != "test email" {
				t.Fatalf("classification missing from API response: got %q", it.GetClassification())
			}
		}
	}
	if !found {
		t.Fatal("seeded mail record not returned by ListMailRecords")
	}
}
