package biz

import (
	"context"
	"strings"
	"testing"
)

func TestRoutingRuleValidate(t *testing.T) {
	base := func() RoutingRule {
		return RoutingRule{Name: "r1", MatchType: MatchMailclass, MatchValue: "bulk", TargetType: TargetVMTA, TargetID: "v1"}
	}
	tests := []struct {
		name    string
		mutate  func(*RoutingRule)
		wantErr string
	}{
		{"valid defaults priority", func(r *RoutingRule) {}, ""},
		{"missing name", func(r *RoutingRule) { r.Name = "" }, "ROUTING_NAME_REQUIRED"},
		{"bad match type", func(r *RoutingRule) { r.MatchType = "bad" }, "ROUTING_MATCH_TYPE_INVALID"},
		{"missing match value", func(r *RoutingRule) { r.MatchValue = "" }, "ROUTING_MATCH_VALUE_REQUIRED"},
		{"bad target type", func(r *RoutingRule) { r.TargetType = "bad" }, "ROUTING_TARGET_TYPE_INVALID"},
		{"missing target", func(r *RoutingRule) { r.TargetID = "" }, "ROUTING_TARGET_REQUIRED"},
		{"priority too high", func(r *RoutingRule) { r.Priority = 5000 }, "ROUTING_PRIORITY_RANGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if r.Priority != 100 {
					t.Fatalf("expected default priority 100, got %d", r.Priority)
				}
				return
			}
			de, ok := AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain error, got %v", err)
			}
			if de.Reason != tt.wantErr {
				t.Fatalf("expected reason %q, got %q", tt.wantErr, de.Reason)
			}
		})
	}
}

func TestMailclassMatchHeaderDefaultsAndPreservesCase(t *testing.T) {
	// A mailclass rule with no header defaults to X-Mail-Class and preserves
	// the value's case (header values can be case-sensitive).
	r := RoutingRule{Name: "promo", MatchType: MatchMailclass, MatchValue: "PromoBlast",
		TargetType: TargetVMTA, TargetID: "v1"}
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MatchHeader != DefaultMailClassHeader {
		t.Fatalf("expected default header %q, got %q", DefaultMailClassHeader, r.MatchHeader)
	}
	if r.MatchValue != "PromoBlast" {
		t.Fatalf("mailclass value case should be preserved, got %q", r.MatchValue)
	}

	// A custom header is honored; an invalid header name is rejected.
	r2 := RoutingRule{Name: "c", MatchType: MatchMailclass, MatchHeader: "X-Campaign-Type",
		MatchValue: "v", TargetType: TargetVMTA, TargetID: "v1"}
	if err := r2.Validate(); err != nil || r2.MatchHeader != "X-Campaign-Type" {
		t.Fatalf("custom header not honored: header=%q err=%v", r2.MatchHeader, err)
	}
	bad := RoutingRule{Name: "c", MatchType: MatchMailclass, MatchHeader: "bad header",
		MatchValue: "v", TargetType: TargetVMTA, TargetID: "v1"}
	assertReason(t, bad.Validate(), "ROUTING_MATCH_HEADER_INVALID")

	// Recipient matches clear the header and lowercase the value.
	rcpt := RoutingRule{Name: "r", MatchType: MatchRecipientEmail, MatchHeader: "ignored",
		MatchValue: "Test@Test.com", TargetType: TargetVMTA, TargetID: "v1"}
	if err := rcpt.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rcpt.MatchHeader != "" || rcpt.MatchValue != "test@test.com" {
		t.Fatalf("recipient match should clear header + lowercase value: %+v", rcpt)
	}
}

func TestRoutingHigherPriorityWinsInRender(t *testing.T) {
	// The user's scenario: a recipient rule and a mailclass rule. The renderer
	// orders ROUTES by descending priority so the higher-priority rule is
	// evaluated (and wins) first.
	snap := ConfigSnapshot{
		VMTAs: []*VMTA{
			{ID: "v1", Name: "vmta-1", ListenerID: "lst-1", IPAddress: "203.0.113.1", EHLOName: "a.example.com", Status: VMTAStatusActive},
			{ID: "v2", Name: "vmta-2", ListenerID: "lst-1", IPAddress: "203.0.113.2", EHLOName: "b.example.com", Status: VMTAStatusActive},
		},
		Routes: []*RoutingRule{
			{ID: "r1", Name: "by-recipient", MatchType: MatchRecipientEmail, MatchValue: "test@test.com",
				Priority: 200, TargetType: TargetVMTA, TargetID: "v1", Status: RoutingStatusActive},
			{ID: "r2", Name: "by-mailclass", MatchType: MatchMailclass, MatchHeader: "X-Mail-Class", MatchValue: "blah",
				Priority: 100, TargetType: TargetVMTA, TargetID: "v2", Status: RoutingStatusActive},
		},
	}
	r, err := RenderKumoConfig(snap)
	if err != nil || !r.Valid {
		t.Fatalf("render: err=%v valid=%v issues=%v", err, r.Valid, r.LintIssues)
	}
	recipIdx := strings.Index(r.Content, `match_value = "test@test.com"`)
	classIdx := strings.Index(r.Content, `match_value = "blah"`)
	if recipIdx < 0 || classIdx < 0 || recipIdx > classIdx {
		t.Fatalf("higher-priority recipient rule should be listed first:\n%s", r.Content)
	}
	// The mailclass rule reads its configured header.
	if !strings.Contains(r.Content, `msg:get_first_named_header_value(route.match_header)`) {
		t.Fatalf("mailclass match must read the rule's header:\n%s", r.Content)
	}
}

func TestRoutingRuleNormalizesMatchValue(t *testing.T) {
	r := RoutingRule{Name: "r", MatchType: MatchRecipientDomain, MatchValue: "  Example.COM ", TargetType: TargetVMTAGroup, TargetID: "g1"}
	if err := r.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MatchValue != "example.com" {
		t.Fatalf("expected normalized match value, got %q", r.MatchValue)
	}
}

// fakeOutboundRepo is an in-memory OutboundConfigRepo for use-case tests.
type fakeOutboundRepo struct {
	vmtas     map[string]bool
	listeners map[string]bool
	targets   map[string]bool
	created   []*RoutingRule
}

func (f *fakeOutboundRepo) CreateListener(_ context.Context, l *Listener) (*Listener, error) {
	l.ID = "lst-1"
	return l, nil
}
func (f *fakeOutboundRepo) UpdateListener(_ context.Context, id string, l *Listener) (*Listener, error) {
	l.ID = id
	return l, nil
}
func (f *fakeOutboundRepo) ListListeners(context.Context, Page) ([]*Listener, error) { return nil, nil }
func (f *fakeOutboundRepo) ListenerExists(_ context.Context, id string) (bool, error) {
	return f.listeners[id], nil
}
func (f *fakeOutboundRepo) CreateVMTA(_ context.Context, v *VMTA) (*VMTA, error) {
	v.ID = "vmta-1"
	return v, nil
}
func (f *fakeOutboundRepo) UpdateVMTA(_ context.Context, id string, v *VMTA) (*VMTA, error) {
	v.ID = id
	return v, nil
}
func (f *fakeOutboundRepo) ListVMTAs(context.Context, string, Page) ([]*VMTA, error) { return nil, nil }
func (f *fakeOutboundRepo) VMTAExists(_ context.Context, id string) (bool, error) {
	return f.vmtas[id], nil
}
func (f *fakeOutboundRepo) CreateVMTAGroup(_ context.Context, g *VMTAGroup) (*VMTAGroup, error) {
	g.ID = "group-1"
	return g, nil
}
func (f *fakeOutboundRepo) UpdateVMTAGroup(_ context.Context, id string, g *VMTAGroup) (*VMTAGroup, error) {
	g.ID = id
	return g, nil
}
func (f *fakeOutboundRepo) ListVMTAGroups(context.Context, Page) ([]*VMTAGroup, error) {
	return nil, nil
}
func (f *fakeOutboundRepo) CreateRoutingRule(_ context.Context, r *RoutingRule) (*RoutingRule, error) {
	r.ID = "rule-1"
	f.created = append(f.created, r)
	return r, nil
}
func (f *fakeOutboundRepo) UpdateRoutingRule(_ context.Context, id string, r *RoutingRule) (*RoutingRule, error) {
	r.ID = id
	f.created = append(f.created, r)
	return r, nil
}
func (f *fakeOutboundRepo) ListRoutingRules(context.Context, string, string, Page) ([]*RoutingRule, error) {
	return nil, nil
}
func (f *fakeOutboundRepo) TargetExists(_ context.Context, targetType, id string) (bool, error) {
	return f.targets[targetType+":"+id], nil
}

// ownerCtx returns a context with a full-permission identity.
func ownerCtx() context.Context {
	return WithIdentity(context.Background(), &Identity{
		UserID:      "tester",
		Permissions: NewPermissionSet([]string{string(PermAll)}),
		MFAVerified: true,
	})
}

func TestCreateRoutingRuleRequiresExistingTarget(t *testing.T) {
	repo := &fakeOutboundRepo{targets: map[string]bool{}}
	uc := NewOutboundConfigUsecase(repo, nil)
	_, err := uc.CreateRoutingRule(ownerCtx(), &RoutingRule{
		Name: "r", MatchType: MatchMailclass, MatchValue: "bulk", TargetType: TargetVMTA, TargetID: "missing",
	})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "ROUTING_TARGET_MISSING" {
		t.Fatalf("expected ROUTING_TARGET_MISSING, got %v", err)
	}
}

func TestCreateRoutingRuleUnauthorized(t *testing.T) {
	repo := &fakeOutboundRepo{}
	uc := NewOutboundConfigUsecase(repo, nil)
	// Identity with only read permission must be denied write.
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet([]string{string(PermRoutingRead)}),
		MFAVerified: true,
	})
	_, err := uc.CreateRoutingRule(ctx, &RoutingRule{
		Name: "r", MatchType: MatchMailclass, MatchValue: "bulk", TargetType: TargetVMTA, TargetID: "v1",
	})
	de, ok := AsDomainError(err)
	if !ok || de.Kind != KindForbidden {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestCreateRoutingRuleSucceeds(t *testing.T) {
	repo := &fakeOutboundRepo{targets: map[string]bool{"vmta:v1": true}}
	uc := NewOutboundConfigUsecase(repo, nil)
	out, err := uc.CreateRoutingRule(ownerCtx(), &RoutingRule{
		Name: "r", MatchType: MatchMailclass, MatchValue: "bulk", TargetType: TargetVMTA, TargetID: "v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "rule-1" {
		t.Fatalf("expected persisted rule id, got %q", out.ID)
	}
}
