// DsnstreamServer adapts a *dsnstream.RedisConsumer into a kratos
// transport.Server, mirroring LogstreamServer. Disabled (no-op) when
// IRIS_BOUNCE_DOMAIN is unset — operators opt in by configuring a bounce
// domain. The Redis stream the consumer reads from is shared with the
// log-stream pipeline; the dsnstream consumer is gated by BounceDomain
// rather than by a separate URL because there's no scenario in which
// you'd run iris with one but not the other.
package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/menta2k/iris/backend/pkg/dsnstream"
)

// DsnstreamServer satisfies kratos transport.Server. nil consumer = disabled.
type DsnstreamServer struct {
	consumer *dsnstream.RedisConsumer
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewDsnstreamServer reads env vars and constructs the consumer. Disabled
// when either IRIS_BOUNCE_DOMAIN or IRIS_LOGSTREAM_REDIS_URL is unset
// (the latter because the kumomta side puts DSN bodies on the same
// Redis instance).
func NewDsnstreamServer(persister dsnstream.Persister) (*DsnstreamServer, error) {
	if strings.TrimSpace(os.Getenv("IRIS_BOUNCE_DOMAIN")) == "" {
		log.Printf("dsnstream: IRIS_BOUNCE_DOMAIN unset — DSN consumer disabled")
		return &DsnstreamServer{}, nil
	}
	url := strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_REDIS_URL"))
	if url == "" {
		log.Printf("dsnstream: IRIS_LOGSTREAM_REDIS_URL unset — DSN consumer disabled")
		return &DsnstreamServer{}, nil
	}
	cfg := dsnstream.DefaultRedisConfig()
	cfg.RedisURL = url
	cfg.Persister = persister
	cfg.VerpSecret = strings.TrimSpace(os.Getenv("IRIS_VERP_SECRET"))
	if v := strings.TrimSpace(os.Getenv("IRIS_DSNSTREAM_NAME")); v != "" {
		cfg.Stream = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_DSNSTREAM_GROUP")); v != "" {
		cfg.Group = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_DSNSTREAM_CONSUMER")); v != "" {
		cfg.Consumer = v
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_DSNSTREAM_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WorkerCount = n
		}
	}
	c, err := dsnstream.NewRedisConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("dsnstream: %w", err)
	}
	return &DsnstreamServer{consumer: c}, nil
}

func (s *DsnstreamServer) Start(ctx context.Context) error {
	if s.consumer == nil {
		return nil
	}
	cctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.consumer.Start(cctx); err != nil && cctx.Err() == nil {
			log.Printf("dsnstream: consumer exited with error: %v", err)
		}
	}()
	return nil
}

func (s *DsnstreamServer) Stop(ctx context.Context) error {
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
