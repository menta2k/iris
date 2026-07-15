package data

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// testDB connects to IRIS_TEST_DSN and applies migrations, skipping when the DSN
// is unset (mirrors the contract harness). Returns a migrated *DB.
func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("IRIS_TEST_DSN")
	if dsn == "" {
		t.Skip("IRIS_TEST_DSN not set; skipping data integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, cleanup, err := NewDB(ctx, conf.Database{DSN: dsn, MaxConns: 4, MinConns: 1})
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(cleanup)
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDashboardDeliveryStats(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if _, err := db.Pool.Exec(ctx,
		`TRUNCATE mail_records, vmtas RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// A VMTA whose name matches the egress source, so the LEFT JOIN recovers its id.
	var vmtaID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO vmtas (name, ip_address, ehlo_name, status)
		VALUES ('ip-a', '203.0.113.10', 'mail.ip-a.example', 'active')
		RETURNING id`).Scan(&vmtaID); err != nil {
		t.Fatalf("insert vmta: %v", err)
	}

	insert := func(mid, src, domain, status string, at time.Time, n int) {
		t.Helper()
		for range n {
			if _, err := db.Pool.Exec(ctx, `
				INSERT INTO mail_records
					(message_id, event_time, mailclass, sender, recipient,
					 recipient_domain, egress_source, status, record_type)
				VALUES ($5, $1, 'default', 's@example.com', 'r@'||$2, $2, $3, $4, 'Delivery')`,
				at, domain, src, status, mid); err != nil {
				t.Fatalf("insert mail_record: %v", err)
			}
		}
	}

	now := time.Now()
	recent := now.Add(-10 * time.Minute)
	old := now.Add(-2 * time.Hour)

	insert("m", "ip-a", "gmail.com", biz.MailSent, recent, 9)
	insert("m", "ip-a", "gmail.com", biz.MailBounced, recent, 1)
	// Deferred is counted as DISTINCT messages, not attempts: two distinct
	// messages deferred, and one of them (md1) deferred twice — so 3 deferred
	// rows but only 2 distinct messages.
	insert("md1", "ip-a", "gmail.com", biz.MailDeferred, recent, 2)
	insert("md2", "ip-a", "gmail.com", biz.MailDeferred, recent, 1)
	insert("m", "ip-a", "yahoo.com", biz.MailSent, recent, 1)
	insert("m", "ip-x", "gmail.com", biz.MailSent, recent, 1) // egress with no matching VMTA
	insert("m", "", "gmail.com", biz.MailReceived, recent, 1) // reception: empty egress, excluded
	insert("m", "ip-a", "gmail.com", biz.MailSent, old, 5)    // outside the window, excluded

	repo := NewDashboardRepo(db)
	rows, err := repo.DeliveryStats(ctx, now.Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("delivery stats: %v", err)
	}

	byKey := map[string]biz.WarmupDeliveryStat{}
	for _, r := range rows {
		byKey[r.VMTAName+"|"+r.RecipientDomain] = r
	}

	// ip-a / gmail.com: 9 sent, 1 bounced, 2 deferred (distinct messages md1+md2,
	// though md1 deferred twice) — the 5 old sents excluded.
	g := byKey["ip-a|gmail.com"]
	if g.Sent != 9 || g.Bounced != 1 || g.Deferred != 2 {
		t.Fatalf("ip-a/gmail counts = %d/%d/%d, want 9/1/2", g.Sent, g.Bounced, g.Deferred)
	}
	if g.VMTAID != vmtaID {
		t.Fatalf("ip-a/gmail vmta_id = %q, want %q", g.VMTAID, vmtaID)
	}

	// ip-x has no VMTA row → empty id, still counted.
	x := byKey["ip-x|gmail.com"]
	if x.Sent != 1 || x.VMTAID != "" {
		t.Fatalf("ip-x/gmail = sent %d id %q, want sent 1 id \"\"", x.Sent, x.VMTAID)
	}

	// Reception (empty egress) and the received status must not appear.
	if _, ok := byKey["|gmail.com"]; ok {
		t.Fatal("empty-egress reception row leaked into stats")
	}

	// Most-active (VMTA, domain) sorts first.
	if len(rows) == 0 || rows[0].RecipientDomain != "gmail.com" || rows[0].VMTAName != "ip-a" {
		t.Fatalf("expected ip-a/gmail.com first, got %+v", rows)
	}
}
