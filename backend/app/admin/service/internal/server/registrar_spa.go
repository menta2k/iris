// SPA registration. We expose two surfaces so the *same* compiled
// frontend bundle works in dev and in prod:
//
//   - /api/v1/*  →  rewritten to /v1/* and re-dispatched into the same
//                   kratos mux. The Vue request client uses /api as its
//                   baseURL (Vite proxy in dev, same-origin in prod).
//
//   - /            →  embedded SPA (assets + index.html fallback).
//
// Registration order is load-bearing: this file is called LAST from
// RegisterServices so the more specific /v1/* routes registered earlier
// win on direct hits.
package server

import (
	"net/http"
	"strings"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/menta2k/iris/backend/pkg/spa"
)

// registerSPA mounts the /api/v1 strip-rewrite and the catch-all SPA
// handler on the kratos HTTP server.
func registerSPA(hs *kratoshttp.Server) {
	// /api/v1/* → /v1/* re-dispatch. We *can't* simply register the
	// existing handlers under both prefixes because they're registered
	// individually across several files; the cleanest decoupling is to
	// rewrite the request and re-enter the server's own mux. The
	// rewritten path no longer matches /api/v1/, so this prefix handler
	// can't recurse into itself.
	hs.HandlePrefix("/api/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, "/api")
		r2.RequestURI = r2.URL.RequestURI()
		hs.ServeHTTP(w, r2)
	}))

	// Embedded SPA. Registered last; serves everything that didn't
	// match /v1, /api/v1, or any other registered handler.
	hs.HandlePrefix("/", spa.Handler())
}
