package biz

import (
	"context"
	"io"
	"net/http"
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
