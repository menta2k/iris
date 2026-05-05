package data

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/menta2k/iris/backend/pkg/middleware/audit"
)

type capturingPersister struct {
	mu      sync.Mutex
	written [][]*audit.Entry
	count   int64
}

func (p *capturingPersister) WriteBatch(ctx context.Context, batch []*audit.Entry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]*audit.Entry, len(batch))
	copy(cp, batch)
	p.written = append(p.written, cp)
	atomic.AddInt64(&p.count, int64(len(batch)))
	return nil
}

func TestAuditWriterFlushesBatch(t *testing.T) {
	p := &capturingPersister{}
	w := NewAuditWriter(p, 100, 4, time.Hour)
	defer w.Stop()
	for i := 0; i < 4; i++ {
		require.NoError(t, w.Write(context.Background(), &audit.Entry{Operation: "op"}))
	}
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&p.count) == 4
	}, time.Second, 5*time.Millisecond)
}

func TestAuditWriterFlushesOnTick(t *testing.T) {
	p := &capturingPersister{}
	w := NewAuditWriter(p, 100, 1000, 50*time.Millisecond)
	defer w.Stop()
	for i := 0; i < 3; i++ {
		require.NoError(t, w.Write(context.Background(), &audit.Entry{Operation: "op"}))
	}
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&p.count) == 3
	}, time.Second, 10*time.Millisecond)
}

func TestAuditWriterDropsWhenFull(t *testing.T) {
	p := &capturingPersister{}
	w := NewAuditWriter(p, 1, 1000, time.Hour)
	defer w.Stop()
	for i := 0; i < 100; i++ {
		_ = w.Write(context.Background(), &audit.Entry{Operation: "op"})
	}
	require.Greater(t, w.Dropped(), uint64(0), "writer should report drops when queue is full")
}

func TestAuditWriterDrainsOnStop(t *testing.T) {
	p := &capturingPersister{}
	w := NewAuditWriter(p, 100, 1000, time.Hour)
	for i := 0; i < 5; i++ {
		require.NoError(t, w.Write(context.Background(), &audit.Entry{Operation: "op"}))
	}
	w.Stop()
	require.Equal(t, int64(5), atomic.LoadInt64(&p.count))
}
