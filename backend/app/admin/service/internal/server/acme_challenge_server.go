// AcmeChallengeServer hosts the public-facing :80 endpoint that ACME
// CAs hit during HTTP-01 validation. It serves only
// /.well-known/acme-challenge/<token> (delegated to the shared
// TokenStore the issuer fills); everything else is 404.
//
// Disabled when IRIS_ACME_HTTP_BIND is unset OR set to "off". Default
// bind is :80 — the iris docker container runs as root in dev compose
// so this works out of the box; in production behind a reverse proxy
// the operator should set IRIS_ACME_HTTP_BIND=off and forward
// /.well-known/acme-challenge/* from their proxy to admin-service:8000
// instead. (admin-service serves the same path on :8000 too.)
package server

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/pkg/acmeissuer"
)

// AcmeChallengeServer satisfies the kratos transport.Server contract.
type AcmeChallengeServer struct {
	tokens *acmeissuer.TokenStore
	srv    *http.Server
	bind   string
	done   chan struct{}
}

// NewAcmeChallengeServer reads IRIS_ACME_HTTP_BIND and constructs the
// listener. nil store / "off" bind = disabled (Start/Stop become
// no-ops, matching the LogstreamServer / DsnstreamServer pattern).
func NewAcmeChallengeServer(tokens *acmeissuer.TokenStore) (*AcmeChallengeServer, error) {
	bind := strings.TrimSpace(os.Getenv("IRIS_ACME_HTTP_BIND"))
	if bind == "" {
		bind = ":80"
	}
	if strings.EqualFold(bind, "off") || strings.EqualFold(bind, "disabled") {
		log.Printf("acme-challenge: IRIS_ACME_HTTP_BIND=off — :80 challenge listener disabled (front with a proxy that forwards /.well-known/acme-challenge/* to admin-service)")
		return &AcmeChallengeServer{}, nil
	}
	if tokens == nil {
		return nil, errors.New("acme-challenge: nil TokenStore")
	}
	return &AcmeChallengeServer{tokens: tokens, bind: bind}, nil
}

// Start binds and runs the listener until Stop cancels it. Returns
// nil on bind failure (with a log line) so a misconfigured ACME
// listener never brings down the whole admin-service — the rest of
// the binary stays usable and the operator gets a fixable error
// message in the logs.
func (s *AcmeChallengeServer) Start(ctx context.Context) error {
	if s.tokens == nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/acme-challenge/", s.tokens.ServeHTTP)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	s.srv = &http.Server{
		Addr:              s.bind,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	ln, err := net.Listen("tcp", s.bind)
	if err != nil {
		log.Printf("acme-challenge: bind %s failed: %v — disabling", s.bind, err)
		s.srv = nil
		return nil
	}
	log.Printf("acme-challenge: listening on %s", s.bind)
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("acme-challenge: serve exited: %v", err)
		}
	}()
	return nil
}

// Stop drains in-flight requests and closes the listener.
func (s *AcmeChallengeServer) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("acme-challenge: shutdown: %v", err)
	}
	if s.done != nil {
		<-s.done
	}
	return nil
}
