package biz

import (
	"context"
	"sync"
	"time"
)

// DispatchEvent is a single event handed to the Event Processor for delivery.
// Data carries the event-type-specific payload (JSON-serializable).
type DispatchEvent struct {
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Mailclass  string         `json:"mailclass,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// EventDriver delivers a batch of events to an external destination. A single
// event is delivered as a one-element slice, so drivers implement one path.
// Implementations must be safe for concurrent use.
type EventDriver interface {
	Deliver(ctx context.Context, events []DispatchEvent) error
}

// EventDriverFactory builds a driver for a processor from its DriverConfig. It is
// the single extension point: a new delivery mechanism is a new EventDriver plus
// a factory registered under its driver name.
type EventDriverFactory func(p *EventProcessor) (EventDriver, error)

// EventDriverRegistry maps driver names to factories.
type EventDriverRegistry struct {
	mu        sync.RWMutex
	factories map[string]EventDriverFactory
}

// NewEventDriverRegistry constructs an empty registry.
func NewEventDriverRegistry() *EventDriverRegistry {
	return &EventDriverRegistry{factories: map[string]EventDriverFactory{}}
}

// Register adds (or replaces) the factory for a driver name.
func (r *EventDriverRegistry) Register(name string, f EventDriverFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f
}

// Has reports whether a driver name is registered.
func (r *EventDriverRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

// Build constructs the driver for a processor, or an error when its driver is
// not registered.
func (r *EventDriverRegistry) Build(p *EventProcessor) (EventDriver, error) {
	r.mu.RLock()
	f, ok := r.factories[p.Driver]
	r.mu.RUnlock()
	if !ok {
		return nil, Invalid("EVENT_DRIVER_UNKNOWN", "no delivery driver %q is registered", p.Driver)
	}
	return f(p)
}
