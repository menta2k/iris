// GeoIPUpdaterServer keeps the login firewall's country database current.
// On boot it downloads the current month's DB-IP "IP to Country Lite" file
// if the local copy is missing or stale, then hot-swaps it into the shared
// geoip.Resolver — no restart needed. It then re-checks on a fixed interval
// so a long-running process picks up next month's release automatically.
//
// Everything is best-effort and off the boot path: a failed or slow download
// never blocks startup or login, it just leaves the existing (or no)
// database in place and REGION rules fail open until the next successful
// fetch.
//
// Operator knobs:
//
//   - IRIS_GEOIP_AUTO_UPDATE      (default true) — set false/0/off to disable.
//   - IRIS_GEOIP_UPDATE_INTERVAL  (default 24h)  — how often to re-check.
//   - IRIS_GEOIP_DOWNLOAD_URL     (default DB-IP) — URL template; %s = YYYY-MM.
//   - IRIS_GEOIP_DB_PATH          — where the .mmdb lives (shared with the resolver).
package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/menta2k/iris/backend/pkg/geoip"
)

const defaultGeoIPUpdateInterval = 24 * time.Hour

type GeoIPUpdaterServer struct {
	resolver *geoip.Resolver

	enabled     bool
	interval    time.Duration
	urlTemplate string
	client      *http.Client

	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
}

// NewGeoIPUpdaterServer reads the env knobs and returns the updater. A nil
// resolver (shouldn't happen in practice) disables the loop.
func NewGeoIPUpdaterServer(resolver *geoip.Resolver) *GeoIPUpdaterServer {
	s := &GeoIPUpdaterServer{
		resolver:    resolver,
		enabled:     boolEnvDefaultTrue("IRIS_GEOIP_AUTO_UPDATE"),
		interval:    durationEnv("IRIS_GEOIP_UPDATE_INTERVAL", defaultGeoIPUpdateInterval),
		urlTemplate: geoip.DefaultURLTemplate,
		client:      &http.Client{Timeout: 90 * time.Second},
	}
	if v := strings.TrimSpace(os.Getenv("IRIS_GEOIP_DOWNLOAD_URL")); v != "" {
		s.urlTemplate = v
	}
	if resolver == nil || resolver.Path() == "" {
		s.enabled = false
	}
	return s
}

// Start runs an immediate update check, then ticks every interval.
func (s *GeoIPUpdaterServer) Start(_ context.Context) error {
	if !s.enabled {
		log.Printf("geoip-updater: disabled (IRIS_GEOIP_AUTO_UPDATE=false or no DB path)")
		return nil
	}
	log.Printf("geoip-updater: starting interval=%s path=%s", s.interval, s.resolver.Path())
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.done = make(chan struct{})
		go s.loop(ctx)
	})
	return nil
}

// Stop cancels the loop and waits for the in-flight tick to drain.
func (s *GeoIPUpdaterServer) Stop(_ context.Context) error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	if s.done != nil {
		<-s.done
	}
	return nil
}

func (s *GeoIPUpdaterServer) loop(ctx context.Context) {
	defer close(s.done)
	s.tick(ctx) // boot check
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

func (s *GeoIPUpdaterServer) tick(ctx context.Context) {
	downloaded, err := geoip.EnsureCurrent(ctx, s.resolver.Path(), s.urlTemplate, s.client, time.Now())
	if err != nil {
		// Best-effort: keep using whatever DB (or none) is already loaded.
		log.Printf("geoip-updater: update check failed (keeping existing DB): %v", err)
		return
	}
	if !downloaded {
		return
	}
	if err := s.resolver.Reload(); err != nil {
		log.Printf("geoip-updater: downloaded new DB but reload failed: %v", err)
		return
	}
	log.Printf("geoip-updater: country DB updated at %s", s.resolver.Path())
}

// boolEnvDefaultTrue returns true unless the env value is an explicit
// falsey token. Mirrors boolEnv (acme_renewer_server.go) but defaults on.
func boolEnvDefaultTrue(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}
