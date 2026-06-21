//go:build !embed_ui

package webui

import "testing"

// TestDisabledWithoutTag documents the default: the SPA is not embedded unless
// the binary is built with -tags embed_ui, so dev/CI builds need no frontend.
func TestDisabledWithoutTag(t *testing.T) {
	if Enabled() {
		t.Fatal("webui must be disabled without the embed_ui build tag")
	}
	if Handler() != nil {
		t.Fatal("Handler must be nil when the SPA is not embedded")
	}
}
