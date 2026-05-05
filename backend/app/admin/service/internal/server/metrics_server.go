// MetricsServer adapts a tiny http.Server-with-/metrics into a kratos
// transport.Server so it joins the same Start/Stop lifecycle as the
// gRPC and HTTP transports.
//
// Why a separate listener (not /metrics on the main HTTP port):
//
//   - The main HTTP server is the public UI + API surface. Anything
//     bound there is reachable from the operator UI's network. /metrics
//     is operationally privileged — you don't want a curious user to
//     enumerate event-type counts and per-mail-class volumes.
//
//   - Prometheus scrape configs naturally point at a different host:port
//     anyway. Putting /metrics on a loopback-only listener is the
//     idiomatic Go-services shape (see kube components, vault, etc.).
//
// Default bind: 127.0.0.1:9090. Override via IRIS_METRICS_LISTEN. To
// disable entirely, set IRIS_METRICS_LISTEN to the literal "off".
package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/menta2k/iris/backend/pkg/metrics"
)

// MetricsServer is the kratos transport.Server adapter. nil registry =
// disabled (no listener bound, Start/Stop are no-ops).
type MetricsServer struct {
	addr   string
	srv    *http.Server
	done   chan struct{}
	cancel context.CancelFunc
}

// NewMetricsServer constructs the listener. Returns a no-op server
// (zero-value) when metrics are explicitly disabled or no Metrics
// object was provided — the boot graph stays uniform either way.
func NewMetricsServer(m *metrics.Metrics) *MetricsServer {
	if m == nil {
		return &MetricsServer{}
	}

	addr := strings.TrimSpace(os.Getenv("IRIS_METRICS_LISTEN"))
	if addr == "" {
		addr = "127.0.0.1:9090"
	}
	if strings.EqualFold(addr, "off") || strings.EqualFold(addr, "disabled") {
		log.Printf("metrics: IRIS_METRICS_LISTEN=%q — metrics endpoint disabled", addr)
		return &MetricsServer{}
	}

	mux := http.NewServeMux()
	// promhttp uses the prometheus.HandlerFor against our private
	// registry — not the global default — so the surface stays scoped
	// to metrics we explicitly defined in pkg/metrics.
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{
		// Don't surface internal collector errors to the client; log
		// them server-side and serve whatever was successfully gathered.
		ErrorHandling: promhttp.ContinueOnError,
		// Tight timeout — a stuck metric collector should not hold a
		// scrape connection open indefinitely.
		Timeout: 10 * time.Second,
	}))
	// Cheap liveness probe useful for the local Prometheus's "is the
	// scrape target reachable" check, distinct from kumomta or DB
	// health.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return &MetricsServer{
		addr: addr,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Start binds the listener. Runs the http.Server in a goroutine so
// kratos can move on to the next stage; Stop joins it.
func (s *MetricsServer) Start(_ context.Context) error {
	if s.srv == nil {
		return nil
	}
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		log.Printf("metrics: listening on %s", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics: server exited: %v", err)
		}
	}()
	return nil
}

// Stop gracefully shuts the listener down. Bounded by Stop's ctx.
func (s *MetricsServer) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Printf("metrics: shutdown error: %v", err)
	}
	if s.done != nil {
		<-s.done
	}
	return nil
}
