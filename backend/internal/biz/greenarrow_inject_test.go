package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type captureInjector struct {
	last KumoInjectRequest
	err  error
	n    int
}

func (c *captureInjector) InjectV1(_ context.Context, req KumoInjectRequest) error {
	c.n++
	c.last = req
	return c.err
}

func sampleMessage() GAMessage {
	return GAMessage{
		HTML:      "<p>hi</p>",
		Text:      "hi",
		Subject:   "Welcome",
		To:        []GARecipient{{Email: "user@gmail.com", Name: "User"}},
		FromEmail: "news@example.com",
		FromName:  "Example News",
		Mailclass: "marketing",
		Headers:   []map[string]string{{"X-Feedback-ID": "user@gmail.com:1:1:acme"}},
	}
}

func TestInjectAuthenticate(t *testing.T) {
	uc := NewGreenArrowInjectUsecase(&captureInjector{}, "apiuser", "s3cret", "")
	if !uc.Authenticate("apiuser", "s3cret") {
		t.Error("valid credentials should authenticate")
	}
	if uc.Authenticate("apiuser", "wrong") {
		t.Error("wrong password must not authenticate")
	}
	if uc.Authenticate("nope", "s3cret") {
		t.Error("wrong username must not authenticate")
	}
	// Fail closed when credentials are unconfigured.
	empty := NewGreenArrowInjectUsecase(&captureInjector{}, "", "", "")
	if empty.Authenticate("", "") {
		t.Error("empty configured credentials must never authenticate")
	}
}

func TestInjectRejectsBadCredentials(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "apiuser", "s3cret", "")
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "bad", Message: sampleMessage()})
	de, ok := AsDomainError(err)
	if !ok || de.Kind != KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if inj.n != 0 {
		t.Error("must not forward to KumoMTA on auth failure")
	}
}

func TestInjectMapsToKumoPayload(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "apiuser", "s3cret", "")
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "s3cret", Message: sampleMessage()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := inj.last
	if got.EnvelopeSender != "news@example.com" {
		t.Errorf("envelope_sender = %q", got.EnvelopeSender)
	}
	if got.Content.Subject != "Welcome" || got.Content.HTMLBody != "<p>hi</p>" || got.Content.TextBody != "hi" {
		t.Errorf("content mismapped: %+v", got.Content)
	}
	if got.Content.From.Email != "news@example.com" || got.Content.From.Name != "Example News" {
		t.Errorf("from mismapped: %+v", got.Content.From)
	}
	if len(got.Recipients) != 1 || got.Recipients[0].Email != "user@gmail.com" || got.Recipients[0].Name != "User" {
		t.Errorf("recipients mismapped: %+v", got.Recipients)
	}
	// mailclass becomes the classification header.
	if got.Content.Headers[DefaultMailClassHeader] != "marketing" {
		t.Errorf("mailclass header = %q, want marketing", got.Content.Headers[DefaultMailClassHeader])
	}
	// Custom GreenArrow header is flattened through.
	if got.Content.Headers["X-Feedback-ID"] != "user@gmail.com:1:1:acme" {
		t.Errorf("X-Feedback-ID header missing: %+v", got.Content.Headers)
	}
}

// TestInjectPayloadJSONShape guards the KumoMTA contract: its Content builder
// uses deny_unknown_fields, so the serialized `content` must contain ONLY the
// builder's known keys (no "to" — the To header comes from recipients).
func TestInjectPayloadJSONShape(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "u", "p", "")
	if err := uc.Inject(context.Background(), &GAInjectRequest{Username: "u", Password: "p", Message: sampleMessage()}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := json.Marshal(inj.last)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"envelope_sender", "content", "recipients"} {
		if _, ok := top[k]; !ok {
			t.Errorf("payload missing top-level key %q", k)
		}
	}
	var content map[string]json.RawMessage
	if err := json.Unmarshal(top["content"], &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	allowed := map[string]bool{"subject": true, "text_body": true, "html_body": true, "from": true, "headers": true}
	for k := range content {
		if !allowed[k] {
			t.Errorf("content has key %q not allowed by KumoMTA's Content builder (deny_unknown_fields)", k)
		}
	}
	if _, ok := content["to"]; ok {
		t.Error("content must NOT contain a \"to\" field — the To header is built from recipients")
	}
}

func TestInjectMailclassHeaderNotOverridden(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "u", "p", "")
	msg := sampleMessage()
	// A custom header attempting to override the mailclass header must be ignored.
	msg.Headers = []map[string]string{{DefaultMailClassHeader: "spoofed"}}
	if err := uc.Inject(context.Background(), &GAInjectRequest{Username: "u", Password: "p", Message: msg}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inj.last.Content.Headers[DefaultMailClassHeader] != "marketing" {
		t.Errorf("mailclass header should stay 'marketing', got %q", inj.last.Content.Headers[DefaultMailClassHeader])
	}
}

func TestInjectValidation(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "u", "p", "")
	cases := map[string]func(*GAMessage){
		"no from":    func(m *GAMessage) { m.FromEmail = "" },
		"no subject": func(m *GAMessage) { m.Subject = "" },
		"no body":    func(m *GAMessage) { m.HTML = ""; m.Text = "" },
		"no rcpt":    func(m *GAMessage) { m.To = nil },
	}
	for name, mutate := range cases {
		msg := sampleMessage()
		mutate(&msg)
		err := uc.Inject(context.Background(), &GAInjectRequest{Username: "u", Password: "p", Message: msg})
		if de, ok := AsDomainError(err); !ok || de.Kind != KindInvalidArgument {
			t.Errorf("%s: expected invalid-argument, got %v", name, err)
		}
	}
}

func TestInjectPropagatesInjectorError(t *testing.T) {
	inj := &captureInjector{err: Unavailable("KUMO_INJECT_UNREACHABLE", "boom")}
	uc := NewGreenArrowInjectUsecase(inj, "u", "p", "")
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "u", Password: "p", Message: sampleMessage()})
	if de, ok := AsDomainError(err); !ok || de.Kind != KindUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if !errors.Is(err, err) { // sanity
		t.Fatal("unreachable")
	}
}

// --- DB-managed credential auth ---

type fakeCredStore struct {
	byName    map[string]*InjectionCredential
	touched   []string
	lookupErr error
}

func (f *fakeCredStore) ByUsername(_ context.Context, username string) (*InjectionCredential, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	return f.byName[username], nil
}
func (f *fakeCredStore) TouchLastUsed(_ context.Context, id string) error {
	f.touched = append(f.touched, id)
	return nil
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return h
}

func TestInjectDBCredentialSuccess(t *testing.T) {
	inj := &captureInjector{}
	store := &fakeCredStore{byName: map[string]*InjectionCredential{
		"apiuser": {ID: "c1", Username: "apiuser", PasswordHash: mustHash(t, "longenoughpw"), Enabled: true},
	}}
	// Static config credential is different; the DB one must authenticate.
	uc := NewGreenArrowInjectUsecase(inj, "static", "staticpassword", "").WithCredentialStore(store)
	msg := sampleMessage()
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "longenoughpw", Message: msg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inj.n != 1 {
		t.Errorf("message not injected")
	}
	if len(store.touched) != 1 || store.touched[0] != "c1" {
		t.Errorf("last_used not recorded: %v", store.touched)
	}
}

func TestInjectDBCredentialDisabled(t *testing.T) {
	inj := &captureInjector{}
	store := &fakeCredStore{byName: map[string]*InjectionCredential{
		"apiuser": {ID: "c1", Username: "apiuser", PasswordHash: mustHash(t, "longenoughpw"), Enabled: false},
	}}
	uc := NewGreenArrowInjectUsecase(inj, "", "", "").WithCredentialStore(store)
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "longenoughpw", Message: sampleMessage()})
	if de, ok := AsDomainError(err); !ok || de.Kind != KindUnauthorized {
		t.Fatalf("disabled credential must be rejected, got %v", err)
	}
	if inj.n != 0 {
		t.Error("disabled credential must not inject")
	}
}

func TestInjectConfigFallback(t *testing.T) {
	inj := &captureInjector{}
	store := &fakeCredStore{byName: map[string]*InjectionCredential{}} // no DB creds
	uc := NewGreenArrowInjectUsecase(inj, "static", "staticpassword", "").WithCredentialStore(store)
	// Unknown to the DB, but matches the static config credential.
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "static", Password: "staticpassword", Message: sampleMessage()})
	if err != nil {
		t.Fatalf("config fallback should authenticate: %v", err)
	}
	if inj.n != 1 || len(store.touched) != 0 {
		t.Errorf("config credential should inject without touching a DB row")
	}
}

func TestInjectMailclassRestriction(t *testing.T) {
	inj := &captureInjector{}
	store := &fakeCredStore{byName: map[string]*InjectionCredential{
		"apiuser": {ID: "c1", Username: "apiuser", PasswordHash: mustHash(t, "longenoughpw"), Enabled: true, AllowedMailclasses: []string{"acme_k"}},
	}}
	uc := NewGreenArrowInjectUsecase(inj, "", "", "").WithCredentialStore(store)

	// Allowed mailclass → injected.
	msg := sampleMessage()
	msg.Mailclass = "acme_k"
	if err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "longenoughpw", Message: msg}); err != nil {
		t.Fatalf("allowed mailclass should inject: %v", err)
	}

	// Disallowed mailclass → forbidden, not injected.
	inj.n = 0
	msg2 := sampleMessage()
	msg2.Mailclass = "otherclass"
	err := uc.Inject(context.Background(), &GAInjectRequest{Username: "apiuser", Password: "longenoughpw", Message: msg2})
	if de, ok := AsDomainError(err); !ok || de.Kind != KindForbidden {
		t.Fatalf("disallowed mailclass must be forbidden, got %v", err)
	}
	if inj.n != 0 {
		t.Error("forbidden mailclass must not inject")
	}
}

// TestInjectMailClassMultipleHeaders verifies a configured comma-separated
// header list stamps the mailclass into EVERY header, so HTTP-injected mail
// classifies against routing rules that key on any of them (the production
// X-Mail-Class vs X-GreenArrow mismatch fix).
func TestInjectMailClassMultipleHeaders(t *testing.T) {
	inj := &captureInjector{}
	uc := NewGreenArrowInjectUsecase(inj, "u", "p", "X-GreenArrow-MailClass, X-GreenArrow")
	err := uc.Inject(context.Background(), &GAInjectRequest{
		Username: "u", Password: "p",
		Message: GAMessage{
			Mailclass: "kmx-test",
			FromEmail: "no-reply@cars.bg",
			To:        []GARecipient{{Email: "vesco@jobs.bg"}},
			HTML:      "<p>hi</p>",
			Subject:   "test",
		},
	})
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	h := inj.last.Content.Headers
	if h["X-GreenArrow-MailClass"] != "kmx-test" || h["X-GreenArrow"] != "kmx-test" {
		t.Fatalf("both GreenArrow headers must carry the mailclass, got %+v", h)
	}
	// The old default header is NOT stamped when a custom list is configured.
	if _, ok := h[DefaultMailClassHeader]; ok {
		t.Errorf("X-Mail-Class should not be set when a custom header list is configured: %+v", h)
	}
}
