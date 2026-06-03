package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

// TestDnsCfgToHTTPRedactsSecrets is the guard for the security requirement:
// DNS provider credential VALUES must never appear in an API response. The
// response carries only the names of configured keys.
func TestDnsCfgToHTTPRedactsSecrets(t *testing.T) {
	row := service.AcmeDnsProviderConfigRow{
		Provider: "cloudflare",
		Config: map[string]string{
			"dnsApiToken": "super-secret-token",
			"zoneId":      "Z123",
			"unset":       "",
		},
		UpdatedBy: "alice",
	}

	out := dnsCfgToHTTP(row)

	// Only non-empty keys, sorted; no values.
	want := []string{"dnsApiToken", "zoneId"}
	if !reflect.DeepEqual(out.ConfiguredKeys, want) {
		t.Fatalf("ConfiguredKeys = %v, want %v", out.ConfiguredKeys, want)
	}

	// The serialized response must not contain any secret value anywhere.
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, secret := range []string{"super-secret-token", "Z123"} {
		if strings.Contains(js, secret) {
			t.Fatalf("secret value %q leaked in response JSON: %s", secret, js)
		}
	}
}
