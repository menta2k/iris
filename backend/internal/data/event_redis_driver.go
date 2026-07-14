package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/internal/biz"
)

// redisEventDriver appends each event to a Redis stream via XADD. Implements
// biz.EventDriver. Durable delivery: the stream persists until consumers ack.
type redisEventDriver struct {
	rdb    redis.UniversalClient
	stream string
	format string
}

// NewRedisEventDriverFactory returns the factory registered under the "redis"
// driver name. Config keys: stream (required); addr / password / db (optional —
// when addr is set the events go to that Redis, otherwise to iris's own Redis
// given here as defaultClient).
func NewRedisEventDriverFactory(defaultClient redis.UniversalClient) biz.EventDriverFactory {
	return func(p *biz.EventProcessor) (biz.EventDriver, error) {
		cfg := p.DriverConfig
		stream := strings.TrimSpace(cfg["stream"])
		if stream == "" {
			return nil, fmt.Errorf("redis driver: stream is required")
		}
		rdb := defaultClient
		if addr := strings.TrimSpace(cfg["addr"]); addr != "" {
			db, _ := strconv.Atoi(cfg["db"])
			rdb = redis.NewClient(&redis.Options{Addr: addr, Password: cfg["password"], DB: db})
		}
		if rdb == nil {
			return nil, fmt.Errorf("redis driver: no Redis client available")
		}
		return &redisEventDriver{rdb: rdb, stream: stream, format: cfg["format"]}, nil
	}
}

func (r *redisEventDriver) Deliver(ctx context.Context, events []biz.DispatchEvent) error {
	pipe := r.rdb.Pipeline()
	for _, ev := range events {
		payload, err := json.Marshal(biz.FormatEvent(r.format, ev))
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: r.stream,
			Values: map[string]any{
				"type":        ev.Type,
				"mailclass":   ev.Mailclass,
				"occurred_at": ev.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
				"payload":     string(payload),
			},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis xadd: %w", err)
	}
	return nil
}
