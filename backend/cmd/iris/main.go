// Command iris is the entrypoint for the Iris KumoMTA admin backend. It wires
// configuration, storage, business use cases, and the HTTP/gRPC transports,
// then runs them under a Kratos application with graceful shutdown.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/transport"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/acme"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
	"github.com/menta2k/iris/backend/internal/server"
	"github.com/menta2k/iris/backend/internal/service"
	"github.com/menta2k/iris/backend/internal/worker"
)

// envOr returns the env var value or a default.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDuration parses a duration env var, falling back to def on absence/parse error.
func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/iris.yaml", "path to the configuration file")
	flag.Parse()

	cfg, err := conf.Load(configPath)
	if err != nil {
		panic(err)
	}
	log := biz.NewLogger(cfg.Log.Level)

	// Root context cancelled on interrupt/terminate for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, cleanup, err := buildApp(ctx, cfg, log)
	if err != nil {
		log.Error("startup failed", "error", err.Error())
		os.Exit(1)
	}
	defer cleanup()

	log.Info("starting iris", "http", cfg.Server.HTTP.Addr, "grpc", cfg.Server.GRPC.Addr)
	if err := app.Run(); err != nil {
		log.Error("server exited with error", "error", err.Error())
		os.Exit(1)
	}
	log.Info("iris stopped")
}

// buildApp wires all dependencies and returns the Kratos application plus a
// cleanup function that releases storage connections.
func buildApp(ctx context.Context, cfg *conf.Config, log *slog.Logger) (*kratos.App, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	db, dbCleanup, err := data.NewDB(ctx, cfg.Data.Database)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	cleanups = append(cleanups, dbCleanup)

	if cfg.Data.Database.MigrateOnStart {
		if err := db.Migrate(ctx); err != nil {
			cleanup()
			return nil, nil, err
		}
		log.Info("database migrations applied")
	}

	streams, streamsCleanup, err := data.NewStreams(ctx, cfg.Data.Redis)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	cleanups = append(cleanups, streamsCleanup)

	// Foundational repositories and use cases.
	auditRepo := data.NewAuditRepo(db)
	auditor := biz.NewAuditor(auditRepo)

	// External service adapters: in-memory stub for local dev, file/exec/HTTP
	// adapter (writes the generated policy and reloads KumoMTA) otherwise.
	var kumo biz.KumoMTAAdapter
	if cfg.KumoMTA.Stub {
		kumo = biz.NewStubKumoMTA()
	} else {
		kumo = data.NewFileKumoMTA(cfg.KumoMTA)
	}

	// US2 mail operations: repository, Redis producer, and use case.
	mailOpsRepo := data.NewMailOpsRepo(db)
	opsProducer := worker.NewOpsProducer(streams)

	// US3 identity & audit: repository, MFA provider, use case, session resolver.
	identityRepo := data.NewIdentityRepo(db, auditRepo)
	identityUC := biz.NewIdentityUsecase(identityRepo, biz.NewTOTPMFA(identityRepo, "Iris"), auditor)

	// US1/US4 shared repositories.
	outboundRepo := data.NewOutboundConfigRepo(db)
	domainSafetyRepo := data.NewDomainSafetyRepo(db)
	domainSafetyUC := biz.NewDomainSafetyUsecase(domainSafetyRepo, auditor)
	outboundUC := biz.NewOutboundConfigUsecase(outboundRepo, auditor).
		WithEligibilityChecker(domainSafetyUC)

	// KumoMTA config generation/apply: renders the full configuration snapshot
	// into KumoMTA policy and reloads the service (serialized + audited). The
	// generated policy also wires inbound rspamd filtering and a log_hook that
	// streams KumoMTA's structured logs into Redis for the log consumer below.
	logStreamRedisURL := cfg.KumoMTA.LogStreamRedisURL
	if logStreamRedisURL == "" && cfg.Data.Redis.Addr != "" {
		logStreamRedisURL = "redis://" + cfg.Data.Redis.Addr
	}
	// Config/env defaults; UI-managed global settings (below) override these.
	settingsDefaults := biz.KumoConfigSettings{
		RspamdMode:        cfg.Rspamd.Mode,
		RspamdURL:         cfg.Rspamd.BaseURL,
		LogStreamRedisURL: logStreamRedisURL,
		LogStreamName:     data.StreamMailEvents,
	}
	settingsUC := biz.NewGlobalSettingsUsecase(data.NewGlobalSettingsRepo(db), auditor, settingsDefaults)
	kumoConfigUC := biz.NewKumoConfigUsecase(
		data.NewKumoConfigRepo(outboundRepo, domainSafetyRepo), kumo, mailOpsRepo, auditor, settingsUC)

	// US5 inbound automation: webhook + Rspamd use case and workers.
	inboundRepo := data.NewInboundRepo(db)
	inboundUC := biz.NewInboundUsecase(inboundRepo, auditor, cfg.KumoMTA.Stub)

	// ACME (Let's Encrypt) certificate management. The HTTP-01 token store is
	// shared between the issuer and the challenge listener. Issued PEMs are
	// mirrored to acmeCertDir, which listener TLS paths reference.
	acmeTokens := acme.NewTokenStore()
	acmeRepo := data.NewAcmeRepo(db)
	acmeCertDir := envOr("IRIS_ACME_CERT_DIR", "/opt/kumomta/etc/tls")
	acmeUC := biz.NewAcmeUsecase(acmeRepo, acmeRepo, acmeTokens, acmeCertDir, auditor)

	deps := service.Deps{
		Log:          log,
		Auditor:      auditor,
		Outbound:     outboundUC,
		MailOps:      biz.NewMailOpsUsecase(mailOpsRepo, opsProducer, auditor),
		Identity:     identityUC,
		DomainSafety: domainSafetyUC,
		Inbound:      inboundUC,
		Dashboard:    biz.NewDashboardUsecase(data.NewDashboardRepo(db)),
		KumoConfig:   kumoConfigUC,
		Settings:     settingsUC,
		Acme:         acmeUC,
	}

	svc := service.NewService(deps)

	// Start background workers. Each exits cleanly on context cancellation.
	startWorker(ctx, log, "service-control", worker.NewServiceControlWorker(streams, mailOpsRepo, kumo, log).Run)
	startWorker(ctx, log, "webhook-delivery", worker.NewWebhookWorker(streams, inboundUC, log).Run)
	startWorker(ctx, log, "rspamd-ingest", worker.NewRspamdWorker(streams, inboundUC, log).Run)
	// Ingest KumoMTA's structured logs (streamed by the generated policy's
	// log_hook) into the mail_records hypertable that powers the Logs UI, and
	// fan received messages out to matching inbound webhooks.
	startWorker(ctx, log, "log-stream", worker.NewLogStreamWorker(streams, mailOpsRepo, domainSafetyRepo, settingsUC, data.StreamMailEvents, log).
		WithWebhooks(worker.NewWebhookProducer(streams)).Run)
	// DSN consumer: async bounces captured at the configured bounce domain.
	startWorker(ctx, log, "dsn", worker.NewDSNWorker(streams, mailOpsRepo, domainSafetyRepo, biz.DSNStreamName, log).Run)
	// ACME: HTTP-01 challenge listener (default off) + periodic renewer.
	startWorker(ctx, log, "acme-challenge", worker.NewAcmeChallengeWorker(acmeTokens, envOr("IRIS_ACME_HTTP_BIND", "off"), log).Run)
	startWorker(ctx, log, "acme-renewer", worker.NewAcmeRenewerWorker(acmeUC, envDuration("IRIS_ACME_RENEW_INTERVAL", 12*time.Hour), envDuration("IRIS_ACME_RENEW_BEFORE", 30*24*time.Hour), log).Run)

	authMW := service.AuthMiddleware(cfg.Auth, identityUC)
	checks := []server.ReadinessChecker{db, streams}

	httpSrv := server.NewHTTPServer(cfg.Server, svc, adminv1.OpenAPISpec, checks, authMW)
	grpcSrv := server.NewGRPCServer(cfg.Server, svc, authMW)

	app := kratos.New(
		kratos.Name("iris"),
		kratos.Context(ctx),
		kratos.Server([]transport.Server{httpSrv, grpcSrv}...),
	)
	return app, cleanup, nil
}

// startWorker launches a background worker goroutine, logging unexpected exits
// that are not caused by graceful context cancellation.
func startWorker(ctx context.Context, log *slog.Logger, name string, run func(context.Context) error) {
	go func() {
		if err := run(ctx); err != nil && ctx.Err() == nil {
			log.Error("worker exited", "worker", name, "error", err.Error())
		}
	}()
}
