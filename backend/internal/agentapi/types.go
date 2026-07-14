// Package agentapi defines the wire types shared between iris (control plane)
// and the iris-agent running next to each KumoMTA node. The channel is mTLS:
// iris authenticates with a client certificate issued by the cluster CA, the
// agent serves with a certificate from the same CA.
package agentapi

// Paths served by the agent.
const (
	PathStage    = "/v1/config/stage"
	PathActivate = "/v1/config/activate"
	PathHealth   = "/v1/health"
	// PathKumodPrefix reverse-proxies to the node's localhost-bound kumod HTTP
	// listener (admin API + metrics); the kumod path follows the prefix.
	PathKumodPrefix = "/v1/kumod/"
)

// File is one config file in a bundle, integrity-checked by its SHA-256.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
}

// ConfigBundle carries a rendered KumoMTA configuration to a node.
type ConfigBundle struct {
	// Generation is a monotonically increasing apply counter; the agent rejects
	// bundles whose generation is not greater than the last activated one
	// (replay protection).
	Generation int64 `json:"generation"`
	// Policy is the generated Lua policy (written 0640: embeds DKIM keys).
	Policy File `json:"policy"`
	// Shaping are the sidecar TOML files (written 0644 next to the policy).
	Shaping []File `json:"shaping"`
	// NodeName is the receiving node's registry name; the agent writes it into
	// the per-node identity prelude (iris_node.lua) so log records carry a
	// 'node' meta. It is intentionally OUTSIDE the checksummed policy so the
	// policy stays byte-identical cluster-wide.
	NodeName string `json:"node_name"`
	// Checksum identifies the bundle (the rendered policy checksum).
	Checksum string `json:"checksum"`
	// InitChecksum covers the init block; a change requires restart, not reload.
	InitChecksum string `json:"init_checksum"`
}

// StageReply acknowledges a verified, staged bundle.
type StageReply struct {
	Staged   bool   `json:"staged"`
	Checksum string `json:"checksum"`
}

// ActivateRequest activates a previously staged bundle.
type ActivateRequest struct {
	// Checksum must match the staged bundle.
	Checksum string `json:"checksum"`
	// Restart activates with a service restart instead of a reload (init-block
	// changes are only picked up by kumo.on('init') at startup).
	Restart bool `json:"restart"`
}

// ActivateReply reports the activation outcome.
type ActivateReply struct {
	// Action is "reloaded", "restarted", or a warning that a manual restart is
	// still required (mirrors the local transport's behavior).
	Action          string `json:"action"`
	AppliedChecksum string `json:"applied_checksum"`
}

// Health is the agent heartbeat/health report.
type Health struct {
	// Version is the agent's build version.
	Version string `json:"version"`
	// AppliedChecksum is the checksum of the currently active bundle.
	AppliedChecksum string `json:"applied_checksum"`
	// Generation is the last activated bundle generation.
	Generation int64 `json:"generation"`
	// Kumo is the kumod liveness state: running | degraded | unreachable | unknown.
	Kumo string `json:"kumo"`
}

// Error is the agent's JSON error envelope.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
