package dsnstream

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig configures the DSN-stream consumer. Most fields have
// sensible defaults via DefaultRedisConfig — callers typically only set
// RedisURL, VerpSecret, and Persister.
type RedisConfig struct {
	RedisURL string

	// Stream is the kumomta XADD target. Default: "kumo.dsns".
	Stream string
	// Group is the consumer group name; constant across replicas.
	Group string
	// Consumer is this replica's identity inside the group.
	Consumer string
	// DLQStream is where unparseable entries go. Default: "<Stream>.dlq".
	DLQStream string

	// VerpSecret is the HMAC key used to validate inbound VERP tokens.
	// May be empty when the operator is running in shared-bounce mode
	// (rare); the consumer still parses + persists, just without
	// VERP-driven correlation.
	VerpSecret string

	WorkerCount   int
	BatchSize     int64
	BlockTimeout  time.Duration
	ClaimIdle     time.Duration
	ClaimInterval time.Duration
	HandleTimeout time.Duration

	Persister Persister
}

// Persister is the database-side contract. Returning a non-nil error
// from Insert sends the entry to the DLQ rather than ack'ing it.
type Persister interface {
	Insert(ctx context.Context, parsed *Parsed) error
}

// DefaultRedisConfig returns production defaults. Caller must set
// RedisURL, VerpSecret (when VERP is on), and Persister.
func DefaultRedisConfig() RedisConfig {
	host, _ := os.Hostname()
	return RedisConfig{
		Stream:        "kumo.dsns",
		Group:         "iris-dsn",
		Consumer:      fmt.Sprintf("%s-%d", host, os.Getpid()),
		DLQStream:     "kumo.dsns.dlq",
		WorkerCount:   2,
		BatchSize:     50,
		BlockTimeout:  5 * time.Second,
		ClaimIdle:     2 * time.Minute,
		ClaimInterval: 30 * time.Second,
		HandleTimeout: 30 * time.Second,
	}
}

// RedisConsumer reads kumomta-XADDed DSN bodies, parses them, and forwards
// each to a Persister. Lifecycle mirrors pkg/logstream.RedisConsumer:
// NewRedisConsumer → Start (blocks) → Close.
type RedisConsumer struct {
	cfg    RedisConfig
	rdb    *redis.Client
	workCh chan redisStreamEntry
}

func NewRedisConsumer(cfg RedisConfig) (*RedisConsumer, error) {
	if cfg.RedisURL == "" {
		return nil, errors.New("dsnstream: RedisURL required")
	}
	if cfg.Persister == nil {
		return nil, errors.New("dsnstream: Persister required")
	}
	if cfg.Stream == "" {
		cfg.Stream = "kumo.dsns"
	}
	if cfg.Group == "" {
		cfg.Group = "iris-dsn"
	}
	if cfg.Consumer == "" {
		host, _ := os.Hostname()
		cfg.Consumer = fmt.Sprintf("%s-%d", host, os.Getpid())
	}
	if cfg.DLQStream == "" {
		cfg.DLQStream = cfg.Stream + ".dlq"
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 2
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
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
		return nil, fmt.Errorf("dsnstream: parse redis url: %w", err)
	}
	return &RedisConsumer{
		cfg:    cfg,
		rdb:    redis.NewClient(opts),
		workCh: make(chan redisStreamEntry, cfg.BatchSize),
	}, nil
}

// Close releases the redis connection pool.
func (c *RedisConsumer) Close() error { return c.rdb.Close() }

// Start blocks until ctx is cancelled.
func (c *RedisConsumer) Start(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("dsnstream: redis ping: %w", err)
	}
	if err := c.ensureGroup(ctx); err != nil {
		return fmt.Errorf("dsnstream: ensure group: %w", err)
	}
	log.Printf("dsnstream: starting consumer stream=%s group=%s consumer=%s workers=%d",
		c.cfg.Stream, c.cfg.Group, c.cfg.Consumer, c.cfg.WorkerCount)

	var wg sync.WaitGroup
	for i := 0; i < c.cfg.WorkerCount; i++ {
		wg.Add(1)
		go c.worker(ctx, &wg)
	}
	wg.Add(1)
	go c.claimLoop(ctx, &wg)

	c.readLoop(ctx)

	close(c.workCh)
	wg.Wait()
	log.Printf("dsnstream: consumer stopped")
	return nil
}

func (c *RedisConsumer) ensureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.cfg.Stream, c.cfg.Group, "$").Err()
	if err == nil || strings.Contains(err.Error(), "BUSYGROUP") {
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
			if strings.Contains(err.Error(), "NOGROUP") {
				if rerr := c.ensureGroup(ctx); rerr != nil {
					log.Printf("dsnstream: re-create group after NOGROUP: %v", rerr)
				}
				continue
			}
			log.Printf("dsnstream: xreadgroup failed: %v", err)
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
				case c.workCh <- toEntry(msg):
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
				log.Printf("dsnstream: xautoclaim failed: %v", err)
				continue
			}
			startID = next
			for _, msg := range msgs {
				select {
				case c.workCh <- toEntry(msg):
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

func (c *RedisConsumer) processEntry(ctx context.Context, e redisStreamEntry) {
	wctx, cancel := context.WithTimeout(ctx, c.cfg.HandleTimeout)
	defer cancel()

	if e.parErr != nil {
		c.toDLQ(wctx, e, "stream_decode: "+e.parErr.Error())
		return
	}

	parsed, err := Parse(e.rcpt, c.cfg.VerpSecret, e.raw)
	if err != nil {
		c.toDLQ(wctx, e, "parse: "+err.Error())
		return
	}
	parsed.ReceivedAt = time.Now().UTC()

	if err := c.cfg.Persister.Insert(wctx, parsed); err != nil {
		log.Printf("dsnstream: persister failed id=%s: %v", e.id, err)
		// Don't DLQ on persister error — that's typically a transient
		// DB issue and the entry will be re-claimed and retried.
		return
	}

	if _, err := c.rdb.TxPipelined(wctx, func(p redis.Pipeliner) error {
		p.XAck(wctx, c.cfg.Stream, c.cfg.Group, e.id)
		p.XDel(wctx, c.cfg.Stream, e.id)
		return nil
	}); err != nil {
		log.Printf("dsnstream: ack/del failed id=%s: %v", e.id, err)
	}
}

func (c *RedisConsumer) toDLQ(ctx context.Context, e redisStreamEntry, reason string) {
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.cfg.DLQStream,
		MaxLen: 5000,
		Approx: true,
		Values: map[string]any{
			"original_id": e.id,
			"reason":      reason,
			"rcpt":        e.rcpt,
			"raw":         string(e.raw),
			"ts":          time.Now().Unix(),
		},
	}).Err(); err != nil {
		log.Printf("dsnstream: dlq xadd failed id=%s: %v", e.id, err)
		return
	}
	if _, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.XAck(ctx, c.cfg.Stream, c.cfg.Group, e.id)
		p.XDel(ctx, c.cfg.Stream, e.id)
		return nil
	}); err != nil {
		log.Printf("dsnstream: dlq ack/del failed id=%s: %v", e.id, err)
		return
	}
	log.Printf("dsnstream: sent to DLQ id=%s reason=%s", e.id, reason)
}

// redisStreamEntry is the parsed work item for a worker.
type redisStreamEntry struct {
	id     string
	rcpt   string
	raw    []byte
	parErr error
}

func toEntry(msg redis.XMessage) redisStreamEntry {
	e := redisStreamEntry{id: msg.ID}
	rcpt, _ := msg.Values["rcpt"].(string)
	e.rcpt = rcpt
	rawField, ok := msg.Values["raw"].(string)
	if !ok {
		e.parErr = errors.New("missing or non-string 'raw' field")
		return e
	}
	e.raw = []byte(rawField)
	return e
}
