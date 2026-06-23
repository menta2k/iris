package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MetricPoint is a single (timestamp, value) sample. Value is events per minute.
type MetricPoint struct {
	Timestamp int64
	Value     float64
}

// MetricsSeries is one curated line on the dashboard (e.g. deliveries/min).
type MetricsSeries struct {
	Key    string
	Label  string
	Points []MetricPoint
}

// MetricsTimeseries is the curated mail-flow overview returned to the dashboard.
type MetricsTimeseries struct {
	Series              []MetricsSeries
	Range               string
	StepSeconds         int64
	PrometheusAvailable bool
}

// PrometheusURLProvider supplies the configured Prometheus base URL (empty when
// unset). Satisfied by GlobalSettingsUsecase.
type PrometheusURLProvider interface {
	PrometheusURLNow(ctx context.Context) string
}

// HTTPDoer is the subset of *http.Client the metrics usecase needs.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// MetricsUsecase serves curated mail-flow time-series by querying Prometheus.
// The UI never sees PromQL: the queries are fixed here and only a lookback range
// is selectable.
type MetricsUsecase struct {
	urls   PrometheusURLProvider
	client HTTPDoer
	now    func() time.Time
}

// NewMetricsUsecase constructs the use case. A nil client defaults to a 10s
// http.Client.
func NewMetricsUsecase(urls PrometheusURLProvider, client HTTPDoer) *MetricsUsecase {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &MetricsUsecase{urls: urls, client: client, now: time.Now}
}

// curatedSeries defines the fixed PromQL (sans rate window) for each line. Each
// expression is wrapped as sum(rate(<expr>[<window>])) * 60 → events/minute.
//
// The status label values MUST match what the logstream worker records into
// iris_mail_events_total — i.e. the normalized MailRecord statuses from
// mail_record.go (sent/received/deferred/bounced), NOT the raw KumoMTA event
// types (Delivery/Reception/TransientFailure). Filtering on the raw types
// yields an empty result and a blank chart.
var curatedSeries = []struct {
	key, label, expr string
}{
	{"deliveries", "Deliveries/min", `iris_mail_events_total{status="` + MailSent + `"}`},
	{"receptions", "Receptions/min", `iris_mail_events_total{status="` + MailReceived + `"}`},
	{"deferrals", "Deferrals/min", `iris_mail_events_total{status="` + MailDeferred + `"}`},
	{"bounces", "Bounces/min", `iris_bounces_total`},
}

// rangeParams maps a lookback range to (duration, step, rate-window). Unknown
// ranges fall back to "6h".
func rangeParams(r string) (lookback time.Duration, step time.Duration, window string, eff string) {
	switch strings.TrimSpace(r) {
	case "1h":
		return time.Hour, time.Minute, "2m", "1h"
	case "24h":
		return 24 * time.Hour, 15 * time.Minute, "15m", "24h"
	case "7d":
		return 7 * 24 * time.Hour, time.Hour, "1h", "7d"
	case "6h", "":
		return 6 * time.Hour, 5 * time.Minute, "5m", "6h"
	default:
		return 6 * time.Hour, 5 * time.Minute, "5m", "6h"
	}
}

// Timeseries returns the curated overview for the given range. When no
// Prometheus URL is configured it returns PrometheusAvailable=false (not an
// error) so the dashboard can render an "unconfigured" state.
func (uc *MetricsUsecase) Timeseries(ctx context.Context, rng string) (*MetricsTimeseries, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	lookback, step, window, eff := rangeParams(rng)
	out := &MetricsTimeseries{Range: eff, StepSeconds: int64(step.Seconds())}

	base := ""
	if uc.urls != nil {
		base = strings.TrimRight(strings.TrimSpace(uc.urls.PrometheusURLNow(ctx)), "/")
	}
	if base == "" {
		out.PrometheusAvailable = false
		return out, nil
	}
	out.PrometheusAvailable = true

	end := uc.now()
	start := end.Add(-lookback)
	for _, s := range curatedSeries {
		query := fmt.Sprintf("sum(rate(%s[%s])) * 60", s.expr, window)
		pts, err := uc.queryRange(ctx, base, query, start, end, step)
		if err != nil {
			return nil, Internal(err, "prometheus query %q", s.key)
		}
		out.Series = append(out.Series, MetricsSeries{Key: s.key, Label: s.label, Points: pts})
	}
	return out, nil
}

// queryRange calls Prometheus' /api/v1/query_range and flattens the (single,
// aggregated) matrix result into points. An empty result yields no points.
func (uc *MetricsUsecase) queryRange(ctx context.Context, base, query string, start, end time.Time, step time.Duration) ([]MetricPoint, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.FormatInt(int64(step.Seconds()), 10))
	endpoint := base + "/api/v1/query_range?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := uc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus returned HTTP %d", resp.StatusCode)
	}

	var pr struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Values [][]any `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", pr.Error)
	}
	if len(pr.Data.Result) == 0 {
		return nil, nil
	}
	var pts []MetricPoint
	for _, v := range pr.Data.Result[0].Values {
		if len(v) != 2 {
			continue
		}
		ts, ok := v[0].(float64)
		if !ok {
			continue
		}
		raw, ok := v[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue // NaN/Inf strings are skipped
		}
		pts = append(pts, MetricPoint{Timestamp: int64(ts), Value: val})
	}
	return pts, nil
}
