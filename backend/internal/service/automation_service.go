package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListAutomationRules returns all TSA automation rules.
func (s *Service) ListAutomationRules(ctx context.Context, _ *adminv1.ListAutomationRulesRequest) (*adminv1.ListAutomationRulesReply, error) {
	if s.automation == nil {
		return nil, notImplemented("ListAutomationRules")
	}
	items, err := s.automation.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListAutomationRules", err)
	}
	out := &adminv1.ListAutomationRulesReply{}
	for _, a := range items {
		out.Items = append(out.Items, automationToProto(a))
	}
	return out, nil
}

// CreateAutomationRule adds a rule.
func (s *Service) CreateAutomationRule(ctx context.Context, req *adminv1.CreateAutomationRuleRequest) (*adminv1.AutomationRule, error) {
	if s.automation == nil {
		return nil, notImplemented("CreateAutomationRule")
	}
	out, err := s.automation.Create(ctx, &biz.AutomationRule{
		Domain: req.GetDomain(), Regex: req.GetRegex(), Action: req.GetAction(),
		ConfigName: req.GetConfigName(), ConfigValue: req.GetConfigValue(),
		Trigger: req.GetTrigger(), Duration: req.GetDuration(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateAutomationRule", err)
	}
	return automationToProto(out), nil
}

// UpdateAutomationRule edits a rule.
func (s *Service) UpdateAutomationRule(ctx context.Context, req *adminv1.UpdateAutomationRuleRequest) (*adminv1.AutomationRule, error) {
	if s.automation == nil {
		return nil, notImplemented("UpdateAutomationRule")
	}
	out, err := s.automation.Update(ctx, req.GetId(), &biz.AutomationRule{
		Domain: req.GetDomain(), Regex: req.GetRegex(), Action: req.GetAction(),
		ConfigName: req.GetConfigName(), ConfigValue: req.GetConfigValue(),
		Trigger: req.GetTrigger(), Duration: req.GetDuration(), Status: req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateAutomationRule", err)
	}
	return automationToProto(out), nil
}

// SetAutomationRuleStatus enables/disables a rule.
func (s *Service) SetAutomationRuleStatus(ctx context.Context, req *adminv1.SetAutomationRuleStatusRequest) (*adminv1.AutomationRule, error) {
	if s.automation == nil {
		return nil, notImplemented("SetAutomationRuleStatus")
	}
	out, err := s.automation.SetStatus(ctx, req.GetId(), req.GetStatus())
	if err != nil {
		return nil, s.fail(ctx, "SetAutomationRuleStatus", err)
	}
	return automationToProto(out), nil
}

func automationToProto(a *biz.AutomationRule) *adminv1.AutomationRule {
	p := &adminv1.AutomationRule{
		Id:          a.ID,
		Domain:      a.Domain,
		Regex:       a.Regex,
		Action:      a.Action,
		ConfigName:  a.ConfigName,
		ConfigValue: a.ConfigValue,
		Trigger:     a.Trigger,
		Duration:    a.Duration,
		Status:      a.Status,
	}
	if !a.CreatedAt.IsZero() {
		p.CreatedAt = a.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !a.UpdatedAt.IsZero() {
		p.UpdatedAt = a.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return p
}
