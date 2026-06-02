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
