package biz

import (
	"net"
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
		DKIM:            []*DKIMDomain{{Domain: "example.com", Selector: "s1", Status: DKIMReady}},
		FBLEndpoints:    []*FBLEndpoint{{Domain: "example.com", FeedbackAddress: "fbl@example.com", Status: FBLApproved}},
		DMARCReportAddr: "dmarc@kmx.example.com",
	}
}

func TestDiagnoseInvalidEmail(t *testing.T) {
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, fakeResolver{}, nil)
	if _, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "not-an-email"}); err == nil {
		t.Fatal("expected invalid-email error")
	}
}

func TestDiagnoseItemsAndRouting(t *testing.T) {
	dns := fakeResolver{
		txt: map[string][]string{
			"example.com":               {"v=spf1 ip4:198.51.100.7 -all"},
			"_dmarc.example.com":        {"v=DMARC1; p=reject; rua=mailto:dmarc@kmx.example.com!10m"},
			"s1._domainkey.example.com": {"v=DKIM1; k=rsa; p=abc"},
		},
	}
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, dns, nil)
	res, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "news@Example.com", Mailclass: "bulk"})
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if res.Domain != "example.com" {
		t.Fatalf("domain = %q", res.Domain)
	}
	for name, want := range map[string]string{
		"DKIM signing":          CheckPass,
		"SPF":                   CheckPass,
		"DMARC":                 CheckPass,
		"DMARC reporting (rua)": CheckPass, // rua includes the configured address
		"Feedback loop":         CheckPass,
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

func TestDiagnoseBounceDomain(t *testing.T) {
	// Template-derived, aligned bounce domain with MX → listener and SPF → egress.
	snap := diagnoseSnap()
	snap.BounceDomain = "bounce.kumo.fallback.net" // global fallback (misaligned)
	snap.BounceDomainTemplate = "bounce.kumo.{domain}"
	snap.LogStreamRedisURL = "redis://r:6379"

	dns := fakeResolver{
		mx:   map[string][]*net.MX{"bounce.kumo.example.com": {{Host: "edge.example.net.", Pref: 10}}},
		host: map[string][]string{"edge.example.net": {"203.0.113.5"}}, // checkMX strips the MX trailing dot; == listener IP
		txt:  map[string][]string{"bounce.kumo.example.com": {"v=spf1 ip4:198.51.100.7 -all"}},
	}
	uc := NewDiagnoseUsecase(fakeLoader{snap: snap}, dns, nil)
	res, err := uc.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "news@example.com"})
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	// Return-path is the per-domain template value and aligns with example.com.
	rp := itemByName(res.Items, "Bounce return-path")
	if rp == nil || rp.Status != CheckPass {
		t.Fatalf("bounce return-path: want pass, got %+v", rp)
	}
	if len(rp.Records) != 1 || rp.Records[0] != "bounce.kumo.example.com" {
		t.Fatalf("bounce return-path domain: %+v", rp.Records)
	}
	if mx := itemByName(res.Items, "Bounce MX (bounce.kumo.example.com)"); mx == nil || mx.Status != CheckPass {
		t.Fatalf("bounce MX: want pass, got %+v", mx)
	}
	if spf := itemByName(res.Items, "Bounce SPF (bounce.kumo.example.com)"); spf == nil || spf.Status != CheckPass {
		t.Fatalf("bounce SPF: want pass, got %+v", spf)
	}

	// No template and a misaligned global bounce domain → warn (DMARC on DKIM only).
	snap2 := diagnoseSnap()
	snap2.BounceDomain = "bounce.kumo.fallback.net"
	uc2 := NewDiagnoseUsecase(fakeLoader{snap: snap2}, fakeResolver{}, nil)
	res2, err := uc2.Diagnose(ownerCheckCtx(), DiagnoseRequest{FromEmail: "news@example.com"})
	if err != nil {
		t.Fatalf("diagnose2: %v", err)
	}
	if rp := itemByName(res2.Items, "Bounce return-path"); rp == nil || rp.Status != CheckWarn {
		t.Fatalf("misaligned bounce return-path: want warn, got %+v", rp)
	}
}

func TestDiagnoseDefaultRoutingWhenNoMatch(t *testing.T) {
	uc := NewDiagnoseUsecase(fakeLoader{snap: diagnoseSnap()}, fakeResolver{}, nil)
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
