package data

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/menta2k/iris/backend/internal/agentapi"
	"github.com/menta2k/iris/backend/internal/biz"
)

// agentTransport manages a remote KumoMTA node through its iris-agent over an
// mTLS HTTP channel: stage (checksummed bundle) then activate (reload/restart).
type agentTransport struct {
	nodeName string
	baseURL  string
	client   *http.Client
}

var _ nodeTransport = (*agentTransport)(nil)

func newAgentTransport(node *biz.MTANode, client *http.Client) *agentTransport {
	return &agentTransport{
		nodeName: node.Name,
		baseURL:  strings.TrimRight(node.AgentURL, "/"),
		client:   client,
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// bundleFor packages a rendered config for the agent wire protocol.
func bundleFor(rendered biz.RenderedConfig, generation int64, nodeName string) agentapi.ConfigBundle {
	b := agentapi.ConfigBundle{
		Generation: generation,
		NodeName:   nodeName,
		Policy: agentapi.File{
			Name:    "iris_generated.lua",
			Content: rendered.Content,
			SHA256:  sha256Hex(rendered.Content),
		},
		Checksum:     rendered.Checksum,
		InitChecksum: rendered.InitChecksum,
	}
	for name, body := range shapingFiles(rendered) {
		b.Shaping = append(b.Shaping, agentapi.File{
			Name:    name,
			Content: body,
			SHA256:  sha256Hex(body),
		})
	}
	return b
}

// applyConfig stages the bundle on the agent and activates it.
func (t *agentTransport) applyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool, generation int64) (string, error) {
	var stage agentapi.StageReply
	if err := t.post(ctx, agentapi.PathStage, bundleFor(rendered, generation, t.nodeName), &stage); err != nil {
		return "", err
	}
	if !stage.Staged || stage.Checksum != rendered.Checksum {
		return "", biz.Unavailable("MTA_NODE_STAGE_MISMATCH", "node %s staged checksum %q does not match %q", t.nodeName, stage.Checksum, rendered.Checksum)
	}
	var act agentapi.ActivateReply
	if err := t.post(ctx, agentapi.PathActivate, agentapi.ActivateRequest{Checksum: rendered.Checksum, Restart: restart}, &act); err != nil {
		return "", err
	}
	return act.Action, nil
}

// reload activates the currently staged/active config epoch via the agent.
func (t *agentTransport) reload(ctx context.Context) error {
	var act agentapi.ActivateReply
	return t.post(ctx, agentapi.PathActivate, agentapi.ActivateRequest{}, &act)
}

// status maps the agent health report onto a kumod status.
func (t *agentTransport) status(ctx context.Context) biz.KumoStatus {
	h, err := t.health(ctx)
	if err != nil {
		return biz.KumoStatus{State: "unreachable"}
	}
	if h.Kumo == "" {
		return biz.KumoStatus{State: "unknown"}
	}
	return biz.KumoStatus{State: h.Kumo}
}

// adminAvailable: the agent always proxies its localhost kumod listener.
func (t *agentTransport) adminAvailable() bool { return true }

// kumodURL maps a kumod path onto the agent's authenticated reverse proxy.
func (t *agentTransport) kumodURL(path string) string {
	return t.baseURL + strings.TrimRight(agentapi.PathKumodPrefix, "/") + path
}

// adminGET fetches a kumod admin/metrics path through the agent proxy.
func (t *agentTransport) adminGET(ctx context.Context, path string) ([]byte, error) {
	return kumodGET(ctx, t.client, t.kumodURL(path), path)
}

// adminJSON sends a JSON admin request through the agent proxy.
func (t *agentTransport) adminJSON(ctx context.Context, method, path string, payload any) error {
	return kumodJSON(ctx, t.client, method, t.kumodURL(path), path, payload)
}

// inject posts a built message to the node's kumod injection API via the agent.
func (t *agentTransport) inject(ctx context.Context, body []byte) error {
	return kumodInject(ctx, t.client, t.kumodURL("/api/inject/v1"), body)
}

// nodeIPs fetches the node's assignable IP addresses from its agent.
func (t *agentTransport) nodeIPs(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+agentapi.PathIPs, nil)
	if err != nil {
		return nil, biz.Internal(err, "build agent ips request")
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, biz.Unavailable("MTA_NODE_AGENT_UNREACHABLE", "node %s agent unreachable: %v", t.nodeName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, biz.Unavailable("MTA_NODE_AGENT_FAILED", "node %s agent ips returned status %d", t.nodeName, resp.StatusCode)
	}
	var out agentapi.NodeIPs
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, biz.Internal(err, "decode agent ips")
	}
	return out.IPs, nil
}

// health fetches the agent heartbeat (version, applied checksum, kumod state).
func (t *agentTransport) health(ctx context.Context) (*agentapi.Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+agentapi.PathHealth, nil)
	if err != nil {
		return nil, biz.Internal(err, "build agent health request")
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, biz.Unavailable("MTA_NODE_AGENT_UNREACHABLE", "node %s agent unreachable: %v", t.nodeName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, biz.Unavailable("MTA_NODE_AGENT_FAILED", "node %s agent health returned status %d", t.nodeName, resp.StatusCode)
	}
	var h agentapi.Health
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&h); err != nil {
		return nil, biz.Internal(err, "decode agent health")
	}
	return &h, nil
}

// post sends a JSON request to the agent and decodes the JSON reply, mapping
// agent error envelopes onto domain errors.
func (t *agentTransport) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return biz.Internal(err, "encode agent request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return biz.Internal(err, "build agent request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return biz.Unavailable("MTA_NODE_AGENT_UNREACHABLE", "node %s agent unreachable: %v", t.nodeName, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return biz.Internal(err, "read agent response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e agentapi.Error
		if json.Unmarshal(raw, &e) == nil && e.Code != "" {
			return biz.Unavailable("MTA_NODE_AGENT_FAILED", "node %s agent: %s (%s)", t.nodeName, e.Message, e.Code)
		}
		return biz.Unavailable("MTA_NODE_AGENT_FAILED", "node %s agent %s returned status %d", t.nodeName, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return biz.Internal(fmt.Errorf("%w: %s", err, truncate(string(raw), 200)), "decode agent response")
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
