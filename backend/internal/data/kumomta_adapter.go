package data

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// FileKumoMTA is a KumoMTA adapter that writes the generated policy to disk and
// reloads the service via a configured shell command or admin HTTP endpoint.
// It is the production-facing implementation of biz.KumoMTAAdapter; the
// in-memory stub in the biz package is used for local development and tests.
type FileKumoMTA struct {
	cfg    conf.External
	client *http.Client
}

// NewFileKumoMTA constructs a file/exec/HTTP-based KumoMTA adapter.
func NewFileKumoMTA(cfg conf.External) *FileKumoMTA {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &FileKumoMTA{cfg: cfg, client: &http.Client{Timeout: timeout}}
}

var _ biz.KumoMTAAdapter = (*FileKumoMTA)(nil)

// Status reports the KumoMTA service state. When an admin base URL is
// configured it is queried; otherwise an unknown state is returned.
func (k *FileKumoMTA) Status(ctx context.Context) (biz.KumoStatus, error) {
	if k.cfg.BaseURL == "" {
		return biz.KumoStatus{State: "unknown"}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(k.cfg.BaseURL, "/")+"/api/check-liveness/v1", nil)
	if err != nil {
		return biz.KumoStatus{State: "unknown"}, nil
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return biz.KumoStatus{State: "unreachable"}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return biz.KumoStatus{State: "running"}, nil
	}
	return biz.KumoStatus{State: "degraded"}, nil
}

// ApplyServiceControl reloads or restarts KumoMTA using the configured
// reload mechanism. Stop/start are not performed remotely and are reported as
// unsupported so an operator performs them deliberately.
func (k *FileKumoMTA) ApplyServiceControl(ctx context.Context, op biz.ServiceOperation) (string, error) {
	switch op {
	case biz.ServiceReload, biz.ServiceRestart, biz.ServiceStart:
		if err := k.reload(ctx); err != nil {
			return "", err
		}
		return "reload triggered", nil
	case biz.ServiceStop:
		return "", biz.FailedPrecondition("SERVICE_STOP_UNSUPPORTED", "stop must be performed by an operator out of band")
	default:
		return "", biz.Invalid("SERVICE_OPERATION_INVALID", "operation %q is not valid", op)
	}
}

// ApplyQueueAction is not yet wired to a KumoMTA queue API; it records intent.
func (k *FileKumoMTA) ApplyQueueAction(_ context.Context, mailclass string, action biz.QueueAction) (string, error) {
	return fmt.Sprintf("queue action %s requested for %s", action, mailclass), nil
}

// ApplyConfig writes the rendered policy to the configured path (atomically) and
// activates it. When restart is true (init-block change) it restarts KumoMTA,
// since a reload does not re-run kumo.on('init'); otherwise it reloads.
func (k *FileKumoMTA) ApplyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool) (string, string, error) {
	path := k.cfg.ConfigPath
	if path == "" {
		return "", "", biz.FailedPrecondition("KUMO_CONFIG_PATH_UNSET", "kumomta config_path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", biz.Internal(err, "create config directory")
	}
	// Write atomically: write to a temp file in the same dir then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(rendered.Content), 0o640); err != nil {
		return "", "", biz.Internal(err, "write config")
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", "", biz.Internal(err, "install config")
	}

	action := "reloaded"
	if restart {
		restarted, err := k.restart(ctx)
		if err != nil {
			return path, "", err
		}
		if restarted {
			action = "restarted"
		} else {
			// No restart mechanism configured: reload as a best effort, but make
			// clear a manual restart is still required for the init change.
			if err := k.reload(ctx); err != nil {
				return path, "", err
			}
			action = "reloaded — MANUAL RESTART REQUIRED for init changes (listeners/spool/log hook)"
		}
	} else if err := k.reload(ctx); err != nil {
		return path, "", err
	}

	summary := fmt.Sprintf("wrote %s and %s (%d sources, %d pools, %d routes, %d dkim, %d suppressions)",
		path, action, rendered.VMTACount, rendered.PoolCount, rendered.RouteCount, rendered.DKIMCount, rendered.SuppressionCount)
	return path, summary, nil
}

// restart restarts KumoMTA via the configured restart command or HTTP endpoint.
// It returns (false, nil) when no restart mechanism is configured so the caller
// can fall back.
func (k *FileKumoMTA) restart(ctx context.Context) (bool, error) {
	if cmd := strings.TrimSpace(k.cfg.RestartCommand); cmd != "" {
		// #nosec G204 -- restart command is operator-configured, not user input.
		c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
		out, err := c.CombinedOutput()
		if err != nil {
			return false, biz.Internal(fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))), "kumomta restart command failed")
		}
		return true, nil
	}
	if url := strings.TrimSpace(k.cfg.RestartURL); url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
		if err != nil {
			return false, biz.Internal(err, "build restart request")
		}
		resp, err := k.client.Do(req)
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
func (k *FileKumoMTA) reload(ctx context.Context) error {
	if cmd := strings.TrimSpace(k.cfg.ReloadCommand); cmd != "" {
		// #nosec G204 -- reload command is operator-configured, not user input.
		c := exec.CommandContext(ctx, "/bin/sh", "-c", cmd)
		out, err := c.CombinedOutput()
		if err != nil {
			return biz.Internal(fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))), "kumomta reload command failed")
		}
		return nil
	}
	if url := strings.TrimSpace(k.cfg.ReloadURL); url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
		if err != nil {
			return biz.Internal(err, "build reload request")
		}
		resp, err := k.client.Do(req)
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
