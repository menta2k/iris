//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// clusterEgressSnapshot builds a config where node1 (the kumod under test)
// receives mail, and routing sends one mail class to a VMTA owned by node2 and
// another to a VMTA owned by node1:
//
//   - vmta-remote lives on node2, whose kumo-proxy is at proxyEndpoint. Its
//     egress source renders a socks5_proxy pointer, so node1 delivers that
//     mail THROUGH node2's proxy — the sink sees the connection from node2's
//     proxy IP (proxyNodeIP).
//   - vmta-local lives on node1 (no proxy): a direct delivery, so the sink sees
//     the connection from node1's own egress IP (kumodIP).
//
// This is the "received on node1, egress from node2" scenario end-to-end
// through real kumod + real kumo-proxy: routing is unchanged (mail class →
// egress pool), only the final TCP hop is proxied.
func clusterEgressSnapshot(proxyEndpoint string) biz.ConfigSnapshot {
	return biz.ConfigSnapshot{
		Nodes: []*biz.MTANode{
			{ID: "node1", Name: "node1", Status: biz.MTANodeStatusActive},
			{ID: "node2", Name: "node2", Status: biz.MTANodeStatusActive,
				ProxyHost: proxyNodeIP, ProxyPort: proxyNodePort},
		},
		Listeners: []*biz.Listener{
			// Trust the rig subnet so the throwaway injector sidecar (a dynamic
			// IP in this range) may relay to the sink.
			{ID: "lst-1", Name: "edge", IPAddress: kumodIP, Port: 2525, Hostname: "mx.e2e.test",
				RelayHosts: []string{e2eSubnet}, Role: biz.ListenerRoleSubmission, Status: biz.ListenerStatusActive},
		},
		VMTAs: []*biz.VMTA{
			// Owned by node2: its IP is the proxy's IP, which the proxy binds as
			// the onward source. Egresses via node2's kumo-proxy.
			{ID: "v-remote", Name: "vmta-remote", NodeID: "node2", IPAddress: proxyNodeIP,
				EHLOName: "remote.node2.test", Status: biz.VMTAStatusActive},
			// Owned by node1 (local): kumod binds its own IP and delivers directly.
			{ID: "v-local", Name: "vmta-local", NodeID: "node1", IPAddress: kumodIP,
				EHLOName: "local.node1.test", Status: biz.VMTAStatusActive},
		},
		Routes: []*biz.RoutingRule{
			{ID: "r-remote", Name: "to-node2", MatchType: biz.MatchMailclass, MatchHeader: "X-Mail-Class",
				MatchValue: "bulk", Priority: 100, TargetType: biz.TargetVMTA, TargetID: "v-remote",
				Status: biz.RoutingStatusActive},
			{ID: "r-local", Name: "to-node1", MatchType: biz.MatchMailclass, MatchHeader: "X-Mail-Class",
				MatchValue: "promo", Priority: 90, TargetType: biz.TargetVMTA, TargetID: "v-local",
				Status: biz.RoutingStatusActive},
		},
		EgressEHLODefault: "default.egress.test",
		LogStreamRedisURL: "redis://iris-redis:6379",
		LogStreamName:     "iris.mail.events.e2e",
		HTTPListen:        "127.0.0.1:8000",
	}
}

// TestClusterCrossNodeEgress proves the core cluster mechanic end-to-end: a
// message received on node1 whose mail class routes to a VMTA on node2 leaves
// the internet from node2 (via its kumo-proxy), while a message routed to a
// node1-local VMTA leaves directly from node1. The sink's captured peer IP is
// the physical proof of which node each message egressed from; the EHLO
// confirms which VMTA won the route.
func TestClusterCrossNodeEgress(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	proxyEndpoint := startClusterProxy(t)
	r := startRig(t, clusterEgressSnapshot(proxyEndpoint))

	// bulk → vmta-remote (node2, via proxy); promo → vmta-local (node1, direct).
	r.inject("user@sink.test", "X-Mail-Class: bulk")
	r.inject("user@sink.test", "X-Mail-Class: promo")

	msgs := r.waitForSink(2, 45*time.Second)

	byEHLO := map[string]capturedMsg{}
	for _, m := range msgs {
		byEHLO[m.EHLO] = m
	}

	remote, ok := byEHLO["remote.node2.test"]
	if !ok {
		t.Fatalf("no delivery via vmta-remote; captured: %+v", msgs)
	}
	if remote.PeerIP != proxyNodeIP {
		t.Errorf("cross-node egress FAILED: bulk mail should leave from node2's proxy %s, but the sink saw it from %s",
			proxyNodeIP, remote.PeerIP)
	} else {
		t.Logf("OK: bulk received on node1 egressed from node2 proxy %s (EHLO %s)", remote.PeerIP, remote.EHLO)
	}

	local, ok := byEHLO["local.node1.test"]
	if !ok {
		t.Fatalf("no delivery via vmta-local; captured: %+v", msgs)
	}
	if local.PeerIP != kumodIP {
		t.Errorf("local egress wrong: promo mail should leave directly from node1 %s, but the sink saw it from %s",
			kumodIP, local.PeerIP)
	} else {
		t.Logf("OK: promo egressed directly from node1 %s (EHLO %s)", local.PeerIP, local.EHLO)
	}
}
