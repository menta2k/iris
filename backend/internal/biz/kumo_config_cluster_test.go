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
	if !strings.Contains(rMulti.Content, "kumo.configure_redis_throttles { node = LOGSTREAM_REDIS_NODE, cluster = LOGSTREAM_REDIS_CLUSTER }") {
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

// TestRenderNodeIdentity verifies the policy loads the per-node prelude and
// stamps the 'node' meta on both submission paths, keeping the policy itself
// node-agnostic (byte-identical across the cluster).
func TestRenderNodeIdentity(t *testing.T) {
	snap := clusterSnap(nil, []*VMTA{{ID: "v1", Name: "vmta-a", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}})
	snap.LogStreamRedisURL = "redis://redis:6379"
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v", r.LintIssues)
	}
	for _, want := range []string{
		`pcall(dofile, "/opt/kumomta/etc/policy/iris_node.lua")`,
		"msg:set_meta('node', NODE_NAME)",
		"meta = { 'tenant', 'mailclass', 'node' }",
	} {
		if !strings.Contains(r.Content, want) {
			t.Errorf("missing node identity wiring %q", want)
		}
	}
	// Both hooks must stamp the meta (SMTP reception + HTTP injection).
	if strings.Count(r.Content, "msg:set_meta('node', NODE_NAME)") < 2 {
		t.Errorf("node meta must be set in both submission hooks:\n%s", r.Content)
	}
	if got := NodePreludeContent("node1"); got != "return { name = \"node1\" }\n" {
		t.Errorf("NodePreludeContent = %q", got)
	}
}

// TestRenderNodeAwareListeners verifies node-pinned listeners render inside a
// NODE_NAME guard (so only that node binds them) while an unpinned listener
// binds on every node — all from one byte-identical policy.
func TestRenderNodeAwareListeners(t *testing.T) {
	snap := ConfigSnapshot{
		Nodes: []*MTANode{
			{ID: "n1", Name: "node1", Status: MTANodeStatusActive},
			{ID: "n2", Name: "node2", Status: MTANodeStatusActive, ProxyHost: "10.0.0.2", ProxyPort: 1080},
		},
		Listeners: []*Listener{
			{ID: "l1", Name: "sub-node1", IPAddress: "10.0.0.1", Port: 2587, Hostname: "mx1.example.com",
				RelayHosts: []string{"10.0.0.0/8"}, Role: ListenerRoleSubmission, Status: ListenerStatusActive,
				NodeID: "n1", NodeName: "node1"},
			{ID: "l2", Name: "sub-node2", IPAddress: "10.0.0.2", Port: 2587, Hostname: "mx2.example.com",
				RelayHosts: []string{"10.0.0.0/8"}, Role: ListenerRoleSubmission, Status: ListenerStatusActive,
				NodeID: "n2", NodeName: "node2"},
			{ID: "l3", Name: "mx-all", IPAddress: "10.0.0.9", Port: 2525, Hostname: "mx.example.com",
				Role: ListenerRoleInbound, Status: ListenerStatusActive}, // unpinned: every node
		},
		VMTAs: []*VMTA{{ID: "v1", Name: "vmta-a", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}},
		DKIM:  []*DKIMDomain{{ID: "d1", Domain: "example.com", Selector: "s1", PrivateKeyRef: testDKIMKeyPEM, Status: DKIMReady}},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !r.Valid {
		t.Fatalf("policy failed lint: %v", r.LintIssues)
	}
	// node1's listener is guarded to node1, node2's to node2.
	if !strings.Contains(r.Content, `if NODE_NAME == "node1" then`) ||
		!strings.Contains(r.Content, `if NODE_NAME == "node2" then`) {
		t.Errorf("expected per-node NODE_NAME listener guards:\n%s", r.Content)
	}
	if !strings.Contains(r.Content, `listen = "10.0.0.1:2587"`) || !strings.Contains(r.Content, `listen = "10.0.0.2:2587"`) {
		t.Errorf("expected both node submission binds rendered")
	}
	// The unpinned inbound listener is NOT wrapped in a guard.
	idx := strings.Index(r.Content, `listen = "10.0.0.9:2525"`)
	if idx < 0 {
		t.Fatalf("unpinned listener not rendered")
	}
	// Sanity: guard count equals the two pinned listeners.
	if got := strings.Count(r.Content, "if NODE_NAME == "); got != 2 {
		t.Errorf("expected exactly 2 node-guarded listeners, got %d", got)
	}
}

// TestRenderRedisClusterConfig verifies the generated kumod policy uses a
// cluster-enabled redis client (node array + cluster=true) when a Redis
// Cluster is configured — so kumod follows MOVED/ASK just like the iris
// cluster client, instead of failing on a single-node connection.
func TestRenderRedisClusterConfig(t *testing.T) {
	base := clusterSnap(nil, []*VMTA{{ID: "v1", Name: "vmta-a", IPAddress: "203.0.113.10", EHLOName: "a.example.com", Status: VMTAStatusActive}})

	// Single node (default): plain string node, cluster=false.
	single := base
	single.LogStreamRedisURL = "redis://redis:6379"
	rs, err := RenderKumoConfig(single)
	if err != nil {
		t.Fatalf("render single: %v", err)
	}
	if !strings.Contains(rs.Content, `local LOGSTREAM_REDIS_NODE = "redis://redis:6379"`) ||
		!strings.Contains(rs.Content, "local LOGSTREAM_REDIS_CLUSTER = false") {
		t.Errorf("single-node redis config wrong:\n%s", firstLines(rs.Content, 30))
	}
	if !strings.Contains(rs.Content, "redis.open { node = LOGSTREAM_REDIS_NODE, cluster = LOGSTREAM_REDIS_CLUSTER") {
		t.Errorf("redis.open should reference the node+cluster locals")
	}

	// Cluster: multiple seed nodes → array node + cluster=true.
	cluster := base
	cluster.LogStreamRedisURL = "redis://10.1.114.1:7000"
	cluster.LogStreamRedisNodes = []string{"redis://10.1.114.1:7000", "redis://10.1.114.2:7000", "redis://10.1.114.3:7000"}
	rc, err := RenderKumoConfig(cluster)
	if err != nil {
		t.Fatalf("render cluster: %v", err)
	}
	if !strings.Contains(rc.Content, `local LOGSTREAM_REDIS_NODE = { "redis://10.1.114.1:7000", "redis://10.1.114.2:7000", "redis://10.1.114.3:7000" }`) {
		t.Errorf("cluster node array wrong:\n%s", firstLines(rc.Content, 30))
	}
	if !strings.Contains(rc.Content, "local LOGSTREAM_REDIS_CLUSTER = true") {
		t.Errorf("cluster flag should be true")
	}

	// Single seed but explicitly clustered (cluster fronted by one endpoint).
	forced := base
	forced.LogStreamRedisURL = "redis://redis-cluster:6379"
	forced.LogStreamRedisCluster = true
	rf, err := RenderKumoConfig(forced)
	if err != nil {
		t.Fatalf("render forced: %v", err)
	}
	if !strings.Contains(rf.Content, "local LOGSTREAM_REDIS_CLUSTER = true") {
		t.Errorf("forced cluster flag should be true even with one seed")
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
