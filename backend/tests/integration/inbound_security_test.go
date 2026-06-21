package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestWebhookURLValidationAndSecretRedaction verifies that insecure (non-HTTPS)
// destinations and inline secret material are rejected, and that webhook secret
// refs are never returned by the list endpoint.
func TestWebhookURLValidationAndSecretRedaction(t *testing.T) {
	db := setupDB(t)
	// allowInsecure=false: production-like, HTTPS required.
	uc := biz.NewInboundUsecase(data.NewInboundRepo(db), nil, false)
	ctx := ownerCtx()

	_, err := uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "insecure", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "http://plain.example.com/hook",
	})
	if de, ok := biz.AsDomainError(err); !ok || de.Reason != "WEBHOOK_URL_INSECURE" {
		t.Fatalf("expected WEBHOOK_URL_INSECURE, got %v", err)
	}

	_, err = uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "inline-secret", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "https://hooks.example.com", SecretRef: "-----BEGIN KEY-----",
	})
	if de, ok := biz.AsDomainError(err); !ok || de.Reason != "WEBHOOK_SECRET_INLINE" {
		t.Fatalf("expected WEBHOOK_SECRET_INLINE, got %v", err)
	}

	// A valid rule with a secret ref must not leak the secret on list.
	if _, err := uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "ok", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "https://hooks.example.com/in", SecretRef: "vault://hooks/example",
	}); err != nil {
		t.Fatalf("create valid webhook: %v", err)
	}
	list, err := uc.ListWebhookRules(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	for _, w := range list {
		if w.SecretRef != "" {
			t.Fatalf("secret ref must not be exposed in list response")
		}
	}
}

// TestUnauthorizedWebhookWriteDenied verifies write permission is enforced.
func TestUnauthorizedWebhookWriteDenied(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewInboundUsecase(data.NewInboundRepo(db), nil, true)
	ctx := biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "00000000-0000-0000-0000-000000000003",
		Permissions: biz.NewPermissionSet([]string{string(biz.PermWebhookRead)}),
		MFAVerified: true,
	})
	_, err := uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "x", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "https://hooks.example.com",
	})
	if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
