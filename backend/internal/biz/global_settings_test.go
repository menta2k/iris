package biz

import (
	"context"
	"testing"
)

func TestGlobalSettingsValidate(t *testing.T) {
	// Empty is valid (all fields optional).
	assertReason(t, (&GlobalSettings{}).Validate(), "")

	// rspamd enabled requires an http(s) URL.
	assertReason(t, (&GlobalSettings{RspamdMode: "enforce"}).Validate(), "SETTINGS_RSPAMD_URL_INVALID")
	assertReason(t, (&GlobalSettings{RspamdMode: "tag", RspamdURL: "ftp://x"}).Validate(), "SETTINGS_RSPAMD_URL_INVALID")
	assertReason(t, (&GlobalSettings{RspamdMode: "enforce", RspamdURL: "http://rspamd:11334"}).Validate(), "")
	assertReason(t, (&GlobalSettings{RspamdMode: "bogus"}).Validate(), "SETTINGS_RSPAMD_MODE_INVALID")

	// off mode does not require a URL.
	assertReason(t, (&GlobalSettings{RspamdMode: "off"}).Validate(), "")

	// Other field validation.
	assertReason(t, (&GlobalSettings{EgressEHLODomain: "not valid host"}).Validate(), "SETTINGS_EHLO_INVALID")
	assertReason(t, (&GlobalSettings{LogStreamRedisURL: "http://nope"}).Validate(), "SETTINGS_REDIS_URL_INVALID")
	assertReason(t, (&GlobalSettings{LogStreamRedisURL: "redis://redis:6379"}).Validate(), "")
	assertReason(t, (&GlobalSettings{EsmtpListen: "noport"}).Validate(), "SETTINGS_ESMTP_LISTEN_INVALID")
	assertReason(t, (&GlobalSettings{HTTPListen: "0.0.0.0:8000"}).Validate(), "")

	// Delivery-rate durations use KumoMTA syntax.
	assertReason(t, (&GlobalSettings{EgressRetryInterval: "20m"}).Validate(), "")
	assertReason(t, (&GlobalSettings{EgressMaxRetryInterval: "2h"}).Validate(), "")
	assertReason(t, (&GlobalSettings{EgressMaxAge: "7d"}).Validate(), "")
	assertReason(t, (&GlobalSettings{EgressRetryInterval: "20 minutes"}).Validate(), "SETTINGS_DURATION_INVALID")
	assertReason(t, (&GlobalSettings{EgressMaxAge: "soon"}).Validate(), "SETTINGS_DURATION_INVALID")

	// Bounce/DSN pipeline fields.
	assertReason(t, (&GlobalSettings{BounceDomain: "bounce.example.com"}).Validate(), "")
	assertReason(t, (&GlobalSettings{BounceDomain: "not a domain"}).Validate(), "SETTINGS_BOUNCE_DOMAIN_INVALID")
	assertReason(t, (&GlobalSettings{FBLDomains: []string{"fbl.example.com"}}).Validate(), "")
	assertReason(t, (&GlobalSettings{FBLDomains: []string{"fbl.a.example", "fbl.b.example"}}).Validate(), "")
	assertReason(t, (&GlobalSettings{FBLDomains: []string{"not a domain"}}).Validate(), "SETTINGS_FBL_DOMAIN_INVALID")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: 5}).Validate(), "")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: -1}).Validate(), "SETTINGS_SOFT_THRESHOLD_RANGE")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: 1001}).Validate(), "SETTINGS_SOFT_THRESHOLD_RANGE")
}

func TestGlobalSettingsNormalizes(t *testing.T) {
	g := &GlobalSettings{
		RspamdMode: "  ENFORCE ", RspamdURL: " http://r:1 ", EgressEHLODomain: " MAIL.Example.COM ",
		// FBL domains are trimmed, lower-cased, de-duped, and empties dropped.
		FBLDomains: []string{" FBL.example.COM ", "fbl.example.com", "fbl2.example.com", "  "},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.RspamdMode != "enforce" || g.EgressEHLODomain != "mail.example.com" {
		t.Fatalf("not normalized: %+v", g)
	}
	wantFBL := []string{"fbl.example.com", "fbl2.example.com"}
	if len(g.FBLDomains) != len(wantFBL) {
		t.Fatalf("FBL domains: got %v want %v", g.FBLDomains, wantFBL)
	}
	for i, d := range wantFBL {
		if g.FBLDomains[i] != d {
			t.Fatalf("FBL domains: got %v want %v", g.FBLDomains, wantFBL)
		}
	}
}

// fakeSettingsRepo backs the Effective-merge test.
type fakeSettingsRepo struct{ row *GlobalSettings }

func (f *fakeSettingsRepo) Get(context.Context) (*GlobalSettings, error) { return f.row, nil }
func (f *fakeSettingsRepo) Update(_ context.Context, in *GlobalSettings, _ string) (*GlobalSettings, error) {
	f.row = in
	return in, nil
}

func TestEffectiveMergesOverDefaults(t *testing.T) {
	// Stored rspamd mode/url override the defaults; empty fields fall back.
	repo := &fakeSettingsRepo{row: &GlobalSettings{RspamdMode: "tag", RspamdURL: "http://r:1"}}
	uc := NewGlobalSettingsUsecase(repo, nil, KumoConfigSettings{
		RspamdMode: "off", RspamdURL: "http://default", LogStreamName: "iris.mail.events",
	})
	eff, err := uc.Effective(context.Background())
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if eff.RspamdMode != "tag" || eff.RspamdURL != "http://r:1" {
		t.Fatalf("stored settings should override defaults: %+v", eff)
	}
	if eff.LogStreamName != "iris.mail.events" {
		t.Fatalf("empty stored field should fall back to default: %+v", eff)
	}
}

func TestParseFlexDuration(t *testing.T) {
	ok := map[string]string{"12h": "12h0m0s", "30d": "720h0m0s", "1h30m": "1h30m0s", "90s": "1m30s"}
	for in, want := range ok {
		d, valid := ParseFlexDuration(in)
		if !valid || d.String() != want {
			t.Errorf("ParseFlexDuration(%q) = %v,%v want %s", in, d, valid, want)
		}
	}
	for _, bad := range []string{"", "banana", "10", "5x", "h"} {
		if _, valid := ParseFlexDuration(bad); valid {
			t.Errorf("ParseFlexDuration(%q) should be invalid", bad)
		}
	}
}

func TestAdminSettingsValidation(t *testing.T) {
	// TLS enabled without a cert domain is rejected.
	s := &GlobalSettings{AdminTLSEnabled: true}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: TLS enabled without cert domain")
	}
	// Bad bind address rejected.
	s = &GlobalSettings{AdminHTTPAddr: "not-a-host-port"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: bad admin_http_addr")
	}
	// Bad renew duration rejected.
	s = &GlobalSettings{AcmeRenewInterval: "soon"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: bad acme_renew_interval")
	}
	// Valid config accepted.
	s = &GlobalSettings{AdminHTTPAddr: ":8080", AdminTLSEnabled: true, AdminTLSCertDomain: "*.kmx.jobs.bg", AcmeRenewInterval: "12h", AcmeRenewBefore: "30d"}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}
