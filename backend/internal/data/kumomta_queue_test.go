package data

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/menta2k/iris/backend/internal/conf"
)

func TestQueueSummaryParsesMetricsAndSuspensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			_, _ = io.WriteString(w, `# HELP scheduled_by_domain scheduled messages
scheduled_by_domain{domain="example.com"} 5
scheduled_by_domain{domain="example.com"} 2
scheduled_count{queue="x"} 99
ready_count{queue="x"} 3
`)
		case "/api/admin/suspend/v1":
			_, _ = io.WriteString(w, `[{"id":"s1","domain":"example.com","reason":"maintenance"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	k := NewFileKumoMTA(conf.External{BaseURL: srv.URL})
	qs, err := k.QueueSummary(context.Background())
	if err != nil {
		t.Fatalf("QueueSummary: %v", err)
	}
	byDom := map[string]int64{}
	suspended := map[string]bool{}
	for _, q := range qs {
		byDom[q.Domain] = q.Depth
		suspended[q.Domain] = q.Suspended
	}
	if byDom["example.com"] != 5 || byDom["example.com"] != 2 {
		t.Fatalf("depths: %+v", byDom)
	}
	if !suspended["example.com"] || suspended["example.com"] {
		t.Fatalf("suspended flags wrong: %+v", suspended)
	}
}

func TestSuspendAndBounceBuildRequests(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	k := NewFileKumoMTA(conf.External{BaseURL: srv.URL})

	if _, err := k.SuspendQueue(context.Background(), "example.com", "maint"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if gotPath != "POST /api/admin/suspend/v1" {
		t.Fatalf("suspend path: %s", gotPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(gotBody), &body)
	if body["domain"] != "example.com" || body["reason"] != "maint" {
		t.Fatalf("suspend body: %s", gotBody)
	}

	if _, err := k.BounceQueue(context.Background(), "example.com", ""); err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if gotPath != "POST /api/admin/bounce/v1" {
		t.Fatalf("bounce path: %s", gotPath)
	}
}

func TestQueueSummaryNoBaseURL(t *testing.T) {
	k := NewFileKumoMTA(conf.External{})
	qs, err := k.QueueSummary(context.Background())
	if err != nil || qs != nil {
		t.Fatalf("expected nil,nil with no base url; got %v %v", qs, err)
	}
}
