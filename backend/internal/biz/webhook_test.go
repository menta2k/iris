package biz

import "testing"

func TestWebhookRuleValidate(t *testing.T) {
	valid := func() WebhookRule {
		return WebhookRule{Name: "w", MatchType: MatchRecipientDomain, MatchValue: "example.com", DestinationURL: "https://hooks.example.com/in"}
	}
	// HTTPS valid.
	w := valid()
	if err := w.Validate(false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.TimeoutSeconds != 10 || w.RetryPolicy.MaxAttempts != 5 {
		t.Fatalf("expected defaults applied, got %+v", w)
	}

	tests := []struct {
		name          string
		mutate        func(*WebhookRule)
		allowInsecure bool
		wantErr       string
	}{
		{"missing name", func(w *WebhookRule) { w.Name = "" }, false, "WEBHOOK_NAME_REQUIRED"},
		{"bad match type", func(w *WebhookRule) { w.MatchType = "mailclass" }, false, "WEBHOOK_MATCH_TYPE_INVALID"},
		{"bad url", func(w *WebhookRule) { w.DestinationURL = "://nope" }, false, "WEBHOOK_URL_INVALID"},
		{"http rejected", func(w *WebhookRule) { w.DestinationURL = "http://hooks.example.com" }, false, "WEBHOOK_URL_INSECURE"},
		{"http allowed in dev", func(w *WebhookRule) { w.DestinationURL = "http://localhost:9000" }, true, ""},
		{"inline secret", func(w *WebhookRule) { w.SecretRef = "-----BEGIN SECRET" }, false, "WEBHOOK_SECRET_INLINE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := valid()
			tt.mutate(&w)
			assertReason(t, w.Validate(tt.allowInsecure), tt.wantErr)
		})
	}
}

func TestWebhookDeliveryStatesExist(t *testing.T) {
	for _, s := range []string{WebhookPending, WebhookDelivered, WebhookRetrying, WebhookFailed, WebhookCancelled} {
		if s == "" {
			t.Fatal("empty webhook state constant")
		}
	}
}
