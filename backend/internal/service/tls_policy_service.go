package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListTLSPolicies returns the require-TLS destination-domain policies.
func (s *Service) ListTLSPolicies(ctx context.Context, req *adminv1.ListTLSPoliciesRequest) (*adminv1.ListTLSPoliciesReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("ListTLSPolicies")
	}
	page := pageFrom(req.GetPage())
	items, err := s.domainSafety.ListTLSPolicies(ctx, req.GetSearch(), page)
	if err != nil {
		return nil, s.fail(ctx, "ListTLSPolicies", err)
	}
	out := &adminv1.ListTLSPoliciesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, p := range items {
		out.Items = append(out.Items, tlsPolicyToProto(p))
	}
	return out, nil
}

// CreateTLSPolicy adds a require-TLS policy for a destination domain.
func (s *Service) CreateTLSPolicy(ctx context.Context, req *adminv1.CreateTLSPolicyRequest) (*adminv1.TLSPolicy, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("CreateTLSPolicy")
	}
	out, err := s.domainSafety.CreateTLSPolicy(ctx, &biz.TLSPolicy{
		Domain: req.GetDomain(),
		Mode:   req.GetMode(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateTLSPolicy", err)
	}
	return tlsPolicyToProto(out), nil
}

// DeleteTLSPolicy removes a require-TLS policy by id.
func (s *Service) DeleteTLSPolicy(ctx context.Context, req *adminv1.DeleteTLSPolicyRequest) (*adminv1.DeleteTLSPolicyReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("DeleteTLSPolicy")
	}
	if err := s.domainSafety.DeleteTLSPolicy(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteTLSPolicy", err)
	}
	return &adminv1.DeleteTLSPolicyReply{}, nil
}

func tlsPolicyToProto(p *biz.TLSPolicy) *adminv1.TLSPolicy {
	out := &adminv1.TLSPolicy{
		Id:     p.ID,
		Domain: p.Domain,
		Mode:   p.Mode,
		Status: p.Status,
		Source: p.Source,
	}
	if !p.CreatedAt.IsZero() {
		out.CreatedAt = p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}
