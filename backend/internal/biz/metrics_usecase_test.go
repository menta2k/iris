package biz

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type fakePromURL struct{ url string }

func (f fakePromURL) PrometheusURLNow(context.Context) string { return f.url }

type fakeDoer struct {
	lastURL string
	body    string
	status  int
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.lastURL = req.URL.String()
	st := f.status
	if st == 0 {
		st = http.StatusOK
	}
	return &http.Response{
		StatusCode: st,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

func TestMetricsUnavailableWhenNoURL(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: ""}, &fakeDoer{})
	ts, err := uc.Timeseries(ownerCtx(), "6h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.PrometheusAvailable {
		t.Fatal("expected PrometheusAvailable=false when URL unset")
	}
	if len(ts.Series) != 0 {
		t.Fatalf("expected no series, got %d", len(ts.Series))
	}
}

func TestMetricsParsesQueryRange(t *testing.T) {
	// A single aggregated matrix series with two points.
	doer := &fakeDoer{body: `{"status":"success","data":{"resultType":"matrix",
		"result":[{"metric":{},"values":[[1782160000,"12.5"],[1782160300,"0"]]}]}}`}
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090/"}, doer)

	ts, err := uc.Timeseries(ownerCtx(), "1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ts.PrometheusAvailable {
		t.Fatal("expected PrometheusAvailable=true")
	}
	if ts.Range != "1h" || ts.StepSeconds != 60 {
		t.Fatalf("unexpected range/step: %s/%d", ts.Range, ts.StepSeconds)
	}
	if len(ts.Series) != len(curatedSeries) {
		t.Fatalf("expected %d series, got %d", len(curatedSeries), len(ts.Series))
	}
	first := ts.Series[0]
	if len(first.Points) != 2 || first.Points[0].Value != 12.5 || first.Points[0].Timestamp != 1782160000 {
		t.Fatalf("unexpected points: %+v", first.Points)
	}
	// Base URL trailing slash trimmed; query_range path used.
	if !strings.Contains(doer.lastURL, "/api/v1/query_range?") || strings.Contains(doer.lastURL, "9090//api") {
		t.Fatalf("unexpected request URL: %s", doer.lastURL)
	}
}

func TestMetricsRequiresPermission(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	ctx := WithIdentity(context.Background(), &Identity{Permissions: NewPermissionSet(nil), MFAVerified: true})
	if _, err := uc.Timeseries(ctx, "6h"); err == nil {
		t.Fatal("expected permission denied without dashboard:read")
	}
}

// recordingDoer captures every request URL, in order (the shared fakeDoer only
// keeps the last one).
type recordingDoer struct {
	urls []string
	body string
}

func (r *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Header:     make(http.Header),
	}, nil
}

// TestMetricsCuratedStatusLabels is a regression test for the blank mail-flow
// chart: the curated PromQL must filter on the status values iris actually
// records (sent/received/deferred via mail_record.go), not the raw KumoMTA event
// types (Delivery/Reception/TransientFailure). One query_range request is made
// per curated series, in order.
func TestMetricsCuratedStatusLabels(t *testing.T) {
	doer := &recordingDoer{body: `{"status":"success","data":{"result":[]}}`}
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, doer)

	if _, err := uc.Timeseries(ownerCtx(), "6h"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doer.urls) != len(curatedSeries) {
		t.Fatalf("expected %d queries, got %d", len(curatedSeries), len(doer.urls))
	}
	want := []string{
		`iris_mail_events_total{status="sent"}`,
		`iris_mail_events_total{status="received"}`,
		`iris_mail_events_total{status="deferred"}`,
		`iris_bounces_total`,
	}
	for i, w := range want {
		dec, err := url.QueryUnescape(doer.urls[i])
		if err != nil {
			t.Fatalf("decode url %d: %v", i, err)
		}
		if !strings.Contains(dec, w) {
			t.Fatalf("query %d: want expr containing %q, got %s", i, w, dec)
		}
	}
}

func TestDeCumulate(t *testing.T) {
	// Cumulative le buckets arrive unsorted; per-bucket counts are the deltas.
	samples := []promSample{
		{Metric: map[string]string{"le": "1"}, Value: 5},
		{Metric: map[string]string{"le": "0.5"}, Value: 3},
		{Metric: map[string]string{"le": "+Inf"}, Value: 10},
		{Metric: map[string]string{"le": "5"}, Value: 8.4}, // fractional → rounded
	}
	buckets, total := deCumulate(samples)
	if total != 10 {
		t.Fatalf("total = %d, want 10", total)
	}
	wantLe := []string{"0.5", "1", "5", "+Inf"}
	wantCount := []int64{3, 2, 3, 2} // 3, 5-3, round(8.4)-5, 10-8
	if len(buckets) != len(wantLe) {
		t.Fatalf("got %d buckets, want %d", len(buckets), len(wantLe))
	}
	for i, b := range buckets {
		if b.Le != wantLe[i] || b.Count != wantCount[i] {
			t.Fatalf("bucket %d = {le:%s count:%d}, want {le:%s count:%d}", i, b.Le, b.Count, wantLe[i], wantCount[i])
		}
	}
}

func TestQueueTimeHistogramUnavailableWhenNoURL(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: ""}, &fakeDoer{})
	h, err := uc.QueueTimeHistogram(ownerCtx(), "6h", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.PrometheusAvailable {
		t.Fatal("expected PrometheusAvailable=false without a URL")
	}
	if h.Range != "6h" {
		t.Fatalf("range = %q, want 6h", h.Range)
	}
}
