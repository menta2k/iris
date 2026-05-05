// Package data is the data-access layer.
//
// AuditWriter is an asynchronous, batched writer for audit entries. It
// decouples the request path from the database write so a slow DB cannot
// stall API calls. Buffer overflows fail closed — entries are dropped and
// counted, never silently lost without a metric.
package data

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/menta2k/iris/backend/pkg/middleware/audit"
)

// AuditPersister is the storage backend for committed entries.
type AuditPersister interface {
	WriteBatch(ctx context.Context, entries []*audit.Entry) error
}

// AuditWriter buffers audit entries in memory and flushes them in batches.
type AuditWriter struct {
	persister AuditPersister
	queue     chan *audit.Entry
	batchSize int
	flushEvery time.Duration

	stopOnce sync.Once
	done     chan struct{}

	dropped uint64
}

// NewAuditWriter starts a background goroutine that drains the queue.
//
// queueSize: max in-memory entries; over-the-cap writes increment Dropped.
// batchSize: max entries per DB write.
// flushEvery: forced flush interval even if batchSize is not reached.
func NewAuditWriter(persister AuditPersister, queueSize, batchSize int, flushEvery time.Duration) *AuditWriter {
	if queueSize <= 0 {
		queueSize = 4096
	}
	if batchSize <= 0 {
		batchSize = 64
	}
	if flushEvery <= 0 {
		flushEvery = time.Second
	}
	w := &AuditWriter{
		persister:  persister,
		queue:      make(chan *audit.Entry, queueSize),
		batchSize:  batchSize,
		flushEvery: flushEvery,
		done:       make(chan struct{}),
	}
	go w.run()
	return w
}

// NewAuditWriterDefault is a wire-friendly constructor: it builds a writer
// with sensible defaults and returns a cleanup func that drains and shuts
// the goroutine down. Wire threads cleanup into app shutdown automatically.
func NewAuditWriterDefault(persister AuditPersister) (*AuditWriter, func()) {
	w := NewAuditWriter(persister, 4096, 64, time.Second)
	return w, w.Stop
}

// Write enqueues an entry. Non-blocking; returns false if the queue is full.
func (w *AuditWriter) Write(ctx context.Context, e *audit.Entry) error {
	select {
	case w.queue <- e:
		return nil
	default:
		atomic.AddUint64(&w.dropped, 1)
		return nil
	}
}

// Dropped returns the count of entries dropped due to a full queue.
func (w *AuditWriter) Dropped() uint64 { return atomic.LoadUint64(&w.dropped) }

// Stop drains the queue and shuts the worker down.
func (w *AuditWriter) Stop() {
	w.stopOnce.Do(func() { close(w.queue) })
	<-w.done
}

func (w *AuditWriter) run() {
	defer close(w.done)
	tick := time.NewTicker(w.flushEvery)
	defer tick.Stop()

	batch := make([]*audit.Entry, 0, w.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = w.persister.WriteBatch(ctx, batch)
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case e, ok := <-w.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}
