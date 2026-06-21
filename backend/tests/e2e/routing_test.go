//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// routingSnapshot builds an iris config with two VMTAs that announce distinct
// EHLO names and two mailclass routing rules that send each class to a
// different VMTA. Because the sink records the EHLO of the delivering egress
// source, the EHLO at the sink tells us which VMTA kumod actually routed to.
//
// All addresses are the rig's single static container IP (bindable for both the
// listener and the egress source); routing is observed purely via EHLO. The
// recipient domain is sink.test so the rig's Test resolver delivers to the sink.
func routingSnapshot(staticIP string) biz.ConfigSnapshot {
	return biz.ConfigSnapshot{
		Listeners: []*biz.Listener{
			{ID: "lst-1", Name: "edge", IPAddress: staticIP, Port: 2525, Hostname: "mx.e2e.test",
				Status: biz.ListenerStatusActive},
		},
		VMTAs: []*biz.VMTA{
			{ID: "v-bulk", Name: "vmta-bulk", ListenerID: "lst-1", IPAddress: staticIP,
				EHLOName: "bulk.egress.test", Status: biz.VMTAStatusActive},
			{ID: "v-promo", Name: "vmta-promo", ListenerID: "lst-1", IPAddress: staticIP,
				EHLOName: "promo.egress.test", Status: biz.VMTAStatusActive},
		},
		Routes: []*biz.RoutingRule{
			{ID: "r-bulk", Name: "bulk", MatchType: biz.MatchMailclass, MatchHeader: "X-Mail-Class",
				MatchValue: "bulk", Priority: 100, TargetType: biz.TargetVMTA, TargetID: "v-bulk",
				Status: biz.RoutingStatusActive},
			{ID: "r-promo", Name: "promo", MatchType: biz.MatchMailclass, MatchHeader: "X-Mail-Class",
				MatchValue: "promo", Priority: 90, TargetType: biz.TargetVMTA, TargetID: "v-promo",
				Status: biz.RoutingStatusActive},
		},
		EgressEHLODefault: "default.egress.test",
		LogStreamRedisURL: "redis://iris-redis:6379",
		LogStreamName:     "iris.mail.events.e2e",
		HTTPListen:        "127.0.0.1:8000",
	}
}

// TestRoutingByMailclass proves the generated routing logic end-to-end through a
// real kumod: a message tagged X-Mail-Class: bulk leaves via vmta-bulk and one
// tagged promo leaves via vmta-promo, observed by the EHLO each announces to the
// sink. This exercises classify_mail + select_pool + get_egress_pool +
// get_egress_source in the actual reception/delivery path, not just the rendered
// strings.
func TestRoutingByMailclass(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	r := startRig(t, routingSnapshot(kumodIP))

	r.inject("user@sink.test", "X-Mail-Class: bulk")
	r.inject("user@sink.test", "X-Mail-Class: promo")

	msgs := r.waitForSink(2, 30*time.Second)

	byEHLO := map[string]int{}
	for _, m := range msgs {
		byEHLO[m.EHLO]++
	}
	if byEHLO["bulk.egress.test"] == 0 {
		t.Errorf("expected a delivery via vmta-bulk (EHLO bulk.egress.test); got EHLOs %v", byEHLO)
	}
	if byEHLO["promo.egress.test"] == 0 {
		t.Errorf("expected a delivery via vmta-promo (EHLO promo.egress.test); got EHLOs %v", byEHLO)
	}
}
