package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListMTANodes returns all registered cluster nodes.
func (s *Service) ListMTANodes(ctx context.Context, req *adminv1.ListMTANodesRequest) (*adminv1.ListMTANodesReply, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("ListMTANodes")
	}
	items, err := s.mtaNodes.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListMTANodes", err)
	}
	out := &adminv1.ListMTANodesReply{}
	for _, n := range items {
		out.Items = append(out.Items, mtaNodeToProto(n))
	}
	return out, nil
}

// GetMTANode returns one node by id.
func (s *Service) GetMTANode(ctx context.Context, req *adminv1.GetMTANodeRequest) (*adminv1.MTANode, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("GetMTANode")
	}
	n, err := s.mtaNodes.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "GetMTANode", err)
	}
	return mtaNodeToProto(n), nil
}

// CreateMTANode registers a cluster node.
func (s *Service) CreateMTANode(ctx context.Context, req *adminv1.CreateMTANodeRequest) (*adminv1.MTANode, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("CreateMTANode")
	}
	out, err := s.mtaNodes.Create(ctx, &biz.MTANode{
		Name:      req.GetName(),
		AgentURL:  req.GetAgentUrl(),
		ProxyHost: req.GetProxyHost(),
		ProxyPort: int(req.GetProxyPort()),
		Status:    req.GetStatus(),
		Notes:     req.GetNotes(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateMTANode", err)
	}
	return mtaNodeToProto(out), nil
}

// UpdateMTANode edits operator-owned node fields.
func (s *Service) UpdateMTANode(ctx context.Context, req *adminv1.UpdateMTANodeRequest) (*adminv1.MTANode, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("UpdateMTANode")
	}
	out, err := s.mtaNodes.Update(ctx, &biz.MTANode{
		ID:        req.GetId(),
		Name:      req.GetName(),
		AgentURL:  req.GetAgentUrl(),
		ProxyHost: req.GetProxyHost(),
		ProxyPort: int(req.GetProxyPort()),
		Status:    req.GetStatus(),
		Notes:     req.GetNotes(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateMTANode", err)
	}
	return mtaNodeToProto(out), nil
}

// DeleteMTANode removes a node from the registry.
func (s *Service) DeleteMTANode(ctx context.Context, req *adminv1.DeleteMTANodeRequest) (*adminv1.DeleteMTANodeReply, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("DeleteMTANode")
	}
	if err := s.mtaNodes.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteMTANode", err)
	}
	return &adminv1.DeleteMTANodeReply{Ok: true}, nil
}

// IssueMTANodeEnrollToken mints a single-use agent-enrollment bootstrap token.
func (s *Service) IssueMTANodeEnrollToken(ctx context.Context, req *adminv1.IssueMTANodeEnrollTokenRequest) (*adminv1.IssueMTANodeEnrollTokenReply, error) {
	if s.clusterEnroll == nil {
		return nil, notImplemented("IssueMTANodeEnrollToken")
	}
	token, expiresAt, err := s.clusterEnroll.IssueToken(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "IssueMTANodeEnrollToken", err)
	}
	return &adminv1.IssueMTANodeEnrollTokenReply{Token: token, ExpiresAt: formatTime(expiresAt)}, nil
}

// GetMTANodeIPs returns a node's assignable IP addresses for the UI IP pickers.
func (s *Service) GetMTANodeIPs(ctx context.Context, req *adminv1.GetMTANodeIPsRequest) (*adminv1.GetMTANodeIPsReply, error) {
	if s.mtaNodes == nil {
		return nil, notImplemented("GetMTANodeIPs")
	}
	ips, err := s.mtaNodes.NodeIPs(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "GetMTANodeIPs", err)
	}
	return &adminv1.GetMTANodeIPsReply{Ips: ips}, nil
}

func mtaNodeToProto(n *biz.MTANode) *adminv1.MTANode {
	p := &adminv1.MTANode{
		Id:              n.ID,
		Name:            n.Name,
		AgentUrl:        n.AgentURL,
		ProxyHost:       n.ProxyHost,
		ProxyPort:       int32(n.ProxyPort),
		Status:          n.Status,
		CertFingerprint: n.CertFingerprint,
		Version:         n.Version,
		AppliedChecksum: n.AppliedChecksum,
		KumoState:       n.KumoState,
		Notes:           n.Notes,
	}
	if n.LastSeenAt != nil {
		p.LastSeenAt = formatTime(*n.LastSeenAt)
	}
	return p
}
