// HTTPSServer is a TLS-terminating reverse proxy that fronts the
// existing plain :8000 admin server with operator-configured cert and
// key paths. Reading config from global_settings on Start means an
// operator can flip iris into HTTPS without a binary rebuild — only a
// container restart, which kratos's lifecycle handles cleanly.
//
// Why a reverse proxy instead of a second kratos.Server with the same
// routes? Reverse-proxying lets the existing plain HTTP server stay
// the single registration target for every handler, so adding a route
// doesn't require touching this file. The cost — one extra in-process
// hop — is negligible compared to TLS handshake setup on the hot path.
//
// Cert hot-reload: the `GetCertificate` callback re-reads the PEM
// pair on every handshake. tls.Config caches certs internally per
// session, so a renewal that rewrites the file in place is picked up
// on the next new connection without a process restart.
//
// Listen-address / cert-path hot-reload: Start registers an OnChange
// observer with GlobalSettingsService. When an operator saves new
// https_listen / cert / key values, the observer fires Reload, which
// re-reads the settings and re-binds the socket — no process restart.
// Reload is idempotent (a no-op when the effective config is
// unchanged) so edits to unrelated fields don't blip the listener.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type HTTPSServer struct {
	gsvc *service.GlobalSettingsService

	mu   sync.Mutex
	srv  *http.Server
	done chan struct{}
	// Effective config of the live listener, used to skip needless
	// re-binds when an unrelated global_settings field changes.
	curBind string
	curCert string
	curKey  string
}

// NewHTTPSServer is wired by kratos. It just stashes the service
// pointer; bind / cert paths are resolved lazily on Start so a
// global_settings update doesn't require process recompilation.
func NewHTTPSServer(gsvc *service.GlobalSettingsService) *HTTPSServer {
	return &HTTPSServer{gsvc: gsvc}
}

// Start binds the listener from the current global_settings and
// registers the reload observer so later settings updates re-apply
// live. If https_listen is empty or the cert/key files don't exist,
// the listener stays down (the observer can still bring it up later).
func (s *HTTPSServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gsvc.OnChange(s.Reload)
	s.applyLocked()
	return nil
}

// Reload re-reads global_settings and re-binds the listener to match.
// Fired on a detached goroutine by GlobalSettingsService.OnChange, so
// it must not assume the caller's request context — it uses
// context.Background() throughout.
func (s *HTTPSServer) Reload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyLocked()
}

// applyLocked (re)configures the listener from the current settings.
// Caller must hold s.mu. Idempotent: when the desired bind/cert/key
// match what's already serving it returns without touching the socket.
func (s *HTTPSServer) applyLocked() {
	row, err := s.gsvc.Get(context.Background())
	if err != nil {
		log.Printf("https-server: read global_settings failed (%v) — listener unchanged", err)
		return
	}
	bind := strings.TrimSpace(row.HTTPSListen)
	cert := strings.TrimSpace(row.HTTPSCertPemPath)
	key := strings.TrimSpace(row.HTTPSKeyPemPath)

	// Unchanged effective config → leave the running listener alone.
	if s.srv != nil && bind == s.curBind && cert == s.curCert && key == s.curKey {
		return
	}

	// Tear down whatever is currently bound before re-binding. Doing
	// this even when the new config is "disabled" means clearing
	// https_listen actually stops the listener.
	s.stopLocked()

	if bind == "" || cert == "" || key == "" {
		log.Printf("https-server: not configured (https_listen / cert / key empty) — disabled")
		return
	}
	upstream, err := url.Parse(httpsUpstreamURL())
	if err != nil {
		log.Printf("https-server: parse upstream %q: %v — disabled", httpsUpstreamURL(), err)
		return
	}

	// Probe-load once to fail loudly if the operator pointed at
	// non-existent paths; subsequent loads happen per-handshake via
	// GetCertificate.
	if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
		log.Printf("https-server: initial LoadX509KeyPair(%s, %s): %v — disabled", cert, key, err)
		return
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			c, err := tls.LoadX509KeyPair(cert, key)
			if err != nil {
				return nil, fmt.Errorf("https-server: reload cert: %w", err)
			}
			return &c, nil
		},
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	// Preserve the public hostname the client used so backend
	// middleware and audit logs see the real Host header.
	original := proxy.Director
	proxy.Director = func(r *http.Request) {
		original(r)
		r.Header.Set("X-Forwarded-Proto", "https")
		if r.Header.Get("X-Forwarded-For") == "" {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				r.Header.Set("X-Forwarded-For", host)
			}
		}
	}

	srv := &http.Server{
		Addr:              bind,
		Handler:           proxy,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		log.Printf("https-server: bind %s failed: %v — disabled", bind, err)
		return
	}
	tlsLn := tls.NewListener(ln, tlsCfg)
	log.Printf("https-server: listening on %s (cert=%s) → %s", bind, cert, upstream)
	done := make(chan struct{})
	s.srv, s.done = srv, done
	s.curBind, s.curCert, s.curKey = bind, cert, key
	go func() {
		defer close(done)
		if err := srv.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("https-server: serve exited: %v", err)
		}
	}()
}

// stopLocked drains in-flight requests and closes the current
// listener, resetting the effective-config tracking. Caller must hold
// s.mu. Safe to call when nothing is running.
func (s *HTTPSServer) stopLocked() {
	if s.srv == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("https-server: shutdown: %v", err)
	}
	if s.done != nil {
		<-s.done
	}
	s.srv, s.done = nil, nil
	s.curBind, s.curCert, s.curKey = "", "", ""
}

// Stop drains in-flight requests and closes the listener (kratos
// lifecycle teardown).
func (s *HTTPSServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	return nil
}

// httpsUpstreamURL returns the in-process upstream — the existing
// plain HTTP server. Reading the kratos rest.addr config file just to
// reverse-proxy to it would couple this file to bootstrap internals;
// instead we hardcode the well-known dev/prod default and let the
// operator override via env if they ever change it.
func httpsUpstreamURL() string {
	if v := strings.TrimSpace(os.Getenv("IRIS_HTTPS_UPSTREAM")); v != "" {
		return v
	}
	return "http://127.0.0.1:8000"
}
