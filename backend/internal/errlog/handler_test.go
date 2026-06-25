package errlog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// fakeSink collects inserted entries for assertions.
type fakeSink struct {
	mu      sync.Mutex
	entries []Entry
}

func (f *fakeSink) Insert(_ context.Context, entries []Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entries...)
	return nil
}

func (f *fakeSink) snapshot() []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Entry, len(f.entries))
	copy(out, f.entries)
	return out
}

// waitFor polls until cond is true or the deadline passes.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

func newTestHandler(sink Sink, base slog.Handler) (*Handler, context.CancelFunc) {
	if base == nil {
		base = slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	h := New(base, sink, Options{
		BatchSize:     1,
		FlushInterval: 10 * time.Millisecond,
		Redact:        func(k string) bool { return k == "password" },
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = h.Run(ctx) }()
	return h, cancel
}

func TestHandlerCapturesWarnAndError(t *testing.T) {
	sink := &fakeSink{}
	h, cancel := newTestHandler(sink, nil)
	defer cancel()

	log := slog.New(h).With("worker", "dmarc")
	log.Info("ignored info")
	log.Warn("drop unparseable dmarc report", "id", "42")
	log.Error("persist failed", "password", "hunter2", "error", errors.New("boom"))

	waitFor(t, func() bool { return len(sink.snapshot()) == 2 })

	got := sink.snapshot()
	byMsg := map[string]Entry{}
	for _, e := range got {
		byMsg[e.Message] = e
	}

	warn, ok := byMsg["drop unparseable dmarc report"]
	if !ok {
		t.Fatalf("warn entry not captured: %+v", got)
	}
	if warn.Level != "warn" {
		t.Errorf("level = %q, want warn", warn.Level)
	}
	if warn.Worker != "dmarc" {
		t.Errorf("worker = %q, want dmarc", warn.Worker)
	}
	if warn.Detail["id"] != "42" {
		t.Errorf("detail[id] = %v, want 42", warn.Detail["id"])
	}
	if _, present := warn.Detail["worker"]; present {
		t.Errorf("worker should be lifted out of detail, got %v", warn.Detail)
	}

	errEntry := byMsg["persist failed"]
	if errEntry.Level != "error" {
		t.Errorf("level = %q, want error", errEntry.Level)
	}
	if errEntry.Detail["password"] != redactedValue {
		t.Errorf("password not redacted: %v", errEntry.Detail["password"])
	}
	if errEntry.Detail["error"] != "boom" {
		t.Errorf("error detail = %v, want boom", errEntry.Detail["error"])
	}
}

func TestHandlerInfoNotCaptured(t *testing.T) {
	sink := &fakeSink{}
	h, cancel := newTestHandler(sink, nil)
	defer cancel()

	slog.New(h).Info("nothing to see")
	// Give the drain a chance; nothing should arrive.
	time.Sleep(50 * time.Millisecond)
	if n := len(sink.snapshot()); n != 0 {
		t.Fatalf("info captured: %d entries", n)
	}
}

func TestHandlerCapturesWarnAboveBaseLevel(t *testing.T) {
	// Base only emits Error to stdout, but the error log must still capture Warn.
	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})
	sink := &fakeSink{}
	h := New(base, sink, Options{BatchSize: 1, FlushInterval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = h.Run(ctx) }()

	slog.New(h).With("worker", "x").Warn("still captured")
	waitFor(t, func() bool { return len(sink.snapshot()) == 1 })
}

func TestHandlerFlushesOnShutdown(t *testing.T) {
	sink := &fakeSink{}
	// Large batch + long interval so nothing flushes until shutdown drains it.
	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := New(base, sink, Options{BatchSize: 1000, FlushInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = h.Run(ctx); close(done) }()

	slog.New(h).Error("buffered until shutdown")
	cancel()
	<-done
	if n := len(sink.snapshot()); n != 1 {
		t.Fatalf("shutdown drain lost entries: got %d, want 1", n)
	}
}
