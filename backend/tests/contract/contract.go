// Package contract holds in-process contract tests that exercise the generated
// API handler layer (the Service) against DB-backed use cases, asserting the
// request/response shapes defined by the proto contract. Tests skip unless
// IRIS_TEST_DSN is set.
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

// newService builds a fully-wired Service backed by the test database. The
// Redis-dependent producer is replaced with a no-op so contract tests do not
// require Redis.
func newService(t *testing.T) *service.Service {
	t.Helper()
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
	if _, err := db.Pool.Exec(ctx, `
		TRUNCATE routing_rules, vmta_group_members, vmta_groups, vmtas, listeners,
		         suppression_entries, dkim_domains, webhook_rules,
		         rspamd_filter_results, mail_records, config_state,
		         user_roles, iris_users, roles,
		         audit_entries, service_control_requests RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Reset the singleton settings row to defaults (it is not truncated).
	if _, err := db.Pool.Exec(ctx, `
		UPDATE global_settings SET rspamd_mode='', rspamd_url='', egress_ehlo_domain='',
			log_stream_redis_url='', esmtp_listen='', http_listen='', updated_by='' WHERE id=1`); err != nil {
		t.Fatalf("reset settings: %v", err)
	}

	auditRepo := data.NewAuditRepo(db)
	auditor := biz.NewAuditor(auditRepo)
	outboundRepo := data.NewOutboundConfigRepo(db)
	domainSafetyRepo := data.NewDomainSafetyRepo(db)
	domainSafety := biz.NewDomainSafetyUsecase(domainSafetyRepo, auditor)
	mailOpsRepo := data.NewMailOpsRepo(db)
	// The settings use case is both an API surface and the config generator's
	// effective-settings provider (mirrors main.go wiring).
	settingsUC := biz.NewGlobalSettingsUsecase(
		data.NewGlobalSettingsRepo(db), auditor,
		biz.KumoConfigSettings{LogStreamName: data.StreamMailEvents})

	return service.NewService(service.Deps{
		Auditor:      auditor,
		Outbound:     biz.NewOutboundConfigUsecase(outboundRepo, auditor).WithEligibilityChecker(domainSafety),
		MailOps:      biz.NewMailOpsUsecase(mailOpsRepo, noopProducer{}, auditor).WithQueueAdmin(noopQueueAdmin{}),
		Identity:     biz.NewIdentityUsecase(data.NewIdentityRepo(db, auditRepo), biz.NewPlaceholderMFA(), auditor),
		DomainSafety: domainSafety,
		Inbound:      biz.NewInboundUsecase(data.NewInboundRepo(db), auditor, true),
		FBL:          biz.NewFBLUsecase(data.NewFBLRepo(db), auditor),
		Dashboard:    biz.NewDashboardUsecase(data.NewDashboardRepo(db)),
		KumoConfig: biz.NewKumoConfigUsecase(
			data.NewKumoConfigRepo(outboundRepo, domainSafetyRepo, data.NewInboundRepo(db), data.NewFBLRepo(db)), biz.NewStubKumoMTA(), mailOpsRepo, auditor,
			settingsUC),
		Settings: settingsUC,
	})
}

// noopProducer satisfies biz.CommandProducer without Redis.
type noopProducer struct{}

func (noopProducer) PublishQueueCommand(context.Context, string, string, string) (string, error) {
	return "noop", nil
}
func (noopProducer) PublishServiceCommand(context.Context, string, string) (string, error) {
	return "noop", nil
}

// noopQueueAdmin satisfies biz.KumoQueueAdmin without a live kumod.
type noopQueueAdmin struct{}

func (noopQueueAdmin) QueueSummary(context.Context) ([]*biz.QueueState, error) { return nil, nil }
func (noopQueueAdmin) SuspendQueue(context.Context, string, string) (string, error) {
	return "ok", nil
}
func (noopQueueAdmin) ResumeQueue(context.Context, string) (string, error)         { return "ok", nil }
func (noopQueueAdmin) BounceQueue(context.Context, string, string) (string, error) { return "ok", nil }

// seedListener creates a listener and returns its id, for VMTAs to attach to.
func seedListener(t *testing.T, svc *service.Service, name, ip string) string {
	t.Helper()
	l, err := svc.CreateListener(ownerCtx(), &adminv1.CreateListenerRequest{
		Name: name, IpAddress: ip, Port: 25, Hostname: "mta.example.com",
	})
	if err != nil {
		t.Fatalf("seed listener: %v", err)
	}
	return l.GetId()
}

// ownerCtx returns a context with a full-permission identity (valid UUID actor).
func ownerCtx() context.Context {
	return biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "00000000-0000-0000-0000-000000000000",
		Permissions: biz.NewPermissionSet([]string{string(biz.PermAll)}),
		MFAVerified: true,
	})
}
