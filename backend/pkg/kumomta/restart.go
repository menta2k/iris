package kumomta

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CmdRestarter restarts kumod by running a configured command, typically
// "sudo systemctl try-restart kumomta.service" on a host-native install.
//
// KumoMTA evaluates its init handler — listeners, relay_hosts, spool, the
// log hook — only once at process start. An epoch reload re-evaluates the
// per-message callbacks but never re-binds listeners, so changes to those
// init-level settings only take effect after a full restart.
type CmdRestarter struct {
	argv []string
}

// NewCmdRestarter parses a command line into argv. A blank command yields
// (nil, false) so callers can treat "not configured" (fall back to reload)
// distinctly from a real value. Splitting is whitespace-based; the expected
// commands ("sudo systemctl try-restart kumomta.service") contain no quoted
// arguments.
func NewCmdRestarter(command string) (*CmdRestarter, bool) {
	argv := strings.Fields(strings.TrimSpace(command))
	if len(argv) == 0 {
		return nil, false
	}
	return &CmdRestarter{argv: argv}, true
}

// Restart runs the configured command. On failure the combined output is
// included so the operator sees why (e.g. a missing sudoers grant).
func (r *CmdRestarter) Restart(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.argv[0], r.argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kumomta restart %v: %w: %s", r.argv, err, strings.TrimSpace(string(out)))
	}
	return nil
}
