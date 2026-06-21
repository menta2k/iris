package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/data"
)

// TestAcmeDnsProviderRoundTrip exercises the singleton DNS-01 provider store:
// default empty, save a provider + config, read it back, then clear.
func TestAcmeDnsProviderRoundTrip(t *testing.T) {
	db := setupDB(t)
	repo := data.NewAcmeRepo(db)
	ctx := context.Background()

	// Default row is empty (created by the migration).
	p, err := repo.GetDnsProvider(ctx)
	if err != nil {
		t.Fatalf("get default: %v", err)
	}
	if p.Configured() {
		t.Fatalf("expected unconfigured by default, got %+v", p)
	}

	// Save a cloudflare config.
	cfg := map[string]string{"dnsApiToken": "secret-token", "dnsTTL": "300"}
	if err := repo.SaveDnsProvider(ctx, "cloudflare", cfg, "tester"); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := repo.GetDnsProvider(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Provider != "cloudflare" || got.Config["dnsApiToken"] != "secret-token" || got.Config["dnsTTL"] != "300" {
		t.Fatalf("unexpected stored provider: %+v", got)
	}

	// Clear resets it.
	if err := repo.ClearDnsProvider(ctx, "tester"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	cleared, err := repo.GetDnsProvider(ctx)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if cleared.Configured() {
		t.Fatalf("expected cleared, got %+v", cleared)
	}
}
