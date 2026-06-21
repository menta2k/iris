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
	assertReason(t, (&GlobalSettings{FBLDomain: "fbl.example.com"}).Validate(), "")
	assertReason(t, (&GlobalSettings{FBLDomain: "not a domain"}).Validate(), "SETTINGS_FBL_DOMAIN_INVALID")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: 5}).Validate(), "")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: -1}).Validate(), "SETTINGS_SOFT_THRESHOLD_RANGE")
	assertReason(t, (&GlobalSettings{SoftBounceThreshold: 1001}).Validate(), "SETTINGS_SOFT_THRESHOLD_RANGE")
}

func TestGlobalSettingsNormalizes(t *testing.T) {
	g := &GlobalSettings{RspamdMode: "  ENFORCE ", RspamdURL: " http://r:1 ", EgressEHLODomain: " MAIL.Example.COM "}
	if err := g.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.RspamdMode != "enforce" || g.EgressEHLODomain != "mail.example.com" {
		t.Fatalf("not normalized: %+v", g)
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
