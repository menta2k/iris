// Package promquery is a thin, typed wrapper over the Prometheus HTTP
// query API. It exists so the dashboard service doesn't sprinkle raw
// PromQL strings + URL parsing across multiple files, and so tests can
// stub a single interface instead of a real Prometheus.
//
// Why not just use github.com/prometheus/client_golang/api/prometheus/v1
// directly? Two reasons:
//
//  1. The upstream client returns the raw `model.Vector` /
//     `model.Matrix` types from prometheus/common. Those are fine for a
//     scientific notebook but awkward as a service-layer return type
//     (every caller would need to import prometheus/common). The
//     wrapper translates into plain `[]Sample` / `[]Series` shapes
//     that round-trip cleanly into JSON.
//
//  2. Every call needs a context with a hard timeout. Centralising
//     that here means we can't forget it on a new query.
//
// Construction:
//
//   q, err := promquery.New("http://prometheus:9090")
//   if err != nil { ... }
//   v, err := q.QueryInstant(ctx, `sum(rate(iris_log_events_total[5m]))`)
package promquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	prom "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// DefaultTimeout caps every Prometheus call. Dashboard pages refresh
// every few seconds; a slow Prometheus shouldn't be allowed to pin a
// goroutine. Override per-call by deriving a tighter context.
const DefaultTimeout = 5 * time.Second

// Sample is one (label-set, scalar) point at a specific timestamp.
// The label map is kept as plain string→string for trivial JSON.
type Sample struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
	At     time.Time         `json:"at"`
}

// SeriesPoint is one (timestamp, value) pair in a time-series.
type SeriesPoint struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

// Series is a single time-series with its label set.
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []SeriesPoint     `json:"points"`
}

// API is the slice of operations the dashboard service uses. Mocking
// just this interface in tests is far cleaner than spinning up a fake
// Prometheus.
type API interface {
	// QueryInstant runs a PromQL expression at a single point in time
	// (Prometheus default: now). Returns one Sample per matching series.
	// An expression that evaluates to a scalar yields one Sample with
	// an empty Labels map.
	QueryInstant(ctx context.Context, query string) ([]Sample, error)

	// QueryRange runs a PromQL expression across a time range with the
	// given step. Useful for the dashboard's trend chart.
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error)

	// Healthy returns nil when Prometheus is reachable. Used at boot
	// to log a warning if the URL is mis-configured.
	Healthy(ctx context.Context) error
}

// Client is the production implementation backed by the upstream
// Prometheus HTTP API client.
type Client struct {
	api promv1.API
}

// New builds a Client for the given Prometheus base URL
// (e.g. "http://prometheus:9090"). Empty URL returns a descriptive
// error so the bootstrap layer can log a warning and fall back to a
// no-op API.
func New(baseURL string) (*Client, error) {
	u := strings.TrimSpace(baseURL)
	if u == "" {
		return nil, errors.New("promquery: base URL required")
	}
	c, err := prom.NewClient(prom.Config{Address: u})
	if err != nil {
		return nil, fmt.Errorf("promquery: client: %w", err)
	}
	return &Client{api: promv1.NewAPI(c)}, nil
}

// QueryInstant implements API.
func (c *Client) QueryInstant(ctx context.Context, query string) ([]Sample, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	v, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("promquery: instant %q: %w", query, err)
	}
	return toSamples(v), nil
}

// QueryRange implements API.
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	v, _, err := c.api.QueryRange(ctx, query, promv1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("promquery: range %q: %w", query, err)
	}
	matrix, ok := v.(model.Matrix)
	if !ok {
		// QueryRange should always return a Matrix; a non-matrix is a
		// Prometheus-side surprise we can't usefully recover from.
		return nil, fmt.Errorf("promquery: range %q: unexpected result type %T", query, v)
	}
	out := make([]Series, 0, len(matrix))
	for _, ss := range matrix {
		s := Series{
			Labels: labelsToMap(ss.Metric),
			Points: make([]SeriesPoint, 0, len(ss.Values)),
		}
		for _, p := range ss.Values {
			s.Points = append(s.Points, SeriesPoint{
				At:    p.Timestamp.Time(),
				Value: float64(p.Value),
			})
		}
		out = append(out, s)
	}
	return out, nil
}

// Healthy hits the Prometheus build-info endpoint as a cheap reachability
// probe. Doesn't tell us anything about scrape health; that's fine — we
// just need to know the URL is sane.
func (c *Client) Healthy(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := c.api.Buildinfo(ctx)
	if err != nil {
		return fmt.Errorf("promquery: ping: %w", err)
	}
	return nil
}

// withTimeout wraps the inbound context unless it already has a tighter
// deadline.
func withTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if d, ok := parent.Deadline(); ok && time.Until(d) <= DefaultTimeout {
		return parent, func() {}
	}
	return context.WithTimeout(parent, DefaultTimeout)
}

// toSamples converts an instant-query response into the wrapper's
// Sample shape. Handles the three result types the API can hand back.
func toSamples(v model.Value) []Sample {
	switch t := v.(type) {
	case *model.Scalar:
		return []Sample{{
			Labels: map[string]string{},
			Value:  float64(t.Value),
			At:     t.Timestamp.Time(),
		}}
	case model.Vector:
		out := make([]Sample, 0, len(t))
		for _, s := range t {
			out = append(out, Sample{
				Labels: labelsToMap(s.Metric),
				Value:  float64(s.Value),
				At:     s.Timestamp.Time(),
			})
		}
		return out
	default:
		// Matrix and String types aren't expected from instant queries;
		// returning empty is benign — the caller already handles the
		// "no data" case.
		return nil
	}
}

func labelsToMap(m model.Metric) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[string(k)] = string(v)
	}
	return out
}

// ----------------------------------------------------------------------------
// Noop — used when IRIS_PROMETHEUS_URL is unset.
// ----------------------------------------------------------------------------

// Noop is the API implementation returned when no Prometheus URL is
// configured. Every method returns an empty-result + a sentinel error
// so HTTP handlers can map to "503: metrics backend not configured".
type Noop struct{}

// ErrNotConfigured is the sentinel returned by Noop. Handlers compare
// errors with errors.Is(err, promquery.ErrNotConfigured) to decide on
// the response status.
var ErrNotConfigured = errors.New("promquery: prometheus url not configured")

// QueryInstant always returns ErrNotConfigured.
func (Noop) QueryInstant(context.Context, string) ([]Sample, error) {
	return nil, ErrNotConfigured
}

// QueryRange always returns ErrNotConfigured.
func (Noop) QueryRange(context.Context, string, time.Time, time.Time, time.Duration) ([]Series, error) {
	return nil, ErrNotConfigured
}

// Healthy always returns ErrNotConfigured.
func (Noop) Healthy(context.Context) error { return ErrNotConfigured }
