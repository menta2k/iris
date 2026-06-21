//go:build !embed_ui

// Package webui optionally serves the built frontend SPA from the backend
// binary. By default (no `embed_ui` build tag) it is disabled, so normal
// development and CI builds do not require a frontend build. Production images
// build the frontend and compile with `-tags embed_ui` to bake the SPA in.
package webui

import "net/http"

// Enabled reports whether the SPA is embedded in this build.
func Enabled() bool { return false }

// Handler returns the SPA handler, or nil when not embedded.
func Handler() http.Handler { return nil }
