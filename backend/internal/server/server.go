// Package server wires the Kratos HTTP and gRPC transports, including health
// and readiness endpoints and shared middleware.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/webui"
)

// ReadinessChecker reports whether a dependency is ready to serve traffic.
type ReadinessChecker interface {
	Health(ctx context.Context) error
}

// NewHTTPServer builds the HTTP transport, registers the admin service, and
// exposes health/readiness endpoints plus the OpenAPI document. When tlsConf is
// non-nil the server serves HTTPS on the same address.
func NewHTTPServer(c conf.Server, svc adminv1.IrisAdminServiceHTTPServer, openapi []byte, checks []ReadinessChecker, tlsConf *tls.Config, mws ...middleware.Middleware) *kratoshttp.Server {
	opts := []kratoshttp.ServerOption{
		kratoshttp.Middleware(append([]middleware.Middleware{recovery.Recovery()}, mws...)...),
	}
	if c.HTTP.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.HTTP.Addr))
	}
	if c.HTTP.Timeout > 0 {
		opts = append(opts, kratoshttp.Timeout(c.HTTP.Timeout))
	}
	if tlsConf != nil {
		opts = append(opts, kratoshttp.TLSConfig(tlsConf))
	}
	srv := kratoshttp.NewServer(opts...)

	// Liveness: process is up.
	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Readiness: dependencies are reachable.
	srv.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		for _, c := range checks {
			if err := c.Health(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status": "unavailable", "error": err.Error(),
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	// Serve the generated OpenAPI contract for Swagger UI consumption.
	if len(openapi) > 0 {
		srv.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(openapi)
		})
	}
	// Prometheus metrics (mail/VMTA/bounce/webhook counters driven by the workers).
	srv.Handle("/metrics", promhttp.Handler())

	adminv1.RegisterIrisAdminServiceHTTPServer(srv, svc)

	// Serve the embedded SPA last so it acts as a fallback: the specific routes
	// above (API, health, metrics, openapi) are matched first; everything else
	// is served by the SPA (only in `embed_ui` builds).
	if webui.Enabled() {
		srv.HandlePrefix("/", webui.Handler())
	}
	return srv
}

// NewGRPCServer builds the gRPC transport and registers the admin service.
func NewGRPCServer(c conf.Server, svc adminv1.IrisAdminServiceServer, mws ...middleware.Middleware) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.Middleware(append([]middleware.Middleware{recovery.Recovery()}, mws...)...),
	}
	if c.GRPC.Addr != "" {
		opts = append(opts, grpc.Address(c.GRPC.Addr))
	}
	if c.GRPC.Timeout > 0 {
		opts = append(opts, grpc.Timeout(c.GRPC.Timeout))
	}
	srv := grpc.NewServer(opts...)
	adminv1.RegisterIrisAdminServiceServer(srv, svc)
	return srv
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
