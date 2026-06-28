// Package integration contains TimescaleDB/Redis-backed integration tests.
// Tests are skipped unless IRIS_TEST_DSN is set, so the suite is safe to run in
// environments without a database while still exercising the real stack in CI.
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
)

// setupDB connects to the test database, applies migrations, and truncates the
// configuration tables so each test starts from a clean state. It skips the
// calling test when IRIS_TEST_DSN is not configured.
func setupDB(t *testing.T) *data.DB {
	t.Helper()
	dsn := os.Getenv("IRIS_TEST_DSN")
	if dsn == "" {
		t.Skip("IRIS_TEST_DSN not set; skipping database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, cleanup, err := data.NewDB(ctx, conf.Database{DSN: dsn, MaxConns: 4, MinConns: 1})
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(cleanup)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	truncate(t, db)
	return db
}

func truncate(t *testing.T, db *data.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Pool.Exec(ctx, `
		TRUNCATE config_state, routing_rules, vmta_group_members, vmta_groups, vmtas, listeners,
		         suppression_entries, dkim_domains, rspamd_filter_results,
		         mail_records, bounce_records, feedback_reports,
		         mailclass_queues, queue_snapshots,
		         user_roles, iris_users, roles,
		         audit_entries, service_control_requests RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// setupStreams connects to the test Redis, skipping when IRIS_TEST_REDIS is not
// set. It returns a Streams helper and a unique stream/group suffix so parallel
// test runs do not interfere.
func setupStreams(t *testing.T) *data.Streams {
	t.Helper()
	addr := os.Getenv("IRIS_TEST_REDIS")
	if addr == "" {
		t.Skip("IRIS_TEST_REDIS not set; skipping Redis integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	streams, cleanup, err := data.NewStreams(ctx, conf.Redis{Addr: addr, ConsumerName: "test-consumer"})
	if err != nil {
		t.Fatalf("connect test redis: %v", err)
	}
	t.Cleanup(cleanup)
	return streams
}

// ownerCtx returns a context carrying a full-permission identity for tests.
func ownerCtx() context.Context {
	return biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "test-owner",
		Permissions: biz.NewPermissionSet([]string{string(biz.PermAll)}),
		MFAVerified: true,
	})
}

// readerCtx returns a context with only read permissions for negative tests.
func readerCtx() context.Context {
	return biz.WithIdentity(context.Background(), &biz.Identity{
		UserID: "test-reader",
		Permissions: biz.NewPermissionSet([]string{
			string(biz.PermVMTARead), string(biz.PermRoutingRead),
		}),
		MFAVerified: true,
	})
}
