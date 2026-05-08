// DsnstreamServer adapts a *dsnstream.RedisConsumer into a kratos
// transport.Server, mirroring LogstreamServer. Disabled (no-op) when
// the bounce pipeline isn't configured — i.e. when neither
// IRIS_BOUNCE_DOMAIN / IRIS_BOUNCE_SENDER_DOMAINS env vars nor the
// matching fields on the global_settings DB row are set.
//
// Why both env AND DB: the renderer (pkg/kumopolicy) overlays the
// global_settings row on top of env values, so an operator who
// configures the bounce pipeline ONLY via the UI would correctly get
// the inbound catcher emitted into init.lua but — without this
// dual-source check — would have no consumer running. DSNs would pile
// up on the Redis stream with nothing draining them. Always check
// both sources here so the UI flow works end-to-end.
package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	"github.com/menta2k/iris/backend/pkg/dsnstream"
)

// DsnstreamServer satisfies kratos transport.Server. nil consumer = disabled.
type DsnstreamServer struct {
	consumer *dsnstream.RedisConsumer
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewDsnstreamServer constructs the consumer if the bounce pipeline is
// enabled (env OR DB) and the log-stream Redis URL is set. The
// GlobalSettingsService dependency is used only for the boot-time DB
// read; consult-on-toggle would require a service restart anyway since
// we don't hot-reload the consumer config.
func NewDsnstreamServer(persister dsnstream.Persister, gsvc *service.GlobalSettingsService) (*DsnstreamServer, error) {
	if !bouncePipelineEnabled(gsvc) {
		log.Printf("dsnstream: bounce domain unset (env + global_settings) — DSN consumer disabled")
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

// bouncePipelineEnabled returns true when *any* of the four bounce
// configuration knobs is set: env IRIS_BOUNCE_DOMAIN /
// IRIS_BOUNCE_SENDER_DOMAINS, or the matching fields on the
// global_settings DB row. Mirrors the predicate the renderer uses so
// boot-time consumer gating stays in sync with what the policy emits.
//
// A read failure on the DB row is logged but treated as "not
// configured" — fail-safe: better to leave the consumer disabled than
// drain DSNs we can't actually correlate or persist.
func bouncePipelineEnabled(gsvc *service.GlobalSettingsService) bool {
	if strings.TrimSpace(os.Getenv("IRIS_BOUNCE_DOMAIN")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("IRIS_BOUNCE_SENDER_DOMAINS")) != "" {
		return true
	}
	if gsvc == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row, err := gsvc.Get(ctx)
	if err != nil {
		log.Printf("dsnstream: read global_settings failed (%v) — treating as disabled", err)
		return false
	}
	if row == nil {
		return false
	}
	return strings.TrimSpace(row.BounceDomain) != "" || len(row.BounceSenderDomains) > 0
}
