// Package spa serves the bundled Vue 3 frontend straight out of the
// admin-service binary. The static assets are embedded at compile time
// from `pkg/spa/dist/`, which the build pipeline populates by copying
// `frontend/apps/admin/dist/` after `pnpm build`.
//
// Two surfaces:
//
//   - GET /assets/<file>          → serve the file from the embed FS.
//                                   Cache-Control is fingerprint-aware:
//                                   Vite emits hashed filenames under
//                                   `assets/`, so those are immutable.
//                                   Everything else (index.html, robots,
//                                   etc.) is no-cache because they're
//                                   the entry points clients re-fetch.
//
//   - GET /<route>                → SPA fallback. If no asset matches we
//                                   serve index.html so client-side
//                                   routing works after a hard reload
//                                   ( /policy/editor, /queues, … ).
//
// API routes (/v1/*, /api/v1/*) are NOT served from here — the kratos
// HTTP server registers them earlier with HandleFunc / HandlePrefix and
// they win over this catch-all because the registration order matters.
//
// When the embed is empty (developer building without running
// `pnpm build` first) the handler serves a small "frontend not
// embedded" page rather than a confusing 404 — explains the fix.
package spa

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// distPath roots all embedded paths at the on-disk `dist/` subdir; the
// `all:` prefix in //go:embed pulls in dotfiles too (Vite emits a
// `.vite/` manifest the dev tooling sometimes references).
const distRoot = "dist"

// indexFile is the SPA entry point.
const indexFile = "index.html"

// Handler returns the http.Handler that serves the embedded SPA. Mount
// it on the HTTP server LAST, so /v1, /api/v1, healthz, etc. win.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, distRoot)
	if err != nil {
		// Should never happen for a static path; if the build copied
		// nothing in we surface that with a clear runtime message.
		return placeholderHandler(fmt.Sprintf("spa: embed sub %q: %v", distRoot, err))
	}
	if !hasIndex(sub) {
		return placeholderHandler(
			"frontend not embedded — run `pnpm --filter @vben/web-antd build` " +
				"and copy frontend/apps/admin/dist/ → backend/pkg/spa/dist/ before " +
				"`go build`. The Dockerfile does this automatically.",
		)
	}
	fileSrv := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only GET / HEAD make sense for static assets — anything else
		// reaching this fallback is almost certainly a misrouted API call.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = indexFile
		}

		// Probe the embed for the requested path. If it's missing OR
		// it's a directory, fall back to index.html — that's the SPA
		// contract: the Vue router resolves the route on the client.
		if isMissingOrDir(sub, clean) {
			serveIndex(w, r, sub)
			return
		}

		// Long-lived cache for fingerprinted assets. Vite emits hashed
		// filenames like `jse/index-index-DdlNZRo5.js` or
		// `css/app-AbC123Xy.css`; the `-<hash>.<ext>` suffix is the
		// signal. Everything else (index.html, _app.config.js,
		// favicon, robots.txt) gets no-cache so a redeploy doesn't
		// strand clients on stale shells.
		if hashedAsset.MatchString(clean) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileSrv.ServeHTTP(w, r)
	})
}

// hashedAsset matches `<name>-<hash>.<ext>` where <hash> is 6+ chars of
// base64url alphabet. Tight enough to skip plain `app.config.js` (which
// must be re-fetched on each deploy) and loose enough to catch every
// Vite output convention (assets/, jse/, css/, fonts/, …).
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{6,}\.[a-z0-9]+$`)

// hasIndex returns true iff dist/index.html exists in the embed. Used at
// startup so a placeholder page can be substituted when the build
// pipeline didn't copy the frontend in.
func hasIndex(sub fs.FS) bool {
	_, err := fs.Stat(sub, indexFile)
	return err == nil
}

// isMissingOrDir tells the SPA-fallback path "should I serve index.html
// instead of forwarding this to the file server?". A missing file or a
// directory both want index.html; a real file should serve normally.
func isMissingOrDir(sub fs.FS, p string) bool {
	info, err := fs.Stat(sub, p)
	if err != nil {
		// fs.ErrNotExist is the only expected error here. Anything else
		// (permission, etc.) we treat as missing for the SPA fallback;
		// the alternative would be a 500 on a benign route hit.
		return errors.Is(err, fs.ErrNotExist) || true
	}
	return info.IsDir()
}

// serveIndex reads and serves dist/index.html with a no-cache header.
// Hot-replacing the server doesn't update the embed mid-process; clients
// re-fetch the entry on every nav so they pick up new asset hashes.
func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	b, err := fs.ReadFile(sub, indexFile)
	if err != nil {
		http.Error(w, "index.html missing from embed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
	_ = r // signature parity; the file server form takes r too
}

// placeholderHandler returns a 200 HTML page with a clear message about
// what's wrong. Returns 200 (not 500) so health checks still pass — the
// admin API is fully functional, just the bundled UI isn't there.
func placeholderHandler(reason string) http.Handler {
	page := []byte(`<!doctype html>
<meta charset="utf-8">
<title>iris — frontend not embedded</title>
<style>body{font:14px/1.5 system-ui,sans-serif;max-width:720px;margin:4em auto;padding:0 1em;color:#333}code{background:#f4f4f4;padding:.1em .4em;border-radius:3px}</style>
<h1>Frontend not embedded</h1>
<p>` + escapeHTML(reason) + `</p>
<p>The admin API at <code>/v1/*</code> is unaffected. To embed the UI:</p>
<pre>cd frontend &amp;&amp; pnpm install &amp;&amp; pnpm --filter @vben/web-antd build
cp -r apps/admin/dist/* ../backend/pkg/spa/dist/
cd ../backend &amp;&amp; go build ./...</pre>
`)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(page)
	})
}

// escapeHTML is a minimal HTML escaper for the placeholder page. We
// avoid pulling html/template here just to render one error string.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}
