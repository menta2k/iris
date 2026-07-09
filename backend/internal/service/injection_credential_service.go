package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListInjectionCredentials returns all injection API credentials (no passwords).
func (s *Service) ListInjectionCredentials(ctx context.Context, req *adminv1.ListInjectionCredentialsRequest) (*adminv1.ListInjectionCredentialsReply, error) {
	if s.injectionCreds == nil {
		return nil, notImplemented("ListInjectionCredentials")
	}
	items, err := s.injectionCreds.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListInjectionCredentials", err)
	}
	out := &adminv1.ListInjectionCredentialsReply{}
	for _, c := range items {
		out.Items = append(out.Items, injectionCredentialToProto(c))
	}
	return out, nil
}

// CreateInjectionCredential adds a credential.
func (s *Service) CreateInjectionCredential(ctx context.Context, req *adminv1.CreateInjectionCredentialRequest) (*adminv1.InjectionCredential, error) {
	if s.injectionCreds == nil {
		return nil, notImplemented("CreateInjectionCredential")
	}
	out, err := s.injectionCreds.Create(ctx, &biz.InjectionCredential{
		Username:           req.GetUsername(),
		Label:              req.GetLabel(),
		Enabled:            req.GetEnabled(),
		AllowedMailclasses: req.GetAllowedMailclasses(),
	}, req.GetPassword())
	if err != nil {
		return nil, s.fail(ctx, "CreateInjectionCredential", err)
	}
	return injectionCredentialToProto(out), nil
}

// UpdateInjectionCredential edits metadata (label, enabled, mailclasses).
func (s *Service) UpdateInjectionCredential(ctx context.Context, req *adminv1.UpdateInjectionCredentialRequest) (*adminv1.InjectionCredential, error) {
	if s.injectionCreds == nil {
		return nil, notImplemented("UpdateInjectionCredential")
	}
	out, err := s.injectionCreds.Update(ctx, &biz.InjectionCredential{
		ID:                 req.GetId(),
		Label:              req.GetLabel(),
		Enabled:            req.GetEnabled(),
		AllowedMailclasses: req.GetAllowedMailclasses(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateInjectionCredential", err)
	}
	return injectionCredentialToProto(out), nil
}

// SetInjectionCredentialPassword rotates a credential's password.
func (s *Service) SetInjectionCredentialPassword(ctx context.Context, req *adminv1.SetInjectionCredentialPasswordRequest) (*adminv1.InjectionCredential, error) {
	if s.injectionCreds == nil {
		return nil, notImplemented("SetInjectionCredentialPassword")
	}
	out, err := s.injectionCreds.SetPassword(ctx, req.GetId(), req.GetPassword())
	if err != nil {
		return nil, s.fail(ctx, "SetInjectionCredentialPassword", err)
	}
	return injectionCredentialToProto(out), nil
}

// DeleteInjectionCredential removes a credential by id.
func (s *Service) DeleteInjectionCredential(ctx context.Context, req *adminv1.DeleteInjectionCredentialRequest) (*adminv1.DeleteInjectionCredentialReply, error) {
	if s.injectionCreds == nil {
		return nil, notImplemented("DeleteInjectionCredential")
	}
	if err := s.injectionCreds.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteInjectionCredential", err)
	}
	return &adminv1.DeleteInjectionCredentialReply{Ok: true}, nil
}

func injectionCredentialToProto(c *biz.InjectionCredential) *adminv1.InjectionCredential {
	p := &adminv1.InjectionCredential{
		Id:                 c.ID,
		Username:           c.Username,
		Label:              c.Label,
		Enabled:            c.Enabled,
		AllowedMailclasses: c.AllowedMailclasses,
		CreatedAt:          formatTime(c.CreatedAt),
		UpdatedAt:          formatTime(c.UpdatedAt),
	}
	if c.LastUsedAt != nil {
		p.LastUsedAt = formatTime(*c.LastUsedAt)
	}
	return p
}
