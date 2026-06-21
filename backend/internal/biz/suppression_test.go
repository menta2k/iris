package biz

import (
	"context"
	"testing"
)

func TestSuppressionValidateAndNormalize(t *testing.T) {
	s := &SuppressionEntry{Type: "EMAIL", Value: "  User@Example.COM "}
	if err := s.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Value != "user@example.com" {
		t.Fatalf("expected normalized value, got %q", s.Value)
	}
	if s.Status != SuppressActive || s.Source != "manual" {
		t.Fatalf("expected defaults, got status=%q source=%q", s.Status, s.Source)
	}
}

func TestSuppressionValidateErrors(t *testing.T) {
	assertReason(t, (&SuppressionEntry{Type: "email", Value: "no-at-sign"}).Validate(), "SUPPRESSION_EMAIL_INVALID")
	assertReason(t, (&SuppressionEntry{Type: "domain", Value: "a@b.com"}).Validate(), "SUPPRESSION_DOMAIN_INVALID")
	assertReason(t, (&SuppressionEntry{Type: "other", Value: "x"}).Validate(), "SUPPRESSION_TYPE_INVALID")
}

func TestSuppressionMatching(t *testing.T) {
	email := &SuppressionEntry{Type: SuppressEmail, Value: "user@example.com", Status: SuppressActive}
	if !email.MatchesSuppression("User@Example.com") {
		t.Fatal("email suppression should match case-insensitively")
	}
	if email.MatchesSuppression("other@example.com") {
		t.Fatal("email suppression should not match a different address")
	}

	domain := &SuppressionEntry{Type: SuppressDomain, Value: "example.com", Status: SuppressActive}
	if !domain.MatchesSuppression("anyone@example.com") {
		t.Fatal("domain suppression should match the domain")
	}
	if domain.MatchesSuppression("anyone@other.com") {
		t.Fatal("domain suppression should not match a different domain")
	}

	disabled := &SuppressionEntry{Type: SuppressEmail, Value: "user@example.com", Status: SuppressDisabled}
	if disabled.MatchesSuppression("user@example.com") {
		t.Fatal("disabled suppression must not match")
	}
}

func TestRecipientDomain(t *testing.T) {
	if RecipientDomain("User@Example.COM") != "example.com" {
		t.Fatal("should extract lowercased domain")
	}
	if RecipientDomain("nobody") != "" {
		t.Fatal("no-at-sign should return empty domain")
	}
}

// fakeEligibility implements RecipientEligibilityChecker for the integration test.
type fakeEligibility struct{ blocked map[string]bool }

func (f fakeEligibility) IsRecipientEligible(_ context.Context, recipient string) (bool, error) {
	return !f.blocked[recipient], nil
}

func TestOutboundEvaluateRecipientUsesSuppression(t *testing.T) {
	uc := NewOutboundConfigUsecase(&fakeOutboundRepo{}, nil).
		WithEligibilityChecker(fakeEligibility{blocked: map[string]bool{"blocked@example.com": true}})
	ok, err := uc.EvaluateRecipient(ownerCtx(), "blocked@example.com")
	if err != nil || ok {
		t.Fatalf("expected blocked recipient ineligible, got ok=%v err=%v", ok, err)
	}
	ok, err = uc.EvaluateRecipient(ownerCtx(), "ok@example.com")
	if err != nil || !ok {
		t.Fatalf("expected allowed recipient eligible, got ok=%v err=%v", ok, err)
	}
}
