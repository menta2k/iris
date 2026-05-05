// Redis-stream consumer for kumomta log records. The kumomta-side policy
// XADDs JSON-encoded log records into a stream (default: "kumo.events");
// this consumer reads them via XREADGROUP, persists into the LogEvent and
// FeedbackReport ent tables, and acks. Failed entries land in a DLQ stream
// so they're recoverable without blocking the main flow.
package logstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/pkg/metrics"
)

// RedisConfig configures the consumer. Most fields have sensible defaults
// surfaced via DefaultRedisConfig — callers typically only need to set
// RedisURL and Persister.
type RedisConfig struct {
	// RedisURL is the redis://… URL the consumer connects to.
	RedisURL string

	// Stream is the kumomta XADD target (default: "kumo.events").
	Stream string

	// Group is the consumer group name; must be the same across replicas
	// for at-least-once with load-balancing semantics.
	Group string

	// Consumer is this replica's identity inside the group; defaults to
	// "<hostname>-<pid>" so two consumers on the same host don't collide.
	Consumer string

	// DLQStream is where unparseable / repeatedly failing entries are
	// shunted; default: "<Stream>.dlq".
	DLQStream string

	WorkerCount   int
	BatchSize     int64
	BlockTimeout  time.Duration
	ClaimIdle     time.Duration
	ClaimInterval time.Duration
	HandleTimeout time.Duration

	// Persister handles the actual database writes. Tests inject a fake;
	// production wires *data.RedisLogPersister (which closes over an ent
	// client).
	Persister Persister

	// Metrics is optional. When non-nil, the consumer increments
	// per-event-type counters and a duration histogram on every
	// processed record. Tests pass nil; production wires the shared
	// *metrics.Metrics from cmd/server.
	Metrics *metrics.Metrics
}

// Persister is the database-side contract the consumer requires. Splitting
// this from the ent client lets us swap in fakes for tests and keeps this
// package free of ent imports.
type Persister interface {
	InsertLog(ctx context.Context, lr *RedisLogRecord, raw string) error
	InsertFeedback(ctx context.Context, lr *RedisLogRecord, raw string) error
	AutoSuppress(ctx context.Context, lr *RedisLogRecord) error
}

// DefaultRedisConfig returns a config with sensible production defaults.
// Caller must still set RedisURL and Persister.
func DefaultRedisConfig() RedisConfig {
	host, _ := os.Hostname()
	return RedisConfig{
		Stream:        "kumo.events",
		Group:         "kumo-ui-tracker",
		Consumer:      fmt.Sprintf("%s-%d", host, os.Getpid()),
		DLQStream:     "kumo.events.dlq",
		WorkerCount:   4,
		BatchSize:     100,
		BlockTimeout:  5 * time.Second,
		ClaimIdle:     2 * time.Minute,
		ClaimInterval: 30 * time.Second,
		HandleTimeout: 30 * time.Second,
	}
}

// RedisConsumer reads kumomta log records from a Redis stream and forwards
// them to a Persister. Lifecycle: NewRedisConsumer → Start (blocks until
// ctx is cancelled) → Close.
type RedisConsumer struct {
	cfg    RedisConfig
	rdb    *redis.Client
	stats  *ConsumerStats
	workCh chan redisStreamEntry
}

// NewRedisConsumer wires the redis client and validates the config.
func NewRedisConsumer(cfg RedisConfig) (*RedisConsumer, error) {
	if cfg.RedisURL == "" {
		return nil, errors.New("logstream: RedisURL required")
	}
	if cfg.Persister == nil {
		return nil, errors.New("logstream: Persister required")
	}
	if cfg.Stream == "" {
		cfg.Stream = "kumo.events"
	}
	if cfg.Group == "" {
		cfg.Group = "kumo-ui-tracker"
	}
	if cfg.Consumer == "" {
		host, _ := os.Hostname()
		cfg.Consumer = fmt.Sprintf("%s-%d", host, os.Getpid())
	}
	if cfg.DLQStream == "" {
		cfg.DLQStream = cfg.Stream + ".dlq"
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = 5 * time.Second
	}
	if cfg.ClaimIdle <= 0 {
		cfg.ClaimIdle = 2 * time.Minute
	}
	if cfg.ClaimInterval <= 0 {
		cfg.ClaimInterval = 30 * time.Second
	}
	if cfg.HandleTimeout <= 0 {
		cfg.HandleTimeout = 30 * time.Second
	}
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("logstream: parse redis url: %w", err)
	}
	return &RedisConsumer{
		cfg:    cfg,
		rdb:    redis.NewClient(opts),
		stats:  &ConsumerStats{},
		workCh: make(chan redisStreamEntry, cfg.BatchSize),
	}, nil
}

// Stats returns the live counter set. Atomic loads only — safe to call
// from any goroutine.
func (c *RedisConsumer) Stats() *ConsumerStats { return c.stats }

// Close releases the underlying redis connection pool.
func (c *RedisConsumer) Close() error { return c.rdb.Close() }

// Start blocks until ctx is cancelled. Spawns N workers + one auto-claim
// goroutine; the calling goroutine runs the read loop.
func (c *RedisConsumer) Start(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("logstream: redis ping: %w", err)
	}
	if err := c.ensureGroup(ctx); err != nil {
		return fmt.Errorf("logstream: ensure group: %w", err)
	}
	log.Printf("logstream: starting consumer stream=%s group=%s consumer=%s workers=%d",
		c.cfg.Stream, c.cfg.Group, c.cfg.Consumer, c.cfg.WorkerCount)

	var wg sync.WaitGroup
	for i := 0; i < c.cfg.WorkerCount; i++ {
		wg.Add(1)
		go c.worker(ctx, &wg)
	}
	wg.Add(1)
	go c.claimLoop(ctx, &wg)
	if c.cfg.Metrics != nil {
		// Cheap XPENDING-summary poll (no message scan, just the
		// counter) so the gauge stays accurate without burdening
		// Redis. Runs only when metrics are wired.
		wg.Add(1)
		go c.pendingGaugeLoop(ctx, &wg)
	}

	c.readLoop(ctx)

	close(c.workCh)
	wg.Wait()
	log.Printf("logstream: consumer stopped processed=%d failed=%d claimed=%d dlq=%d",
		c.stats.Processed.Load(), c.stats.Failed.Load(),
		c.stats.Claimed.Load(), c.stats.DLQ.Load())
	return nil
}

func (c *RedisConsumer) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.cfg.Stream, c.cfg.Group, "$").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

func (c *RedisConsumer) readLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.cfg.Group,
			Consumer: c.cfg.Consumer,
			Streams:  []string{c.cfg.Stream, ">"},
			Count:    c.cfg.BatchSize,
			Block:    c.cfg.BlockTimeout,
		}).Result()

		if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// NOGROUP can happen if the stream key was deleted under us
			// (e.g. an operator ran DEL during a redeploy). Recreate the
			// group in-place; new entries will be picked up on the next
			// iteration.
			if strings.Contains(err.Error(), "NOGROUP") {
				if rerr := c.ensureGroup(ctx); rerr != nil {
					log.Printf("logstream: re-create group after NOGROUP failed: %v", rerr)
				} else {
					log.Printf("logstream: re-created consumer group after NOGROUP")
				}
				continue
			}
			log.Printf("logstream: xreadgroup failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		for _, stream := range res {
			for _, msg := range stream.Messages {
				select {
				case c.workCh <- toRedisEntry(msg):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (c *RedisConsumer) claimLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(c.cfg.ClaimInterval)
	defer t.Stop()

	startID := "0-0"
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			msgs, next, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
				Stream:   c.cfg.Stream,
				Group:    c.cfg.Group,
				Consumer: c.cfg.Consumer,
				MinIdle:  c.cfg.ClaimIdle,
				Start:    startID,
				Count:    c.cfg.BatchSize,
			}).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				log.Printf("logstream: xautoclaim failed: %v", err)
				continue
			}
			startID = next
			for _, msg := range msgs {
				c.stats.Claimed.Add(1)
				select {
				case c.workCh <- toRedisEntry(msg):
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (c *RedisConsumer) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for entry := range c.workCh {
		c.processEntry(ctx, entry)
	}
}

func (c *RedisConsumer) processEntry(ctx context.Context, entry redisStreamEntry) {
	wctx, cancel := context.WithTimeout(ctx, c.cfg.HandleTimeout)
	defer cancel()

	// Capture wall-clock for the per-event histogram. We measure the
	// full path (parse error → DLQ, or persist → ack), not just the
	// successful insert, so the histogram surfaces real-world latency
	// including failure paths.
	start := time.Now()
	eventType := entry.record.Type
	if eventType == "" {
		eventType = "Unknown"
	}

	if entry.parErr != nil {
		c.toDLQ(wctx, entry, "parse_error: "+entry.parErr.Error())
		c.recordDropped("parse_error")
		c.observeDuration(eventType, start)
		return
	}

	if err := c.handleEvent(wctx, &entry.record, entry.raw); err != nil {
		c.stats.Failed.Add(1)
		log.Printf("logstream: handler failed id=%s type=%s: %v",
			entry.id, entry.record.Type, err)
		c.recordDropped("persist_error")
		c.observeDuration(eventType, start)
		return
	}

	if _, err := c.rdb.TxPipelined(wctx, func(p redis.Pipeliner) error {
		p.XAck(wctx, c.cfg.Stream, c.cfg.Group, entry.id)
		p.XDel(wctx, c.cfg.Stream, entry.id)
		return nil
	}); err != nil {
		log.Printf("logstream: xack/xdel failed id=%s: %v", entry.id, err)
		c.recordDropped("ack_error")
		c.observeDuration(eventType, start)
		return
	}
	c.stats.Processed.Add(1)
	c.recordProcessed(eventType, mailClassFromHeaders(entry.record.Headers))
	c.observeDuration(eventType, start)
}

// pendingGaugeLoop refreshes the iris_log_stream_pending gauge by
// asking Redis for the consumer-group's pending count. We use the
// summary form of XPENDING (no message scan) so it stays cheap even
// at high stream rates.
func (c *RedisConsumer) pendingGaugeLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			res, err := c.rdb.XPending(ctx, c.cfg.Stream, c.cfg.Group).Result()
			if err != nil {
				// Not fatal — gauge just won't update this tick. We
				// don't log to avoid drowning the console; the next
				// successful tick overwrites the stale value.
				continue
			}
			c.cfg.Metrics.LogStreamPending.Set(float64(res.Count))
		}
	}
}

// mailClassFromHeaders extracts the X-Kumo-Mail-Class header value
// from the kumomta log record. Kumomta hands us the header map in
// canonical case; we accept either the canonical form or a
// lower-cased fallback to be defensive against future changes.
//
// Returns "" when the header is absent — that's the legitimate
// "unclassified" bucket and shows up in metrics with empty label.
func mailClassFromHeaders(h map[string]any) string {
	const canon = "X-Kumo-Mail-Class"
	if v, ok := h[canon]; ok {
		return headerStr(v)
	}
	if v, ok := h["x-kumo-mail-class"]; ok {
		return headerStr(v)
	}
	return ""
}

func headerStr(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []any:
		if len(x) == 0 {
			return ""
		}
		if s, ok := x[0].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// recordProcessed increments the per-event counter. Splits the metric
// access from the hot path so a nil Metrics (test mode, or no scrape
// configured) is a no-op rather than a nil dereference.
func (c *RedisConsumer) recordProcessed(eventType, mailClass string) {
	if c.cfg.Metrics == nil {
		return
	}
	c.cfg.Metrics.LogEventsTotal.WithLabelValues(eventType, mailClass).Inc()
}

func (c *RedisConsumer) recordDropped(reason string) {
	if c.cfg.Metrics == nil {
		return
	}
	c.cfg.Metrics.LogEventsDropped.WithLabelValues(reason).Inc()
}

func (c *RedisConsumer) observeDuration(eventType string, start time.Time) {
	if c.cfg.Metrics == nil {
		return
	}
	c.cfg.Metrics.LogEventDuration.WithLabelValues(eventType).Observe(time.Since(start).Seconds())
}

func (c *RedisConsumer) toDLQ(ctx context.Context, entry redisStreamEntry, reason string) {
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]any{
			"original_id": entry.id,
			"reason":      reason,
			"raw":         entry.raw,
			"ts":          time.Now().Unix(),
		},
	}).Err(); err != nil {
		log.Printf("logstream: dlq xadd failed id=%s: %v", entry.id, err)
		return
	}
	if _, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.XAck(ctx, c.cfg.Stream, c.cfg.Group, entry.id)
		p.XDel(ctx, c.cfg.Stream, entry.id)
		return nil
	}); err != nil {
		log.Printf("logstream: dlq ack/del failed id=%s: %v", entry.id, err)
		return
	}
	c.stats.DLQ.Add(1)
	if c.cfg.Metrics != nil {
		c.cfg.Metrics.LogEventsDropped.WithLabelValues("deadletter").Inc()
	}
	log.Printf("logstream: sent to DLQ id=%s reason=%s", entry.id, reason)
}

func (c *RedisConsumer) handleEvent(ctx context.Context, lr *RedisLogRecord, raw string) error {
	if err := c.cfg.Persister.InsertLog(ctx, lr, raw); err != nil {
		return fmt.Errorf("insert log: %w", err)
	}
	if lr.Type != "Feedback" || lr.FeedbackReport == nil {
		return nil
	}
	if err := c.cfg.Persister.InsertFeedback(ctx, lr, raw); err != nil {
		log.Printf("logstream: feedback insert failed: %v", err)
		// Non-fatal: the LogEvent row already captured the raw payload.
	}
	if err := c.cfg.Persister.AutoSuppress(ctx, lr); err != nil {
		log.Printf("logstream: auto-suppress failed: %v", err)
	}
	return nil
}

// redisStreamEntry is the parsed work item for a worker. Either record is
// valid (parErr nil) or parErr explains why the entry is unusable.
type redisStreamEntry struct {
	id     string
	raw    string
	record RedisLogRecord
	parErr error
}

func toRedisEntry(msg redis.XMessage) redisStreamEntry {
	e := redisStreamEntry{id: msg.ID}
	dataField, ok := msg.Values["data"].(string)
	if !ok {
		e.parErr = errors.New("missing or non-string 'data' field")
		return e
	}
	e.raw = dataField
	if err := json.Unmarshal([]byte(dataField), &e.record); err != nil {
		e.parErr = fmt.Errorf("json decode: %w", err)
	}
	return e
}
