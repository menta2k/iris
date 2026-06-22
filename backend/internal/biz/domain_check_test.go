package biz

import (
	"context"
	"net"
	"testing"
)

type fakeResolver struct {
	mx   map[string][]*net.MX
	txt  map[string][]string
	host map[string][]string
}

func (f fakeResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return f.mx[name], nil
}
func (f fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	return f.txt[name], nil
}
func (f fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	return f.host[host], nil
}

type fakeLoader struct{ snap ConfigSnapshot }

func (f fakeLoader) Snapshot(context.Context) (ConfigSnapshot, error) { return f.snap, nil }

func ownerCheckCtx() context.Context {
	return WithIdentity(context.Background(), &Identity{
		UserID: "u", Permissions: NewPermissionSet([]string{string(PermAll)}), MFAVerified: true,
	})
}

func itemByName(items []CheckItem, name string) *CheckItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func TestDomainCheckAllPass(t *testing.T) {
	loader := fakeLoader{snap: ConfigSnapshot{
		Listeners: []*Listener{{IPAddress: "203.0.113.5"}},
		VMTAs:     []*VMTA{{IPAddress: "198.51.100.7"}},
		DKIM:      []*DKIMDomain{{Domain: "kmx.example.com", Selector: "default"}},
	}}
	dns := fakeResolver{
		mx:   map[string][]*net.MX{"kmx.example.com": {{Host: "mx.kmx.example.com.", Pref: 10}}},
		host: map[string][]string{"mx.kmx.example.com": {"203.0.113.5"}},
		txt: map[string][]string{
			"kmx.example.com":                   {"v=spf1 ip4:198.51.100.7 -all"},
			"default._domainkey.kmx.example.com": {"v=DKIM1; k=rsa; p=MIIBI..."},
		},
	}
	res, err := NewDomainCheckUsecase(loader, dns).Check(ownerCheckCtx(), "kmx.example.com")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, name := range []string{"MX", "SPF", "DKIM (default)"} {
		it := itemByName(res.Items, name)
		if it == nil || it.Status != CheckPass {
			t.Fatalf("%s: want pass, got %+v", name, it)
		}
	}
}

func TestDomainCheckFailures(t *testing.T) {
	loader := fakeLoader{snap: ConfigSnapshot{
		Listeners: []*Listener{{IPAddress: "203.0.113.5"}},
		VMTAs:     []*VMTA{{IPAddress: "198.51.100.7"}},
	}}
	dns := fakeResolver{
		// MX points elsewhere; SPF authorizes a different IP; no DKIM configured.
		mx:   map[string][]*net.MX{"d.example": {{Host: "mx.other.net.", Pref: 10}}},
		host: map[string][]string{"mx.other.net": {"10.10.10.10"}},
		txt:  map[string][]string{"d.example": {"v=spf1 ip4:9.9.9.9 -all"}},
	}
	res, err := NewDomainCheckUsecase(loader, dns).Check(ownerCheckCtx(), "d.example")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if it := itemByName(res.Items, "MX"); it.Status != CheckWarn {
		t.Fatalf("MX: want warn (points elsewhere), got %+v", it)
	}
	if it := itemByName(res.Items, "SPF"); it.Status != CheckFail {
		t.Fatalf("SPF: want fail (egress not authorized), got %+v", it)
	}
	if it := itemByName(res.Items, "DKIM"); it.Status != CheckWarn {
		t.Fatalf("DKIM: want warn (none configured), got %+v", it)
	}
}

func TestDomainCheckNoMXNoSPF(t *testing.T) {
	res, err := NewDomainCheckUsecase(fakeLoader{}, fakeResolver{}).Check(ownerCheckCtx(), "bare.example")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if itemByName(res.Items, "MX").Status != CheckFail {
		t.Fatal("MX should fail with no records")
	}
	if itemByName(res.Items, "SPF").Status != CheckFail {
		t.Fatal("SPF should fail with no record")
	}
}
