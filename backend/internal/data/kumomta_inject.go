package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/metrics"
)

// injectTargets returns the nodes eligible to accept NEW mail: active only
// (draining nodes finish their queues but take no new work) and with an admin
// channel configured.
func (k *FileKumoMTA) injectTargets(ctx context.Context) ([]applyTarget, error) {
	targets, err := k.adminTargets(ctx)
	if err != nil {
		return nil, err
	}
	if k.nodes == nil {
		return targets, nil // implicit single local node
	}
	nodes, err := k.nodes.ListNodes(ctx)
	if err != nil {
		return nil, biz.Internal(err, "list cluster nodes")
	}
	drainingByName := map[string]bool{}
	for _, n := range nodes {
		if n.Status == biz.MTANodeStatusDraining {
			drainingByName[n.Name] = true
		}
	}
	out := targets[:0:0]
	for _, t := range targets {
		if drainingByName[t.name] {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// InjectV1 forwards a built message to KumoMTA's HTTP injection API
// (POST /api/inject/v1). With a cluster registry the message is offered to the
// active nodes round-robin, failing over to the next node when one is
// unreachable — any node can accept any message, since routing is content
// based. kumod assembles the MIME, then the iris-generated policy's
// http_message_generated hook stamps Message-ID/Date and DKIM-signs before the
// message is queued.
func (k *FileKumoMTA) InjectV1(ctx context.Context, req biz.KumoInjectRequest) error {
	targets, err := k.injectTargets(ctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return biz.Unavailable("KUMO_INJECT_UNCONFIGURED", "no KumoMTA injection endpoint configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return biz.Internal(err, "marshal kumo inject request")
	}

	// Order the ring so the message's egress-owning node(s) come first — keeping
	// its whole lifecycle node-local and avoiding the cross-node kumo-proxy hop —
	// then the rest as failover. When routing can't be resolved (no mailclass
	// match, unknown class, resolver unset) this is a plain round-robin, exactly
	// as before.
	ordered, affinity := k.orderInjectTargets(req, targets)

	var lastErr error
	for _, t := range ordered {
		err := t.transport.inject(ctx, body)
		if err == nil {
			metrics.InjectionRouting.WithLabelValues(injectOutcome(affinity, t, ordered)).Inc()
			return nil
		}
		// A rejection (non-2xx from kumod) is authoritative — the same message
		// would be rejected everywhere — so only unreachability fails over.
		var de *biz.DomainError
		if errors.As(err, &de) && de.Reason == "KUMO_INJECT_FAILED" {
			return err
		}
		lastErr = fmt.Errorf("node %s: %w", t.name, err)
		biz.LoggerFrom(ctx).Warn("injection failover", "node", t.name, "error", err.Error())
	}
	return lastErr
}

// orderInjectTargets returns targets ordered by egress affinity: the owning
// node(s) first (round-robined among themselves when a group spans several),
// then the remaining nodes as failover. affinity reports whether an owning node
// was resolved AND is currently an eligible target — false means the result is a
// plain round-robin (no route match, or the owner is down/draining). The
// injectRR counter always advances so load spreads within whichever set is
// tried first.
func (k *FileKumoMTA) orderInjectTargets(req biz.KumoInjectRequest, targets []applyTarget) (ordered []applyTarget, affinity bool) {
	rr := func(ts []applyTarget) []applyTarget {
		if len(ts) <= 1 {
			return ts
		}
		start := int(k.injectRR.Add(1)-1) % len(ts)
		out := make([]applyTarget, 0, len(ts))
		for i := range ts {
			out = append(out, ts[(start+i)%len(ts)])
		}
		return out
	}

	if k.affinity == nil || len(targets) <= 1 {
		return rr(targets), false
	}
	prefer, ok := k.affinity.NodeFor(req)
	if !ok {
		return rr(targets), false
	}
	preferred := make(map[string]bool, len(prefer))
	for _, id := range prefer {
		preferred[id] = true
	}
	var owners, rest []applyTarget
	for _, t := range targets {
		if k.isPreferredTarget(t, preferred) {
			owners = append(owners, t)
		} else {
			rest = append(rest, t)
		}
	}
	if len(owners) == 0 {
		// The egress-owning node isn't an eligible target right now (down,
		// draining, or not registered) — round-robin the whole ring as failover.
		return rr(targets), false
	}
	return append(rr(owners), rest...), true
}

// isPreferredTarget reports whether target t is one of the egress-owning nodes.
// A VMTA with an empty node ID means the local/co-located node, so the empty
// string matches the local transport.
func (k *FileKumoMTA) isPreferredTarget(t applyTarget, preferred map[string]bool) bool {
	if preferred[t.nodeID] {
		return true
	}
	return preferred[""] && t.transport == k.local
}

// injectOutcome labels the routing decision for the metric.
func injectOutcome(affinity bool, accepted applyTarget, ordered []applyTarget) string {
	if !affinity {
		return "round_robin"
	}
	// With affinity, the owning node(s) were tried first; if the accepting node
	// was the first tried it went local, otherwise it failed over.
	if len(ordered) > 0 && accepted.name == ordered[0].name {
		return "affinity_local"
	}
	return "affinity_failover"
}

// StubInjector is a no-op KumoInjector for local development (KumoMTA stub
// mode): it accepts every message so the injection endpoint can be exercised
// without a live kumod.
type StubInjector struct{}

var _ biz.KumoInjector = StubInjector{}

// InjectV1 discards the message and reports success.
func (StubInjector) InjectV1(context.Context, biz.KumoInjectRequest) error { return nil }
