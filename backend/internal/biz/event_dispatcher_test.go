package biz

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeDriver struct {
	mu         sync.Mutex
	deliveries [][]DispatchEvent
}

func (f *fakeDriver) Deliver(_ context.Context, events []DispatchEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries = append(f.deliveries, append([]DispatchEvent{}, events...))
	return nil
}
func (f *fakeDriver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deliveries)
}

type fakeProcSource struct{ ps []*EventProcessor }

func (s fakeProcSource) ActiveEventProcessors(context.Context) ([]*EventProcessor, error) {
	return s.ps, nil
}

func newTestDispatcher(drv EventDriver, ps ...*EventProcessor) *EventDispatcher {
	reg := NewEventDriverRegistry()
	reg.Register("test", func(*EventProcessor) (EventDriver, error) { return drv, nil })
	d := NewEventDispatcher(fakeProcSource{ps}, reg, nil, nil)
	d.activeProcessors(context.Background()) // prime the cache
	return d
}

func TestEventDispatcherSingleAndBatch(t *testing.T) {
	ctx := context.Background()
	drv := &fakeDriver{}
	single := &EventProcessor{ID: "p1", Name: "s", EventTypes: []string{EventBounce}, Driver: "test", Mode: EventModeSingle, Status: "active"}
	batch := &EventProcessor{ID: "p2", Name: "b", EventTypes: []string{EventBounce}, Mailclasses: []string{"promo"}, Driver: "test", Mode: EventModeBatch, BatchMaxSize: 3, Status: "active"}
	d := newTestDispatcher(drv, single, batch)

	// Three promo bounces: single delivers each immediately (3×[1]); batch buffers
	// then flushes at size 3 (1×[3]).
	for i := 0; i < 3; i++ {
		d.handle(ctx, DispatchEvent{Type: EventBounce, Mailclass: "promo"})
	}
	if drv.count() != 4 {
		t.Fatalf("expected 4 deliveries (3 single + 1 batch flush), got %d", drv.count())
	}
	var singles, batched int
	for _, b := range drv.deliveries {
		if len(b) == 1 {
			singles++
		} else if len(b) == 3 {
			batched++
		}
	}
	if singles != 3 || batched != 1 {
		t.Fatalf("expected 3 single + 1 batch-of-3, got singles=%d batched=%d", singles, batched)
	}

	// A bounce with a non-matching mailclass hits only the single (all-class) rule.
	d.handle(ctx, DispatchEvent{Type: EventBounce, Mailclass: "other"})
	if drv.count() != 5 {
		t.Fatalf("mismatched mailclass should hit single only, got %d", drv.count())
	}

	// A non-subscribed event type is ignored entirely.
	before := drv.count()
	d.handle(ctx, DispatchEvent{Type: EventDMARCReceived, Mailclass: "promo"})
	if drv.count() != before {
		t.Fatalf("unsubscribed event type must not deliver")
	}
}

func TestEventDispatcherBatchFlushByTime(t *testing.T) {
	ctx := context.Background()
	drv := &fakeDriver{}
	batch := &EventProcessor{ID: "p1", Name: "b", EventTypes: []string{EventBounce}, Driver: "test", Mode: EventModeBatch, BatchMaxSize: 100, BatchMaxWait: "5s", Status: "active"}
	d := newTestDispatcher(drv, batch)

	d.handle(ctx, DispatchEvent{Type: EventBounce})
	// Not full and within the window → nothing delivered yet.
	d.flushDue(ctx)
	if drv.count() != 0 {
		t.Fatalf("should not flush before the wait window")
	}
	// Age the buffer past the window → flushDue delivers it.
	d.batches["p1"].oldest = time.Now().Add(-10 * time.Second)
	d.flushDue(ctx)
	if drv.count() != 1 {
		t.Fatalf("expected 1 time-based flush, got %d", drv.count())
	}
}
