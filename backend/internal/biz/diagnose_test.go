package biz

import (
	"testing"
)

func diagnoseSnap() ConfigSnapshot {
	return ConfigSnapshot{
		Listeners: []*Listener{{ID: "l1", Name: "edge", IPAddress: "203.0.113.5"}},
		VMTAs: []*VMTA{
			{ID: "v1", Name: "bulk-vmta", IPAddress: "198.51.100.7", ListenerName: "edge", Status: VMTAStatusActive},
		},
		Routes: []*RoutingRule{
			{Name: "bulk", MatchType: MatchMailclass, MatchValue: "bulk", Priority: 100,
				TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
		},
		DKIM:         []*DKIMDomain{{Domain: "example.com", Selector: "s1", Status: DKIMReady}},
		FBLEndpoints: []*FBLEndpoint{{Domain: "example.com", FeedbackAddress: "fbl@example.com", Status: FBLApproved}},
	}
}

func TestDiagnoseInvalidEmail(t *testing.T) {
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, fakeResolver{})
	if _, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "not-an-email"}); err == nil {
		t.Fatal("expected invalid-email error")
	}
}

func TestDiagnoseItemsAndRouting(t *testing.T) {
	dns := fakeResolver{
		txt: map[string][]string{
			"example.com":               {"v=spf1 ip4:198.51.100.7 -all"},
			"_dmarc.example.com":        {"v=DMARC1; p=reject"},
			"s1._domainkey.example.com": {"v=DKIM1; k=rsa; p=abc"},
		},
	}
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, dns)
	res, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "news@Example.com", Mailclass: "bulk"})
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if res.Domain != "example.com" {
		t.Fatalf("domain = %q", res.Domain)
	}
	for name, want := range map[string]string{
		"DKIM signing":  CheckPass,
		"SPF":           CheckPass,
		"DMARC":         CheckPass,
		"Feedback loop": CheckPass,
	} {
		it := itemByName(res.Items, name)
		if it == nil || it.Status != want {
			t.Fatalf("%s: want %s, got %+v", name, want, it)
		}
	}
	// Mailclass "bulk" routes to v1 → its pool, IP, listener.
	r := res.Routing
	if r.MatchedRule != "bulk" || r.EgressPool != "bulk-vmta" {
		t.Fatalf("routing rule/pool: %+v", r)
	}
	if len(r.EgressIPs) != 1 || r.EgressIPs[0] != "198.51.100.7" {
		t.Fatalf("egress IPs: %+v", r.EgressIPs)
	}
	if len(r.Listeners) != 1 || r.Listeners[0] != "edge" {
		t.Fatalf("listeners: %+v", r.Listeners)
	}
}

func TestDiagnoseDefaultRoutingWhenNoMatch(t *testing.T) {
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, fakeResolver{})
	// No mailclass/recipient → no rule matches → default pool, note set.
	res, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "news@example.com"})
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if res.Routing.MatchedRule != "" || res.Routing.EgressPool != "default" {
		t.Fatalf("expected default routing, got %+v", res.Routing)
	}
	if res.Routing.Note == "" {
		t.Fatal("expected an explanatory note for default routing")
	}
}
