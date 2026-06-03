package service

import (
	"context"
	"errors"
	"testing"
)

// fakeDnsCfgStore is a minimal AcmeDnsProviderConfigStore for testing the
// merge semantics of UpsertDnsProviderConfig.
type fakeDnsCfgStore struct {
	existing   map[string]map[string]string // provider -> stored config
	lastUpsert AcmeDnsProviderConfigRow
}

func (f *fakeDnsCfgStore) List(context.Context) ([]AcmeDnsProviderConfigRow, error) {
	return nil, nil
}
func (f *fakeDnsCfgStore) Get(_ context.Context, provider string) (*AcmeDnsProviderConfigRow, error) {
	cfg, ok := f.existing[provider]
	if !ok {
		return nil, errors.New("not found")
	}
	return &AcmeDnsProviderConfigRow{Provider: provider, Config: cfg}, nil
}
func (f *fakeDnsCfgStore) Upsert(_ context.Context, in AcmeDnsProviderConfigRow, _ string) (*AcmeDnsProviderConfigRow, error) {
	f.lastUpsert = in
	return &in, nil
}
func (f *fakeDnsCfgStore) Delete(context.Context, string) error { return nil }

func newAcmeForTest(store AcmeDnsProviderConfigStore) *AcmeService {
	return NewAcmeService(nil, nil, store, nil, "")
}

// A blank submitted field must keep the stored secret; a non-empty field
// overwrites. This is what lets the UI edit one credential without ever
// seeing (or re-typing) the others.
func TestUpsertDnsProviderConfigMergeKeepsExistingSecrets(t *testing.T) {
	store := &fakeDnsCfgStore{existing: map[string]map[string]string{
		"cloudflare": {"dnsApiToken": "OLD-TOKEN", "zoneId": "Z1"},
	}}
	svc := newAcmeForTest(store)

	if _, err := svc.UpsertDnsProviderConfig(context.Background(), AcmeDnsProviderConfigRow{
		Provider: "cloudflare",
		Config:   map[string]string{"dnsApiToken": "", "zoneId": "Z2"},
	}, "tester"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got := store.lastUpsert.Config
	if got["dnsApiToken"] != "OLD-TOKEN" {
		t.Fatalf("blank field must keep stored secret, got %q", got["dnsApiToken"])
	}
	if got["zoneId"] != "Z2" {
		t.Fatalf("non-empty field must overwrite, got %q", got["zoneId"])
	}
}

func TestUpsertDnsProviderConfigNewProviderStoresProvided(t *testing.T) {
	store := &fakeDnsCfgStore{existing: map[string]map[string]string{}}
	svc := newAcmeForTest(store)

	if _, err := svc.UpsertDnsProviderConfig(context.Background(), AcmeDnsProviderConfigRow{
		Provider: "cloudflare",
		Config:   map[string]string{"dnsApiToken": "NEW"},
	}, "tester"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if store.lastUpsert.Config["dnsApiToken"] != "NEW" {
		t.Fatalf("new provider must store provided value, got %q", store.lastUpsert.Config["dnsApiToken"])
	}
}

func TestUpsertDnsProviderConfigRejectsUnknownProvider(t *testing.T) {
	svc := newAcmeForTest(&fakeDnsCfgStore{existing: map[string]map[string]string{}})
	if _, err := svc.UpsertDnsProviderConfig(context.Background(), AcmeDnsProviderConfigRow{
		Provider: "definitely-not-a-provider",
		Config:   map[string]string{"x": "y"},
	}, "tester"); err == nil {
		t.Fatal("expected unknown-provider rejection")
	}
}
