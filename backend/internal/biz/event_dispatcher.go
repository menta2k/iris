package biz

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// EventProcessorSource supplies the active processors (implemented by the repo).
type EventProcessorSource interface {
	ActiveEventProcessors(ctx context.Context) ([]*EventProcessor, error)
}

// EventEmitter is the producer-facing handle for emitting events, satisfied by
// *EventDispatcher. Producers hold this (nil is a valid no-op).
type EventEmitter interface {
	Emit(ev DispatchEvent)
}

// EventErrorSink records a delivery failure (implemented by the worker-error log).
// Optional; nil disables structured error capture.
type EventErrorSink interface {
	RecordEventError(ctx context.Context, processor, driver, detail string)
}

const (
	eventCacheTTL  = 30 * time.Second // how long the active-processor list is cached
	eventFlushTick = time.Second      // batch flush timer resolution
	eventBufferCap = 4096             // in-memory emit buffer before events are dropped
	eventDeliverTO = 30 * time.Second // per-delivery timeout
)

// EventDispatcher fans emitted events out to the matching processors. Producers
// call Emit (non-blocking); a single background Run loop matches each event
// against the cached active processors and delivers it via each processor's
// driver — immediately for single mode, or buffered and flushed by size/time for
// batch mode. Delivery is best-effort with retry; failures go to the error sink.
type EventDispatcher struct {
	source   EventProcessorSource
	registry *EventDriverRegistry
	errs     EventErrorSink
	log      *slog.Logger

	in chan DispatchEvent

	// Cached active processors + their built drivers, refreshed every eventCacheTTL.
	cacheAt    time.Time
	processors []*EventProcessor
	drivers    map[string]EventDriver // processor id -> driver
	driverKey  map[string]string      // processor id -> config fingerprint

	// Per-processor batch buffers (batch mode only).
	batches map[string]*eventBatch

	dropped int
	mu      sync.Mutex
}

type eventBatch struct {
	events []DispatchEvent
	oldest time.Time
}

// NewEventDispatcher constructs a dispatcher. Call Run in a goroutine.
func NewEventDispatcher(source EventProcessorSource, registry *EventDriverRegistry, errs EventErrorSink, log *slog.Logger) *EventDispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &EventDispatcher{
		source: source, registry: registry, errs: errs, log: log,
		in:      make(chan DispatchEvent, eventBufferCap),
		drivers: map[string]EventDriver{}, driverKey: map[string]string{},
		batches: map[string]*eventBatch{},
	}
}

// Emit queues an event for delivery. Non-blocking: if the buffer is full the
// event is dropped and counted (surfaced in logs) so a slow/unreachable endpoint
// never back-pressures mail processing.
func (d *EventDispatcher) Emit(ev DispatchEvent) {
	if d == nil {
		return
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	select {
	case d.in <- ev:
	default:
		d.mu.Lock()
		d.dropped++
		n := d.dropped
		d.mu.Unlock()
		d.log.Warn("event dispatcher buffer full; dropping event", "type", ev.Type, "dropped_total", n)
	}
}

// Run consumes emitted events until ctx is cancelled, flushing batches on a
// ticker. Intended to be run in its own goroutine.
func (d *EventDispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(eventFlushTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.flushAll(ctx)
			return ctx.Err()
		case ev := <-d.in:
			d.handle(ctx, ev)
		case <-ticker.C:
			d.flushDue(ctx)
		}
	}
}

// handle matches one event against the active processors and dispatches it.
func (d *EventDispatcher) handle(ctx context.Context, ev DispatchEvent) {
	for _, p := range d.activeProcessors(ctx) {
		if !p.Matches(ev.Type, ev.Mailclass) {
			continue
		}
		if p.Mode == EventModeBatch {
			d.appendBatch(ctx, p, ev)
			continue
		}
		d.deliver(ctx, p, []DispatchEvent{ev})
	}
}

func (d *EventDispatcher) appendBatch(ctx context.Context, p *EventProcessor, ev DispatchEvent) {
	b := d.batches[p.ID]
	if b == nil {
		b = &eventBatch{oldest: time.Now()}
		d.batches[p.ID] = b
	}
	b.events = append(b.events, ev)
	if len(b.events) >= p.BatchMaxSize {
		d.flush(ctx, p, b)
	}
}

// flushDue flushes batches whose wait window has elapsed.
func (d *EventDispatcher) flushDue(ctx context.Context) {
	now := time.Now()
	for _, p := range d.activeProcessors(ctx) {
		b := d.batches[p.ID]
		if b == nil || len(b.events) == 0 {
			continue
		}
		wait := p.batchWait()
		if wait > 0 && now.Sub(b.oldest) >= wait {
			d.flush(ctx, p, b)
		}
	}
}

// flushAll flushes every buffered batch (on shutdown).
func (d *EventDispatcher) flushAll(ctx context.Context) {
	for id, b := range d.batches {
		if b == nil || len(b.events) == 0 {
			continue
		}
		if p := d.processorByID(id); p != nil {
			d.flush(ctx, p, b)
		}
	}
}

func (d *EventDispatcher) flush(ctx context.Context, p *EventProcessor, b *eventBatch) {
	if len(b.events) == 0 {
		return
	}
	events := b.events
	b.events = nil
	b.oldest = time.Time{}
	d.deliver(ctx, p, events)
}

// deliver builds the processor's driver and sends the events, with a short retry.
func (d *EventDispatcher) deliver(ctx context.Context, p *EventProcessor, events []DispatchEvent) {
	driver, err := d.driverFor(p)
	if err != nil {
		d.fail(ctx, p, err.Error())
		return
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
		dctx, cancel := context.WithTimeout(ctx, eventDeliverTO)
		lastErr = driver.Deliver(dctx, events)
		cancel()
		if lastErr == nil {
			return
		}
	}
	d.fail(ctx, p, lastErr.Error())
}

func (d *EventDispatcher) fail(ctx context.Context, p *EventProcessor, detail string) {
	d.log.Error("event delivery failed", "processor", p.Name, "driver", p.Driver, "error", detail)
	if d.errs != nil {
		d.errs.RecordEventError(ctx, p.Name, p.Driver, detail)
	}
}

// activeProcessors returns the cached active processors, refreshing on TTL.
func (d *EventDispatcher) activeProcessors(ctx context.Context) []*EventProcessor {
	if !d.cacheAt.IsZero() && time.Since(d.cacheAt) < eventCacheTTL {
		return d.processors
	}
	ps, err := d.source.ActiveEventProcessors(ctx)
	if err != nil {
		d.log.Error("load event processors", "error", err.Error())
		return d.processors // serve stale on error
	}
	d.processors = ps
	d.cacheAt = time.Now()
	return ps
}

func (d *EventDispatcher) processorByID(id string) *EventProcessor {
	for _, p := range d.processors {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// driverFor returns a cached driver for the processor, rebuilding when its config
// changed (fingerprinted by driver + updated_at).
func (d *EventDispatcher) driverFor(p *EventProcessor) (EventDriver, error) {
	key := p.Driver + "|" + p.UpdatedAt.String()
	if d.driverKey[p.ID] == key {
		if drv := d.drivers[p.ID]; drv != nil {
			return drv, nil
		}
	}
	drv, err := d.registry.Build(p)
	if err != nil {
		return nil, err
	}
	d.drivers[p.ID] = drv
	d.driverKey[p.ID] = key
	return drv, nil
}
