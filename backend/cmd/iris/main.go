// Command iris is the entrypoint for the Iris KumoMTA admin backend. It wires
// configuration, storage, business use cases, and the HTTP/gRPC transports,
// then runs them under a Kratos application with graceful shutdown.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
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
	"github.com/menta2k/iris/backend/internal/errlog"
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

	// Real auth requires a signing secret for session tokens. Fail fast rather
	// than mint tokens an attacker could forge with an empty key.
	if !cfg.Auth.DevBypass && cfg.Auth.SessionToken == "" {
		return nil, nil, fmt.Errorf("auth.session_token_secret (IRIS_SESSION_SECRET) must be set when dev_bypass is disabled")
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
	var queueAdmin biz.KumoQueueAdmin // live kumod queue control (nil in stub mode)
	if cfg.KumoMTA.Stub {
		kumo = biz.NewStubKumoMTA()
	} else {
		fk := data.NewFileKumoMTA(cfg.KumoMTA)
		kumo = fk
		queueAdmin = fk
	}

	// US2 mail operations: repository, Redis producer, and use case.
	mailOpsRepo := data.NewMailOpsRepo(db)
	opsProducer := worker.NewOpsProducer(streams)

	// US3 identity & audit: repository, MFA provider, use case, session resolver.
	identityRepo := data.NewIdentityRepo(db, auditRepo)
	mfaProvider := biz.NewTOTPMFA(identityRepo, "Iris")
	identityUC := biz.NewIdentityUsecase(identityRepo, mfaProvider, auditor)

	// Authentication: signed session tokens, password login, MFA-gated sessions.
	sessions := biz.NewSessionManager(cfg.Auth.SessionToken, cfg.Auth.SessionTTL)
	authUC := biz.NewAuthUsecase(identityRepo, mfaProvider, sessions, auditor, cfg.Auth.MFARequired)

	// Optionally seed the first admin from the environment on an empty database.
	if err := bootstrapAdmin(ctx, identityRepo, log); err != nil {
		cleanup()
		return nil, nil, err
	}

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
	// VERP signing key, derived from the session secret (shared by the policy
	// generator and the DSN worker).
	verpKey := biz.DeriveVerpKey(cfg.Auth.SessionToken)
	// Config/env defaults; UI-managed global settings (below) override these.
	settingsDefaults := biz.KumoConfigSettings{
		RspamdMode:        cfg.Rspamd.Mode,
		RspamdURL:         cfg.Rspamd.BaseURL,
		LogStreamRedisURL: logStreamRedisURL,
		LogStreamName:     data.StreamMailEvents,
		// KumoMTA ships the IANA bounce-classifier rules on standard installs.
		// Set IRIS_BOUNCE_CLASSIFIER_FILE="" to disable on installs without it.
		BounceClassifierFile: envOr("IRIS_BOUNCE_CLASSIFIER_FILE", "/opt/kumomta/share/bounce_classifier/iana.toml"),
		// VERP signing key, derived from the session secret so the policy and the
		// DSN worker agree without separate storage. Empty (e.g. dev_bypass with
		// no secret) disables VERP.
		BounceVerpSecret: verpKey,
	}
	settingsRepo := data.NewGlobalSettingsRepo(db)
	settingsUC := biz.NewGlobalSettingsUsecase(settingsRepo, auditor, settingsDefaults)
	// Suppression list lives in Redis (write-through cache + per-entry TTL); the
	// rendered policy consults it instead of an inline table. Attach the cache and
	// TTL provider now that settingsUC exists, then backfill Redis from the DB so
	// a restart/flush stays consistent. domainSafetyRepo is a pointer, so the
	// already-constructed usecase/snapshot loader pick up the cache too.
	suppCache := data.NewSuppressionCache(streams.Client)
	domainSafetyRepo.WithSuppressionCache(suppCache, settingsUC.SuppressionTTLNow)
	if active, lerr := domainSafetyRepo.ListActiveSuppressions(ctx); lerr != nil {
		log.Warn("suppression backfill: list active failed", "error", lerr.Error())
	} else if n, berr := suppCache.Backfill(ctx, active, time.Now().UTC()); berr != nil {
		log.Warn("suppression backfill: redis populate failed", "error", berr.Error())
	} else {
		log.Info("suppression cache backfilled", "entries", n)
	}
	// US5 inbound automation: webhook + Rspamd use case and workers.
	inboundRepo := data.NewInboundRepo(db)
	fblRepo := data.NewFBLRepo(db)

	kumoSnapshotRepo := data.NewKumoConfigRepo(outboundRepo, domainSafetyRepo, inboundRepo, fblRepo)
	kumoConfigUC := biz.NewKumoConfigUsecase(kumoSnapshotRepo, kumo, mailOpsRepo, auditor, settingsUC)
	// Domain bounce-readiness checker (MX/SPF/DKIM via live DNS).
	domainCheckUC := biz.NewDomainCheckUsecase(kumoSnapshotRepo, nil)
	// Tools: sender diagnose + RBL/DNSBL check (live DNS).
	diagnoseUC := biz.NewDiagnoseUsecase(kumoSnapshotRepo, nil, settingsUC)
	rblUC := biz.NewRBLUsecase(kumoSnapshotRepo, nil)
	// DMARC aggregate-report parsing.
	dmarcRepo := data.NewDMARCRepo(db)
	dmarcUC := biz.NewDMARCUsecase(dmarcRepo, auditor)

	// Generic worker error log: the repo is both the read API source and the
	// sink behind the errlog slog handler that captures worker Warn/Error events.
	workerErrorRepo := data.NewWorkerErrorRepo(db)
	workerErrorUC := biz.NewWorkerErrorUsecase(workerErrorRepo)

	inboundUC := biz.NewInboundUsecase(inboundRepo, auditor, cfg.KumoMTA.Stub)
	fblUC := biz.NewFBLUsecase(fblRepo, auditor)

	// ACME (Let's Encrypt) certificate management. The HTTP-01 token store is
	// shared between the issuer and the challenge listener. Issued PEMs are
	// mirrored to acmeCertDir, which listener TLS paths reference.
	acmeTokens := acme.NewTokenStore()
	acmeRepo := data.NewAcmeRepo(db)
	acmeCertDir := envOr("IRIS_ACME_CERT_DIR", "/opt/kumomta/etc/tls")
	acmeUC := biz.NewAcmeUsecase(acmeRepo, acmeRepo, acmeRepo, acmeTokens, acmeCertDir, auditor)

	// Operator-configurable admin server + renew schedule, read from global
	// settings at startup (a restart applies changes). Global settings may
	// override the HTTP bind and enable HTTPS using an issued certificate;
	// unreadable cert config falls back to plain HTTP rather than failing boot.
	adminServerConf := cfg.Server
	var adminTLS *tls.Config
	renewInterval := envDuration("IRIS_ACME_RENEW_INTERVAL", 12*time.Hour)
	renewBefore := envDuration("IRIS_ACME_RENEW_BEFORE", 30*24*time.Hour)
	if gs, gerr := settingsRepo.Get(ctx); gerr == nil {
		if gs.AdminHTTPAddr != "" {
			adminServerConf.HTTP.Addr = gs.AdminHTTPAddr
		}
		if gs.AdminTLSEnabled && gs.AdminTLSCertDomain != "" {
			if tc, terr := loadAdminTLS(ctx, acmeRepo, gs.AdminTLSCertDomain); terr != nil {
				log.Error("admin TLS disabled: certificate could not be loaded; serving plain HTTP",
					"domain", gs.AdminTLSCertDomain, "error", terr.Error())
			} else {
				adminTLS = tc
				log.Info("admin HTTPS enabled", "domain", gs.AdminTLSCertDomain, "addr", adminServerConf.HTTP.Addr)
			}
		}
		if d, ok := biz.ParseFlexDuration(gs.AcmeRenewInterval); ok {
			renewInterval = d
		}
		if d, ok := biz.ParseFlexDuration(gs.AcmeRenewBefore); ok {
			renewBefore = d
		}
	}

	deps := service.Deps{
		Log:          log,
		Auditor:      auditor,
		Outbound:     outboundUC,
		MailOps:      biz.NewMailOpsUsecase(mailOpsRepo, opsProducer, auditor).WithQueueAdmin(queueAdmin),
		Identity:     identityUC,
		Auth:         authUC,
		DomainSafety: domainSafetyUC,
		Inbound:      inboundUC,
		FBL:          fblUC,
		Dashboard:    biz.NewDashboardUsecase(data.NewDashboardRepo(db)),
		Metrics:      biz.NewMetricsUsecase(settingsUC, nil),
		KumoConfig:   kumoConfigUC,
		Settings:     settingsUC,
		Acme:         acmeUC,
		DomainCheck:  domainCheckUC,
		Diagnose:     diagnoseUC,
		RBL:          rblUC,
		DMARC:        dmarcUC,
		WorkerErrors: workerErrorUC,
	}

	svc := service.NewService(deps)

	// Wrap the base log handler so every Warn/Error a worker emits is also
	// mirrored into the worker_error_logs store. Workers get a logger tagged with
	// their name; the supervisor (startWorker) keeps the plain stdout logger so a
	// sink failure can never recurse back through the DB handler.
	errHandler := errlog.New(log.Handler(), workerErrorRepo, errlog.Options{
		Redact: biz.IsSensitiveKey,
		OnError: func(err error) {
			fmt.Fprintf(os.Stderr, "errlog sink: %v\n", err)
		},
	})
	workerLog := slog.New(errHandler)
	wlog := func(name string) *slog.Logger { return workerLog.With("worker", name) }

	// Start background workers. Each exits cleanly on context cancellation.
	startWorker(ctx, log, "errlog-flush", errHandler.Run)
	startWorker(ctx, log, "service-control", worker.NewServiceControlWorker(streams, mailOpsRepo, kumo, wlog("service-control")).Run)
	startWorker(ctx, log, "rspamd-ingest", worker.NewRspamdWorker(streams, inboundUC, wlog("rspamd-ingest")).Run)
	// Ingest KumoMTA's structured logs (streamed by the generated policy's
	// log_hook) into the mail_records hypertable that powers the Logs UI.
	// Inbound webhooks are delivered in-policy by kumod (make.webhook_post),
	// which forwards the raw message — so no webhook fan-out worker here.
	startWorker(ctx, log, "log-stream", worker.NewLogStreamWorker(streams, mailOpsRepo, domainSafetyRepo, settingsUC, data.StreamMailEvents, wlog("log-stream")).Run)
	// DSN consumer: async bounces captured at the configured bounce domain.
	startWorker(ctx, log, "dsn", worker.NewDSNWorker(streams, mailOpsRepo, domainSafetyRepo, verpKey, biz.DSNStreamName, wlog("dsn")).Run)
	startWorker(ctx, log, "dmarc", worker.NewDMARCWorker(streams, dmarcUC, biz.DMARCStreamName, wlog("dmarc")).Run)
	// ACME: HTTP-01 challenge listener (default off) + periodic renewer.
	startWorker(ctx, log, "acme-challenge", worker.NewAcmeChallengeWorker(acmeTokens, envOr("IRIS_ACME_HTTP_BIND", "off"), wlog("acme-challenge")).Run)
	startWorker(ctx, log, "acme-renewer", worker.NewAcmeRenewerWorker(acmeUC, renewInterval, renewBefore, wlog("acme-renewer")).Run)

	authMW := service.AuthMiddleware(cfg.Auth, authUC)
	checks := []server.ReadinessChecker{db, streams}

	httpSrv := server.NewHTTPServer(adminServerConf, svc, adminv1.OpenAPISpec, checks, adminTLS, authMW)
	grpcSrv := server.NewGRPCServer(cfg.Server, svc, authMW)

	app := kratos.New(
		kratos.Name("iris"),
		kratos.Context(ctx),
		kratos.Server([]transport.Server{httpSrv, grpcSrv}...),
	)
	return app, cleanup, nil
}

// bootstrapAdmin seeds the first administrator from the environment when the
// user table is empty. It is a no-op unless BOTH IRIS_BOOTSTRAP_ADMIN_EMAIL and
// IRIS_BOOTSTRAP_ADMIN_PASSWORD are set, and it never overwrites an existing
// install. The account is created active with the owner role and no MFA
// enrollment yet — the first login drives MFA enrollment when required.
func bootstrapAdmin(ctx context.Context, repo biz.IdentityRepo, log *slog.Logger) error {
	email := os.Getenv("IRIS_BOOTSTRAP_ADMIN_EMAIL")
	password := os.Getenv("IRIS_BOOTSTRAP_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}
	n, err := repo.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap admin: count users: %w", err)
	}
	if n > 0 {
		log.Info("bootstrap admin skipped; users already exist")
		return nil
	}
	hash, err := biz.HashPassword(password)
	if err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	user := &biz.IrisUser{
		Email:        email,
		DisplayName:  "Administrator",
		Status:       biz.UserActive,
		MFARequired:  false,
		Roles:        []string{biz.RoleOwner},
		PasswordHash: hash,
	}
	if err := user.Validate(); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	if _, err := repo.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("bootstrap admin: create user: %w", err)
	}
	log.Info("bootstrap admin created", "email", user.Email)
	return nil
}

// loadAdminTLS builds a TLS config for the admin server from the issued
// certificate whose domain matches. It errors (rather than panics) so the
// caller can fall back to plain HTTP.
func loadAdminTLS(ctx context.Context, repo biz.AcmeCertificateRepo, domain string) (*tls.Config, error) {
	cert, err := repo.GetCertificateByDomain(ctx, domain)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, fmt.Errorf("no issued certificate for domain %q", domain)
	}
	if cert.CertPath == "" || cert.KeyPath == "" {
		return nil, fmt.Errorf("certificate for %q has no on-disk paths", domain)
	}
	pair, err := tls.LoadX509KeyPair(cert.CertPath, cert.KeyPath)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, nil
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
