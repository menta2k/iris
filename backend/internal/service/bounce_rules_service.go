package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListBounceRules returns the bounce classification & response ruleset.
func (s *Service) ListBounceRules(ctx context.Context, _ *adminv1.ListBounceRulesRequest) (*adminv1.ListBounceRulesReply, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("ListBounceRules")
	}
	items, err := s.bounceRules.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListBounceRules", err)
	}
	return bounceRulesReply(items), nil
}

// CreateBounceRule adds an overlay rule.
func (s *Service) CreateBounceRule(ctx context.Context, req *adminv1.CreateBounceRuleRequest) (*adminv1.BounceRule, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("CreateBounceRule")
	}
	out, err := s.bounceRules.Create(ctx, &biz.BounceActionRule{
		SMTPCode: req.GetSmtpCode(), EnhancedCode: req.GetEnhancedCode(), Provider: req.GetProvider(),
		Pattern: req.GetPattern(), Class: req.GetClass(), Category: req.GetCategory(), Action: req.GetAction(),
		ActionConfig: req.GetActionConfig(), SuggestedAction: req.GetSuggestedAction(), Priority: int(req.GetPriority()),
		MinAttempts: int(req.GetMinAttempts()), SuppressTTL: req.GetSuppressTtl(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateBounceRule", err)
	}
	return bounceRuleToProto(out), nil
}

// UpdateBounceRule edits a rule.
func (s *Service) UpdateBounceRule(ctx context.Context, req *adminv1.UpdateBounceRuleRequest) (*adminv1.BounceRule, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("UpdateBounceRule")
	}
	out, err := s.bounceRules.Update(ctx, req.GetId(), &biz.BounceActionRule{
		SMTPCode: req.GetSmtpCode(), EnhancedCode: req.GetEnhancedCode(), Provider: req.GetProvider(),
		Pattern: req.GetPattern(), Class: req.GetClass(), Category: req.GetCategory(), Action: req.GetAction(),
		ActionConfig: req.GetActionConfig(), SuggestedAction: req.GetSuggestedAction(), Priority: int(req.GetPriority()),
		MinAttempts: int(req.GetMinAttempts()), SuppressTTL: req.GetSuppressTtl(),
		Status: req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateBounceRule", err)
	}
	return bounceRuleToProto(out), nil
}

// DeleteBounceRule removes a rule.
func (s *Service) DeleteBounceRule(ctx context.Context, req *adminv1.DeleteBounceRuleRequest) (*adminv1.DeleteBounceRuleReply, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("DeleteBounceRule")
	}
	if err := s.bounceRules.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteBounceRule", err)
	}
	return &adminv1.DeleteBounceRuleReply{}, nil
}

// ResetBounceRules restores the curated defaults (keeping overlay rules).
func (s *Service) ResetBounceRules(ctx context.Context, _ *adminv1.ResetBounceRulesRequest) (*adminv1.ListBounceRulesReply, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("ResetBounceRules")
	}
	items, err := s.bounceRules.ResetToDefaults(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ResetBounceRules", err)
	}
	return bounceRulesReply(items), nil
}

// TestBounceDiagnostic matches a bounce against the active ruleset.
func (s *Service) TestBounceDiagnostic(ctx context.Context, req *adminv1.TestBounceDiagnosticRequest) (*adminv1.TestBounceDiagnosticReply, error) {
	if s.bounceRules == nil {
		return nil, notImplemented("TestBounceDiagnostic")
	}
	res, err := s.bounceRules.TestDiagnostic(ctx, biz.BounceSignature{
		SMTPCode: req.GetSmtpCode(), Domain: req.GetDomain(), Diagnostic: req.GetDiagnostic(),
		Attempts: int(req.GetAttempts()),
	})
	if err != nil {
		return nil, s.fail(ctx, "TestBounceDiagnostic", err)
	}
	reply := &adminv1.TestBounceDiagnosticReply{
		SmtpCode:        res.Signature.SMTPCode,
		EnhancedCode:    res.Signature.EnhancedCode,
		Provider:        res.Signature.Provider,
		Matched:         res.Matched != nil,
		EffectiveAction: biz.BounceActionRetry,
	}
	if res.Matched != nil {
		reply.Rule = bounceRuleToProto(res.Matched)
		reply.EffectiveAction = res.Matched.Action
	}
	return reply, nil
}

func bounceRulesReply(items []*biz.BounceActionRule) *adminv1.ListBounceRulesReply {
	out := &adminv1.ListBounceRulesReply{}
	for _, r := range items {
		out.Items = append(out.Items, bounceRuleToProto(r))
	}
	return out
}

func bounceRuleToProto(r *biz.BounceActionRule) *adminv1.BounceRule {
	p := &adminv1.BounceRule{
		Id:              r.ID,
		SmtpCode:        r.SMTPCode,
		EnhancedCode:    r.EnhancedCode,
		Provider:        r.Provider,
		Pattern:         r.Pattern,
		Class:           r.Class,
		Category:        r.Category,
		Action:          r.Action,
		ActionConfig:    r.ActionConfig,
		SuggestedAction: r.SuggestedAction,
		Priority:        int32(r.Priority),
		MinAttempts:     int32(r.MinAttempts),
		SuppressTtl:     r.SuppressTTL,
		Source:          r.Source,
		Status:          r.Status,
	}
	if !r.CreatedAt.IsZero() {
		p.CreatedAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !r.UpdatedAt.IsZero() {
		p.UpdatedAt = r.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return p
}
