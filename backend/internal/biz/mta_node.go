package biz

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// MTA node status values. Semantics mirror VMTA statuses: an active node
// participates fully; a draining node stops receiving new work but finishes
// what it has; a disabled node is ignored by rendering and control fan-out.
const (
	MTANodeStatusActive   = "active"
	MTANodeStatusDisabled = "disabled"
	MTANodeStatusDraining = "draining"
)

// MTANode is one KumoMTA host in the cluster registry.
//
// A node with an empty AgentURL is the legacy co-located instance managed via
// the local file/reload transport; a node with an AgentURL is managed through
// the mTLS-authenticated iris-agent running on that host. ProxyHost/ProxyPort
// point at the node's kumo-proxy on the private cluster network and are used
// when rendering egress sources for VMTAs that live on this node.
type MTANode struct {
	ID       string
	Name     string
	AgentURL string
	// ProxyHost must be a private-range IP: kumo-proxy is unauthenticated, so
	// iris refuses to render egress sources pointing at a public address.
	ProxyHost string
	ProxyPort int
	Status    string
	// CertFingerprint is the SHA-256 fingerprint of the agent's enrolled client
	// certificate; empty until enrollment completes.
	CertFingerprint string

	// Reported by the agent; read-only for operators.
	Version         string
	AppliedChecksum string
	LastSeenAt      *time.Time

	Notes string
}

// ValidMTANodeStatus reports whether status is a known node status.
func ValidMTANodeStatus(status string) bool {
	switch status {
	case MTANodeStatusActive, MTANodeStatusDisabled, MTANodeStatusDraining:
		return true
	default:
		return false
	}
}

// cgnatRange is 100.64.0.0/10 (RFC 6598), commonly used by WireGuard/Tailscale
// meshes; netip.Addr.IsPrivate does not cover it.
var cgnatRange = netip.MustParsePrefix("100.64.0.0/10")

// privateProxyAddr reports whether ip is acceptable as a kumo-proxy endpoint:
// loopback, link-local, RFC1918/ULA private, or CGNAT (overlay-mesh) space.
func privateProxyAddr(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() ||
		cgnatRange.Contains(ip.Unmap())
}

// Validate checks MTANode invariants before persistence.
func (n *MTANode) Validate() error {
	n.Name = strings.TrimSpace(n.Name)
	n.AgentURL = strings.TrimSpace(n.AgentURL)
	n.ProxyHost = strings.TrimSpace(n.ProxyHost)
	if n.Status == "" {
		n.Status = MTANodeStatusActive
	}

	if n.Name == "" {
		return Invalid("MTA_NODE_NAME_REQUIRED", "node name is required")
	}
	if len(n.Name) > 128 {
		return Invalid("MTA_NODE_NAME_TOO_LONG", "node name must be at most 128 characters")
	}
	if !dnsNameRe.MatchString(n.Name) {
		return Invalid("MTA_NODE_NAME_INVALID", "node name %q must be a DNS-safe label (it becomes the node identity in logs)", n.Name)
	}
	if n.AgentURL != "" {
		u, err := url.Parse(n.AgentURL)
		if err != nil || u.Host == "" {
			return Invalid("MTA_NODE_AGENT_URL_INVALID", "agent_url %q is not a valid URL", n.AgentURL)
		}
		// The agent channel carries config bundles including DKIM keys; only
		// mutually-authenticated HTTPS is acceptable.
		if u.Scheme != "https" {
			return Invalid("MTA_NODE_AGENT_URL_SCHEME", "agent_url must use https (the agent channel is mTLS-only)")
		}
		if u.Path != "" && u.Path != "/" {
			return Invalid("MTA_NODE_AGENT_URL_PATH", "agent_url must not include a path")
		}
	}
	if (n.ProxyHost == "") != (n.ProxyPort == 0) {
		return Invalid("MTA_NODE_PROXY_PARTIAL", "proxy_host and proxy_port must be set together")
	}
	if n.ProxyHost != "" {
		ip, err := netip.ParseAddr(n.ProxyHost)
		if err != nil {
			return Invalid("MTA_NODE_PROXY_HOST_INVALID", "proxy_host %q is not a valid IP address", n.ProxyHost)
		}
		if ip.IsUnspecified() || ip.IsMulticast() {
			return Invalid("MTA_NODE_PROXY_HOST_INVALID", "proxy_host must be a concrete unicast IP")
		}
		if !privateProxyAddr(ip) {
			return Invalid("MTA_NODE_PROXY_HOST_PUBLIC", "proxy_host %q is a public address; kumo-proxy is unauthenticated and must only be reachable on a private cluster network", n.ProxyHost)
		}
		if n.ProxyPort < 1 || n.ProxyPort > 65535 {
			return Invalid("MTA_NODE_PROXY_PORT_RANGE", "proxy_port must be between 1 and 65535")
		}
	}
	if !ValidMTANodeStatus(n.Status) {
		return Invalid("MTA_NODE_STATUS_INVALID", "status %q is not valid", n.Status)
	}
	if len(n.Notes) > 2000 {
		return Invalid("MTA_NODE_NOTES_TOO_LONG", "notes must be at most 2000 characters")
	}
	return nil
}

// ProxyEndpoint returns "host:port" for the node's kumo-proxy, or "" when the
// node exposes no proxy.
func (n *MTANode) ProxyEndpoint() string {
	if n.ProxyHost == "" {
		return ""
	}
	return net.JoinHostPort(n.ProxyHost, itoa(n.ProxyPort))
}

// Local reports whether the node is managed through the local file/reload
// transport (the legacy co-located deployment) rather than a remote agent.
func (n *MTANode) Local() bool { return n.AgentURL == "" }

// MTANodeEnrollToken is a single-use bootstrap token binding an agent
// enrollment to a node row. The plaintext token is only available at issuance;
// only its bcrypt hash is persisted.
type MTANodeEnrollToken struct {
	ID        string
	NodeID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedBy string
	CreatedAt time.Time
}

// Expired reports whether the token is past its expiry at the given time.
func (t *MTANodeEnrollToken) Expired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}

// MTANodeRepo is the persistence boundary for the cluster node registry.
type MTANodeRepo interface {
	ListNodes(ctx context.Context) ([]*MTANode, error)
	GetNode(ctx context.Context, id string) (*MTANode, error)
	CreateNode(ctx context.Context, n *MTANode) (*MTANode, error)
	UpdateNode(ctx context.Context, n *MTANode) (*MTANode, error)
	DeleteNode(ctx context.Context, id string) error
	// SetNodeCertFingerprint pins the enrolled agent certificate.
	SetNodeCertFingerprint(ctx context.Context, id, fingerprint string) error
	// RecordNodeHeartbeat stores agent-reported state (version, applied config
	// checksum) and bumps last_seen_at.
	RecordNodeHeartbeat(ctx context.Context, id, version, appliedChecksum string) error

	// Enrollment tokens (single-use, bcrypt-hashed).
	CreateEnrollToken(ctx context.Context, t *MTANodeEnrollToken) (*MTANodeEnrollToken, error)
	// OpenEnrollTokens returns unused, unexpired tokens for the node.
	OpenEnrollTokens(ctx context.Context, nodeID string) ([]*MTANodeEnrollToken, error)
	// ConsumeEnrollToken atomically marks the token used; it fails if the token
	// was already used or is expired.
	ConsumeEnrollToken(ctx context.Context, id string) error
}
