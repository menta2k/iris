package data

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

func newGADriver(t *testing.T, url, maxBatch string) biz.EventDriver {
	t.Helper()
	d, err := NewGreenArrowDriverFactory()(&biz.EventProcessor{
		Driver:       biz.EventDriverGreenArrow,
		DriverConfig: map[string]string{"url": url, "max_batch_size": maxBatch},
	})
	if err != nil {
		t.Fatalf("build driver: %v", err)
	}
	return d
}

func hardBounce(rcpt string) biz.DispatchEvent {
	return biz.DispatchEvent{
		Type: biz.EventBounce, OccurredAt: time.Now().UTC(), Mailclass: "acme_s",
		Data: map[string]any{
			"recipient": rcpt, "smtp_status": "550", "diagnostic": "550 5.1.1 user unknown",
			"classification": "InvalidRecipient", "bounce_type": "hard",
		},
	}
}

func TestGreenArrowDriverPostsBareArray(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		contentType = r.Header.Get("Content-Type")
		mu.Unlock()
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	d := newGADriver(t, srv.URL, "20")
	if err := d.Deliver(context.Background(), []biz.DispatchEvent{hardBounce("a@x.com")}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("want 1 POST, got %d", len(bodies))
	}
	if contentType != "application/json" {
		t.Fatalf("content-type = %q", contentType)
	}
	// Body must be a bare top-level array (not wrapped in {"events":...}).
	var arr []map[string]any
	if err := json.Unmarshal(bodies[0], &arr); err != nil {
		t.Fatalf("body is not a JSON array: %v — %s", err, bodies[0])
	}
	// One bad-address hard bounce → bounce_all + bounce_bad_address.
	if len(arr) != 2 {
		t.Fatalf("want 2 objects in array, got %d", len(arr))
	}
}

func TestGreenArrowDriverChunksToMaxBatch(t *testing.T) {
	var mu sync.Mutex
	var counts []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var arr []map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &arr)
		mu.Lock()
		counts = append(counts, len(arr))
		mu.Unlock()
	}))
	defer srv.Close()

	// 5 hard bad-address bounces → 10 GreenArrow objects; max_batch_size 4 → 4+4+2.
	evs := make([]biz.DispatchEvent, 5)
	for i := range evs {
		evs[i] = hardBounce("r@x.com")
	}
	d := newGADriver(t, srv.URL, "4")
	if err := d.Deliver(context.Background(), evs); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []int{4, 4, 2}
	if len(counts) != len(want) {
		t.Fatalf("want %d POSTs, got %d (%v)", len(want), len(counts), counts)
	}
	for i, c := range counts {
		if c != want[i] {
			t.Fatalf("chunk sizes = %v, want %v", counts, want)
		}
	}
}

func TestGreenArrowDriverNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	d := newGADriver(t, srv.URL, "20")
	if err := d.Deliver(context.Background(), []biz.DispatchEvent{hardBounce("a@x.com")}); err == nil {
		t.Fatal("expected error on 500 response")
	}
}
