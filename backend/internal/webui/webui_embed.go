//go:build embed_ui

package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// content holds the built frontend. The dist directory is produced by the
// frontend build and copied here before `go build -tags embed_ui` (see the
// Makefile `build-embed` target / backend.Dockerfile). all: includes hashed
// asset files.
//
//go:embed all:dist
var content embed.FS

// Enabled reports whether the SPA is embedded in this build.
func Enabled() bool { return true }

// Handler serves the embedded SPA: real files are served directly, and any
// other (non-API) path falls back to index.html so client-side routing works.
func Handler() http.Handler {
	dist, err := fs.Sub(content, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Unmatched API paths must 404 as API errors, not return the SPA.
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean != "" && clean != "index.html" {
			if _, err := fs.Stat(dist, clean); err == nil {
				// Vite asset filenames are content-hashed, so cache them hard.
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback for client-side routes (e.g. /outbound/vmtas).
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
