package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
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

	// Round-robin start offset, then walk the ring until one node accepts.
	start := int(k.injectRR.Add(1)-1) % len(targets)
	var lastErr error
	for i := range targets {
		t := targets[(start+i)%len(targets)]
		err := t.transport.inject(ctx, body)
		if err == nil {
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

// StubInjector is a no-op KumoInjector for local development (KumoMTA stub
// mode): it accepts every message so the injection endpoint can be exercised
// without a live kumod.
type StubInjector struct{}

var _ biz.KumoInjector = StubInjector{}

// InjectV1 discards the message and reports success.
func (StubInjector) InjectV1(context.Context, biz.KumoInjectRequest) error { return nil }
