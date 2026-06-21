// Command rspamdstub is a fake rspamd /checkv2 endpoint for the e2e suite. It
// runs in a container on the rig network; kumod (running the iris-generated
// policy with rspamd enabled) POSTs each inbound message to it. The verdict is
// driven by the recipient so tests are deterministic: a recipient containing
// "spam" is rejected, anything else passes.
//
// Stdlib-only so it cross-compiles to a static binary the harness mounts into a
// stock alpine image.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

func main() {
	addr := os.Getenv("RSPAMD_ADDR")
	if addr == "" {
		addr = "0.0.0.0:11334"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/checkv2", func(w http.ResponseWriter, r *http.Request) {
		rcpt := r.Header.Get("Rcpt")
		action, score := "no action", 0.5
		if strings.Contains(strings.ToLower(rcpt), "spam") {
			action, score = "reject", 15.0
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"action": action, "score": score})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_ = http.ListenAndServe(addr, mux)
}
