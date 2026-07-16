package data

import (
	"context"

	"github.com/menta2k/iris/backend/internal/biz"
)

// KumoConfigRepo assembles the full active configuration snapshot used to render
// a KumoMTA policy. It composes the outbound, domain-safety, and inbound repos.
type KumoConfigRepo struct {
	outbound    *OutboundConfigRepo
	safety      *DomainSafetyRepo
	inbound     *InboundRepo
	routes      *InboundRouteRepo
	fbl         *FBLRepo
	warmup      *WarmupRepo
	blueprint   *BlueprintRepo
	automation  *AutomationRepo
	bounceRules *BounceRuleRepo
	nodes       *MTANodeRepo
}

// NewKumoConfigRepo constructs the snapshot loader.
func NewKumoConfigRepo(outbound *OutboundConfigRepo, safety *DomainSafetyRepo, inbound *InboundRepo, routes *InboundRouteRepo, fbl *FBLRepo, warmup *WarmupRepo, blueprint *BlueprintRepo, automation *AutomationRepo, bounceRules *BounceRuleRepo, nodes *MTANodeRepo) *KumoConfigRepo {
	return &KumoConfigRepo{outbound: outbound, safety: safety, inbound: inbound, routes: routes, fbl: fbl, warmup: warmup, blueprint: blueprint, automation: automation, bounceRules: bounceRules, nodes: nodes}
}

var _ biz.ConfigSnapshotLoader = (*KumoConfigRepo)(nil)

// Snapshot loads all configuration entities needed to render the policy. It uses
// a large bounded page so the full active set is captured.
func (r *KumoConfigRepo) Snapshot(ctx context.Context) (biz.ConfigSnapshot, error) {
	page := biz.Page{Size: biz.MaxPageSize, Offset: 0}
	var snap biz.ConfigSnapshot
	var err error

	if snap.Listeners, err = r.outbound.ListListeners(ctx, page); err != nil {
		return snap, err
	}
	if snap.VMTAs, err = r.outbound.ListVMTAs(ctx, "", page); err != nil {
		return snap, err
	}
	if snap.Groups, err = r.outbound.ListVMTAGroups(ctx, page); err != nil {
		return snap, err
	}
	if r.nodes != nil {
		if snap.Nodes, err = r.nodes.ListNodes(ctx); err != nil {
			return snap, err
		}
	}
	if snap.Routes, err = r.outbound.ListRoutingRules(ctx, "", "", page); err != nil {
		return snap, err
	}
	if snap.DKIM, err = r.safety.ListDKIMDomains(ctx, page); err != nil {
		return snap, err
	}
	// Suppressions are enforced via Redis (see SuppressionCache), not rendered
	// into the policy, so they are intentionally not loaded into the snapshot.
	if snap.TLSPolicies, err = r.safety.ListTLSPolicies(ctx, "", page); err != nil {
		return snap, err
	}
	if r.routes != nil {
		if snap.InboundRoutes, err = r.routes.ListInboundRoutesForPolicy(ctx); err != nil {
			return snap, err
		}
	}
	if r.fbl != nil {
		if snap.FBLEndpoints, err = r.fbl.ListFBLEndpointsForPolicy(ctx); err != nil {
			return snap, err
		}
	}
	if r.warmup != nil {
		if snap.WarmupSchedules, err = r.warmup.ListActiveWarmupsForPolicy(ctx); err != nil {
			return snap, err
		}
	}
	if r.blueprint != nil {
		if snap.Blueprints, err = r.blueprint.ListActiveBlueprintsForPolicy(ctx); err != nil {
			return snap, err
		}
	}
	if r.automation != nil {
		if snap.AutomationRules, err = r.automation.ListActiveAutomationForPolicy(ctx); err != nil {
			return snap, err
		}
	}
	if r.bounceRules != nil {
		if snap.BounceRules, err = r.bounceRules.ListActiveBounceRules(ctx); err != nil {
			return snap, err
		}
	}
	return snap, nil
}
