package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/menta2k/iris/backend/internal/conf"
)

// Stream names carried over Redis Streams. They mirror data-model.md.
const (
	StreamMailEvents      = "iris.mail.events"
	StreamBounceEvents    = "iris.bounce.events"
	StreamFeedbackEvents  = "iris.feedback.events"
	StreamWebhookDelivery = "iris.webhook.deliveries"
	StreamQueueCommands   = "iris.queue.commands"
	StreamRspamdResults   = "iris.rspamd.results"
	StreamServiceCommands = "iris.service.commands"
)

// Streams wraps a Redis client and exposes producer/consumer helpers for
// consumer-group based stream processing with explicit acknowledgement.
type Streams struct {
	Client   *redis.Client
	consumer string
}

// NewStreams connects to Redis and returns a Streams helper.
func NewStreams(ctx context.Context, c conf.Redis) (*Streams, func(), error) {
	client := redis.NewClient(&redis.Options{
		Addr:         c.Addr,
		Password:     c.Password,
		DB:           c.DB,
		DialTimeout:  orDefault(c.DialTimeout, 5*time.Second),
		ReadTimeout:  orDefault(c.ReadTimeout, 3*time.Second),
		WriteTimeout: orDefault(c.WriteTimeout, 3*time.Second),
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("ping redis: %w", err)
	}

	consumer := c.ConsumerName
	if consumer == "" {
		consumer = "iris-1"
	}
	s := &Streams{Client: client, consumer: consumer}
	cleanup := func() { _ = client.Close() }
	return s, cleanup, nil
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}

// Health checks Redis connectivity for readiness probes.
func (s *Streams) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.Client.Ping(ctx).Err()
}

// Publish appends a message to a stream and returns the generated ID.
func (s *Streams) Publish(ctx context.Context, stream string, values map[string]any) (string, error) {
	id, err := s.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd %s: %w", stream, err)
	}
	return id, nil
}

// EnsureGroup creates a consumer group for a stream if it does not yet exist,
// reading from the beginning of the stream.
func (s *Streams) EnsureGroup(ctx context.Context, stream, group string) error {
	err := s.Client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && !isBusyGroup(err) {
		return fmt.Errorf("create group %s/%s: %w", stream, group, err)
	}
	return nil
}

func isBusyGroup(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

// StreamMessage is a single message read from a stream.
type StreamMessage struct {
	Stream string
	ID     string
	Values map[string]any
}

// Consume blocks (up to block) reading new messages for the group, returning at
// most count messages. It returns nil with no error on timeout.
func (s *Streams) Consume(ctx context.Context, stream, group string, count int64, block time.Duration) ([]StreamMessage, error) {
	res, err := s.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: s.consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("xreadgroup %s/%s: %w", stream, group, err)
	}
	var out []StreamMessage
	for _, st := range res {
		for _, m := range st.Messages {
			out = append(out, StreamMessage{Stream: st.Stream, ID: m.ID, Values: m.Values})
		}
	}
	return out, nil
}

// Ack acknowledges processed messages.
func (s *Streams) Ack(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.Client.XAck(ctx, stream, group, ids...).Err()
}
