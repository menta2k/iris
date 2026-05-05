// Dashboard registrar — three GET endpoints under /v1/dashboard/* that
// translate Prometheus query results into JSON shapes the operator UI
// renders directly. None of these are mutating, so they bypass the
// audit wrapper.
//
// All three return 503 with code=METRICS_NOT_CONFIGURED when no
// Prometheus URL is wired, so the UI can swap the chart for a "No
// metrics backend configured" placeholder rather than spinning forever.
package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	"github.com/menta2k/iris/backend/pkg/promquery"
)

// RegisterDashboardHTTP mounts the dashboard endpoints. Called from
// the same place as RegisterKumoHTTP — see registrar.go.
func RegisterDashboardHTTP(hs *kratoshttp.Server, s *service.DashboardService) {
	hs.HandleFunc("/v1/dashboard/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := s.Summary(ctx)
		if err != nil {
			writeDashErr(w, err, "summary")
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	hs.HandleFunc("/v1/dashboard/event-rates", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		// range= and step= are operator-tunable but clamped: a too-fine
		// (range, step) pair would overload Prometheus with thousands of
		// points. The clamps are loose enough for normal "last hour" /
		// "last day" dashboards but cap the worst case.
		rangeDur := parseDuration(r.URL.Query().Get("range"), time.Hour, time.Minute, 7*24*time.Hour)
		step := parseDuration(r.URL.Query().Get("step"), time.Minute, 15*time.Second, 30*time.Minute)

		// At most ~720 points per chart — anything finer is wasted on a
		// browser-side renderer and burns Prometheus.
		if rangeDur/step > 720 {
			step = rangeDur / 720
		}

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		out, err := s.EventRates(ctx, rangeDur, step)
		if err != nil {
			writeDashErr(w, err, "event-rates")
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	hs.HandleFunc("/v1/dashboard/by-class", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use GET")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := s.ByClass(ctx)
		if err != nil {
			writeDashErr(w, err, "by-class")
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// writeDashErr maps the dashboard-specific error vocabulary to HTTP.
// promquery.ErrNotConfigured → 503 (the metrics backend is missing,
// not malformed input). Everything else → 502 (Prometheus reachable
// but failed the query, e.g. malformed PromQL).
func writeDashErr(w http.ResponseWriter, err error, op string) {
	if errors.Is(err, promquery.ErrNotConfigured) {
		writeErr(w, http.StatusServiceUnavailable, "METRICS_NOT_CONFIGURED",
			"IRIS_PROMETHEUS_URL is unset; "+op+" is unavailable")
		return
	}
	writeErr(w, http.StatusBadGateway, "PROMETHEUS_ERROR", err.Error())
}

// parseDuration parses a Go-style duration string with bounds. Returns
// the default when the input is empty / malformed; clamps to [min, max]
// so the handler cannot be tricked into pinning a goroutine on a
// too-long range or DoS'ing Prometheus on a too-fine step.
func parseDuration(raw string, def, lo, hi time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	// Tolerate a bare integer (treat as seconds) as well as Go duration strings.
	if n, err := strconv.Atoi(raw); err == nil {
		raw = strconv.Itoa(n) + "s"
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	if d < lo {
		return lo
	}
	if d > hi {
		return hi
	}
	return d
}
