// Package errlog provides a slog.Handler that mirrors a worker's Warn/Error
// records into a durable error-log store, in addition to the normal (stdout)
// handler. It exists so operational failures that previously only reached
// stdout — for example an unparseable DMARC report dropped by the dmarc
// worker — become visible and queryable in the UI.
//
// Writes to the store are buffered and flushed by a single background goroutine
// (Handler.Run); enqueue never blocks the calling worker, and a full buffer
// drops rather than stalls. The sink's own failures are reported via
// Options.OnError and never fed back through this handler (which would recurse).
package errlog

import (
	"context"
	"sync/atomic"
	"time"
)

// Entry is one captured worker log record bound for the error-log store.
type Entry struct {
	Time    time.Time
	Level   string // "warn" | "error"
	Worker  string
	Message string
	Detail  map[string]any
}

// Sink persists batches of captured entries. Implemented by the data layer.
type Sink interface {
	Insert(ctx context.Context, entries []Entry) error
}

// Options configure a Handler. The zero value is valid; New fills sensible
// defaults for the unset fields.
type Options struct {
	// BufferSize bounds the in-memory queue; when full, new entries are dropped
	// so logging never blocks a worker's hot path. Default 1024.
	BufferSize int
	// BatchSize caps how many entries are flushed to the sink at once. Default 64.
	BatchSize int
	// FlushInterval is the maximum delay before a partial batch is flushed.
	// Default 1s.
	FlushInterval time.Duration
	// Redact reports whether an attribute key holds sensitive data; matching
	// values are replaced with a placeholder before persistence. Optional.
	Redact func(key string) bool
	// OnError reports a sink failure. Optional. It MUST NOT log through the same
	// logger that feeds this handler, or it will recurse.
	OnError func(error)
}

// asyncSink buffers entries and drains them to the underlying Sink from a single
// goroutine driven by run.
type asyncSink struct {
	sink    Sink
	ch      chan Entry
	opts    Options
	dropped atomic.Uint64
}

// enqueue offers an entry to the buffer without blocking; a full buffer drops it.
func (a *asyncSink) enqueue(e Entry) {
	select {
	case a.ch <- e:
	default:
		a.dropped.Add(1)
	}
}

// run drains the buffer into the sink until ctx is cancelled, batching by size
// and flush interval, then performs a final drain+flush so queued entries are
// not lost on shutdown.
func (a *asyncSink) run(ctx context.Context) {
	ticker := time.NewTicker(a.opts.FlushInterval)
	defer ticker.Stop()

	batch := make([]Entry, 0, a.opts.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Detach from ctx cancellation so the final flush on shutdown still runs,
		// but keep a bound so a wedged DB can't hang termination.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if err := a.sink.Insert(fctx, batch); err != nil && a.opts.OnError != nil {
			a.opts.OnError(err)
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			a.drain(&batch, flush)
			return
		case e := <-a.ch:
			batch = append(batch, e)
			if len(batch) >= a.opts.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// drain empties the channel into the batch (flushing on the way) and performs a
// final flush. Used on shutdown.
func (a *asyncSink) drain(batch *[]Entry, flush func()) {
	for {
		select {
		case e := <-a.ch:
			*batch = append(*batch, e)
			if len(*batch) >= a.opts.BatchSize {
				flush()
			}
		default:
			flush()
			return
		}
	}
}
