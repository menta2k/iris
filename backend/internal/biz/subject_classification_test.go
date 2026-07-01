package biz

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeSubject(t *testing.T) {
	cases := map[string]string{
		"Your order #12345 has shipped":   "your order has shipped",
		"Your order #67890 has shipped":   "your order has shipped",
		"RE: Re: Password reset":          "password reset",
		"Fwd: Invoice 2024-0007":          "invoice",
		"  Newsletter!!!  ":               "newsletter",
		"12345 67890":                     "",
		"FW:  Meeting  at  3pm  tomorrow": "meeting at pm tomorrow",
	}
	for in, want := range cases {
		if got := NormalizeSubject(in); got != want {
			t.Errorf("NormalizeSubject(%q) = %q, want %q", in, got, want)
		}
	}
	// Two subjects differing only by order number must collapse to one key.
	if NormalizeSubject("Your order #12345 has shipped") != NormalizeSubject("Your order #99 has shipped") {
		t.Error("subjects differing only by digits should normalize equally")
	}
}

func TestNormalizeLabel(t *testing.T) {
	cases := map[string]string{
		"Order Confirmation":    "order confirmation",
		"\"shipping update\"":   "shipping update",
		"password reset please": "password reset", // truncated to 2 words
		"Invoice.":              "invoice",
		"":                      "",
	}
	for in, want := range cases {
		if got := normalizeLabel(in); got != want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubjectClassificationValidate(t *testing.T) {
	// Valid rule normalizes subject/label and defaults source.
	c := &SubjectClassification{Subject: "Your Order #5 Shipped", Label: "Shipping Update Now"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.SubjectNormalized != "your order shipped" {
		t.Errorf("normalized = %q", c.SubjectNormalized)
	}
	if c.Label != "shipping update" { // truncated to 2 words
		t.Errorf("label = %q", c.Label)
	}
	if c.Source != ClassificationSourceManual {
		t.Errorf("source = %q, want manual", c.Source)
	}
	// Empty subject and label are rejected.
	if err := (&SubjectClassification{Subject: "", Label: "x"}).Validate(); err == nil {
		t.Error("empty subject should be rejected")
	}
	if err := (&SubjectClassification{Subject: "hello", Label: ""}).Validate(); err == nil {
		t.Error("empty label should be rejected")
	}
	// A subject that normalizes to nothing is rejected.
	if err := (&SubjectClassification{Subject: "12345", Label: "x"}).Validate(); err == nil {
		t.Error("digit-only subject should be rejected")
	}
}

// --- classifier usecase (fakes) ---

type fakeClassRepo struct {
	match     *SubjectClassification
	upserted  *SubjectClassification
	incrCalls int
}

func (f *fakeClassRepo) BestMatch(context.Context, string, float64) (*SubjectClassification, error) {
	return f.match, nil
}
func (f *fakeClassRepo) Upsert(_ context.Context, c *SubjectClassification) (*SubjectClassification, error) {
	f.upserted = c
	return c, nil
}
func (f *fakeClassRepo) IncrementHit(context.Context, string) error { f.incrCalls++; return nil }
func (f *fakeClassRepo) List(context.Context) ([]*SubjectClassification, error) {
	return nil, nil
}
func (f *fakeClassRepo) Create(_ context.Context, c *SubjectClassification) (*SubjectClassification, error) {
	return c, nil
}
func (f *fakeClassRepo) Update(_ context.Context, c *SubjectClassification) (*SubjectClassification, error) {
	return c, nil
}
func (f *fakeClassRepo) Delete(context.Context, string) error { return nil }

type fakeAI struct {
	reply  string
	err    error
	called int
}

func (f *fakeAI) ClassifySubject(context.Context, string, string, string) (string, error) {
	f.called++
	return f.reply, f.err
}

type fixedPolicy ClassifyPolicy

func (p fixedPolicy) ClassifyPolicyNow(context.Context) ClassifyPolicy { return ClassifyPolicy(p) }

func onPolicy() fixedPolicy {
	return fixedPolicy{Enabled: true, Model: "m", Threshold: 0.4, APIBase: "http://x"}
}

func TestClassifierDisabled(t *testing.T) {
	repo := &fakeClassRepo{}
	ai := &fakeAI{reply: "spam"}
	uc := NewSubjectClassifierUsecase(repo, ai, fixedPolicy{Enabled: false})
	got, err := uc.Classify(context.Background(), "anything")
	if err != nil || got != "" {
		t.Fatalf("disabled: got %q err %v, want empty", got, err)
	}
	if ai.called != 0 {
		t.Error("AI must not be called when disabled")
	}
}

func TestClassifierTrigramHit(t *testing.T) {
	repo := &fakeClassRepo{match: &SubjectClassification{ID: "1", Label: "invoice"}}
	ai := &fakeAI{reply: "should-not-be-used"}
	uc := NewSubjectClassifierUsecase(repo, ai, onPolicy())
	got, err := uc.Classify(context.Background(), "Invoice #42")
	if err != nil || got != "invoice" {
		t.Fatalf("got %q err %v, want invoice", got, err)
	}
	if ai.called != 0 {
		t.Error("AI must not be called on a trigram hit")
	}
	if repo.incrCalls != 1 {
		t.Errorf("hit counter bumps = %d, want 1", repo.incrCalls)
	}
}

func TestClassifierAIFallback(t *testing.T) {
	repo := &fakeClassRepo{match: nil}
	ai := &fakeAI{reply: "Order Confirmation Extra"}
	uc := NewSubjectClassifierUsecase(repo, ai, onPolicy())
	got, err := uc.Classify(context.Background(), "Thanks for your order 900")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "order confirmation" { // normalized + truncated to 2 words
		t.Fatalf("got %q, want order confirmation", got)
	}
	if ai.called != 1 {
		t.Errorf("AI calls = %d, want 1", ai.called)
	}
	if repo.upserted == nil || repo.upserted.Label != "order confirmation" ||
		repo.upserted.Source != ClassificationSourceAI {
		t.Errorf("expected AI result cached, got %+v", repo.upserted)
	}
}

func TestClassifierNoAIConfigured(t *testing.T) {
	repo := &fakeClassRepo{match: nil}
	uc := NewSubjectClassifierUsecase(repo, nil, onPolicy()) // nil AI
	got, err := uc.Classify(context.Background(), "novel subject line")
	if err != nil || got != "" {
		t.Fatalf("no-AI: got %q err %v, want empty", got, err)
	}
	if repo.upserted != nil {
		t.Error("nothing should be cached without AI")
	}
}

func TestClassifierAIError(t *testing.T) {
	repo := &fakeClassRepo{match: nil}
	ai := &fakeAI{err: errors.New("boom")}
	uc := NewSubjectClassifierUsecase(repo, ai, onPolicy())
	if _, err := uc.Classify(context.Background(), "subject"); err == nil {
		t.Error("expected the AI transport error to propagate")
	}
}
