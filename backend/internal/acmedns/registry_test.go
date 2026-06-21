package acmedns

import "testing"

func TestRegistryListsAllProviders(t *testing.T) {
	names := ListProviders()
	want := []string{
		"acmedns", "clouddns", "cloudflare", "digitalocean", "easydns",
		"gcloud", "httpreq", "hurricane", "pdns", "route53",
	}
	if len(names) != len(want) {
		t.Fatalf("expected %d providers, got %d: %v", len(want), len(names), names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("provider %d: want %q got %q (not sorted?)", i, n, names[i])
		}
	}
}

func TestGetProviderValidatesRequiredFields(t *testing.T) {
	// cloudflare requires dnsApiToken.
	if _, err := GetProvider("cloudflare", map[string]string{}); err == nil {
		t.Fatal("expected error for missing required field dnsApiToken")
	}
	if _, err := GetProvider("cloudflare", map[string]string{"dnsApiToken": "tok"}); err != nil {
		t.Fatalf("cloudflare with token should construct: %v", err)
	}
}

func TestGetProviderUnknown(t *testing.T) {
	if _, err := GetProvider("nope", map[string]string{}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if IsRegistered("nope") {
		t.Fatal("unknown provider should not be registered")
	}
	if !IsRegistered("route53") {
		t.Fatal("route53 should be registered")
	}
}

func TestProviderInfoFields(t *testing.T) {
	info, err := GetProviderInfo("route53")
	if err != nil {
		t.Fatalf("get info: %v", err)
	}
	if len(info.RequiredFields) != 3 {
		t.Fatalf("route53 should have 3 required fields, got %v", info.RequiredFields)
	}
	// Mutating the returned slice must not affect the registry.
	info.RequiredFields[0] = "mutated"
	again, _ := GetProviderInfo("route53")
	if again.RequiredFields[0] == "mutated" {
		t.Fatal("GetProviderInfo must return a deep copy")
	}
}

func TestBadIntConfigRejected(t *testing.T) {
	if _, err := GetProvider("cloudflare", map[string]string{"dnsApiToken": "t", "dnsTTL": "banana"}); err == nil {
		t.Fatal("expected error for non-integer dnsTTL")
	}
}
