package spa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With only .gitkeep in dist/, the embed has no index.html so the
// handler must serve the placeholder page. Verifies both the failure
// signal (text) and the contract (200 status — health checks unaffected).
func TestHandlerPlaceholderWhenNoIndex(t *testing.T) {
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Frontend not embedded") {
		t.Fatalf("placeholder page not served: %q", rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("placeholder must be no-store; got %q", rr.Header().Get("Cache-Control"))
	}
}

func TestHashedAssetPattern(t *testing.T) {
	// Hashed (immutable) — every Vite/Vben emitter produces this shape.
	hashed := []string{
		"jse/index-index-DdlNZRo5.js",
		"assets/app-AbC123Xy.css",
		"css/index-Ux2-pq.css",
		"fonts/inter-1234567.woff2",
	}
	for _, p := range hashed {
		if !hashedAsset.MatchString(p) {
			t.Errorf("expected hashed match: %q", p)
		}
	}
	// Not hashed (must re-fetch on every deploy).
	plain := []string{
		"index.html",
		"_app.config.js",
		"favicon.ico",
		"robots.txt",
		"manifest.webmanifest",
	}
	for _, p := range plain {
		if hashedAsset.MatchString(p) {
			t.Errorf("expected plain (no match): %q", p)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	cases := map[string]string{
		`a & b`:    `a &amp; b`,
		`<script>`: `&lt;script&gt;`,
		`"x"`:      `&quot;x&quot;`,
	}
	for in, want := range cases {
		if got := escapeHTML(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}
