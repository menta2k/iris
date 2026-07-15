package worker

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/acme"
	"github.com/menta2k/iris/backend/internal/biz"
)

// AcmeChallengeWorker serves the public /.well-known/acme-challenge/<token>
// endpoint ACME CAs hit during HTTP-01 validation, backed by the shared
// TokenStore the issuer fills. Disabled (a no-op that blocks until shutdown)
// when bind is empty or "off" — operators behind a reverse proxy disable it and
// forward the challenge path to the API server instead.
type AcmeChallengeWorker struct {
	tokens *acme.TokenStore
	bind   string
	log    *slog.Logger
}

// NewAcmeChallengeWorker constructs the worker.
func NewAcmeChallengeWorker(tokens *acme.TokenStore, bind string, log *slog.Logger) *AcmeChallengeWorker {
	return &AcmeChallengeWorker{tokens: tokens, bind: strings.TrimSpace(bind), log: log}
}

// Run binds and serves until the context is cancelled.
func (w *AcmeChallengeWorker) Run(ctx context.Context) error {
	if w.bind == "" || strings.EqualFold(w.bind, "off") || w.tokens == nil {
		w.log.Info("acme-challenge listener disabled (set IRIS_ACME_HTTP_BIND to enable)")
		<-ctx.Done()
		return ctx.Err()
	}
	srv := &http.Server{Addr: w.bind, Handler: http.HandlerFunc(w.tokens.ServeHTTP), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	w.log.Info("acme-challenge listener started", "bind", w.bind)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errc:
		// A bind failure must not bring down the process — log and idle.
		w.log.Error("acme-challenge listener exited", "error", err.Error())
		<-ctx.Done()
		return ctx.Err()
	}
}

// AcmeRenewerWorker periodically renews certificates approaching expiry.
type AcmeRenewerWorker struct {
	uc       *biz.AcmeUsecase
	config   ConfigApplier // optional; applies the policy so renewed certs reach every node
	interval time.Duration
	before   time.Duration
	log      *slog.Logger
}

// NewAcmeRenewerWorker constructs the renewer. interval is the scan cadence;
// before is how far ahead of expiry to renew. config, when non-nil, is applied
// after a renewal so the new cert propagates to the cluster (a renewal changes
// only file content, not the policy text, so nothing else would push it).
// Sensible defaults are applied.
func NewAcmeRenewerWorker(uc *biz.AcmeUsecase, config ConfigApplier, interval, before time.Duration, log *slog.Logger) *AcmeRenewerWorker {
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	if before <= 0 {
		before = 30 * 24 * time.Hour
	}
	return &AcmeRenewerWorker{uc: uc, config: config, interval: interval, before: before, log: log}
}

// Run scans for due renewals on each tick until the context is cancelled.
func (w *AcmeRenewerWorker) Run(ctx context.Context) error {
	if w.uc == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	w.log.Info("acme-renewer started", "interval", w.interval.String(), "before", w.before.String())
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.scan(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.scan(ctx)
		}
	}
}

func (w *AcmeRenewerWorker) scan(ctx context.Context) {
	cutoff := time.Now().Add(w.before)
	n, err := w.uc.RenewDue(ctx, cutoff)
	if err != nil {
		w.log.Error("acme renewal scan failed", "error", err.Error())
		return
	}
	if n > 0 {
		w.log.Info("acme renewals completed", "count", n)
		// Push the renewed cert(s) to KumoMTA (and, in a cluster, every node):
		// the policy references the cert by path, so an apply is what re-reads
		// the file content and ships it. Reload-level change (no init drift).
		if w.config != nil {
			if _, err := w.config.ApplyForAutomation(ctx, "acme-renewer"); err != nil {
				w.log.Error("apply after acme renewal failed", "error", err.Error())
			} else {
				w.log.Info("applied policy after acme renewal (certs propagated)")
			}
		}
	}
}
