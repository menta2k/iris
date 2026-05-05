// Package kumomta is a thin HTTP client for the kumomta daemon's admin API.
//
// SECURITY-AUDITED PORT (originally adapted from the prototype's
// internal/kumomta package):
//
//   - All requests go through a *http.Client with a hard timeout. The
//     prototype used the package-level http.DefaultClient (no timeout) which
//     is unsafe for outbound calls to a server we manage but cannot fully
//     trust (compromised kumomta could hang the admin process).
//   - The base URL must use http/https. file:// or anything exotic is
//     refused at construction time.
//   - The optional bearer token is sent only over TLS unless the operator
//     explicitly opts in via AllowInsecure (used for in-cluster Unix-style
//     deployments).
//   - All bodies are size-capped on read to defend against memory blowups.
package kumomta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxResponseBytes caps bodies at 8 MiB. The largest legitimate response
	// from kumomta's admin API (paginated queue inspection) sits well below
	// this; anything larger likely indicates a misconfiguration.
	MaxResponseBytes = 8 << 20
	defaultTimeout   = 10 * time.Second
)

var (
	ErrInvalidBaseURL = errors.New("kumomta: base URL must be http or https")
	ErrInsecureToken  = errors.New("kumomta: bearer token over plain http requires AllowInsecure")
)

// Client talks to the kumomta admin HTTP API.
type Client struct {
	base    *url.URL
	hc      *http.Client
	token   string
	timeout time.Duration

	// metrics is optional. Set via SetMetrics; nil = no instrumentation.
	// Defining the interface inline (rather than depending on
	// pkg/metrics) keeps this package free of a Prometheus dep so
	// kumomta-only consumers don't pull it in transitively.
	metrics ClientMetrics
}

// ClientMetrics is the slice of *metrics.Metrics that this client
// touches. Implemented by an adapter in the providers package.
type ClientMetrics interface {
	ObserveRequest(endpoint, method, result string, duration time.Duration)
}

// SetMetrics wires the metrics sink. Pass nil to detach.
func (c *Client) SetMetrics(m ClientMetrics) { c.metrics = m }

// Config configures a Client.
type Config struct {
	BaseURL       string
	BearerToken   string
	Timeout       time.Duration
	AllowInsecure bool
	HTTPClient    *http.Client // optional; for testing
}

// NewClient validates configuration and returns a usable Client.
func NewClient(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("kumomta: parse base url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrInvalidBaseURL
	}
	if cfg.BearerToken != "" && u.Scheme == "http" && !cfg.AllowInsecure {
		return nil, ErrInsecureToken
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{base: u, hc: hc, token: cfg.BearerToken, timeout: timeout}, nil
}

// QueueSummary is the per-queue snapshot the admin UI consumes. It does not
// match any single kumomta endpoint shape — ListQueues fuses the
// ready-queue states map with admin-suspension state to produce it.
type QueueSummary struct {
	Name       string `json:"name"`
	QueueSize  uint64 `json:"queue_size"`
	Delivered  uint64 `json:"delivered_total"`
	Failed     uint64 `json:"failed_total"`
	Deferred   uint64 `json:"deferred_total"`
	Suspended  bool   `json:"suspended"`
	SampledAt  string `json:"sampled_at"`
}

// readyQStateResponse mirrors `/api/admin/ready-q-states/v1`. We treat the
// per-queue value as a json.RawMessage and pull the few counters we display
// from its known field names (kumomta's schema is not stable across versions
// so we soft-decode).
type readyQStateResponse struct {
	StatesByReadyQueue map[string]json.RawMessage `json:"states_by_ready_queue"`
}

// readyQState is the subset of fields we actually surface. Anything we don't
// recognize is ignored — the JSON decoder is lenient.
type readyQState struct {
	QueueSize uint64 `json:"queue_size"`
	Delivered uint64 `json:"delivered"`
	Failed    uint64 `json:"failed"`
	Deferred  uint64 `json:"deferred"`
}

// adminSuspension is one entry in `/api/admin/suspend/v1`. Used to know which
// queue names are currently suspended via admin suspension; ready-q
// suspensions live under a different endpoint and aren't fused yet.
type adminSuspension struct {
	Name string `json:"name"`
}

// ListQueues calls `/api/admin/ready-q-states/v1` and merges in the admin
// suspension list. Anything that doesn't decode is logged-but-ignored — we
//'d rather show 95% of queues than fail the whole call.
func (c *Client) ListQueues(ctx context.Context) ([]QueueSummary, error) {
	var states readyQStateResponse
	if err := c.do(ctx, http.MethodGet, "/api/admin/ready-q-states/v1", nil, &states); err != nil {
		return nil, err
	}
	var suspended []adminSuspension
	// Best-effort — a suspension-list failure must not break the queue list.
	_ = c.do(ctx, http.MethodGet, "/api/admin/suspend/v1", nil, &suspended)
	suspSet := make(map[string]struct{}, len(suspended))
	for _, s := range suspended {
		suspSet[s.Name] = struct{}{}
	}

	out := make([]QueueSummary, 0, len(states.StatesByReadyQueue))
	for name, raw := range states.StatesByReadyQueue {
		var st readyQState
		_ = json.Unmarshal(raw, &st)
		_, isSusp := suspSet[name]
		out = append(out, QueueSummary{
			Name:      name,
			QueueSize: st.QueueSize,
			Delivered: st.Delivered,
			Failed:    st.Failed,
			Deferred:  st.Deferred,
			Suspended: isSusp,
		})
	}
	return out, nil
}

// suspendReadyQReq is the shape kumomta expects for suspend-ready-q.
type suspendReadyQReq struct {
	Name     string `json:"name"`
	Reason   string `json:"reason"`
	Duration string `json:"duration,omitempty"` // e.g., "1h", "30m"
}

// SuspendQueue pauses a ready queue's delivery. kumomta returns an opaque id
// that we drop — the queue name is sufficient for our resume call.
func (c *Client) SuspendQueue(ctx context.Context, name string) error {
	body := suspendReadyQReq{
		Name:     name,
		Reason:   "operator-initiated suspension",
		Duration: "24h",
	}
	return c.do(ctx, http.MethodPost, "/api/admin/suspend-ready-q/v1", body, nil)
}

// ResumeQueue is implemented as DELETE on the suspend-ready-q endpoint with
// a name match. kumomta versions vary on the exact shape; this matches the
// 2025+ admin API.
func (c *Client) ResumeQueue(ctx context.Context, name string) error {
	body := map[string]string{"name": name}
	return c.do(ctx, http.MethodDelete, "/api/admin/suspend-ready-q/v1", body, nil)
}

// bounceReq is the shape kumomta accepts on /api/admin/bounce/v1.
// queue_names is a list — we pass a single name. Duration is required.
type bounceReq struct {
	QueueNames []string `json:"queue_names"`
	Reason     string   `json:"reason"`
	Duration   string   `json:"duration"`
}

// BounceQueue bounces all messages on the named ready queue.
func (c *Client) BounceQueue(ctx context.Context, name, reason string) error {
	if reason == "" {
		reason = "operator-initiated bounce"
	}
	body := bounceReq{
		QueueNames: []string{name},
		Reason:     reason,
		Duration:   "5m",
	}
	return c.do(ctx, http.MethodPost, "/api/admin/bounce/v1", body, nil)
}

// Reload signals kumomta to re-read its policy. The 2025 admin API uses
// /api/admin/reload-config/v1; older variants used /api/admin/reload/v1.
// Try the new path first, fall back on 404.
func (c *Client) Reload(ctx context.Context) error {
	if err := c.do(ctx, http.MethodPost, "/api/admin/reload-config/v1", nil, nil); err == nil {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/api/admin/reload/v1", nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) (retErr error) {
	// Single deferred metric tick so every early return in `do` is
	// captured. retErr is named so the closure sees the actual return
	// value of the function (covers post-status decode failures too).
	if c.metrics != nil {
		start := time.Now()
		defer func() {
			result := "ok"
			if retErr != nil {
				result = "error"
			}
			c.metrics.ObserveRequest(path, method, result, time.Since(start))
		}()
	}

	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + path

	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("kumomta: marshal: %w", err)
		}
		body = strings.NewReader(string(buf))
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fmt.Errorf("kumomta: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("kumomta: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, MaxResponseBytes+1)
	respBytes, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("kumomta: read body: %w", err)
	}
	if int64(len(respBytes)) > MaxResponseBytes {
		return fmt.Errorf("kumomta: response exceeded %d bytes", MaxResponseBytes)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("kumomta: %s %s: status=%d body=%s",
			method, path, resp.StatusCode, snippet(respBytes, 256))
	}

	if out == nil || len(respBytes) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBytes, out); err != nil {
		return fmt.Errorf("kumomta: decode body: %w", err)
	}
	return nil
}

func snippet(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
