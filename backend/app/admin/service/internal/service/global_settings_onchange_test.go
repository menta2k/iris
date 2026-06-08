package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeGSStore is a minimal in-memory GlobalSettingsStore for exercising
// the service's validation + notification behaviour without a database.
type fakeGSStore struct {
	row GlobalSettingsRow
}

func (f *fakeGSStore) Get(context.Context) (*GlobalSettingsRow, error) {
	r := f.row
	return &r, nil
}

func (f *fakeGSStore) Update(_ context.Context, in GlobalSettingsRow, actor string) (*GlobalSettingsRow, error) {
	in.UpdatedBy = actor
	f.row = in
	r := in
	return &r, nil
}

// TestUpdateFiresOnChange verifies that a successful Update notifies
// every registered observer — this is the hook the HTTPS listener
// relies on to re-bind when listen settings change.
func TestUpdateFiresOnChange(t *testing.T) {
	svc := NewGlobalSettingsService(&fakeGSStore{})

	var wg sync.WaitGroup
	wg.Add(2)
	var mu sync.Mutex
	calls := 0
	cb := func() {
		mu.Lock()
		calls++
		mu.Unlock()
		wg.Done()
	}
	svc.OnChange(cb)
	svc.OnChange(cb)
	svc.OnChange(nil) // must be ignored, not panic

	if _, err := svc.Update(context.Background(), GlobalSettingsRow{}, "tester"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnChange callbacks did not fire within 2s")
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected 2 observer invocations, got %d", calls)
	}
}

// TestUpdateValidationDoesNotNotify ensures observers are not fired when
// Update rejects the input — a rejected save must not trigger a re-bind.
func TestUpdateValidationDoesNotNotify(t *testing.T) {
	svc := NewGlobalSettingsService(&fakeGSStore{})

	fired := make(chan struct{}, 1)
	svc.OnChange(func() { fired <- struct{}{} })

	// https_listen set without cert/key paths → validation error.
	_, err := svc.Update(context.Background(), GlobalSettingsRow{HTTPSListen: ":8443"}, "tester")
	if err == nil {
		t.Fatal("expected validation error for https_listen without cert/key")
	}

	select {
	case <-fired:
		t.Fatal("observer fired despite Update validation failure")
	case <-time.After(200 * time.Millisecond):
		// expected: no notification
	}
}

func TestUpdateRejectsBadEgressDuration(t *testing.T) {
	svc := NewGlobalSettingsService(&fakeGSStore{})
	_, err := svc.Update(context.Background(),
		GlobalSettingsRow{EgressRetryInterval: "banana"}, "tester")
	if err == nil {
		t.Fatal("expected rejection of invalid duration")
	}
}

func TestUpdateAcceptsValidEgressDurations(t *testing.T) {
	svc := NewGlobalSettingsService(&fakeGSStore{})
	_, err := svc.Update(context.Background(), GlobalSettingsRow{
		EgressRetryInterval:    "5m",
		EgressMaxRetryInterval: "2h",
		EgressMaxAge:           "7d",
	}, "tester")
	if err != nil {
		t.Fatalf("valid durations rejected: %v", err)
	}
}

func TestUpdateValidatesRspamd(t *testing.T) {
	svc := NewGlobalSettingsService(&fakeGSStore{})
	ctx := context.Background()

	// Bad mode rejected.
	if _, err := svc.Update(ctx, GlobalSettingsRow{RspamdMode: "maybe"}, "t"); err == nil {
		t.Fatal("expected bad rspamd_mode to be rejected")
	}
	// enforce without a URL rejected.
	if _, err := svc.Update(ctx, GlobalSettingsRow{RspamdMode: "enforce"}, "t"); err == nil {
		t.Fatal("expected enforce without url to be rejected")
	}
	// non-http url rejected.
	if _, err := svc.Update(ctx, GlobalSettingsRow{RspamdMode: "tag", RspamdURL: "ftp://x"}, "t"); err == nil {
		t.Fatal("expected non-http url to be rejected")
	}
	// Valid config accepted.
	if _, err := svc.Update(ctx, GlobalSettingsRow{RspamdMode: "enforce", RspamdURL: "http://127.0.0.1:11333"}, "t"); err != nil {
		t.Fatalf("valid rspamd config rejected: %v", err)
	}
	// off / empty accepted.
	if _, err := svc.Update(ctx, GlobalSettingsRow{RspamdMode: ""}, "t"); err != nil {
		t.Fatalf("empty rspamd config rejected: %v", err)
	}
}
