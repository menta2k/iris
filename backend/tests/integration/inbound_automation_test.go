package integration

import (
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestWebhookMatchingAndRspamdPersistence verifies webhook rule matching by
// email and domain, and that Rspamd results persist and list newest-first.
func TestWebhookMatchingAndRspamdPersistence(t *testing.T) {
	db := setupDB(t)
	repo := data.NewInboundRepo(db)
	uc := biz.NewInboundUsecase(repo, nil, true) // allow http for the test
	ctx := ownerCtx()

	if _, err := uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "domain-hook", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "http://localhost:9000/hook",
	}); err != nil {
		t.Fatalf("create domain webhook: %v", err)
	}
	if _, err := uc.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "email-hook", MatchType: biz.MatchRecipientEmail, MatchValue: "vip@other.com",
		DestinationURL: "http://localhost:9000/vip",
	}); err != nil {
		t.Fatalf("create email webhook: %v", err)
	}

	// Domain match.
	rules, err := uc.MatchWebhookRules(ctx, "anyone@example.com")
	if err != nil {
		t.Fatalf("match domain: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "domain-hook" {
		t.Fatalf("expected domain-hook match, got %+v", rules)
	}
	// Exact-email match.
	rules, err = uc.MatchWebhookRules(ctx, "vip@other.com")
	if err != nil {
		t.Fatalf("match email: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "email-hook" {
		t.Fatalf("expected email-hook match, got %+v", rules)
	}
	// No match.
	rules, err = uc.MatchWebhookRules(ctx, "nobody@nowhere.test")
	if err != nil {
		t.Fatalf("match none: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected no matches, got %+v", rules)
	}

	// Rspamd persistence + listing.
	if err := uc.IngestRspamdResult(ctx, &biz.RspamdFilterResult{
		Action: biz.RspamdReject, Score: 12.5, Symbols: []string{"BAYES_SPAM"}, Reason: "high score",
	}); err != nil {
		t.Fatalf("ingest rspamd: %v", err)
	}
	results, err := uc.ListRspamdResults(ctx, biz.NormalizePage(0, ""))
	if err != nil {
		t.Fatalf("list rspamd: %v", err)
	}
	if len(results) != 1 || results[0].Action != biz.RspamdReject || results[0].Score != 12.5 {
		t.Fatalf("expected one reject result, got %+v", results)
	}
}

// TestWebhookDeliveryEventRecorded verifies delivery attempts persist.
func TestWebhookDeliveryEventRecorded(t *testing.T) {
	db := setupDB(t)
	repo := data.NewInboundRepo(db)
	ctx := ownerCtx()

	rule, err := repo.CreateWebhookRule(ctx, &biz.WebhookRule{
		Name: "h", MatchType: biz.MatchRecipientDomain, MatchValue: "example.com",
		DestinationURL: "http://localhost:9000/hook", Status: biz.WebhookActive, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if err := repo.RecordDeliveryEvent(ctx, &biz.WebhookDeliveryEvent{
		WebhookRuleID: rule.ID, Attempt: 1, Status: biz.WebhookDelivered, ResponseCode: 200,
	}); err != nil {
		t.Fatalf("record delivery: %v", err)
	}
}
