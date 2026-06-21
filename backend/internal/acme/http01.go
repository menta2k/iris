package acme

import (
	"net/http"
	"strings"
	"sync"

	"github.com/go-acme/lego/v4/challenge"
)

// TokenStore holds the in-flight HTTP-01 tokens served under
// /.well-known/acme-challenge/<token>. lego calls Present(token, keyAuth)
// before validation and CleanUp(token) after; both map onto an in-memory map.
// One instance is shared between the issuer (writer) and the challenge server
// (reader); it is safe across goroutines.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string // token → keyAuth
}

// NewTokenStore constructs an empty store.
func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: map[string]string{}}
}

// Present is the lego challenge.Provider hook (HTTP-01 keys on token only).
func (s *TokenStore) Present(_, token, keyAuth string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = keyAuth
	return nil
}

// CleanUp removes a token after the CA has validated it.
func (s *TokenStore) CleanUp(_, token, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
	return nil
}

// ServeHTTP serves /.well-known/acme-challenge/<token>; everything else 404s.
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

var _ challenge.Provider = (*TokenStore)(nil)
