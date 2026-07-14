package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListVMTAs returns configured VMTAs (US1).
func (s *Service) ListVMTAs(ctx context.Context, req *adminv1.ListVMTAsRequest) (*adminv1.ListVMTAsReply, error) {
	if s.outbound == nil {
		return nil, notImplemented("ListVMTAs")
	}
	page := pageFrom(req.GetPage())
	items, err := s.outbound.ListVMTAs(ctx, req.GetStatus(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListVMTAs", err)
	}
	out := &adminv1.ListVMTAsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, v := range items {
		out.Items = append(out.Items, vmtaToProto(v))
	}
	return out, nil
}

// CreateVMTA creates a VMTA (US1).
func (s *Service) CreateVMTA(ctx context.Context, req *adminv1.CreateVMTARequest) (*adminv1.VMTA, error) {
	if s.outbound == nil {
		return nil, notImplemented("CreateVMTA")
	}
	v, err := s.outbound.CreateVMTA(ctx, &biz.VMTA{
		Name:           req.GetName(),
		IPAddress:      req.GetIpAddress(),
		EHLOName:       req.GetEhloName(),
		ListenerID:     req.GetListenerId(),
		MaxConnections: int(req.GetMaxConnections()),
		TLSMode:        req.GetTlsMode(),
		NodeID:         req.GetNodeId(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateVMTA", err)
	}
	return vmtaToProto(v), nil
}

// UpdateVMTA updates an existing VMTA (US1).
func (s *Service) UpdateVMTA(ctx context.Context, req *adminv1.UpdateVMTARequest) (*adminv1.VMTA, error) {
	if s.outbound == nil {
		return nil, notImplemented("UpdateVMTA")
	}
	v, err := s.outbound.UpdateVMTA(ctx, req.GetId(), &biz.VMTA{
		Name:           req.GetName(),
		IPAddress:      req.GetIpAddress(),
		EHLOName:       req.GetEhloName(),
		ListenerID:     req.GetListenerId(),
		MaxConnections: int(req.GetMaxConnections()),
		Status:         req.GetStatus(),
		Notes:          req.GetNotes(),
		TLSMode:        req.GetTlsMode(),
		NodeID:         req.GetNodeId(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateVMTA", err)
	}
	return vmtaToProto(v), nil
}

// ListVMTAGroups returns configured VMTA groups (US1).
func (s *Service) ListVMTAGroups(ctx context.Context, req *adminv1.ListVMTAGroupsRequest) (*adminv1.ListVMTAGroupsReply, error) {
	if s.outbound == nil {
		return nil, notImplemented("ListVMTAGroups")
	}
	page := pageFrom(req.GetPage())
	groups, err := s.outbound.ListVMTAGroups(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListVMTAGroups", err)
	}
	out := &adminv1.ListVMTAGroupsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(groups))}}
	for _, g := range groups {
		out.Items = append(out.Items, groupToProto(g))
	}
	return out, nil
}

// CreateVMTAGroups creates a VMTA group (US1).
func (s *Service) CreateVMTAGroups(ctx context.Context, req *adminv1.CreateVMTAGroupRequest) (*adminv1.VMTAGroup, error) {
	if s.outbound == nil {
		return nil, notImplemented("CreateVMTAGroups")
	}
	g := &biz.VMTAGroup{Name: req.GetName()}
	for _, m := range req.GetMembers() {
		g.Members = append(g.Members, biz.VMTAGroupMember{VMTAID: m.GetVmtaId(), Weight: int(m.GetWeight())})
	}
	out, err := s.outbound.CreateVMTAGroup(ctx, g)
	if err != nil {
		return nil, s.fail(ctx, "CreateVMTAGroups", err)
	}
	return groupToProto(out), nil
}

// UpdateVMTAGroup updates an existing VMTA group (US1).
func (s *Service) UpdateVMTAGroup(ctx context.Context, req *adminv1.UpdateVMTAGroupRequest) (*adminv1.VMTAGroup, error) {
	if s.outbound == nil {
		return nil, notImplemented("UpdateVMTAGroup")
	}
	g := &biz.VMTAGroup{Name: req.GetName(), Status: req.GetStatus()}
	for _, m := range req.GetMembers() {
		g.Members = append(g.Members, biz.VMTAGroupMember{VMTAID: m.GetVmtaId(), Weight: int(m.GetWeight())})
	}
	out, err := s.outbound.UpdateVMTAGroup(ctx, req.GetId(), g)
	if err != nil {
		return nil, s.fail(ctx, "UpdateVMTAGroup", err)
	}
	return groupToProto(out), nil
}

// ListRoutingRules returns routing rules (US1).
func (s *Service) ListRoutingRules(ctx context.Context, req *adminv1.ListRoutingRulesRequest) (*adminv1.ListRoutingRulesReply, error) {
	if s.outbound == nil {
		return nil, notImplemented("ListRoutingRules")
	}
	page := pageFrom(req.GetPage())
	rules, err := s.outbound.ListRoutingRules(ctx, req.GetMatchType(), req.GetMatchValue(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListRoutingRules", err)
	}
	out := &adminv1.ListRoutingRulesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(rules))}}
	for _, r := range rules {
		out.Items = append(out.Items, routingToProto(r))
	}
	return out, nil
}

// CreateRoutingRule creates a routing rule (US1).
func (s *Service) CreateRoutingRule(ctx context.Context, req *adminv1.CreateRoutingRuleRequest) (*adminv1.RoutingRule, error) {
	if s.outbound == nil {
		return nil, notImplemented("CreateRoutingRule")
	}
	out, err := s.outbound.CreateRoutingRule(ctx, &biz.RoutingRule{
		Name:            req.GetName(),
		MatchType:       req.GetMatchType(),
		MatchHeader:     req.GetMatchHeader(),
		MatchValue:      req.GetMatchValue(),
		Conditions:      routingConditionsFromProto(req.GetConditions()),
		Priority:        int(req.GetPriority()),
		TargetType:      req.GetTargetType(),
		TargetID:        req.GetTargetId(),
		AssignMailclass: req.GetAssignMailclass(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateRoutingRule", err)
	}
	return routingToProto(out), nil
}

// UpdateRoutingRule updates an existing routing rule (US1).
func (s *Service) UpdateRoutingRule(ctx context.Context, req *adminv1.UpdateRoutingRuleRequest) (*adminv1.RoutingRule, error) {
	if s.outbound == nil {
		return nil, notImplemented("UpdateRoutingRule")
	}
	out, err := s.outbound.UpdateRoutingRule(ctx, req.GetId(), &biz.RoutingRule{
		Name:            req.GetName(),
		MatchType:       req.GetMatchType(),
		MatchHeader:     req.GetMatchHeader(),
		MatchValue:      req.GetMatchValue(),
		Conditions:      routingConditionsFromProto(req.GetConditions()),
		Priority:        int(req.GetPriority()),
		TargetType:      req.GetTargetType(),
		TargetID:        req.GetTargetId(),
		AssignMailclass: req.GetAssignMailclass(),
		Status:          req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateRoutingRule", err)
	}
	return routingToProto(out), nil
}

func vmtaToProto(v *biz.VMTA) *adminv1.VMTA {
	return &adminv1.VMTA{
		Id: v.ID, Name: v.Name, IpAddress: v.IPAddress, EhloName: v.EHLOName, Status: v.Status, Notes: v.Notes,
		ListenerId: v.ListenerID, ListenerName: v.ListenerName, MaxConnections: int32(v.MaxConnections),
		TlsMode: v.TLSMode, NodeId: v.NodeID, NodeName: v.NodeName,
	}
}

// ListListeners returns ESMTP listeners (Listeners).
func (s *Service) ListListeners(ctx context.Context, req *adminv1.ListListenersRequest) (*adminv1.ListListenersReply, error) {
	if s.outbound == nil {
		return nil, notImplemented("ListListeners")
	}
	page := pageFrom(req.GetPage())
	items, err := s.outbound.ListListeners(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListListeners", err)
	}
	out := &adminv1.ListListenersReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, l := range items {
		out.Items = append(out.Items, listenerToProto(l))
	}
	return out, nil
}

// CreateListener creates an ESMTP listener.
func (s *Service) CreateListener(ctx context.Context, req *adminv1.CreateListenerRequest) (*adminv1.Listener, error) {
	if s.outbound == nil {
		return nil, notImplemented("CreateListener")
	}
	out, err := s.outbound.CreateListener(ctx, listenerFromCreate(req))
	if err != nil {
		return nil, s.fail(ctx, "CreateListener", err)
	}
	return listenerToProto(out), nil
}

// UpdateListener updates an ESMTP listener.
func (s *Service) UpdateListener(ctx context.Context, req *adminv1.UpdateListenerRequest) (*adminv1.Listener, error) {
	if s.outbound == nil {
		return nil, notImplemented("UpdateListener")
	}
	out, err := s.outbound.UpdateListener(ctx, req.GetId(), &biz.Listener{
		Name: req.GetName(), IPAddress: req.GetIpAddress(), Port: int(req.GetPort()),
		Hostname: req.GetHostname(), TLSEnabled: req.GetTlsEnabled(),
		TLSCertPath: req.GetTlsCertPath(), TLSKeyPath: req.GetTlsKeyPath(),
		RequireAuth: req.GetRequireAuth(), MaxMessageSize: req.GetMaxMessageSize(),
		RelayHosts: req.GetRelayHosts(), Status: req.GetStatus(), Role: req.GetRole(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateListener", err)
	}
	return listenerToProto(out), nil
}

func listenerFromCreate(req *adminv1.CreateListenerRequest) *biz.Listener {
	return &biz.Listener{
		Name: req.GetName(), IPAddress: req.GetIpAddress(), Port: int(req.GetPort()),
		Hostname: req.GetHostname(), TLSEnabled: req.GetTlsEnabled(),
		TLSCertPath: req.GetTlsCertPath(), TLSKeyPath: req.GetTlsKeyPath(),
		RequireAuth: req.GetRequireAuth(), MaxMessageSize: req.GetMaxMessageSize(),
		RelayHosts: req.GetRelayHosts(), Role: req.GetRole(),
	}
}

func listenerToProto(l *biz.Listener) *adminv1.Listener {
	return &adminv1.Listener{
		Id: l.ID, Name: l.Name, IpAddress: l.IPAddress, Port: int32(l.Port), Hostname: l.Hostname,
		TlsEnabled: l.TLSEnabled, TlsCertPath: l.TLSCertPath, TlsKeyPath: l.TLSKeyPath,
		RequireAuth: l.RequireAuth, MaxMessageSize: l.MaxMessageSize, RelayHosts: l.RelayHosts,
		Status: l.Status, Role: l.Role,
	}
}

func groupToProto(g *biz.VMTAGroup) *adminv1.VMTAGroup {
	out := &adminv1.VMTAGroup{Id: g.ID, Name: g.Name, Status: g.Status}
	for _, m := range g.Members {
		out.Members = append(out.Members, &adminv1.VMTAGroupMember{VmtaId: m.VMTAID, Weight: int32(m.Weight)})
	}
	return out
}

func routingToProto(r *biz.RoutingRule) *adminv1.RoutingRule {
	out := &adminv1.RoutingRule{
		Id: r.ID, Name: r.Name, MatchType: r.MatchType, MatchHeader: r.MatchHeader, MatchValue: r.MatchValue,
		Priority: int32(r.Priority), TargetType: r.TargetType, TargetId: r.TargetID,
		AssignMailclass: r.AssignMailclass, Status: r.Status,
	}
	for _, c := range r.Conditions {
		out.Conditions = append(out.Conditions, &adminv1.RoutingMatchCondition{Header: c.Header, Value: c.Value})
	}
	return out
}

// routingConditionsFromProto maps proto match conditions to the biz model.
func routingConditionsFromProto(in []*adminv1.RoutingMatchCondition) []biz.RoutingMatchCondition {
	if len(in) == 0 {
		return nil
	}
	out := make([]biz.RoutingMatchCondition, 0, len(in))
	for _, c := range in {
		out = append(out, biz.RoutingMatchCondition{Header: c.GetHeader(), Value: c.GetValue()})
	}
	return out
}
