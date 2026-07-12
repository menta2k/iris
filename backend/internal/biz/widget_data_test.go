package biz

import (
	"context"
	"strings"
	"testing"
)

func TestWidgetDataUnavailableWhenNoURL(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: ""}, &fakeDoer{})
	ts, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "catalog", CatalogKey: "kumo_messages_received_rate", Range: "6h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.PrometheusAvailable {
		t.Fatal("expected PrometheusAvailable=false when URL unset")
	}
}

func TestWidgetDataUnknownCatalogKey(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	_, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "catalog", CatalogKey: "nope"})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "WIDGET_UNKNOWN" {
		t.Fatalf("expected WIDGET_UNKNOWN, got %v", err)
	}
}

func TestWidgetDataPromQLRequired(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	_, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "promql", PromQL: "   "})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "WIDGET_PROMQL_REQUIRED" {
		t.Fatalf("expected WIDGET_PROMQL_REQUIRED, got %v", err)
	}
}

func TestWidgetDataPromQLTooLong(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	_, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "promql", PromQL: strings.Repeat("a", maxPromQLLen+1)})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "WIDGET_PROMQL_TOO_LONG" {
		t.Fatalf("expected WIDGET_PROMQL_TOO_LONG, got %v", err)
	}
}

func TestWidgetDataInvalidSource(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	_, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "bogus"})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "WIDGET_SOURCE_INVALID" {
		t.Fatalf("expected WIDGET_SOURCE_INVALID, got %v", err)
	}
}

func TestWidgetDataRequiresPermission(t *testing.T) {
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, &fakeDoer{})
	ctx := WithIdentity(context.Background(), &Identity{Permissions: NewPermissionSet(nil), MFAVerified: true})
	if _, err := uc.WidgetData(ctx, WidgetDataRequest{Source: "catalog", CatalogKey: "kumo_messages_received_rate"}); err == nil {
		t.Fatal("expected permission error")
	}
}

func TestWidgetDataMultiSeriesCappedAndLabeled(t *testing.T) {
	// Two matrix rows distinguished by a single label → two labeled series.
	doer := &fakeDoer{body: `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"provider":"gmail"},"values":[[1782160000,"3"]]},
		{"metric":{"provider":"yahoo"},"values":[[1782160000,"5"]]}
	]}}`}
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, doer)
	ts, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "catalog", CatalogKey: "kumo_messages_delivered_rate", Range: "6h", GroupBy: "provider"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ts.Series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(ts.Series))
	}
	if ts.Series[0].Label != "gmail" || ts.Series[1].Label != "yahoo" {
		t.Fatalf("unexpected series labels: %q, %q", ts.Series[0].Label, ts.Series[1].Label)
	}
	if !strings.Contains(doer.lastURL, "query_range") || !strings.Contains(doer.lastURL, "by") {
		t.Fatalf("expected grouped query_range request, got %s", doer.lastURL)
	}
}

func TestWidgetDataInstantWidgetUsesInstantQuery(t *testing.T) {
	doer := &fakeDoer{body: `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{},"value":[1782160000,"42"]}
	]}}`}
	uc := NewMetricsUsecase(fakePromURL{url: "http://prom:9090"}, doer)
	ts, err := uc.WidgetData(ownerCtx(), WidgetDataRequest{Source: "catalog", CatalogKey: "kumo_message_count", Range: "1h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(doer.lastURL, "/api/v1/query?") {
		t.Fatalf("expected instant query endpoint, got %s", doer.lastURL)
	}
	if len(ts.Series) != 1 || len(ts.Series[0].Points) != 1 || ts.Series[0].Points[0].Value != 42 {
		t.Fatalf("unexpected instant series: %+v", ts.Series)
	}
}
