package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
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

// SystemTimeseries returns host CPU / memory / per-disk usage over the given
// lookback from the iris_system_* gauges. CPU and memory are single series; each
// monitored disk path is its own series (discovered from the metric's path
// label). PrometheusAvailable=false (not an error) when no Prometheus is set.
func (uc *MetricsUsecase) SystemTimeseries(ctx context.Context, rng string) (*MetricsTimeseries, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	lookback, step, _, eff := rangeParams(rng)
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

	simple := []struct{ key, label, expr string }{
		{"cpu", "CPU %", "iris_system_cpu_percent"},
		{"memory", "Memory %", "iris_system_memory_percent"},
	}
	for _, s := range simple {
		pts, err := uc.queryRange(ctx, base, s.expr, start, end, step)
		if err != nil {
			return nil, Internal(err, "prometheus query %q", s.key)
		}
		out.Series = append(out.Series, MetricsSeries{Key: s.key, Label: s.label, Points: pts})
	}

	// One series per monitored disk path (discovered from the label set).
	for _, p := range uc.discoverDiskPaths(ctx, base) {
		q := fmt.Sprintf(`iris_system_disk_used_percent{path=%q}`, p)
		pts, err := uc.queryRange(ctx, base, q, start, end, step)
		if err != nil {
			return nil, Internal(err, "prometheus disk query %q", p)
		}
		out.Series = append(out.Series, MetricsSeries{Key: "disk:" + p, Label: "Disk " + p, Points: pts})
	}
	return out, nil
}

// discoverDiskPaths lists the path labels present on the disk-usage gauge, so the
// series set matches whatever is currently monitored.
func (uc *MetricsUsecase) discoverDiskPaths(ctx context.Context, base string) []string {
	samples, err := uc.queryInstant(ctx, base, `group by (path) (iris_system_disk_used_percent)`)
	if err != nil {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, s := range samples {
		if p := s.Metric["path"]; p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// QueueTimeBucket is one delivery-queue-time histogram bucket: the per-bucket
// (non-cumulative) count of deliveries whose queue time fell in
// (previous upper bound, UpperBound]. UpperBound is +Inf for the overflow bucket.
type QueueTimeBucket struct {
	Le         string  // Prometheus le label verbatim ("0.5", "+Inf")
	UpperBound float64 // parsed le; math.Inf(1) for "+Inf"
	Count      int64
}

// QueueTimeHistogram is the delivery-queue-time distribution over a window, from
// the iris_mail_queue_time_seconds histogram. Mailclasses lists the classes that
// have data (for the drill-down selector); empty mailclass filter = global.
type QueueTimeHistogram struct {
	Buckets             []QueueTimeBucket
	Mailclasses         []string
	TotalCount          int64
	Range               string
	PrometheusAvailable bool
}

// QueueTimeHistogram returns the delivery queue-time distribution over the given
// lookback. A non-empty mailclass narrows to one class; empty aggregates all
// (the global view). Returns PrometheusAvailable=false (not an error) when no
// Prometheus URL is configured.
func (uc *MetricsUsecase) QueueTimeHistogram(ctx context.Context, rng, mailclass string) (*QueueTimeHistogram, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}
	// eff ∈ {1h,6h,24h,7d} is also a valid Prometheus range-vector window.
	_, _, _, eff := rangeParams(rng)
	out := &QueueTimeHistogram{Range: eff}

	base := ""
	if uc.urls != nil {
		base = strings.TrimRight(strings.TrimSpace(uc.urls.PrometheusURLNow(ctx)), "/")
	}
	if base == "" {
		out.PrometheusAvailable = false
		return out, nil
	}
	out.PrometheusAvailable = true

	selector := "iris_mail_queue_time_seconds_bucket"
	if mc := strings.TrimSpace(mailclass); mc != "" {
		selector = fmt.Sprintf(`iris_mail_queue_time_seconds_bucket{mailclass=%q}`, mc)
	}
	// Cumulative per-le counts over the window (summed across all other labels).
	bucketQuery := fmt.Sprintf(`sum by (le) (increase(%s[%s]))`, selector, eff)
	samples, err := uc.queryInstant(ctx, base, bucketQuery)
	if err != nil {
		return nil, Internal(err, "prometheus queue-time histogram query")
	}
	out.Buckets, out.TotalCount = deCumulate(samples)

	// Available mail classes (for the selector), independent of the filter.
	if classes, err := uc.queryInstant(ctx, base,
		`group by (mailclass) (iris_mail_queue_time_seconds_count)`); err == nil {
		seen := map[string]bool{}
		for _, s := range classes {
			if mc := s.Metric["mailclass"]; mc != "" && mc != labelUnknownValue && !seen[mc] {
				seen[mc] = true
				out.Mailclasses = append(out.Mailclasses, mc)
			}
		}
		sort.Strings(out.Mailclasses)
	}
	return out, nil
}

// labelUnknownValue mirrors metrics.or's placeholder for empty label values.
const labelUnknownValue = "unknown"

// deCumulate turns Prometheus cumulative le buckets into per-bucket counts. The
// input samples each carry an `le` label; output is ordered by ascending upper
// bound. Fractional increase() values (rate extrapolation) are rounded.
func deCumulate(samples []promSample) ([]QueueTimeBucket, int64) {
	type leVal struct {
		le    string
		bound float64
		cum   float64
	}
	var ordered []leVal
	for _, s := range samples {
		raw, ok := s.Metric["le"]
		if !ok {
			continue
		}
		bound := math.Inf(1)
		if raw != "+Inf" {
			b, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			bound = b
		}
		ordered = append(ordered, leVal{le: raw, bound: bound, cum: s.Value})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].bound < ordered[j].bound })

	var buckets []QueueTimeBucket
	var prev float64
	for _, o := range ordered {
		c := o.cum - prev
		prev = o.cum
		if c < 0 {
			c = 0
		}
		buckets = append(buckets, QueueTimeBucket{Le: o.le, UpperBound: o.bound, Count: int64(math.Round(c))})
	}
	// Total = the +Inf cumulative (the last, highest bound), rounded.
	var total int64
	if len(ordered) > 0 {
		total = int64(math.Round(ordered[len(ordered)-1].cum))
	}
	return buckets, total
}

// promSample is one instant-query vector element: its metric labels and value.
type promSample struct {
	Metric map[string]string
	Value  float64
}

// queryInstant calls Prometheus' /api/v1/query and returns the vector result.
func (uc *MetricsUsecase) queryInstant(ctx context.Context, base, query string) ([]promSample, error) {
	q := url.Values{}
	q.Set("query", query)
	endpoint := base + "/api/v1/query?" + q.Encode()

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
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", pr.Error)
	}
	var out []promSample
	for _, r := range pr.Data.Result {
		raw, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue // NaN/Inf strings are skipped
		}
		out = append(out, promSample{Metric: r.Metric, Value: val})
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

// Widget guardrails: raw-PromQL and catalog widgets share these bounds.
const (
	widgetQueryTimeout = 10 * time.Second
	maxWidgetSeries    = 20
	maxPromQLLen       = 2048
)

// WidgetDataRequest asks for one dashboard widget's data. Source is
// "catalog" (CatalogKey resolves a curated WidgetDef) or "promql" (PromQL is a
// raw expression, guarded). GroupBy applies only to catalog widgets that support
// it and is sanitized against the def's label allow-list.
type WidgetDataRequest struct {
	Source     string
	CatalogKey string
	PromQL     string
	Range      string
	GroupBy    string
}

// WidgetData resolves and executes one widget's query, returning the same
// MetricsTimeseries shape the overview uses. Guardrails: a hard 10s timeout
// independent of the caller ctx, a 20-series cap, and a bounded step derived
// from the range. Read-only by construction (only Prometheus query endpoints are
// called). When Prometheus is unconfigured it returns PrometheusAvailable=false.
func (uc *MetricsUsecase) WidgetData(ctx context.Context, req WidgetDataRequest) (*MetricsTimeseries, error) {
	if _, err := RequirePermission(ctx, PermDashboardRead); err != nil {
		return nil, err
	}

	lookback, step, window, eff := rangeParams(req.Range)
	out := &MetricsTimeseries{Range: eff, StepSeconds: int64(step.Seconds())}

	// Resolve the query + whether it is an instant (single-value) widget.
	var query string
	instant := false
	switch strings.TrimSpace(req.Source) {
	case "catalog", "":
		def, ok := lookupWidget(req.CatalogKey)
		if !ok {
			return nil, Invalid("WIDGET_UNKNOWN", "unknown widget %q", req.CatalogKey)
		}
		query = def.resolveTemplate(window, req.GroupBy)
		instant = def.Instant
	case "promql":
		q := strings.TrimSpace(req.PromQL)
		if q == "" {
			return nil, Invalid("WIDGET_PROMQL_REQUIRED", "promql expression is required")
		}
		if len(q) > maxPromQLLen {
			return nil, Invalid("WIDGET_PROMQL_TOO_LONG", "promql exceeds %d characters", maxPromQLLen)
		}
		query = strings.ReplaceAll(q, "$window", window)
	default:
		return nil, Invalid("WIDGET_SOURCE_INVALID", "source must be catalog or promql")
	}

	base := ""
	if uc.urls != nil {
		base = strings.TrimRight(strings.TrimSpace(uc.urls.PrometheusURLNow(ctx)), "/")
	}
	if base == "" {
		out.PrometheusAvailable = false
		return out, nil
	}
	out.PrometheusAvailable = true

	// Hard timeout independent of the caller so a heavy PromQL can't tie up the
	// request indefinitely.
	qctx, cancel := context.WithTimeout(ctx, widgetQueryTimeout)
	defer cancel()

	end := uc.now()
	if instant {
		samples, err := uc.queryInstant(qctx, base, query)
		if err != nil {
			return nil, Internal(err, "prometheus widget query")
		}
		out.Series = instantSamplesToSeries(samples)
		return out, nil
	}

	series, err := uc.queryRangeMulti(qctx, base, query, end.Add(-lookback), end, step)
	if err != nil {
		return nil, Internal(err, "prometheus widget query")
	}
	out.Series = series
	return out, nil
}

// instantSamplesToSeries turns instant vector samples into single-point series,
// capped at maxWidgetSeries. Each sample's distinguishing label becomes the key.
func instantSamplesToSeries(samples []promSample) []MetricsSeries {
	ts := int64(0)
	var out []MetricsSeries
	for i, s := range samples {
		if i >= maxWidgetSeries {
			break
		}
		label := seriesLabel(s.Metric)
		out = append(out, MetricsSeries{
			Key:    label,
			Label:  label,
			Points: []MetricPoint{{Timestamp: ts, Value: s.Value}},
		})
	}
	return out
}

// queryRangeMulti calls /api/v1/query_range and decodes ALL matrix rows into
// separate series (unlike queryRange, which flattens only the first row). Series
// are capped at maxWidgetSeries; each row's label set names the series.
func (uc *MetricsUsecase) queryRangeMulti(ctx context.Context, base, query string, start, end time.Time, step time.Duration) ([]MetricsSeries, error) {
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
				Metric map[string]string `json:"metric"`
				Values [][]any           `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", pr.Error)
	}

	var out []MetricsSeries
	single := len(pr.Data.Result) == 1
	for i, r := range pr.Data.Result {
		if i >= maxWidgetSeries {
			break
		}
		pts := make([]MetricPoint, 0, len(r.Values))
		for _, v := range r.Values {
			if len(v) != 2 {
				continue
			}
			tsF, ok := v[0].(float64)
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
			pts = append(pts, MetricPoint{Timestamp: int64(tsF), Value: val})
		}
		label := seriesLabel(r.Metric)
		if single && label == "" {
			label = "value"
		}
		out = append(out, MetricsSeries{Key: label, Label: label, Points: pts})
	}
	return out, nil
}

// seriesLabel derives a stable display label from a Prometheus metric's label
// set. A single non-name label (the common group-by case) is used verbatim;
// otherwise the remaining labels are joined so series stay distinguishable.
func seriesLabel(metric map[string]string) string {
	if len(metric) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metric))
	for k := range metric {
		if k == "__name__" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		return metric[keys[0]]
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+metric[k])
	}
	if len(parts) == 0 {
		return metric["__name__"]
	}
	return strings.Join(parts, ", ")
}
