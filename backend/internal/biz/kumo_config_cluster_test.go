package biz

import (
	"strings"
	"testing"
)

func clusterSnap(nodes []*MTANode, vmtas []*VMTA) ConfigSnapshot {
	return ConfigSnapshot{
		Nodes: nodes,
		VMTAs: vmtas,
		DKIM:  []*DKIMDomain{{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}
}

// TestRenderClusterProxyEgressSources verifies the core cluster behavior: a
// VMTA owned by a node with a kumo-proxy renders a SOCKS5 proxy pointer (so
// any node can deliver through the owning node's IP), while local VMTAs keep
// their plain source_address. This is what makes "received on node1, egress
// from node2" work without re-queueing the message.
func TestRenderClusterProxyEgressSources(t *testing.T) {
	nodes := []*MTANode{
		{ID: "n1", Name: "node1", Status: MTANodeStatusActive}, // local, no proxy
		{ID: "n2", Name: "node2", Status: MTANodeStatusActive, ProxyHost: "10.20.0.12", ProxyPort: 1080},
	}
	vmtas := []*VMTA{
		{ID: "v1", Name: "vmta-local", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive, NodeID: "n1"},
		{ID: "v2", Name: "vmta-remote", IPAddress: "203.0.113.20", EHLOName: "b.example.com", Status: VMTAStatusActive, NodeID: "n2"},
	}
	r, err := RenderKumoConfig(clusterSnap(nodes, vmtas))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v", r.LintIssues)
	}
	if !strings.Contains(r.Content,
		`SOURCES["vmta-remote"] = { socks5_proxy_server = "10.20.0.12:1080", socks5_proxy_source_address = "203.0.113.20", ehlo_domain = "b.example.com" }`) {
		t.Errorf("remote VMTA should render a socks5 proxy pointer:\n%s", r.Content)
	}
	if !strings.Contains(r.Content,
		`SOURCES["vmta-local"] = { source_address = "203.0.113.10", ehlo_domain = "a.example.com" }`) {
		t.Errorf("local VMTA should keep a plain source_address:\n%s", r.Content)
	}
	// The generic get_egress_source hook must forward the proxy fields.
	if !strings.Contains(r.Content, "socks5_proxy_server = cfg.socks5_proxy_server") {
		t.Errorf("get_egress_source must pass proxy fields through:\n%s", r.Content)
	}
}

// TestRenderClusterDisabledNodeFallsBackToLocalBind verifies a VMTA owned by a
// DISABLED node renders without the proxy pointer (the proxy is gone), and a
// VMTA with an unknown node id is treated as local.
func TestRenderClusterDisabledNodeFallsBackToLocalBind(t *testing.T) {
	nodes := []*MTANode{
		{ID: "n2", Name: "node2", Status: MTANodeStatusDisabled, ProxyHost: "10.20.0.12", ProxyPort: 1080},
	}
	vmtas := []*VMTA{
		{ID: "v2", Name: "vmta-remote", IPAddress: "203.0.113.20", EHLOName: "b.example.com", Status: VMTAStatusActive, NodeID: "n2"},
		{ID: "v3", Name: "vmta-orphan", IPAddress: "203.0.113.30", EHLOName: "c.example.com", Status: VMTAStatusActive, NodeID: "gone"},
	}
	r, err := RenderKumoConfig(clusterSnap(nodes, vmtas))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(r.Content, "socks5_proxy_server") &&
		strings.Contains(r.Content, `socks5_proxy_server = "10.20.0.12:1080"`) {
		t.Errorf("disabled node's proxy must not be rendered:\n%s", r.Content)
	}
	for _, want := range []string{
		`SOURCES["vmta-remote"] = { source_address = "203.0.113.20", ehlo_domain = "b.example.com" }`,
		`SOURCES["vmta-orphan"] = { source_address = "203.0.113.30", ehlo_domain = "c.example.com" }`,
	} {
		if !strings.Contains(r.Content, want) {
			t.Errorf("missing local fallback %q:\n%s", want, r.Content)
		}
	}
}

// TestRenderClusterRedisThrottles verifies cluster-shared throttles are emitted
// in the init block only when more than one participating node exists and a
// Redis URL is configured — and that adding them changes the init checksum
// (forcing a restart, since reload does not re-run init).
func TestRenderClusterRedisThrottles(t *testing.T) {
	vmtas := []*VMTA{{ID: "v1", Name: "vmta-a", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}}

	single := clusterSnap([]*MTANode{{ID: "n1", Name: "node1", Status: MTANodeStatusActive}}, vmtas)
	single.LogStreamRedisURL = "redis://redis:6379"
	rSingle, err := RenderKumoConfig(single)
	if err != nil {
		t.Fatalf("render single: %v", err)
	}
	if strings.Contains(rSingle.Content, "configure_redis_throttles") {
		t.Errorf("single-node cluster must not enable redis throttles:\n%s", rSingle.Content)
	}

	multi := clusterSnap([]*MTANode{
		{ID: "n1", Name: "node1", Status: MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: MTANodeStatusDraining},
		{ID: "n3", Name: "node3", Status: MTANodeStatusDisabled},
	}, vmtas)
	multi.LogStreamRedisURL = "redis://redis:6379"
	rMulti, err := RenderKumoConfig(multi)
	if err != nil {
		t.Fatalf("render multi: %v", err)
	}
	if !rMulti.Valid {
		t.Fatalf("policy failed lint: %v", rMulti.LintIssues)
	}
	if !strings.Contains(rMulti.Content, `kumo.configure_redis_throttles { node = "redis://redis:6379" }`) {
		t.Errorf("multi-node cluster must enable redis throttles:\n%s", rMulti.Content)
	}
	if rSingle.InitChecksum == rMulti.InitChecksum {
		t.Errorf("enabling redis throttles must change the init checksum (restart required)")
	}

	// No Redis configured → never emitted, even multi-node.
	noRedis := clusterSnap([]*MTANode{
		{ID: "n1", Name: "node1", Status: MTANodeStatusActive},
		{ID: "n2", Name: "node2", Status: MTANodeStatusActive},
	}, vmtas)
	rNoRedis, err := RenderKumoConfig(noRedis)
	if err != nil {
		t.Fatalf("render no-redis: %v", err)
	}
	if strings.Contains(rNoRedis.Content, "configure_redis_throttles") {
		t.Errorf("redis throttles need a redis url:\n%s", rNoRedis.Content)
	}
}
