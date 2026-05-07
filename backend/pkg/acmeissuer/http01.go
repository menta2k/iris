package acmeissuer

import (
	"net/http"
	"strings"
	"sync"

	"github.com/go-acme/lego/v4/challenge"
)

// TokenStore holds the in-flight HTTP-01 tokens that the iris admin
// service serves on :80 under /.well-known/acme-challenge/<token>.
// Lego's HTTP-01 flow calls Present(token, keyAuth) before challenge
// validation and CleanUp(token) after — we map both onto an in-memory
// keyed-by-token map.
//
// One instance is shared between the issuer (which writes) and the
// public :80 listener (which reads). It's safe across goroutines.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string // token → keyAuth
}

func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: map[string]string{}}
}

// Present is the lego challenge.Provider hook. domain is unused — the
// HTTP-01 spec keys solely on token at the URL level. We accept
// duplicate Present calls for the same token (lego occasionally
// re-presents on retry) — the second write is a no-op against the
// same value.
func (s *TokenStore) Present(domain, token, keyAuth string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = keyAuth
	return nil
}

// CleanUp removes a token after the CA has validated it.
func (s *TokenStore) CleanUp(domain, token, keyAuth string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

// ServeHTTP serves the /.well-known/acme-challenge/<token> path. Any
// other request gets a 404 — this listener is single-purpose. The
// admin-service registers this on the :80 listener (see
// http01_server.go) and operators front it with whatever proxy /
// firewall they want.
func (s *TokenStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/.well-known/acme-challenge/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, prefix)
	if token == "" {
		http.NotFound(w, r)
		return
	}
	s.mu.RLock()
	keyAuth, ok := s.tokens[token]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(keyAuth))
}

// Compile-time check that the store satisfies lego's challenge.Provider.
var _ challenge.Provider = (*TokenStore)(nil)
