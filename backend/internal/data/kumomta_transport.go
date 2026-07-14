package data

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// nodeTransport applies configuration to and reports the state of one KumoMTA
// node, hiding whether the node is local (file write + reload command/URL) or
// remote (mTLS iris-agent).
type nodeTransport interface {
	// applyConfig installs the rendered bundle and activates it (reload, or
	// restart when the init block changed). generation is a monotonically
	// increasing apply counter used for replay protection on remote nodes.
	applyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool, generation int64) (action string, err error)
	// reload triggers a config-epoch reload without rewriting files.
	reload(ctx context.Context) error
	// status reports kumod liveness on the node.
	status(ctx context.Context) biz.KumoStatus

	// Admin channel to the node's kumod HTTP listener (metrics scrape, queue
	// suspend/bounce, HTTP injection). Local nodes talk to it directly; remote
	// nodes go through the agent's authenticated /v1/kumod reverse proxy.
	// adminAvailable reports whether the channel is configured at all.
	adminAvailable() bool
	adminGET(ctx context.Context, path string) ([]byte, error)
	adminJSON(ctx context.Context, method, path string, payload any) error
	inject(ctx context.Context, body []byte) error
}

// localTransport manages the co-located KumoMTA through the filesystem and the
// configured reload/restart command or admin URL (the pre-cluster behavior).
type localTransport struct {
	cfg    conf.External
	client *http.Client
}

// chgrpConfig sets the group of a written config file to group, so kumod can
// read the 0640 policy when it runs as a different user than the writer. A
// no-op when group is empty. Fails loudly on a bad group or an insufficient
// caller — a silent skip would just reproduce the "kumod can't read its
// policy" outage as a mysterious degraded state.
func chgrpConfig(path, group string) error {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return biz.Invalid("KUMO_CONFIG_GROUP_UNKNOWN", "kumomta.config_group %q not found: %v", group, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return biz.Internal(err, "parse gid for group %q", group)
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return biz.Internal(err, "chgrp %s to %q (the writer must own the file and belong to the group, or be root)", filepath.Base(path), group)
	}
	return nil
}

// writeTLSFile installs one listener TLS cert/key file at its absolute path.
// Written 0640 (a key file may be secret) and chgrp'd to the config group so
// kumod can read it when it runs as a different user than the writer — the same
// treatment the policy gets. A no-op when content is empty (reference only).
// The path is re-validated (absolute, no shell metacharacters) so a bundle can
// only ever land a cert file at a listener-shaped path.
func writeTLSFile(f biz.TLSFile, group string) error {
	if strings.TrimSpace(f.Content) == "" {
		return nil
	}
	if !biz.ValidTLSFilePath(f.Path) {
		return biz.Invalid("KUMO_TLS_PATH_INVALID", "listener TLS file path %q is not an absolute, metacharacter-free path", f.Path)
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return biz.Internal(err, "create TLS file directory for %s", f.Path)
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(f.Content), 0o640); err != nil {
		return biz.Internal(err, "write TLS file %s", f.Path)
	}
	if err := chgrpConfig(tmp, group); err != nil {
		return err
	}
	if err := os.Rename(tmp, f.Path); err != nil {
		return biz.Internal(err, "install TLS file %s", f.Path)
	}
	return nil
}

var _ nodeTransport = (*localTransport)(nil)

// shapingFiles maps the sidecar file names to their rendered content.
func shapingFiles(rendered biz.RenderedConfig) map[string]string {
	return map[string]string{
		"iris-base.toml":       rendered.ShapingBase,
		"iris-warmup.toml":     rendered.ShapingWarmup,
		"iris-automation.toml": rendered.ShapingAutomation,
	}
}

// applyConfig writes the rendered policy to the configured path (atomically)
// and activates it.
func (t *localTransport) applyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool, _ int64) (string, error) {
	path := t.cfg.ConfigPath
	if path == "" {
		return "", biz.FailedPrecondition("KUMO_CONFIG_PATH_UNSET", "kumomta config_path is not configured")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", biz.Internal(err, "create config directory")
	}
	// Write the shaping sidecar files (loaded by the policy via kumo.shaping.load)
	// BEFORE the policy, so they exist when kumod reloads/loads the policy. They
	// are world-readable (0644): they hold only per-provider/per-IP rate limits
	// (no secrets) and must be readable by the kumod/tsa-daemon user even when
	// iris runs as a different user. The policy itself stays 0640 below because it
	// embeds DKIM private keys.
	for name, body := range shapingFiles(rendered) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return "", biz.Internal(err, "write shaping %s", name)
		}
		if err := chgrpConfig(p, t.cfg.ConfigGroup); err != nil {
			return "", err
		}
	}
	// Write atomically: write to a temp file in the same dir then rename. The
	// 0640 policy embeds DKIM keys, so when kumod runs as a different user than
	// the writer (e.g. a root agent + kumod --user iris), config_group must name
	// the group kumod runs as, or kumod cannot read its own policy.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(rendered.Content), 0o640); err != nil {
		return "", biz.Internal(err, "write config")
	}
	if err := chgrpConfig(tmp, t.cfg.ConfigGroup); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", biz.Internal(err, "install config")
	}

	// Listener TLS cert/key files carried in the bundle. Written to their
	// absolute paths (identical on every node) so a centrally-issued cert
	// reaches this node. On the co-located control-plane host this rewrites the
	// same bytes it was read from (a no-op); on a remote node it materializes
	// the shipped cert. Entries with empty content are references only (the node
	// provides the file itself) and are skipped.
	for _, f := range rendered.TLSFiles {
		if err := writeTLSFile(f, t.cfg.ConfigGroup); err != nil {
			return "", err
		}
	}

	action := "reloaded"
	if restart {
		restarted, err := t.restart(ctx)
		if err != nil {
			return "", err
		}
		if restarted {
			action = "restarted"
		} else {
			// No restart mechanism configured: reload as a best effort, but make
			// clear a manual restart is still required for the init change.
			if err := t.reload(ctx); err != nil {
				return "", err
			}
			action = "reloaded — MANUAL RESTART REQUIRED for init changes (listeners/spool/log hook)"
		}
	} else if err := t.reload(ctx); err != nil {
		return "", err
	}
	return action, nil
}

// restart restarts KumoMTA via the configured restart command or HTTP endpoint.
// It returns (false, nil) when no restart mechanism is configured so the caller
// can fall back.
func (t *localTransport) restart(ctx context.Context) (bool, error) {
	if cmd := strings.TrimSpace(t.cfg.RestartCommand); cmd != "" {
		// #nosec G204 -- restart command is operator-configured, not user input.
		c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
		out, err := c.CombinedOutput()
		if err != nil {
			return false, biz.Internal(fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))), "kumomta restart command failed")
		}
		return true, nil
	}
	if url := strings.TrimSpace(t.cfg.RestartURL); url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
		if err != nil {
			return false, biz.Internal(err, "build restart request")
		}
		resp, err := t.client.Do(req)
		if err != nil {
			return false, biz.Unavailable("KUMO_RESTART_UNREACHABLE", "kumomta restart endpoint unreachable: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return false, biz.Unavailable("KUMO_RESTART_FAILED", "kumomta restart returned status %d", resp.StatusCode)
		}
		return true, nil
	}
	return false, nil
}

// reload triggers a KumoMTA reload via the configured command or HTTP endpoint.
func (t *localTransport) reload(ctx context.Context) error {
	if cmd := strings.TrimSpace(t.cfg.ReloadCommand); cmd != "" {
		// #nosec G204 -- reload command is operator-configured, not user input.
		c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
		out, err := c.CombinedOutput()
		if err != nil {
			return biz.Internal(fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))), "kumomta reload command failed")
		}
		return nil
	}
	if url := strings.TrimSpace(t.cfg.ReloadURL); url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
		if err != nil {
			return biz.Internal(err, "build reload request")
		}
		resp, err := t.client.Do(req)
		if err != nil {
			return biz.Unavailable("KUMO_RELOAD_UNREACHABLE", "kumomta reload endpoint unreachable: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return biz.Unavailable("KUMO_RELOAD_FAILED", "kumomta reload returned status %d", resp.StatusCode)
		}
		return nil
	}
	// No reload mechanism configured: the config was written and KumoMTA is
	// expected to pick it up on its own config epoch. Treat as success.
	return nil
}

// adminAvailable reports whether a kumod admin base URL is configured.
func (t *localTransport) adminAvailable() bool {
	return strings.TrimSpace(t.cfg.BaseURL) != ""
}

func (t *localTransport) adminURL(path string) string {
	return strings.TrimRight(t.cfg.BaseURL, "/") + path
}

// adminGET fetches a kumod admin/metrics path directly on the local listener.
func (t *localTransport) adminGET(ctx context.Context, path string) ([]byte, error) {
	return kumodGET(ctx, t.client, t.adminURL(path), path)
}

// adminJSON sends a JSON admin request directly to the local kumod listener.
func (t *localTransport) adminJSON(ctx context.Context, method, path string, payload any) error {
	return kumodJSON(ctx, t.client, method, t.adminURL(path), path, payload)
}

// inject posts a built message to the local kumod HTTP injection API.
func (t *localTransport) inject(ctx context.Context, body []byte) error {
	return kumodInject(ctx, t.client, t.adminURL("/api/inject/v1"), body)
}

// status reports kumod liveness via the admin base URL, or unknown when none is
// configured.
func (t *localTransport) status(ctx context.Context) biz.KumoStatus {
	if t.cfg.BaseURL == "" {
		return biz.KumoStatus{State: "unknown"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.cfg.BaseURL, "/")+"/api/check-liveness/v1", nil)
	if err != nil {
		return biz.KumoStatus{State: "unknown"}
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return biz.KumoStatus{State: "unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return biz.KumoStatus{State: "running"}
	}
	return biz.KumoStatus{State: "degraded"}
}
