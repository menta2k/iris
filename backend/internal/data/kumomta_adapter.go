package data

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
)

// ClusterNodes is the subset of the node registry the adapter needs to fan
// configuration out across the cluster. Satisfied by MTANodeRepo.
type ClusterNodes interface {
	ListNodes(ctx context.Context) ([]*biz.MTANode, error)
	RecordNodeHeartbeat(ctx context.Context, id, version, appliedChecksum string) error
}

// FileKumoMTA is the production KumoMTA adapter. It manages the co-located
// node through the filesystem + reload command/URL (local transport) and, when
// a cluster registry is attached, remote nodes through their mTLS iris-agents.
// With no registered nodes it behaves exactly as the single-node adapter
// always has. It is the production-facing implementation of
// biz.KumoMTAAdapter; the in-memory stub in the biz package is used for local
// development and tests.
type FileKumoMTA struct {
	cfg    conf.External
	client *http.Client
	local  *localTransport

	// Cluster wiring; nil until AttachCluster is called.
	nodes       ClusterNodes
	agentClient *http.Client
	// manageNodePrelude controls whether ApplyConfig writes the local node's
	// identity prelude (iris_node.lua). True on the iris control plane; the
	// agent disables it because it writes the prelude from the bundle's
	// NodeName instead.
	manageNodePrelude bool
	// injectRR distributes HTTP injections across active nodes round-robin.
	injectRR atomic.Uint32
}

// NewFileKumoMTA constructs a file/exec/HTTP-based KumoMTA adapter.
func NewFileKumoMTA(cfg conf.External) *FileKumoMTA {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	return &FileKumoMTA{
		cfg:               cfg,
		client:            client,
		local:             &localTransport{cfg: cfg, client: client},
		manageNodePrelude: true,
	}
}

// DisableNodePrelude turns off local prelude management; used by the agent,
// which writes the prelude from the bundle's NodeName instead.
func (k *FileKumoMTA) DisableNodePrelude() { k.manageNodePrelude = false }

// writeNodePreludeFile writes the per-node identity prelude next to the policy
// (0644: only the node name, no secrets).
func writeNodePreludeFile(configPath, nodeName string) error {
	if configPath == "" {
		return biz.FailedPrecondition("KUMO_CONFIG_PATH_UNSET", "kumomta config_path is not configured")
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return biz.Internal(err, "create config directory")
	}
	path := filepath.Join(dir, biz.NodePreludeFile)
	if err := os.WriteFile(path, []byte(biz.NodePreludeContent(nodeName)), 0o644); err != nil {
		return biz.Internal(err, "write node prelude")
	}
	return nil
}

// AttachCluster enables cluster-aware config distribution: nodes lists the
// registry and agentClient is the mTLS HTTP client used to reach remote
// iris-agents.
func (k *FileKumoMTA) AttachCluster(nodes ClusterNodes, agentClient *http.Client) {
	k.nodes = nodes
	k.agentClient = agentClient
}

var _ biz.KumoMTAAdapter = (*FileKumoMTA)(nil)

// applyTarget is one node the adapter applies configuration to.
type applyTarget struct {
	name      string
	nodeID    string
	transport nodeTransport
}

// applyTargets resolves the set of nodes to manage. With no registry or an
// empty registry it is the single local node (pre-cluster behavior). Disabled
// nodes are skipped; draining nodes still receive configuration.
func (k *FileKumoMTA) applyTargets(ctx context.Context) ([]applyTarget, error) {
	if k.nodes == nil {
		return []applyTarget{{name: "local", transport: k.local}}, nil
	}
	nodes, err := k.nodes.ListNodes(ctx)
	if err != nil {
		return nil, biz.Internal(err, "list cluster nodes")
	}
	var out []applyTarget
	for _, n := range nodes {
		if n.Status == biz.MTANodeStatusDisabled {
			continue
		}
		if n.Local() {
			out = append(out, applyTarget{name: n.Name, nodeID: n.ID, transport: k.local})
			continue
		}
		if k.agentClient == nil {
			return nil, biz.FailedPrecondition("CLUSTER_TLS_UNCONFIGURED",
				"node %s has an agent_url but cluster TLS (cluster.ca_cert/client_cert/client_key) is not configured", n.Name)
		}
		out = append(out, applyTarget{name: n.Name, nodeID: n.ID, transport: newAgentTransport(n, k.agentClient)})
	}
	if len(out) == 0 {
		return []applyTarget{{name: "local", transport: k.local}}, nil
	}
	return out, nil
}

// Status reports the KumoMTA service state. With a cluster registry it is the
// worst state across all non-disabled nodes; otherwise the local node's state.
func (k *FileKumoMTA) Status(ctx context.Context) (biz.KumoStatus, error) {
	targets, err := k.applyTargets(ctx)
	if err != nil {
		return biz.KumoStatus{State: "unknown"}, nil
	}
	worst := biz.KumoStatus{State: "running"}
	rank := map[string]int{"running": 0, "unknown": 1, "degraded": 2, "unreachable": 3}
	var degradedNodes []string
	for _, t := range targets {
		st := t.transport.status(ctx)
		if rank[st.State] > rank[worst.State] {
			worst = st
		}
		if st.State != "running" {
			degradedNodes = append(degradedNodes, fmt.Sprintf("%s=%s", t.name, st.State))
		}
	}
	if len(targets) > 1 && len(degradedNodes) > 0 {
		worst.Detail = strings.Join(degradedNodes, ", ")
	}
	return worst, nil
}

// ApplyServiceControl reloads or restarts KumoMTA on every managed node using
// each node's transport. Stop/start are not performed remotely and are
// reported as unsupported so an operator performs them deliberately.
func (k *FileKumoMTA) ApplyServiceControl(ctx context.Context, op biz.ServiceOperation) (string, error) {
	switch op {
	case biz.ServiceReload, biz.ServiceRestart, biz.ServiceStart:
		targets, err := k.applyTargets(ctx)
		if err != nil {
			return "", err
		}
		var parts []string
		for _, t := range targets {
			if err := t.transport.reload(ctx); err != nil {
				return strings.Join(parts, "; "), fmt.Errorf("node %s: %w", t.name, err)
			}
			parts = append(parts, t.name+": reload triggered")
		}
		if len(targets) == 1 {
			return "reload triggered", nil
		}
		return strings.Join(parts, "; "), nil
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

// ApplyConfig distributes the rendered policy to every managed node and
// activates it (rolling: one node at a time, halting on the first failure so
// remaining nodes keep the previous config). When restart is true (init-block
// change) each node restarts kumod, since a reload does not re-run
// kumo.on('init'); otherwise it reloads.
func (k *FileKumoMTA) ApplyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool) (string, string, error) {
	targets, err := k.applyTargets(ctx)
	if err != nil {
		return "", "", err
	}
	// Wall-clock generation: strictly increasing across applies, used by agents
	// for replay protection.
	generation := time.Now().UnixNano()

	var applied []string
	for _, t := range targets {
		// Local nodes get their identity prelude written here (remote nodes get
		// it from the bundle's NodeName via their agent). It sits next to the
		// policy but outside its checksum, keeping the policy identical
		// cluster-wide while log records carry this node's name.
		if t.transport == k.local && k.manageNodePrelude {
			if err := writeNodePreludeFile(k.cfg.ConfigPath, t.name); err != nil {
				return k.cfg.ConfigPath, "", err
			}
		}
		action, err := t.transport.applyConfig(ctx, rendered, restart, generation)
		if err != nil {
			if len(applied) > 0 {
				return k.cfg.ConfigPath, "", fmt.Errorf("node %s failed (rollout halted; already applied: %s): %w",
					t.name, strings.Join(applied, ", "), err)
			}
			return k.cfg.ConfigPath, "", fmt.Errorf("node %s: %w", t.name, err)
		}
		applied = append(applied, fmt.Sprintf("%s %s", t.name, action))
		if t.nodeID != "" && k.nodes != nil {
			// Best effort: reflect the applied checksum in the registry so the UI
			// can flag drift; the agent heartbeat keeps it fresh afterwards.
			if err := k.nodes.RecordNodeHeartbeat(ctx, t.nodeID, "", rendered.Checksum); err != nil {
				biz.LoggerFrom(ctx).Error("record node heartbeat failed", "node", t.name, "error", err.Error())
			}
		}
	}

	summary := fmt.Sprintf("applied to %s (%d sources, %d pools, %d routes, %d dkim, %d suppressions)",
		strings.Join(applied, ", "), rendered.VMTACount, rendered.PoolCount, rendered.RouteCount, rendered.DKIMCount, rendered.SuppressionCount)
	return k.cfg.ConfigPath, summary, nil
}
