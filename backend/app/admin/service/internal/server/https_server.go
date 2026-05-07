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
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type HTTPSServer struct {
	gsvc *service.GlobalSettingsService
	srv  *http.Server
	done chan struct{}
}

// NewHTTPSServer is wired by kratos. It just stashes the service
// pointer; bind / cert paths are resolved lazily on Start so a
// global_settings update doesn't require process recompilation.
func NewHTTPSServer(gsvc *service.GlobalSettingsService) *HTTPSServer {
	return &HTTPSServer{gsvc: gsvc}
}

// Start reads the current global_settings; if https_listen is empty
// or the cert/key files don't exist, the server is a no-op. The
// reverse-proxy upstream is hardcoded to the local plain HTTP server
// (server.yaml: server.rest.addr) — operators who change that addr
// also need to keep this in sync.
func (s *HTTPSServer) Start(ctx context.Context) error {
	row, err := s.gsvc.Get(context.Background())
	if err != nil {
		log.Printf("https-server: read global_settings failed (%v) — disabled", err)
		return nil
	}
	bind := strings.TrimSpace(row.HTTPSListen)
	cert := strings.TrimSpace(row.HTTPSCertPemPath)
	key := strings.TrimSpace(row.HTTPSKeyPemPath)
	if bind == "" || cert == "" || key == "" {
		log.Printf("https-server: not configured (https_listen / cert / key empty) — disabled")
		return nil
	}
	upstream, err := url.Parse(httpsUpstreamURL())
	if err != nil {
		log.Printf("https-server: parse upstream %q: %v — disabled", httpsUpstreamURL(), err)
		return nil
	}

	// Lazy cert loader. We probe-load once at Start to fail loudly if
	// the operator pointed at non-existent paths; subsequent loads
	// happen per-handshake via GetCertificate.
	if _, err := tls.LoadX509KeyPair(cert, key); err != nil {
		log.Printf("https-server: initial LoadX509KeyPair(%s, %s): %v — disabled", cert, key, err)
		return nil
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

	s.srv = &http.Server{
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
		s.srv = nil
		return nil
	}
	tlsLn := tls.NewListener(ln, tlsCfg)
	log.Printf("https-server: listening on %s (cert=%s) → %s", bind, cert, upstream)
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.srv.Serve(tlsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("https-server: serve exited: %v", err)
		}
	}()
	return nil
}

// Stop drains in-flight requests and closes the listener.
func (s *HTTPSServer) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("https-server: shutdown: %v", err)
	}
	if s.done != nil {
		<-s.done
	}
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
