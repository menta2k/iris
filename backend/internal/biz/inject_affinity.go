package biz

import (
	"strings"
	"sync/atomic"
)

// InjectNodeAffinity resolves which cluster node(s) own the egress VMTA a given
// injected message will be routed to, so HTTP injection can place the message
// on its egress-owning node and avoid the cross-node kumo-proxy hop. ok is false
// when routing can't be resolved (unknown mailclass, recipient-based routing) —
// the caller then falls back to round-robin. Satisfied by *InjectAffinity.
type InjectNodeAffinity interface {
	NodeFor(req KumoInjectRequest) (nodeIDs []string, ok bool)
}

// affinityEntry is the resolved egress-owning node(s) for one (header,value)
// mailclass match, tagged with the routing rule's priority so a higher-priority
// rule wins on a conflicting value (mirrors the renderer's descending-priority
// select_pool).
type affinityEntry struct {
	priority int
	nodes    []string // node IDs; a single "" means the local/co-located node
}

// affinityTable maps a lower-cased mailclass header name → header value →
// egress-owning nodes. It is immutable once built; InjectAffinity swaps whole
// tables atomically so reads are lock-free.
type affinityTable struct {
	byHeader map[string]map[string]affinityEntry
}

// InjectAffinity holds the current mailclass→node table behind an atomic
// pointer. Built off the hot path (on a timer / after apply) and read on every
// injection with a single atomic load + map lookup — no allocation, no lock.
type InjectAffinity struct {
	table atomic.Pointer[affinityTable]
}

// NewInjectAffinity returns an empty resolver (NodeFor reports ok=false until
// Rebuild runs, so the caller round-robins).
func NewInjectAffinity() *InjectAffinity { return &InjectAffinity{} }

// Rebuild replaces the table from the current configuration snapshot. Only
// mailclass routing is modeled (the dominant path); recipient/header-vmta rules
// are intentionally omitted so those messages fall back to round-robin.
func (a *InjectAffinity) Rebuild(snap ConfigSnapshot) {
	a.table.Store(buildAffinityTable(snap))
}

// NodeFor reads the message's mailclass header(s) and returns the egress-owning
// node(s), highest-priority match first. ok is false when nothing matches.
func (a *InjectAffinity) NodeFor(req KumoInjectRequest) ([]string, bool) {
	t := a.table.Load()
	if t == nil || len(t.byHeader) == 0 || len(req.Content.Headers) == 0 {
		return nil, false
	}
	best := affinityEntry{}
	found := false
	for h, v := range req.Content.Headers {
		values := t.byHeader[strings.ToLower(strings.TrimSpace(h))]
		if values == nil {
			continue
		}
		if e, ok := values[v]; ok {
			if !found || e.priority > best.priority {
				best, found = e, true
			}
		}
	}
	if !found || len(best.nodes) == 0 {
		return nil, false
	}
	return best.nodes, true
}

// buildAffinityTable resolves each active mailclass rule to its egress-owning
// node(s) and indexes it by every (header,value) that selects it.
func buildAffinityTable(snap ConfigSnapshot) *affinityTable {
	// vmtaID → owning node ID ("" = local/co-located node).
	vmtaNode := make(map[string]string, len(snap.VMTAs))
	for _, v := range snap.VMTAs {
		if v != nil {
			vmtaNode[v.ID] = v.NodeID
		}
	}
	// groupID → distinct owning node IDs of its member VMTAs.
	groupNodes := make(map[string][]string, len(snap.Groups))
	for _, g := range snap.Groups {
		if g == nil {
			continue
		}
		seen := map[string]bool{}
		var ns []string
		for _, m := range g.Members {
			nid, ok := vmtaNode[m.VMTAID]
			if !ok || seen[nid] {
				continue
			}
			seen[nid] = true
			ns = append(ns, nid)
		}
		groupNodes[g.ID] = ns
	}

	t := &affinityTable{byHeader: map[string]map[string]affinityEntry{}}
	for _, r := range snap.Routes {
		if r == nil || r.MatchType != MatchMailclass || r.Status != RoutingStatusActive {
			continue
		}
		var nodes []string
		switch r.TargetType {
		case TargetVMTA:
			if nid, ok := vmtaNode[r.TargetID]; ok {
				nodes = []string{nid}
			}
		case TargetVMTAGroup:
			nodes = groupNodes[r.TargetID]
		default:
			continue
		}
		if len(nodes) == 0 {
			continue
		}
		// Conditions is the authoritative OR-list; fall back to the mirrored
		// MatchHeader/MatchValue for older rows.
		conds := r.Conditions
		if len(conds) == 0 && r.MatchHeader != "" {
			conds = []RoutingMatchCondition{{Header: r.MatchHeader, Value: r.MatchValue}}
		}
		for _, c := range conds {
			h := strings.ToLower(strings.TrimSpace(c.Header))
			if h == "" || c.Value == "" {
				continue
			}
			values := t.byHeader[h]
			if values == nil {
				values = map[string]affinityEntry{}
				t.byHeader[h] = values
			}
			// Higher priority wins on a conflicting value (descending-priority
			// select_pool); ties keep the first seen.
			if e, ok := values[c.Value]; !ok || r.Priority > e.priority {
				values[c.Value] = affinityEntry{priority: r.Priority, nodes: nodes}
			}
		}
	}
	return t
}
