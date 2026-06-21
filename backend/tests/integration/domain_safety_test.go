package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestSuppressionOutboundEligibility verifies that an active suppression entry
// blocks a recipient from outbound eligibility, by exact email and by domain.
func TestSuppressionOutboundEligibility(t *testing.T) {
	db := setupDB(t)
	repo := data.NewDomainSafetyRepo(db)
	uc := biz.NewDomainSafetyUsecase(repo, nil)
	ctx := ownerCtx()

	if _, err := uc.CreateSuppression(ctx, &biz.SuppressionEntry{Type: biz.SuppressEmail, Value: "blocked@example.com"}); err != nil {
		t.Fatalf("create email suppression: %v", err)
	}
	if _, err := uc.CreateSuppression(ctx, &biz.SuppressionEntry{Type: biz.SuppressDomain, Value: "blocked.example"}); err != nil {
		t.Fatalf("create domain suppression: %v", err)
	}

	cases := map[string]bool{
		"blocked@example.com":    false, // exact email suppressed
		"anyone@blocked.example": false, // domain suppressed
		"ok@example.com":         true,  // eligible
	}
	for recipient, wantEligible := range cases {
		eligible, err := uc.IsRecipientEligible(ctx, recipient)
		if err != nil {
			t.Fatalf("eligibility %s: %v", recipient, err)
		}
		if eligible != wantEligible {
			t.Fatalf("recipient %s eligibility = %v, want %v", recipient, eligible, wantEligible)
		}
	}
}

// TestDKIMSecretRedaction verifies the private key (now PEM material) is never
// returned to API callers via the use case.
func TestDKIMSecretRedaction(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewDomainSafetyUsecase(data.NewDomainSafetyRepo(db), nil)
	ctx := ownerCtx()

	keyPEM, err := biz.GenerateDKIMPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	created, err := uc.CreateDKIMDomain(ctx, &biz.DKIMDomain{
		Domain: "example.com", Selector: "s1", PrivateKeyRef: keyPEM, Status: biz.DKIMReady,
	})
	if err != nil {
		t.Fatalf("create dkim: %v", err)
	}
	if created.PrivateKeyRef != "" {
		t.Fatalf("private key must be stripped from create response")
	}
	// The fingerprint IS derived and returned.
	if created.PublicKeyFingerprint == "" {
		t.Fatalf("expected a derived public key fingerprint")
	}
	list, err := uc.ListDKIMDomains(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list dkim: %v", err)
	}
	for _, d := range list {
		if d.PrivateKeyRef != "" {
			t.Fatalf("private key ref must not be exposed in list response")
		}
	}
}
