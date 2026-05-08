// AcmeRenewerServer wakes on a fixed interval, finds certs whose
// expires_at falls inside the renewal window, and queues a renewal
// for each via the existing AcmeService.RenewCertificate path. The
// service-layer call is async (kicks the lego flow off in a
// goroutine) so this server doesn't block waiting for any one
// renewal — it just queues work and goes back to sleep.
//
// Operator knobs:
//
//   - IRIS_ACME_RENEW_INTERVAL  (default 12h) — how often to scan.
//   - IRIS_ACME_RENEW_BEFORE    (default 720h = 30d) — start renewing
//     this far ahead of expiry.
//   - IRIS_ACME_RENEW_DISABLED  (default unset / false) — kill switch.
//
// Backoff: a row whose status is `failed` and whose updated_at is
// within IRIS_ACME_RENEW_BACKOFF (default 24h) is skipped this cycle.
// That keeps the CA error budget intact when an issuance is
// structurally broken (e.g. DNS not propagating, blocklisted IP).
//
// State is entirely DB-driven — restarts don't reset backoff because
// the `updated_at` column persists. There is no in-memory tracker.
package server

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// Defaults are deliberately conservative: 12h scan interval, 30d
// renewal lead time, 24h post-failure cool-off. Operators tune via
// env. Let's Encrypt rate limits comfortably absorb 12h tick noise
// even with hundreds of certs.
const (
	defaultRenewInterval = 12 * time.Hour
	defaultRenewBefore   = 30 * 24 * time.Hour
	defaultRenewBackoff  = 24 * time.Hour
)

type AcmeRenewerServer struct {
	svc *service.AcmeService

	enabled  bool
	interval time.Duration
	before   time.Duration
	backoff  time.Duration

	cancel context.CancelFunc
	done   chan struct{}
	startOnce sync.Once
}

// NewAcmeRenewerServer reads the env-driven knobs and returns a
// renewer. nil-safe: a missing AcmeService (which can't happen in
// practice but guards against future wiring mistakes) disables the
// loop.
func NewAcmeRenewerServer(svc *service.AcmeService) *AcmeRenewerServer {
	r := &AcmeRenewerServer{
		svc:      svc,
		enabled:  !boolEnv("IRIS_ACME_RENEW_DISABLED"),
		interval: durationEnv("IRIS_ACME_RENEW_INTERVAL", defaultRenewInterval),
		before:   durationEnv("IRIS_ACME_RENEW_BEFORE", defaultRenewBefore),
		backoff:  durationEnv("IRIS_ACME_RENEW_BACKOFF", defaultRenewBackoff),
	}
	if svc == nil {
		r.enabled = false
	}
	return r
}

// Start spawns the scheduling goroutine. It runs immediately (so a
// fresh deploy after long downtime doesn't have to wait the full
// interval to catch a near-expiry cert) and then ticks every
// `interval` until Stop is called.
func (r *AcmeRenewerServer) Start(_ context.Context) error {
	if !r.enabled {
		log.Printf("acme-renewer: disabled (IRIS_ACME_RENEW_DISABLED=true or service missing)")
		return nil
	}
	log.Printf("acme-renewer: starting interval=%s before=%s backoff=%s",
		r.interval, r.before, r.backoff)
	r.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel = cancel
		r.done = make(chan struct{})
		go r.loop(ctx)
	})
	return nil
}

// Stop cancels the loop and waits for the in-flight tick to drain.
// In-flight RenewCertificate calls are themselves async (their own
// goroutines) so this only waits for the scheduler tick, not for
// every renewal to finish — they keep running and either complete
// or get killed at process exit.
func (r *AcmeRenewerServer) Stop(_ context.Context) error {
	if r.cancel == nil {
		return nil
	}
	r.cancel()
	if r.done != nil {
		<-r.done
	}
	return nil
}

func (r *AcmeRenewerServer) loop(ctx context.Context) {
	defer close(r.done)

	// Boot tick: run once immediately so a long-downtime restart
	// doesn't lose a window. Best-effort — failures log and the
	// next interval-tick re-tries.
	r.tick(ctx)

	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

// tick walks every cert in the renewal window and queues a renewal
// for each one that isn't currently in flight or in cool-off. Errors
// from the queue call are logged but never fail the tick — one
// broken row mustn't block the rest.
func (r *AcmeRenewerServer) tick(ctx context.Context) {
	cutoff := time.Now().UTC().Add(r.before)
	rows, err := r.svc.ListCertificatesNearExpiry(ctx, cutoff)
	if err != nil {
		log.Printf("acme-renewer: list near-expiry: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	now := time.Now().UTC()
	queued := 0
	for _, row := range rows {
		if row.Status == "pending" || row.Status == "renewing" {
			// Some other actor (operator, prior tick) is handling
			// this domain. The next tick will recheck.
			continue
		}
		if row.Status == "failed" && now.Sub(row.UpdatedAt) < r.backoff {
			// Cool-off — recently-failed renewals get a window before
			// the next attempt. Logged at INFO so operators can see
			// the system is intentionally holding off, not stuck.
			log.Printf("acme-renewer: skipping %s — last attempt %s ago, backoff=%s",
				row.Domain, now.Sub(row.UpdatedAt).Round(time.Second), r.backoff)
			continue
		}
		expiresIn := time.Duration(0)
		if row.ExpiresAt != nil {
			expiresIn = row.ExpiresAt.Sub(now)
		}
		log.Printf("acme-renewer: queueing %s (expires_in=%s status=%s)",
			row.Domain, expiresIn.Round(time.Hour), row.Status)
		if _, err := r.svc.RenewCertificate(ctx, row.ID); err != nil {
			log.Printf("acme-renewer: queue %s failed: %v", row.Domain, err)
			continue
		}
		queued++
	}
	if queued > 0 {
		log.Printf("acme-renewer: queued %d renewal(s) of %d candidate(s)", queued, len(rows))
	}
}

// boolEnv treats unset / "" / "0" / "false" / "no" / "off" as false;
// anything else as true. Matches the pattern used elsewhere in the
// project.
func boolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// durationEnv parses an env-supplied time.Duration string ("12h",
// "30m", etc.). On parse error or empty value, returns the fallback.
func durationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("acme-renewer: invalid %s=%q (using fallback %s)", key, v, fallback)
		return fallback
	}
	return d
}
