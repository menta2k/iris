package biz

import "testing"

func injectReqWithHeaders(h map[string]string) KumoInjectRequest {
	return KumoInjectRequest{Content: KumoInjectContent{Headers: h}}
}

func affinitySnapshot() ConfigSnapshot {
	return ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v-kmx02", NodeID: "node-kmx02"},
			{ID: "v-local", NodeID: ""}, // local/co-located node
			{ID: "v-kmx", NodeID: "node-kmx"},
		},
		Groups: []*VMTAGroup{
			{ID: "g-multi", Members: []VMTAGroupMember{{VMTAID: "v-kmx", Weight: 1}, {VMTAID: "v-kmx02", Weight: 1}}},
		},
		Routes: []*RoutingRule{
			// mailclass "kmx-test" → single-node VMTA on kmx02.
			{ID: "r1", MatchType: MatchMailclass, Status: RoutingStatusActive, Priority: 100,
				TargetType: TargetVMTA, TargetID: "v-kmx02",
				Conditions: []RoutingMatchCondition{{Header: "X-GreenArrow", Value: "kmx-test"}}},
			// mailclass "loc" → local VMTA (NodeID "").
			{ID: "r2", MatchType: MatchMailclass, Status: RoutingStatusActive, Priority: 50,
				TargetType: TargetVMTA, TargetID: "v-local",
				Conditions: []RoutingMatchCondition{{Header: "X-GreenArrow", Value: "loc"}}},
			// mailclass "spread" → group spanning both nodes.
			{ID: "r3", MatchType: MatchMailclass, Status: RoutingStatusActive, Priority: 50,
				TargetType: TargetVMTAGroup, TargetID: "g-multi",
				Conditions: []RoutingMatchCondition{{Header: "X-GreenArrow", Value: "spread"}}},
			// recipient rule — must NOT contribute to affinity.
			{ID: "r4", MatchType: MatchRecipientDomain, Status: RoutingStatusActive, Priority: 10,
				TargetType: TargetVMTA, TargetID: "v-kmx", MatchValue: "example.com"},
			// disabled rule — ignored.
			{ID: "r5", MatchType: MatchMailclass, Status: RoutingStatusDisabled, Priority: 999,
				TargetType: TargetVMTA, TargetID: "v-kmx",
				Conditions: []RoutingMatchCondition{{Header: "X-GreenArrow", Value: "kmx-test"}}},
		},
	}
}

func TestInjectAffinityResolvesSingleNode(t *testing.T) {
	a := NewInjectAffinity()
	a.Rebuild(affinitySnapshot())

	nodes, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "kmx-test"}))
	if !ok || len(nodes) != 1 || nodes[0] != "node-kmx02" {
		t.Fatalf("kmx-test → %v, %v; want [node-kmx02], true", nodes, ok)
	}
	// Header match is case-insensitive.
	nodes, ok = a.NodeFor(injectReqWithHeaders(map[string]string{"x-greenarrow": "kmx-test"}))
	if !ok || nodes[0] != "node-kmx02" {
		t.Fatalf("case-insensitive header lookup failed: %v, %v", nodes, ok)
	}
}

func TestInjectAffinityLocalNode(t *testing.T) {
	a := NewInjectAffinity()
	a.Rebuild(affinitySnapshot())
	nodes, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "loc"}))
	if !ok || len(nodes) != 1 || nodes[0] != "" {
		t.Fatalf("loc → %v, %v; want [\"\"] (local), true", nodes, ok)
	}
}

func TestInjectAffinityGroupSpansNodes(t *testing.T) {
	a := NewInjectAffinity()
	a.Rebuild(affinitySnapshot())
	nodes, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "spread"}))
	if !ok || len(nodes) != 2 {
		t.Fatalf("spread → %v, %v; want both owning nodes", nodes, ok)
	}
	got := map[string]bool{nodes[0]: true, nodes[1]: true}
	if !got["node-kmx"] || !got["node-kmx02"] {
		t.Fatalf("spread nodes = %v; want node-kmx + node-kmx02", nodes)
	}
}

func TestInjectAffinityFallsBackToRoundRobin(t *testing.T) {
	a := NewInjectAffinity()
	a.Rebuild(affinitySnapshot())

	// Unknown mailclass → no match (round-robin).
	if _, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "does-not-exist"})); ok {
		t.Fatal("unknown mailclass must not resolve an owning node")
	}
	// No mailclass header at all → no match.
	if _, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-Feedback-ID": "x:1:1:y"})); ok {
		t.Fatal("message with no mailclass header must not resolve")
	}
	// Empty table (never rebuilt) → no match.
	if _, ok := NewInjectAffinity().NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "kmx-test"})); ok {
		t.Fatal("empty affinity table must report ok=false")
	}
}

func TestInjectAffinityHighestPriorityWins(t *testing.T) {
	snap := affinitySnapshot()
	// Add a higher-priority active rule for the same value pointing elsewhere.
	snap.Routes = append(snap.Routes, &RoutingRule{
		ID: "r6", MatchType: MatchMailclass, Status: RoutingStatusActive, Priority: 200,
		TargetType: TargetVMTA, TargetID: "v-kmx",
		Conditions: []RoutingMatchCondition{{Header: "X-GreenArrow", Value: "kmx-test"}},
	})
	a := NewInjectAffinity()
	a.Rebuild(snap)
	nodes, ok := a.NodeFor(injectReqWithHeaders(map[string]string{"X-GreenArrow": "kmx-test"}))
	if !ok || len(nodes) != 1 || nodes[0] != "node-kmx" {
		t.Fatalf("higher-priority rule should win: got %v, %v; want [node-kmx]", nodes, ok)
	}
}
