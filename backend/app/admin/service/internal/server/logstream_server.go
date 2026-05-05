// LogstreamServer adapts a *logstream.RedisConsumer into a kratos
// transport.Server so it joins the same Start/Stop lifecycle as the gRPC and
// HTTP servers. When IRIS_LOGSTREAM_REDIS_URL is unset the constructor
// returns a no-op server so the binary still boots — Redis-backed logging is
// optional.
package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/menta2k/iris/backend/pkg/logstream"
	"github.com/menta2k/iris/backend/pkg/metrics"
)

// LogstreamServer satisfies the kratos transport.Server interface. The
// internal consumer (if any) is started in Start and gracefully drained in
// Stop. nil consumer = disabled, used when the env vars aren't set.
type LogstreamServer struct {
	consumer *logstream.RedisConsumer
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewLogstreamServer reads env vars and constructs the consumer. Missing
// IRIS_LOGSTREAM_REDIS_URL is not an error — the server is created in a
// disabled state so callers don't have to special-case the boot graph.
//
// `m` is optional. When non-nil it's threaded into the consumer's
// RedisConfig so per-event-type counters and the duration histogram
// populate on the live data path; nil disables instrumentation
// without touching the consumer logic.
func NewLogstreamServer(persister logstream.Persister, m *metrics.Metrics) (*LogstreamServer, error) {
	url := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_REDIS_URL"))
	if url == "" {
		log.Printf("logstream: IRIS_LOGSTREAM_REDIS_URL unset — Redis log stream disabled")
		return &LogstreamServer{}, nil
	}
	cfg := logstream.DefaultRedisConfig()
	cfg.RedisURL = url
	cfg.Persister = persister
	cfg.Metrics = m
	if v := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_NAME")); v != "" {
		cfg.Stream = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_GROUP")); v != "" {
		cfg.Group = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_CONSUMER")); v != "" {
		cfg.Consumer = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WorkerCount = n
		}
	}
	c, err := logstream.NewRedisConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("logstream: %w", err)
	}
	return &LogstreamServer{consumer: c}, nil
}

// Start runs the consumer until Stop cancels its context. It returns nil
// even if the consumer's read loop fails — the kratos lifecycle treats a
// non-nil error from Start as a fatal init failure, but a Redis blip should
// not bring down the whole admin-service.
func (s *LogstreamServer) Start(ctx context.Context) error {
	if s.consumer == nil {
		return nil
	}
	cctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.consumer.Start(cctx); err != nil && cctx.Err() == nil {
			log.Printf("logstream: consumer exited with error: %v", err)
		}
	}()
	return nil
}

// Stop cancels the consumer context and waits up to Stop's context deadline
// for the read loop to drain.
func (s *LogstreamServer) Stop(ctx context.Context) error {
	if s.consumer == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.done == nil {
		return s.consumer.Close()
	}
	select {
	case <-s.done:
	case <-ctx.Done():
	}
	return s.consumer.Close()
}
